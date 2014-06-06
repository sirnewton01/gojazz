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
	"strings"

	"github.com/howeyc/gopass"
)

const (
	numGoRoutines = 15
	bufferSize    = 1000
)

type FSObject struct {
	Name        string
	Directory   bool
	RTCSCM      SCMObject
	Children    []FSObject
	parentUrl   string
	sandboxPath string
	etag        string
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
	overwrite := flag.Bool("force", false, "Force overwrite of any local changes")
	stream := flag.String("stream", "", "Alternate stream to load")
	flag.Parse()

	if *sandboxPath == "" {
		path, err := os.Getwd()
		if err != nil {
			panic(err)
		}

		path = findSandbox(path)
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
	if *stream != "" {
		fmt.Printf("Stream is %v\n", *stream)
	}
	err = scmLoad(client, *projectName, *sandboxPath, *overwrite, *stream)
	if err == nil {
		fmt.Printf("Load successful\n")
	} else {
		fmt.Printf("%v\n", err.Error())
	}
}

func scmLoad(client *Client, project string, sandbox string, overwrite bool, stream string) error {
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
	if resp.StatusCode != 200 {
		fmt.Printf("Response Status: %v\n", resp.StatusCode)
		b, _ := ioutil.ReadAll(resp.Body)
		fmt.Printf("Response Body\n%v\n", string(b))
		panic("Error")
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
	projecturl := orion_fs + "/" + project

	// Fetch all of the streams from the project
	request, err = http.NewRequest("GET", projecturl, nil)
	if err != nil {
		panic(err)
	}
	resp, err = client.Do(request)
	if err != nil {
		panic(err)
	}
	if resp.StatusCode != 200 {
		fmt.Printf("Response Status: %v\n", resp.StatusCode)
		b, _ := ioutil.ReadAll(resp.Body)
		fmt.Printf("Response Body\n%v\n", string(b))
		panic("Error")
	}
	projectObj := &FSObject{}
	b, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}
	err = json.Unmarshal(b, projectObj)
	if err != nil {
		panic(err)
	}

	var streamObj FSObject
	for _, childStream := range projectObj.Children {
		if childStream.Name == stream {
			streamObj = childStream
			break
		}

		// The default stream for a project has the form "user | projectName Stream"
		if stream == "" && childStream.Name == project+" Stream" {
			streamObj = childStream
		}
	}

	// Still no stream found, fail if user specified a stream, pick the first one otherwise
	if streamObj.Name == "" {
		if stream != "" {
			return errors.New("Stream with name " + stream + " not found")
		}

		if len(projectObj.Children) == 0 {
			return errors.New("No default stream could be found for this project. Is it a Git project?")
		}
		streamObj = projectObj.Children[0]
	}

	streamObj.sandboxPath = sandbox
	streamObj.parentUrl = projecturl

	queue := make(chan FSObject, bufferSize)

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

	// Get the existing status of the sandbox, if available
	status, _ := scmStatus(sandbox)

	if !overwrite && status != nil && !status.unchanged() {
		return errors.New("There are local changes, aborting. Use the status subcommand to find the changes. Try again with '-force=true' to overwrite")
	}

	if status != nil && !status.unchanged() {
		fmt.Printf("Overwriting these files:\n %v", status)
	}

	// Delete the old metadata
	os.Remove(filepath.Join(sandbox, metadataFileName))

	newMetaData := newMetaData()
	newMetaData.initConcurrentWrite()

	loadChild(client, sandbox, streamObj, queue, tracker, status, newMetaData)

	createFiles := func() {
		for {
			fsObject := <-queue
			loadChild(client, sandbox, fsObject, queue, tracker, status, newMetaData)
		}
	}

	// downloading go routines
	for i := 0; i < numGoRoutines; i++ {
		go createFiles()
	}

	<-finished

	// As a last pass, check all of the files at the top of the sandbox to verify
	//  that they are in the metadata. They are either detached from the stream contents
	//  or were added by the user. Either way, they should be deleted.
	dir, err := os.Open(sandbox)
	if err != nil {
		panic(err)
	}
	roots, err := dir.Readdirnames(-1)
	if err != nil {
		panic(err)
	}
	for _, root := range roots {
		if _, ok := newMetaData.get(filepath.Join(sandbox, root), sandbox); !ok {
			err = os.RemoveAll(root)
			if err != nil {
				panic(err)
			}
		}
	}

	newMetaData.save(filepath.Join(sandbox, metadataFileName))

	return nil
}

func extractComponentEtag(rawEtag string) string {
	rawEtag = strings.Replace(rawEtag, "\"", "", -1)
	if rawEtag != "" {
		rawEtag = strings.Split(rawEtag, " ")[1]
	}

	return rawEtag
}

func loadChild(client *Client, sandbox string, fsObject FSObject, queue chan FSObject, tracker chan int, status *status, newMetaData *metaData) {
	client.Log.Printf("Loading %v\n", fsObject.Name)

	url := fsObject.parentUrl

	meta := metaObject{}
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

		if resp.StatusCode != 200 {
			fmt.Printf("Error Loading %v\n", url)
			fmt.Printf("Response Status: %v\n", resp.StatusCode)
			b, _ := ioutil.ReadAll(resp.Body)
			fmt.Printf("Response Body:\n%v\n", string(b))
			panic("Error")
		}

		etag := extractComponentEtag(resp.Header.Get("ETag"))
		if fsObject.etag != "" && fsObject.etag != etag {
			panic("Stream has changed while updating:" + fsObject.etag + " " + etag)
		}

		// Optimization, skip loading a component if there are no changes
		//  and the etag is the same as last time.
		if fsObject.RTCSCM.Type == "Component" {
			componentId := fsObject.RTCSCM.ItemId

			newMetaData.componentEtag[componentId] = etag

			if status != nil && status.unchanged() && status.metaData.componentEtag[componentId] == etag {
				// FIXME this optimization does _not_ work for multiple components

				for k, v := range status.metaData.pathMap {
					newMetaData.pathMap[k] = v
				}

				tracker <- -1
				return
			}
		}

		directoryObj := &FSObject{}
		b, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			panic(err)
		}
		err = json.Unmarshal(b, directoryObj)
		if err != nil {
			panic(err)
		}

		resp.Body.Close()

		// Workspaces and component don't get their own directory, the children are loaded directly
		//  underneath the sandbox root.
		if fsObject.RTCSCM.Type != "Workspace" && fsObject.RTCSCM.Type != "Component" {
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
				for _, remoteChild := range directoryObj.Children {
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

		// Add new tasks for each of the children
		tracker <- len(directoryObj.Children)
		for _, child := range directoryObj.Children {
			child.parentUrl = url
			child.sandboxPath = sandboxPath
			child.etag = etag

			// Try queueing the child for another goroutine to handle it
			// Otherwise, we will recurse depth-first ourselves to make sure
			//  that we don't deadlock
			select {
			case queue <- child:
				break
			default:
				loadChild(client, sandbox, child, queue, tracker, status, newMetaData)
			}
		}
	} else {
		// Check if we need to download anything
		sandboxPath := filepath.Join(fsObject.sandboxPath, fsObject.Name)

		_, err := os.Stat(sandboxPath)
		if err == nil && status != nil {
			// User modified the file
			if status.Modified[sandboxPath] {
				fmt.Printf("%v was modified and is overwritten\n", sandboxPath)
			} else if status.metaData != nil {
				// The file is unchanged locally and in the repository
				oldMeta, ok := status.metaData.get(sandboxPath, sandbox)

				if ok && oldMeta.StateId == fsObject.RTCSCM.StateId && oldMeta.ItemId == fsObject.RTCSCM.ItemId {
					newMetaData.put(oldMeta, sandbox)
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
			fmt.Printf("Response Status: %v\n", resp.StatusCode)
			b, _ := ioutil.ReadAll(resp.Body)
			fmt.Printf("Response Body\n%v\n", string(b))
			panic("Error")
		}

		etag := extractComponentEtag(resp.Header.Get("ETag"))
		if fsObject.etag != "" && fsObject.etag != etag {
			panic("Stream has changed while updating: " + fsObject.etag + " " + etag)
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
		newMetaData.put(meta, sandbox)
	}

	// This task is done
	tracker <- -1
}
