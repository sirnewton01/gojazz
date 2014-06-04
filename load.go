package main

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	"github.com/howeyc/gopass"
)

type FSObject struct {
	Name        string
	Directory   bool
	RTCSCM      SCMObject
	Children    []FSObject
	parentUrl   string
	sandboxPath string
}

type SCMObject struct {
	ItemId  string
	StateId string
	Type    string
}

type ProjectResults struct {
	Projects []Project `json:"projects"`
}

type Project struct {
	CcmBaseUrl string `json:"ccmBaseUrl"`
}

// TODO private projects
func loadOp() {
	if len(os.Args) < 3 {
		fmt.Printf("Provide an IBM DevOps Services project to load.\n")
		flag.PrintDefaults()
		return
	}

	projectName := &os.Args[2]
	os.Args = os.Args[2:]
	sandboxPath := flag.String("sandbox", "", "Location of the sandbox to load the files")
	userId := flag.String("userId", "", "Your IBM DevOps Services user ID")
	flag.Parse()

	if *sandboxPath == "" {
		path, err := os.Getwd()
		if err != nil {
			panic(err)
		}

		sandboxPath = &path
	}

	password := ""

	if *userId != "" {
		fmt.Printf("Password: ")
		password = string(gopass.GetPasswd())
	}

	// Assemble a client with the user credentials
	client, err := NewClient(*userId, password)

	if err != nil {
		panic(err)
	}

	fmt.Printf("Loading '%v' into %v...\n", *projectName, *sandboxPath)
	err = scmLoad(client, *projectName, *sandboxPath)
	if err == nil {
		fmt.Printf("Load successful\n")
	} else {
		fmt.Printf("%v\n", err.Error())
	}
}

func scmLoad(client *Client, project string, sandbox string) error {
	projectEscaped := url.QueryEscape(project)

	// Discover the RTC repo for this project
	request, err := http.NewRequest("GET", jazzHubBaseUrl+"/manage/service/com.ibm.team.jazzhub.common.service.IProjectService/projectsByFilter?token=&startIndex=0&pageSize=2&filter="+projectEscaped, nil)

	if err != nil {
		return err
	}

	resp, err := client.Do(request)
	if err != nil {
		return err
	}

	results := &ProjectResults{}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	err = json.Unmarshal(b, results)
	if err != nil {
		return err
	}

	if len(results.Projects) != 1 {
		return errors.New("Project not found")
	}

	orion_fs := results.Projects[0].CcmBaseUrl + "/service/com.ibm.team.filesystem.service.jazzhub.IOrionFilesystem/pa"

	queue := make(chan FSObject)

	// Track how much work needs to be done and send a signal on the
	//  finished channel when its done
	tracker := make(chan int)
	finished := make(chan bool)
	go func() {
		work := 1
		for work > 0 {
			work += <-tracker
		}
		finished <- true
	}()

	projectObj := FSObject{}
	projectObj.Name = project
	projectObj.parentUrl = orion_fs
	projectObj.Directory = true
	projectObj.sandboxPath = sandbox
	projectObj.RTCSCM.Type = "ProjectArea"

	// Get the existing status of the sandbox, if available
	status, err := scmStatus(sandbox)
	if err != nil {
		status = NewStatus()
	}
	newMetaData := NewMetaData()

	loadChild(client, projectObj, queue, tracker, status, newMetaData)

	createFiles := func() {
		for {
			fsObject := <-queue
			loadChild(client, fsObject, queue, tracker, status, newMetaData)
		}
	}

	// downloading go routines
	for i := 0; i < numGoRoutines; i++ {
		go createFiles()
	}

	<-finished

	newMetaData.Save(filepath.Join(sandbox, ".jazzmeta"))

	return nil
}

