package main

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/howeyc/gopass"
)

const (
	numGoRoutines     = 10
	bufferSize        = 1000
	PASSWORD_FILE_ENV = "DOS_PASSWORD_FILE"
)

var (
	password = ""
)

func init() {
	if password == "" {
		file := os.Getenv(PASSWORD_FILE_ENV)
		if file != "" {
			f, err := os.Open(file)
			if err == nil {
				defer f.Close()
				b, err := ioutil.ReadAll(f)
				if err == nil {
					password = string(b)
				}
			}
		}
	}
}

func loadDefaults() {
	fmt.Printf("gojazz load [<project name> [options]]\n")
	flag.PrintDefaults()
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
	force := flag.Bool("force", false, "Force the load to overwrite any files. Don't prompt.")
	flag.Usage = loadDefaults
	flag.Parse()

	if *sandboxPath == "" {
		path, err := os.Getwd()
		if err != nil {
			panic(err)
		}

		path = findSandbox(path)
		sandboxPath = &path
	}

	if *workspace && *userId == "" {
		fmt.Println("You must provide credentials to use a repository workspace.")
		loadDefaults()
		return
	}

	// Get the existing status of the sandbox, if available
	// Back up any changes that are found
	status, _ := scmStatus(*sandboxPath, BACKUP)

	if status != nil && !status.unchanged() {
		fmt.Printf("Here was the status of your sandbox before loading:\n%v", status)
		fmt.Printf("Your changes have been backed up to this location: %v\n", status.copyPath)
	}

	// Re-use the existing user ID from the metadata, if available
	if *userId == "" && status != nil && status.metaData.userId != "" {
		userId = &status.metaData.userId
	}

	if *userId != "" && password == "" {
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
			fmt.Println("Provide a project to load and try again.")
			loadDefaults()
			return
		}

		project, err := client.findProject(projectName)
		if err != nil {
			panic(err)
		}
		ccmBaseUrl = project.CcmBaseUrl

		// Find a repository workspace with the correct naming convention
		// Failing that, create one.
		if *workspace {
			isstream = false

			streamId := ""

			// User has provided a stream that they want to work on
			if *stream != "" {
				// TODO someday we will support the ability to work on different streams
				//	streamId, err = FindStream(client, ccmBaseUrl, projectName, *stream)
				//	if err != nil {
				//		panic(err)
				//	}
				//	if streamId == "" {
				//		panic(errors.New("Stream with name " + *stream + " not found"))
				//	}
				panic(simpleWarning("Sorry, we don't yet support loading repository workspaces from a specific stream. You can only use the default for now."))
			} else {
				// Otherwise, use a stream that matches the naming convention
				streamId, err = FindStream(client, ccmBaseUrl, projectName, projectName+" Stream")
				if err != nil {
					// TODO perhaps we should prompt the user in this case?
					panic(err)
				}
				if streamId == "" {
					panic(simpleWarning("The default stream for the project could not be found. Is it a Git project?"))
				}
			}

			workspaceId, err = FindWorkspaceForStream(client, ccmBaseUrl, streamId)
			if err != nil {
				panic(err)
			}
			if workspaceId == "" {
				// TODO someday we will be able to create a repository workspace from a stream, for now we use the init project rest call and hope that the workspace is for the stream the user specified
				//	workspaceId, err = CreateWorkspaceFromStream(client, ccmBaseUrl, projectName, *userId, streamId, projectName+" Stream")
				//	if err != nil {
				//		panic(err)
				//	}

				workspaceId, err = initWebIdeProject(client, project, *userId)

				if err != nil {
					panic(err)
				}
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
					panic(simpleWarning("Stream with name " + *stream + " not found"))
				}
			} else {
				// Use the stream with the form "user | projectName Stream"
				workspaceId, err = FindStream(client, ccmBaseUrl, projectName, projectName+" Stream")
				if err != nil {
					panic(err)
				}

				if workspaceId == "" {
					panic(simpleWarning("No default stream could be found for this project. Is it a Git project?"))
				}
			}
		}
	} else {
		projectName = status.metaData.projectName
		isstream = status.metaData.isstream
		workspaceId = status.metaData.workspaceId
		ccmBaseUrl = status.metaData.ccmBaseUrl
	}

	if isstream {
		fmt.Printf("Note: Loading from a stream will not allow you to contribute changes. You must load again using the '-workspace=true' option.\n")
	}

	scmLoad(client, ccmBaseUrl, projectName, workspaceId, isstream, *userId, *sandboxPath, status, *force)

	fmt.Printf("Load Successful\n")

	// If we loaded from a repository workspace then init the web IDE project and
	//  provide a URL for them to manage their changes
	if !isstream {
		project, err := client.findProject(projectName)
		if err != nil {
			panic(err)
		}
		_, err = initWebIdeProject(client, project, *userId)
		if err != nil {
			panic(err)
		}

		fmt.Println("Visit the following link to work with your repository workspace:")
		redirect := fmt.Sprintf(jazzHubBaseUrl + "/code/jazzui/changes.html#" + "/code/jazz/Changes/_/file/" + client.GetJazzId() + "-OrionContent/" + projectName)
		fmt.Printf("https://login.jazz.net/psso/proxy/jazzlogin?redirect_uri=%v\n", url.QueryEscape(redirect))
	}
}

