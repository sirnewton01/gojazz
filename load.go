package main

import (
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/howeyc/gopass"
)

const (
	numGoRoutines = 10
	bufferSize    = 1000
)

type ProjectResults struct {
	Projects []Project `json:"projects"`
}

type Project struct {
	CcmBaseUrl string `json:"ccmBaseUrl"`
}

func loadOp() {
	var projectName string

	streamDef := ""
	stream := &streamDef
	workspaceDef := false
	workspace := &workspaceDef

	// Project name provided
	if len(os.Args) > 1 && !strings.HasPrefix(os.Args[1], "-") {
		projectName = os.Args[1]
		os.Args = os.Args[1:]

		// Providing a workspace or stream is only valid in the context of a project
		stream = flag.String("stream", "", "Alternate stream to load")
		workspace = flag.Bool("workspace", false, "Use a repository workspace to check-in changes (requires authentication).")
	}

	sandboxPath := flag.String("sandbox", "", "Location of the sandbox to load the files")
	userId := flag.String("userId", "", "Your IBM DevOps Services user ID")
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

	if *workspace && *userId == "" {
		fmt.Printf("You must provide credentials to use a repository workspace\n")
		return
	}

	// Get the existing status of the sandbox, if available
	// Back up any changes that are found
	status, _ := scmStatus(*sandboxPath, BACKUP)

	if status != nil && !status.unchanged() {
		fmt.Printf("Here was the status of your sandbox before loading:\n%v", status)
		fmt.Printf("Your changes have been backed up: %v\n", status.copyPath)
	}

	// Re-use the existing user ID from the metadata, if available
	if *userId == "" && status != nil && status.metaData.userId != "" {
		userId = &status.metaData.userId
	}

	if *userId != "" {
		fmt.Printf("Password: ")
		password = string(gopass.GetPasswd())
	}

	// Assemble a client with the user credentials
	client, err := NewClient(*userId, password)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Loading into %v...\n", *sandboxPath)

	var isstream bool
	workspaceId := ""
	ccmBaseUrl := ""

	// This is either a fresh sandbox or project/stream/workspace information was provided
	if status == nil || projectName != "" {
		if projectName == "" {
			panic(errors.New("Provide a project to load"))
		}

		ccmBaseUrl, err = client.findCcmBaseUrl(projectName)
		if err != nil {
			panic(err)
		}

		// Find a repository workspace with the correct naming convention
		// Failing that, create one.
		if *workspace {
			isstream = false

			// FIXME this criteria (based on the name) is not good
			workspaceId, err = FindRepositoryWorkspace(client, ccmBaseUrl, projectName+" Workspace")
			if err != nil {
				panic(err)
			}

			// FIXME this should create a new repository workspace from the specified stream
			if workspaceId == "" {
				panic(errors.New("No repository workspace found\n"))
			}
		} else {
			isstream = true

			// User provided the stream name to load
			if *stream != "" {
				workspaceId, err = FindStream(client, ccmBaseUrl, projectName, *stream)
				if err != nil {
					panic(err)
				}

				if workspaceId == "" {
					panic(errors.New("Stream with name " + *stream + " not found"))
				}
			} else {
				// Use the stream with the form "user | projectName Stream"
				workspaceId, err = FindStream(client, ccmBaseUrl, projectName, projectName+" Stream")
				if err != nil {
					panic(err)
				}

				if workspaceId == "" {
					panic(errors.New("No default stream could be found for this project. Is it a Git project?"))
				}
			}
		}
	} else {
		isstream = status.metaData.isstream
		workspaceId = status.metaData.workspaceId
		ccmBaseUrl = status.metaData.ccmBaseUrl
	}

	if isstream {
		fmt.Printf("Type: Stream\n")
	} else {
		fmt.Printf("Type: Repository Workspace\n")
	}

	scmLoad(client, ccmBaseUrl, workspaceId, isstream, *userId, *sandboxPath, status)

	fmt.Printf("Load Successful\n")
}

func scmLoad(client *Client, ccmBaseUrl string, workspaceId string, stream bool, userId string, sandbox string, status *status) {
	newMetaData := newMetaData()
	newMetaData.initConcurrentWrite()
	newMetaData.isstream = stream
	newMetaData.userId = userId
	newMetaData.ccmBaseUrl = ccmBaseUrl
	newMetaData.workspaceId = workspaceId

	// Delete the old metadata
	metadataFile := filepath.Join(sandbox, metadataFileName)
	os.Remove(metadataFile)

	if status != nil {
		// Delete any files that were added/modified (they should already be backed up)
		for addedPath, _ := range status.Added {
			err := os.RemoveAll(filepath.Join(sandbox, addedPath))
			if err != nil {
				panic(err)
			}
		}
		for modPath, _ := range status.Modified {
			err := os.RemoveAll(filepath.Join(sandbox, modPath))
			if err != nil {
				panic(err)
			}
		}
	} else {
		// Check if there are any files in the sandbox, fail if there are any
		stat, _ := os.Stat(sandbox)

		if stat != nil {
			s, err := os.Open(sandbox)
			if err != nil {
				panic(err)
			}

			children, err := s.Readdirnames(-1)
			if err != nil {
				panic(err)
			}

			if len(children) > 0 {
				panic(errors.New("Sorry, there are files in the sandbox directory that will be clobbered."))
			}
		}
	}

	// Find all of the components of the remote workspace and then walk over each one
	componentIds, err := FindComponents(client, ccmBaseUrl, workspaceId)
	if err != nil {
		panic(err)
	}

	// Walk through the remote components creating directories, if necessary and cleaning up any deleted files
	for _, componentId := range componentIds {
		loadComponent(client, ccmBaseUrl, workspaceId, componentId, sandbox, newMetaData, status)
	}

	newMetaData.save(metadataFile)
}

