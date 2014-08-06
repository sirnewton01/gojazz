package main

import (
	"crypto/sha1"
	"encoding/base64"
	"errors"
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
	// Get the workspace in order to force the authentication to happen
	//  and get the list of components.
	workspaceId := status.metaData.workspaceId
	ccmBaseUrl := status.metaData.ccmBaseUrl

	components, err := FindComponents(client, status.metaData.ccmBaseUrl, status.metaData.workspaceId)
	if err != nil {
		panic(err)
	}

	// TODO Probe the remote workspace to verify that it is in sync
	//   - Warn the user if they are out of sync

	defaultComponentId := ""
	for idx, component := range components {
		if idx == 0 || strings.HasSuffix(component.Name, "Default Component") {
			defaultComponentId = component.ScmInfo.ItemId
			break
		}
	}
	if defaultComponentId == "" {
		panic(errors.New("There are no components in the repository workspace"))
	}

	for modifiedpath, _ := range status.Modified {
		fmt.Printf("%v (Modified)\n", modifiedpath)

		localpath := filepath.Join(sandboxPath, modifiedpath)
		stagepath := filepath.Join(sandboxPath, ".jazzstage", modifiedpath)

		meta, ok := status.metaData.get(localpath, sandboxPath)
		componentId := ""
		if !ok {
			panic("Metadata not found for file")
		} else {
			componentId = meta.ComponentId
		}

		remoteFile, err := Open(client, ccmBaseUrl, workspaceId, componentId, modifiedpath)
		if err != nil {
			panic(err)
		}

		// TODO better checking and matching for the file, perhaps by item ID?
		if remoteFile.info.Directory {
			panic(fmt.Sprintf("Cannot check-in file at path %v. There is a folder at this location on the remote.", modifiedpath))
		}
		// Ooops, this is the wrong file
		if remoteFile.info.ScmInfo.ItemId != meta.ItemId {
			panic(fmt.Sprintf("Cannot check-in file at path %v. It is not the same as the one that was originally loaded", modifiedpath))
		}

		newmeta := checkinFile(client, stagepath, remoteFile)
		newmeta.Path = localpath

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

		localpath := filepath.Join(sandboxPath, addedpath)

		info, err := os.Stat(localpath)
		if err != nil {
			panic(err)
		}

		// We need to find the component to add this file. It will either be the
		//  the parent element, which we may have just added, or its the default component.
		parentMeta, ok := status.metaData.get(filepath.Dir(localpath), sandboxPath)
		componentId := ""
		if ok {
			componentId = parentMeta.ComponentId
		} else {
			componentId = defaultComponentId
		}

		if info.IsDir() {
			remoteFolder, err := Mkdir(client, ccmBaseUrl, workspaceId, componentId, addedpath)
			if err != nil {
				panic(err)
			}

			meta := metaObject{}
			meta.Path = localpath
			meta.ItemId = remoteFolder.info.ScmInfo.ItemId
			meta.StateId = remoteFolder.info.ScmInfo.StateId
			meta.ComponentId = remoteFolder.info.ScmInfo.ComponentId

			status.metaData.simplePut(meta, sandboxPath)
		} else {
			remoteFile, err := Create(client, ccmBaseUrl, workspaceId, componentId, addedpath)
			if err != nil {
				// First, check to see if this is a 404 (Not Found). This can occur when one or more of the
				//  parent directories are not there.

				fileerror, ok := err.(*FileError)

				// TODO create all of the parent directories when this happens
				if ok && fileerror.StatusCode == 404 {
					panic(errors.New(fmt.Sprintf("The parent directory of file %v could not be found. Cannot check it in.", addedpath)))
				} else {
					panic(err)
				}
			}

			stagepath := filepath.Join(sandboxPath, ".jazzstage", addedpath)
			newmeta := checkinFile(client, stagepath, remoteFile)
			newmeta.Path = localpath
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

		componentId := ""

		meta, ok := status.metaData.get(deletedpath, sandboxPath)
		if !ok {
			panic("Metadata not found for deleted item")
		} else {
			componentId = meta.ComponentId
		}

		remotePath, err := filepath.Rel(sandboxPath, deletedpath)
		if err != nil {
			panic(err)
		}

		err = Remove(client, ccmBaseUrl, workspaceId, componentId, remotePath)
		if err != nil {
			// First, check to see if this is a 404 (Not Found). If the file is already deleted
			//  then this is an acceptable resolution to the checkin. One reason it may be already
			//  deleted is that it is a child of a directory that is already deleted.
			fileerror, ok := err.(*FileError)
			if !ok || fileerror.StatusCode != 404 {
				panic(err)
			}
		}

		delete(status.metaData.pathMap, remotePath)
	}

	err = status.metaData.save(filepath.Join(sandboxPath, metadataFileName))
	if err != nil {
		panic(err)
	}

	// Force a reload of the jazzhub sandbox to avoid out of sync when
	//  looking at the changes page
	projectName := status.metaData.projectName

	// TODO All of this is not nearly sufficient, it assumes that the user hit "Edit Code" on the project at least once
	request, err := http.NewRequest("POST", jazzHubBaseUrl+"/code/jazz/Workspace/"+workspaceId+"/file/"+status.metaData.userId+"-OrionContent/"+projectName,
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
	fmt.Println("Visit the following URL to work with your changes, deliver them to the rest of the team and more:")
	fmt.Println(jazzHubBaseUrl + "/code/jazzui/changes.html#" + url.QueryEscape("/code/jazz/Changes/_/file/"+status.metaData.userId+"-OrionContent/"+projectName))
}

func checkinFile(client *Client, localPath string, remoteFile *File) metaObject {
	file, err := os.Open(localPath)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	// Setup the SHA-1 hash of the file contents
	hash := sha1.New()
	tee := io.TeeReader(file, hash)

	newmeta := metaObject{}
	newmeta.ItemId = remoteFile.info.ScmInfo.ItemId

	newmeta.ComponentId = remoteFile.info.ScmInfo.ComponentId

	info, err := os.Stat(localPath)
	if err != nil {
		panic(err)
	}

	newmeta.LastModified = info.ModTime().Unix()
	newmeta.Size = info.Size()

	err = remoteFile.Write(tee)
	if err != nil {
		panic(err)
	}
	remoteFile.Close()

	// Write the hash now that the file contents have been read while uploading to the server
	newmeta.Hash = base64.StdEncoding.EncodeToString(hash.Sum(nil))

	// The new stateId is assigned to the remoteFile after a successful write
	newmeta.StateId = remoteFile.info.ScmInfo.StateId

	// This is the staged file, we can delete it to save disk space since it was uploaded without error
	file.Close()
	os.Remove(localPath)

	return newmeta
}
