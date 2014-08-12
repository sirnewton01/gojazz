package main

import (
	"crypto/sha1"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/howeyc/gopass"
)

func checkinDefaults() {
	fmt.Printf("gojazz checkin [options]\n")
	flag.PrintDefaults()
}

func checkinOp() {
	sandboxPath := flag.String("sandbox", "", "Location of the sandbox to load the files")
	flag.Usage = checkinDefaults
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
		panic(err)
	}

	if status.metaData.isstream {
		panic(simpleWarning("The sandbox is loaded from a stream, which doesn't support check-ins. Load again using a repository workspace."))
		return
	}

	if status.unchanged() {
		panic(simpleWarning("Sandbox is unchanged. Nothing was checked in."))
		return
	}

	fmt.Printf("Password: ")
	password := string(gopass.GetPasswd())

	client, err := NewClient(status.metaData.userId, password)
	if err != nil {
		panic(err)
	}

	scmCheckin(client, status, *sandboxPath)

	// Force a load/reload of the jazzhub sandbox to avoid out of sync when
	//  looking at the changes page
	err = loadWorkspace(client, status.metaData.projectName, status.metaData.workspaceId)
	if err != nil {
		panic(err)
	}
	fmt.Println("Visit the following URL to work with your changes, deliver them to the rest of the team and more:")
	redirect := fmt.Sprintf(jazzHubBaseUrl + "/code/jazzui/changes.html#" + "/code/jazz/Changes/_/file/" + client.GetJazzId() + "-OrionContent/" + status.metaData.projectName)
	fmt.Printf("https://login.jazz.net/psso/proxy/jazzlogin?redirect_uri=%v\n", url.QueryEscape(redirect))
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

	// TODO Probe the remote workspace to verify that it is in sync with this sandbox
	//   - Warn the user if they are out of sync
	//   - Tell them that check-in will make a best effort to find the correct places
	//      to attach the local changes
	//   - Ask them if they wish to proceed

	defaultComponentId := ""
	for idx, component := range components {
		if idx == 0 || strings.HasSuffix(component.Name, "Default Component") {
			defaultComponentId = component.ScmInfo.ItemId
			break
		}
	}
	if defaultComponentId == "" {
		panic(simpleWarning("There are no components in your repository workspace."))
	}

	for modifiedpath, _ := range status.Modified {
		fmt.Printf("%v (Modified)\n", modifiedpath)

		localpath := filepath.Join(sandboxPath, modifiedpath)
		stagepath := filepath.Join(sandboxPath, stageFolder, modifiedpath)

		meta, ok := status.metaData.get(localpath, sandboxPath)
		componentId := ""
		if !ok {
			// This shouldn't happen. Log the stack if it does.
			panic(&JazzError{Msg: "Metadata not found for file that was found in the metadata", Log: true})
		} else {
			componentId = meta.ComponentId
		}

		remoteFile, err := Open(client, ccmBaseUrl, workspaceId, componentId, modifiedpath)
		if err != nil {
			// First, check to see if this is a 404 (Not Found). This can occur when one or more of the
			//  parent directories are not there.
			fileerror, ok := err.(*JazzError)

			if ok && fileerror.StatusCode == 404 {
				fmt.Printf("Cannot check-in file at path %v since it no longer exists at the same location on the remote.\n", modifiedpath)
				fmt.Printf("The file has been temporarily backed up in the following location: %v\n", stagepath)
				continue
			}

			panic(err)
		}

		// TODO better checking and matching for the file, perhaps by item ID?
		if remoteFile.info.Directory {
			fmt.Printf("Cannot check-in file at path %v. There is a folder at this location on the remote.\n", modifiedpath)
			fmt.Printf("The file has been temporarily backed up in the following location: %v\n", stagepath)
			continue
		}
		// Ooops, this is the wrong file
		if remoteFile.info.ScmInfo.ItemId != meta.ItemId {
			fmt.Printf("Cannot check-in file at path %v. It is not the same as the one that was originally loaded.\n", modifiedpath)
			fmt.Printf("The file has been temporarily backed up in the following location: %v\n", stagepath)
			continue
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
				// First, check to see if this is a 404 (Not Found). This can occur when one or more of the
				//  parent directories are not there.
				fileerror, ok := err.(*JazzError)

				if ok && fileerror.StatusCode == 404 {
					// One last crack at this is to create all of the necessary parent directories and then add the file to it
					parentDir := path.Dir(addedpath)
					_, err := MkdirAll(client, ccmBaseUrl, workspaceId, componentId, parentDir)
					if err != nil {
						panic(err)
					}

					// Try again now that the parent directory is there
					remoteFolder, err = Mkdir(client, ccmBaseUrl, workspaceId, componentId, addedpath)
					if err != nil {
						panic(err)
					}
				} else {
					panic(err)
				}
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
				fileerror, ok := err.(*JazzError)

				if ok && fileerror.StatusCode == 404 {
					// One last crack at this is to create all of the necessary parent directories and then add the file to it
					parentDir := path.Dir(addedpath)
					_, err := MkdirAll(client, ccmBaseUrl, workspaceId, componentId, parentDir)
					if err != nil {
						panic(err)
					}

					// Try again now that the parent directory is there
					remoteFile, err = Create(client, ccmBaseUrl, workspaceId, componentId, addedpath)
					if err != nil {
						panic(err)
					}
				} else {
					panic(err)
				}
			}

			stagepath := filepath.Join(sandboxPath, stageFolder, addedpath)
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
			// This should never really happen but log it if it does.
			panic(&JazzError{Msg: "Metadata not found for deleted item discovered in the metadata.", Log: true})
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
			fileerror, ok := err.(*JazzError)
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

	fmt.Println("Checkin Complete")
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