func scmLoad(client *Client, ccmBaseUrl string, projectName string, workspaceId string, stream bool, userId string, sandbox string, status *status, force bool) {
	newMetaData := newMetaData()
	newMetaData.initConcurrentWrite()
	newMetaData.isstream = stream
	newMetaData.userId = userId
	newMetaData.ccmBaseUrl = ccmBaseUrl
	newMetaData.projectName = projectName
	newMetaData.workspaceId = workspaceId

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

			if len(children) > 0 && !force {
				fmt.Println("There are files in the sandbox directory that will be replaced with the remote files.")
				fmt.Print("Do you want to proceed? [Y/n]:")
				reader := bufio.NewReader(os.Stdin)
				answer, _ := reader.ReadString('\n')
				answer = strings.TrimSpace(answer)

				if strings.ToLower(answer) == "n" {
					panic(simpleWarning("Operation Canceled"))
				}
			}
		}
	}

	// Delete the old metadata
	metadataFile := filepath.Join(sandbox, metadataFileName)
	os.Remove(metadataFile)

	// Find all of the components of the remote workspace and then walk over each one
	componentIds, err := FindComponentIds(client, ccmBaseUrl, workspaceId)
	if err != nil {
		panic(err)
	}

	// Walk through the remote components creating directories, if necessary and cleaning up any deleted files
	for _, componentId := range componentIds {
		loadComponent(client, ccmBaseUrl, workspaceId, componentId, sandbox, newMetaData, status)
	}

	// Do a final pass over the top-level elements in the sandbox
	//  to remove any that are no longer registered in the metadata.
	s, err := os.Open(sandbox)
	if err != nil {
		panic(err)
	}
	roots, err := s.Readdirnames(-1)
	if err != nil {
		panic(err)
	}
	for _, root := range roots {
		rootPath := filepath.Join(sandbox, root)

		ignored, err := IsIgnored(rootPath)
		if err != nil {
			panic(err)
		}

		if ignored {
			continue
		}

		_, ok := newMetaData.get(rootPath, sandbox)

		if !ok {
			err = os.RemoveAll(rootPath)
			if err != nil {
				panic(err)
			}
		}
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

				// Backspace and space out the last line that was printed
				for i := 0; i < lastStringLength; i++ {
					fmt.Printf("\b")
				}
				for i := 0; i < lastStringLength; i++ {
					fmt.Printf(" ")
				}
				for i := 0; i < lastStringLength; i++ {
					fmt.Printf("\b")
				}

				bytesLoaded := ""
				// TODO handle Gigabytes?
				if transferred > (1024 * 1024) {
					bytesLoaded = strconv.FormatInt(transferred/(1024*1024), 10) + "MB"
				} else if transferred > 1024 {
					bytesLoaded = strconv.FormatInt(transferred/1024, 10) + "KB"
				} else {
					bytesLoaded = strconv.FormatInt(transferred, 10) + "B"
				}

				lastStringLength, _ = fmt.Printf("Loaded %v (of %v) files. %v", worked, work, bytesLoaded)
			case <-trackerFinish:
				return
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
					// We try one more time with a small timeout
					<-time.After(10 * time.Millisecond)
					remoteFile, err = Open(client, ccmBaseUrl, workspaceId, componentId, pathToDownload)
					if err != nil {
						panic(err)
					}
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
			} else {
				// Check for any children that aren't on the remote
				// Remove the extra ones that aren't being ignored
				localDirectory, err := os.Open(localPath)
				if err != nil {
					return err
				}

				localChildren, err := localDirectory.Readdirnames(-1)
				for _, localChild := range localChildren {
					existsOnRemote := false

					for _, remoteChild := range file.info.Children {
						if remoteChild.Name == localChild {
							existsOnRemote = true
							break
						}
					}

					if !existsOnRemote {
						localChildPath := filepath.Join(localPath, localChild)
						ignored, err := IsIgnored(localChildPath)
						if err != nil {
							return err
						}
						if !ignored {
							err := os.RemoveAll(localChildPath)
							if err != nil {
								return err
							}
						}
					}
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
	// Complete the newline for the progress tracker
	fmt.Printf("\n")

	if err != nil {
		panic(err)
	}
}