func loadChild(client *Client, fsObject FSObject, queue chan FSObject, tracker chan int, status *Status, newMetaData *MetaData) {
	client.Log.Printf("Loading %v\n", fsObject.Name)

	url := fsObject.parentUrl

	meta := MetaObject{}
	meta.ItemId = fsObject.RTCSCM.ItemId
	meta.StateId = fsObject.RTCSCM.StateId

	// Workspaces, streams and component are addressable only by their Item ID's
	if fsObject.RTCSCM.Type == "Workspace" || fsObject.RTCSCM.Type == "Component" {
		url = url + "/" + fsObject.RTCSCM.ItemId
	} else {
		url = url + "/" + fsObject.Name
	}

	if fsObject.Directory {
		sandboxPath := fsObject.sandboxPath

		request, err := http.NewRequest("GET", url, nil)

		if err != nil {
			panic(err)
		}

		resp, err := client.Do(request)
		if err != nil {
			panic(err)
		}

		projectObj := &FSObject{}
		b, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			panic(err)
		}
		err = json.Unmarshal(b, projectObj)
		if err != nil {
			panic(err)
		}

		if resp.StatusCode != 200 {
			fmt.Printf("Error Loading %v/%v\n", fsObject.sandboxPath, fsObject.Name)
			client.Log.Printf("Response Status: %v\n", resp.StatusCode)
			b, _ := ioutil.ReadAll(resp.Body)
			client.Log.Printf("Response Body:\n%v\n", string(b))
			panic("Error")
		}

		resp.Body.Close()

		if fsObject.RTCSCM.Type != "ProjectArea" && fsObject.RTCSCM.Type != "Workspace" && fsObject.RTCSCM.Type != "Component" {
			sandboxPath = filepath.Join(sandboxPath, fsObject.Name)
			meta.Path = sandboxPath

			// Create the directory if it doesn't already exist
			err := os.MkdirAll(sandboxPath, 0700)
			if err != nil {
				panic(err)
			}

			info, err := os.Stat(sandboxPath)
			if err != nil {
				panic(err)
			}

			meta.LasModified = info.ModTime().Unix()
		}

		if fsObject.RTCSCM.Type == "Folder" {
			// If this is a folder (not project area, workspace or component) then delete
			//  any extra children files/folders on disk
			// TODO this would be a good candidate for a shed to put extra stuff that the user may care about

			dir, err := os.Open(sandboxPath)
			if err != nil {
				panic(err)
			}

			children, err := dir.Readdirnames(-1)
			if err != nil {
				panic(err)
			}

			for _, child := range children {
				found := false
				for _, remoteChild := range projectObj.Children {
					if remoteChild.Name == child {
						found = true
						break
					}
				}

				// Delete this file/folder because it no longer exists in the stream
				if !found {
					err = os.RemoveAll(filepath.Join(sandboxPath, child))
					if err != nil {
						panic(err)
					}
				}
			}
		}

		// Pick the first child stream of a project area
		if fsObject.RTCSCM.Type != "ProjectArea" {
			// Add new tasks for each of the children
			tracker <- len(projectObj.Children)
			for _, child := range projectObj.Children {
				child.parentUrl = url
				child.sandboxPath = sandboxPath
				go func(child FSObject) {
					queue <- child
				}(child)
			}
		} else {
			if len(projectObj.Children) == 0 {
				panic("No streams for this project")
			}

			tracker <- 1
			child := projectObj.Children[0]
			child.parentUrl = url
			child.sandboxPath = sandboxPath
			go func(child FSObject) {
				queue <- child
			}(child)
		}
	} else {
		// Check if we need to download anything
		sandboxPath := filepath.Join(fsObject.sandboxPath, fsObject.Name)

		_, err := os.Stat(sandboxPath)
		if err == nil {
			// User modified the file
			if status.Modified[sandboxPath] {
				fmt.Printf("%v was modified and is overwritten\n", sandboxPath)
			} else if status.metaData != nil {
				// The file is unchanged locally and in the repository
				oldMeta := status.metaData.Get(sandboxPath)

				if oldMeta.Path != "" && oldMeta.StateId == fsObject.RTCSCM.StateId && oldMeta.ItemId == fsObject.RTCSCM.ItemId {
					newMetaData.Put(oldMeta)
					tracker <- -1
					return
				}
			}
		}

		// Too bad, we need to download the contents
		request, err := http.NewRequest("GET", url+"?op=readContent", nil)

		if err != nil {
			panic(err)
		}

		resp, err := client.Do(request)
		if err != nil {
			panic(err)
		}

		if resp.StatusCode != 200 {
			fmt.Printf("Error Loading %v/%v\n", fsObject.sandboxPath, fsObject.Name)
			client.Log.Printf("Response Status: %v\n", resp.StatusCode)
			b, _ := ioutil.ReadAll(resp.Body)
			client.Log.Printf("Response Body\n%v\n", string(b))
			panic("Error")
		}

		file, err := os.Create(sandboxPath)

		if err != nil {
			panic(err)
		}

		// Setup the SHA-1 hash of the file contents
		hash := sha1.New()
		tee := io.MultiWriter(file, hash)

		_, err = io.Copy(tee, resp.Body)

		if err != nil {
			panic(err)
		}

		resp.Body.Close()
		file.Close()

		meta.Path = sandboxPath
		info, err := os.Stat(sandboxPath)
		if err != nil {
			panic(err)
		}
		meta.LasModified = info.ModTime().Unix()
		meta.Hash = base64.StdEncoding.EncodeToString(hash.Sum(nil))
		meta.Size = info.Size()
	}

	if meta.Path != "" {
		newMetaData.Put(meta)
	}

	// This task is done
	tracker <- -1
}
