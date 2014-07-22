package main

import (
	"crypto/sha1"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/howeyc/gopass"
)

func checkinOp() {
	sandboxPath := flag.String("sandbox", "", "Location of the sandbox to load the files")
	flag.Parse()

	if *sandboxPath == "" {
		path, err := os.Getwd()
		if err != nil {
			panic(err)
		}

		path = findSandbox(path)
		sandboxPath = &path
	}

	status, err := scmStatus(*sandboxPath, STAGE)
	if err != nil {
		fmt.Printf("%v\n", err.Error())
		return
	}

	if status.metaData.isstream {
		fmt.Printf("The sandbox is loaded from a stream, which doesn't support check-ins. Load again using a repository workspace.\n")
		return
	}

	if status.unchanged() {
		fmt.Printf("Sandbox is unchanged, nothing checked in.\n")
		return
	}

	fmt.Printf("Password: ")
	password := string(gopass.GetPasswd())

	client, err := NewClient(status.metaData.userId, password)
	if err != nil {
		panic(err)
	}

	scmCheckin(client, status, *sandboxPath)
}

func scmCheckin(client *Client, status *status, sandboxPath string) {
	workspaceUrl := status.metaData.projectUrl + "/" + status.metaData.workspaceId

	// Get the workspace in order to force the authentication to happen
	//  and get the list of components.
	req, err := http.NewRequest("GET", workspaceUrl, nil)
	if err != nil {
		panic(err)
	}

	workspaceObj := fetchFSObject(client, req)

	// TODO Probe the remote workspace to verify that it is in sync
	//   - Tell the user if they are out of sync

	defaultComponentId := ""
	for _, component := range workspaceObj.Children {
		if len(workspaceObj.Children) == 0 {
			defaultComponentId = component.RTCSCM.ItemId
			break
		}

		if strings.HasSuffix(component.Name, "Default Component") {
			defaultComponentId = component.RTCSCM.ItemId
			break
		}
	}
	if defaultComponentId == "" {
		defaultComponentId = workspaceObj.Children[0].RTCSCM.ItemId
	}

	for modifiedpath, _ := range status.Modified {
		fmt.Printf("%v (Modified)\n", modifiedpath)
		modifiedpath = filepath.Join(sandboxPath, modifiedpath)

		// TODO handle the case where a file gets replaced with a directory with the same name
		meta, ok := status.metaData.get(modifiedpath, sandboxPath)
		componentId := ""
		if !ok {
			panic("Metadata not found for file")
		} else {
			componentId = meta.ComponentId
		}

		relPath, err := filepath.Rel(sandboxPath, modifiedpath)
		if err != nil {
			panic(err)
		}

		postUrl := workspaceUrl + "/" + componentId + "/" + relPath + "?op=writeContent"

		newmeta := checkinFile(client, modifiedpath, sandboxPath, postUrl)

		status.metaData.simplePut(newmeta, sandboxPath)
	}

	addedFiles := make([]string, len(status.Added))
	idx := 0

	for addedpath, _ := range status.Added {
		addedFiles[idx] = addedpath
		idx++
	}

	sort.StringSlice(addedFiles).Sort()

	for _, addedpath := range addedFiles {
		fmt.Printf("%v (Added)\n", addedpath)
		addedpath = filepath.Join(sandboxPath, addedpath)

		info, err := os.Stat(addedpath)
		if err != nil {
			panic(err)
		}

		remotePath, err := filepath.Rel(sandboxPath, addedpath)
		if err != nil {
			panic(err)
		}

		remoteParent := filepath.Dir(remotePath)

		if remoteParent == "." {
			remoteParent = ""
		}

		name := filepath.Base(remotePath)

		parentMeta, ok := status.metaData.get(filepath.Dir(addedpath), sandboxPath)
		componentId := ""
		if ok {
			componentId = parentMeta.ComponentId
		} else {
			componentId = defaultComponentId
		}

		if info.IsDir() {
			url := workspaceUrl + "/" + componentId + "/" + remoteParent + "?op=createFolder&name=" + name

			request, err := http.NewRequest("POST", url, nil)
			if err != nil {
				panic(err)
			}

			fsObject := fetchFSObject(client, request)

			meta := metaObject{}
			meta.Path = addedpath
			meta.ItemId = fsObject.RTCSCM.ItemId
			meta.StateId = fsObject.RTCSCM.StateId
			meta.ComponentId = fsObject.RTCSCM.ComponentId

			status.metaData.simplePut(meta, sandboxPath)
		} else {
			// Pre-create the empty file and then check it in
			postUrl := workspaceUrl + "/" + componentId + "/" + remoteParent + "?op=createFile&name=" + name
			createRequest, err := http.NewRequest("POST", postUrl, nil)
			if err != nil {
				panic(err)
			}

			resp, err := client.Do(createRequest)
			if err != nil {
				panic(err)
			}

			if resp.StatusCode != 200 {
				fmt.Printf("Response Status: %v\n", resp.StatusCode)
				b, _ := ioutil.ReadAll(resp.Body)
				fmt.Printf("Response Body\n%v\n", string(b))
				panic("Error")
			}

			postUrl = workspaceUrl + "/" + componentId + "/" + remotePath + "?op=writeContent"
			newmeta := checkinFile(client, addedpath, sandboxPath, postUrl)
			status.metaData.simplePut(newmeta, sandboxPath)
		}
	}

	deletedFiles := make([]string, len(status.Deleted))
	idx = 0
	for deletedpath, _ := range status.Deleted {
		deletedFiles[idx] = deletedpath
		idx++
	}

	sort.StringSlice(deletedFiles).Sort()

	for idx = len(deletedFiles) - 1; idx >= 0; idx-- {
		deletedpath := deletedFiles[idx]
		fmt.Printf("%v (Deleted)\n", deletedpath)
		deletedpath = filepath.Join(sandboxPath, deletedpath)

		meta, ok := status.metaData.get(deletedpath, sandboxPath)
		if !ok {
			panic("Metadata not found for deleted item")
		}

		remotePath, err := filepath.Rel(sandboxPath, deletedpath)
		if err != nil {
			panic(err)
		}

		postUrl := workspaceUrl + "/" + meta.ComponentId + "/" + remotePath + "?op=delete"

		request, err := http.NewRequest("POST", postUrl, nil)
		if err != nil {
			panic(err)
		}

		_, err = client.Do(request)
		if err != nil {
			panic(err)
		}

		delete(status.metaData.pathMap, remotePath)
	}

	err = status.metaData.save(filepath.Join(sandboxPath, metadataFileName))
	if err != nil {
		panic(err)
	}

	// Force a reload of the jazzhub sandbox to avoid out of sync when
	//  looking at the changes page
	projectId := status.metaData.projectId()

	request, err := http.NewRequest("POST", jazzHubBaseUrl+"/code/jazz/Workspace/"+workspaceObj.RTCSCM.ItemId+"/file/"+status.metaData.userId+"-OrionContent/"+projectId,
		strings.NewReader("{\"Load\": true}"))
	if err != nil {
		panic(err)
	}
	request.Header.Add("Jazz-Version", "2")
	request.Header.Add("X-Requested-With", "XMLHttpRequest")

	response, err := client.Do(request)
	if err != nil {
		panic(err)
	}

	if response.StatusCode > 300 {
		fmt.Printf("Response Status: %v\n", response.StatusCode)
		b, _ := ioutil.ReadAll(response.Body)
		fmt.Printf("Response Body\n%v\n", string(b))
		panic("Error")
	}

	fmt.Println("Checkin Complete")
	fmt.Println("Visit the following URL to deliver your changes to the rest of the team:")
	fmt.Println(jazzHubBaseUrl + "/code/jazzui/changes.html#" + url.QueryEscape("/code/jazz/Changes/_/file/"+status.metaData.userId+"-OrionContent/"+projectId))
}

func checkinFile(client *Client, localPath string, sandboxPath string, postUrl string) metaObject {
	file, err := os.Open(localPath)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	// Setup the SHA-1 hash of the file contents
	hash := sha1.New()
	tee := io.TeeReader(file, hash)

	req, err := http.NewRequest("POST", postUrl, tee)
	if err != nil {
		panic(err)
	}

	fsObject := fetchFSObject(client, req)

	newmeta := metaObject{}
	newmeta.Path = sandboxPath
	newmeta.ItemId = fsObject.RTCSCM.ItemId
	newmeta.StateId = fsObject.RTCSCM.StateId
	newmeta.ComponentId = fsObject.RTCSCM.ComponentId

	info, err := os.Stat(localPath)
	if err != nil {
		panic(err)
	}

	newmeta.LasModified = info.ModTime().Unix()
	newmeta.Size = info.Size()

	newmeta.Hash = base64.StdEncoding.EncodeToString(hash.Sum(nil))

	file.Close()
	err = os.Remove(localPath)
	if err != nil {
		panic(err)
	}

	return newmeta
}