func loadComponent(client *Client, ccmBaseUrl string, workspaceId string, componentId string, sandbox string, newMetaData *metaData, status *status) {
	// Optimization: if status is unchanged and the component's ETag is the same
	//  then we can skip downloading this component
	if status != nil && status.unchanged() {
		// TODO implement the optimization
	}

	// Queue of paths to download (empty string means we are done)
	downloadQueue := make(chan string, bufferSize)
	// Queue of finished messages from the go routines
	finished := make(chan bool)

	// Load status updates
	trackerFinish := make(chan bool)
	workTracker := make(chan bool)
	workTransfer := make(chan int64)
	go func() {
		work := 0
		worked := 0
		transferred := int64(0)
		lastStringLength := 0

		for {
			select {
			case moreBytes := <-workTransfer:
				transferred += moreBytes
			case added := <-workTracker:
				if added {
					work += 1
				} else {
					worked += 1
				}

				// Backspace out the last line that was printed
				for i := 0; i < lastStringLength; i++ {
					fmt.Printf("\b")
				}

				lastStringLength, _ = fmt.Printf("Loaded %v (of %v) files. Bytes loaded: %v", worked, work, transferred)
			case <-trackerFinish:
				fmt.Print("\n")
			}
		}
	}()

	// Downloading gorountine
	downloadFiles := func() {
		for {
			select {
			case pathToDownload := <-downloadQueue:
				if pathToDownload == "" {
					// We're done
					finished <- true
					return
				}

				workTracker <- true

				remoteFile, err := Open(client, ccmBaseUrl, workspaceId, componentId, pathToDownload)
				if err != nil {
					panic(err)
				}

				scmInfo := remoteFile.info.ScmInfo
				localPath := filepath.Join(sandbox, pathToDownload)

				// Optimization: State ID is the same as last time and there were no local modifications
				if status != nil && !status.Modified[pathToDownload] && !status.Deleted[pathToDownload] {
					prevMeta, ok := status.metaData.get(localPath, sandbox)

					if ok && prevMeta.StateId == scmInfo.StateId {
						// Push the old metadata forward for this file
						remoteFile.Close()
						newMetaData.put(prevMeta, sandbox)
						workTracker <- false
						continue
					}
				}

				localFile, err := os.Create(filepath.Join(sandbox, pathToDownload))
				if err != nil {
					panic(err)
				}

				// Setup the SHA-1 hash of the file contents
				hash := sha1.New()
				tee := io.MultiWriter(localFile, hash)

				numBytes, err := io.Copy(tee, remoteFile)
				if err != nil {
					panic(err)
				}
				
				workTransfer <- numBytes

				localFile.Close()
				remoteFile.Close()

				stat, _ := os.Stat(localPath)

				meta := metaObject{
					Path:         localPath,
					ItemId:       scmInfo.ItemId,
					StateId:      scmInfo.StateId,
					ComponentId:  scmInfo.ComponentId,
					LastModified: stat.ModTime().Unix(),
					Size:         stat.Size(),
					Hash:         base64.StdEncoding.EncodeToString(hash.Sum(nil)),
				}

				newMetaData.put(meta, sandbox)

				workTracker <- false
			}
		}
	}

	for i := 0; i < numGoRoutines; i++ {
		go downloadFiles()
	}

	err := Walk(client, ccmBaseUrl, workspaceId, componentId, func(p string, file File) error {
		localPath := filepath.Join(sandbox, p)

		if file.info.Directory {
			workTracker <- true
			// Create if it doesn't already exist
			stat, _ := os.Stat(localPath)

			if stat == nil {
				err := os.MkdirAll(localPath, 0700)
				if err != nil {
					return err
				}
			} else if !stat.IsDir() {
				// Weird, there's a file with the same name as the directory in the workspace here
				os.Remove(localPath)
				err := os.MkdirAll(localPath, 0700)
				if err != nil {
					return err
				}
			}

			// Push the new metadata for this directory
			scmInfo := file.info.ScmInfo
			meta := metaObject{Path: localPath, ItemId: scmInfo.ItemId, StateId: scmInfo.StateId, ComponentId: scmInfo.ComponentId}
			newMetaData.put(meta, sandbox)

			workTracker <- false
		} else {
			// Push the file path into the queue for download (unless the file hasn't changed)
			downloadQueue <- p
		}

		return nil
	})

	// Send the stop signal to all download routines
	for i := 0; i < numGoRoutines; i++ {
		downloadQueue <- ""
		<-finished
	}

	// Tell the tracker to finish reporting its status
	trackerFinish <- true

	if err != nil {
		panic(err)
	}
}
