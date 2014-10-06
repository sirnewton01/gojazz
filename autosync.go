
package main

import (
	"container/list"
	"fmt"
	"gopkg.in/fsnotify.v1"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func listenForRepoWorkspaceChanges(client *Client, sandboxPath string, workspaceChangeChan chan bool) {

	for {
		// Load our metadata
		metadata := newMetaData()
		err := metadata.load(filepath.Join(sandboxPath, metadataFileName))
		if err != nil {
			panic("Failed to load metadata")
		}

		// Poll for remote changes
		fmt.Println("Polling ", metadata.workspaceId)
		hasChanges := pollForRepoWorkspaceChanges(client, metadata)
		if hasChanges {
			workspaceChangeChan <- true
		}

		// Wait for a while. Since we're polling this number shouldn't be too small
		time.Sleep(20 * time.Second)
	}
}

func pollForRepoWorkspaceChanges(client *Client, md *metaData) bool {
	for compId := range md.componentEtag {
		comp, err := Open(client, md.ccmBaseUrl, md.workspaceId, compId, "")
		if err != nil {
			panic(err)
		}

		if comp.etag != md.componentEtag[compId] {
			return true
		}
	}

	return false
}

// Run our sync op
func runSync(watcher *fsnotify.Watcher, toUpdate *map[string]bool, sandboxPath string, client *Client) {
	fmt.Println("Running sync...")
	paths := make([]string, len(*toUpdate))
	i := 0
	for key := range *toUpdate {
		paths[i] = key
		i++
	}

	status, err := scmStatusSelectively(sandboxPath, &paths, STAGE)
	if err != nil {
		panic(err)
	}

	doSyncOp(client, sandboxPath, status, false)
	fmt.Println("Sync complete")

	*toUpdate = make(map[string]bool)
}

// Add the directories under the given
func subscribeTree(watcher *fsnotify.Watcher, root string) {
	subscriptionList := list.New() // The directories we have yet to subscribe to
	subscriptionList.PushFront(root)

	for subscriptionList.Len() > 0 {
		el := subscriptionList.Back()
		cur := el.Value.(string)
		subscriptionList.Remove(el)

		// Get our children
		f, err := os.Open(cur)
		if err != nil {
			panic(err)
		}

		infos, err := f.Readdir(-1)
		if err != nil {
			panic(err)
		}

		// Save the children for later processing
		for _, child := range infos {
			if child.IsDir() {
				path := filepath.Join(cur, child.Name())
				subscriptionList.PushFront(path)
			}
		}

		watcher.Add(cur)
	}
}

// Updates our Watcher with newly created directories.
func adjustWatchSet(watcher *fsnotify.Watcher, event fsnotify.Event, sandbox Path) {
	// Update our watcher to watch new directories and ignore deleted directories
	if event.Op&fsnotify.Create == fsnotify.Create || event.Op&fsnotify.Remove == fsnotify.Remove {

		// There's a bunch of files starting with .jazz in our sandbox root that
		// look like metadata and temp directories. Don't subscribe to their events.
		eventPath := pathToArray(event.Name)
		if sandbox.isPrefixOf(eventPath) {
			if len(eventPath) > len(sandbox) && strings.HasPrefix(eventPath[len(sandbox)], ".jazz") {
				fmt.Println("Skipping event \"", event.Name, "\" due to .jazz prefix")
				return
			}
		}

		// If we create a new directory, we need to track its contents as well
		stat, err := os.Lstat(event.Name)
		if err != nil {
			fmt.Println("Failed to stat: ", event.Name)
			return
		}

		if stat.IsDir() {
			if event.Op&fsnotify.Remove == fsnotify.Remove {
				fmt.Println("Incremental directory remove ", event.Name)
				watcher.Remove(event.Name)
			} else if event.Op&fsnotify.Create == fsnotify.Create {
				fmt.Println("Incremental directory add ", event.Name)
				watcher.Add(event.Name)
			}
		}
	}
}

func autosyncOp() {
	// Sanity check: are we running in a sandbox?
	path, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	if !isSandbox(path) {
		panic(simpleWarning("Autosync must run in a sandbox. Use 'gojazz load' to get your content onto disk and create a sandbox."))
	}

	sandbox := pathToArray(path)

	// We're in a sandbox. Start our client
	userId, password, err := getCredentials()
	if err != nil {
		panic(err)
	}

	client, err := NewClient(userId, password)
	if err != nil {
		panic(err)
	}

	workspaceChangeChan := make(chan bool)

	fsListenerControlChan := make(chan bool)

	// Start listening for file changes in the sandbox
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		panic(err)
	}
	defer watcher.Close()

	// This goroutine watches for events. The main go routine (later) can
	// disable it by firing events on the fsListenerControlChan. We
	// allow disabling so that we don't record events that occur during
	// the load.
	fsEventChan := make(chan fsnotify.Event)
	go func() {
		trackEventsForSync := true
		for {
			select {
			case event := <-watcher.Events:
				adjustWatchSet(watcher, event, sandbox)
				if trackEventsForSync {
					fsEventChan <- event
				}

			case newState := <-fsListenerControlChan:
				trackEventsForSync = newState
			}
		}
	}()

	// The following goroutine multiplexes the two tasks that autosync performs:
	// listening for filesystem changes (and committing) and listening for
	// remote changes.
	toUpdate := make(map[string]bool)
	syncTimer := time.NewTimer(time.Second * 500)
	syncTimer.Stop()
	go func() {
		for {
			select {
			case event := <-fsEventChan:
				// File/folder in a watched directory has changed. Record the event,
				//  and set our timer for soonish.
				syncTimer.Reset(time.Second * 5)
				toUpdate[event.Name] = true

			case <-syncTimer.C:
				// Our accumulation timer has elapsed, sync
				fmt.Println("Committing local changes to repository")

				fsListenerControlChan <- false
				runSync(watcher, &toUpdate, path, client)
				fsListenerControlChan <- true

			case <-workspaceChangeChan:
				// The repo workspace has changed, sync
				syncTimer.Stop()

				fsListenerControlChan <- false
				runSync(watcher, &toUpdate, path, client)
				fsListenerControlChan <- true

			case err := <-watcher.Errors:
				fmt.Println("error:", err)
			}
		}
	}()
	subscribeTree(watcher, path)

	// Start listening for changes to the remote workspace
	go listenForRepoWorkspaceChanges(client, path, workspaceChangeChan)

	doneChan := make(chan bool)
	fmt.Println(<-doneChan)
}
