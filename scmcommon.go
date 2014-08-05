package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
)

func findSandbox(startingPath string) (p string) {
	_, err := os.Stat(startingPath)
	if err != nil {
		return startingPath
	}

	p = startingPath
	p = filepath.Clean(p)

	for p != "." && !strings.HasSuffix(p, "/") {
		_, err = os.Stat(filepath.Join(p, metadataFileName))
		if err == nil {
			return p
		}

		p = filepath.Dir(p)
	}

	return startingPath
}

// TODO do we really need this?
func fetchFSObject(client *Client, request *http.Request) *FileInfo {
	resp, err := client.Do(request)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		fmt.Printf("Response Status: %v\n", resp.StatusCode)
		b, _ := ioutil.ReadAll(resp.Body)
		fmt.Printf("Response Body\n%v\n", string(b))
		panic("Error")
	}
	fsObject := &FileInfo{}
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}
	err = json.Unmarshal(b, fsObject)
	if err != nil {
		panic(err)
	}

	return fsObject
}

func FindRepositoryWorkspace(client *Client, ccmBaseUrl, workspaceName string) (string, error) {
	// Fetch all of the user's repository workspaces

	url := path.Join(ccmBaseUrl, "/service/com.ibm.team.filesystem.service.jazzhub.IOrionFilesystem/pa")
	url = strings.Replace(url, ":/", "://", 1)

	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	resp, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := ioutil.ReadAll(resp.Body)
		body := string(b)
		return "", &FileError{Msg: resp.Status, StatusCode: resp.StatusCode, Body: body}
	}

	// The filesystem service renders the list of workspaces as a directory.
	// Decode into a file object so that we can get the workspaces, their names and the item ID's
	workspaceList := &FileInfo{}
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", &FileError{Msg: err.Error()}
	}
	err = json.Unmarshal(b, workspaceList)
	if err != nil {
		return "", &FileError{Msg: err.Error()}
	}

	// Return the first workspace that matches the name
	for _, w := range workspaceList.Children {
		if w.Name == workspaceName {
			return w.ScmInfo.ItemId, nil
		}
	}

	return "", nil
}

func FindStream(client *Client, ccmBaseUrl, projectName, streamName string) (string, error) {
	// Fetch all of the user's repository workspaces

	url := path.Join(ccmBaseUrl, "/service/com.ibm.team.filesystem.service.jazzhub.IOrionFilesystem/pa", projectName)
	url = strings.Replace(url, ":/", "://", 1)

	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	resp, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := ioutil.ReadAll(resp.Body)
		body := string(b)
		return "", &FileError{Msg: resp.Status, StatusCode: resp.StatusCode, Body: body}
	}

	// The filesystem service renders the list of streams as a directory.
	// Decode into a file object so that we can get the stream, its name and the item ID's
	streamList := &FileInfo{}
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", &FileError{Msg: err.Error()}
	}
	err = json.Unmarshal(b, streamList)
	if err != nil {
		return "", &FileError{Msg: err.Error()}
	}

	// Return the first stream that matches the name
	for _, s := range streamList.Children {
		if s.Name == streamName {
			return s.ScmInfo.ItemId, nil
		}
	}

	return "", nil
}

func FindComponents(client *Client, ccmBaseUrl string, workspaceId string) ([]string, error) {
	url := path.Join(ccmBaseUrl, "/service/com.ibm.team.filesystem.service.jazzhub.IOrionFilesystem/pa/_/", workspaceId)
	url = strings.Replace(url, ":/", "://", 1)
	result := []string{}

	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return result, err
	}

	resp, err := client.Do(request)
	if err != nil {
		return result, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := ioutil.ReadAll(resp.Body)
		body := string(b)
		return result, &FileError{Msg: resp.Status, StatusCode: resp.StatusCode, Body: body}
	}

	// The filesystem service renders the workspace as a directory.
	// Decode into a file object so that we can get the components and the item ID's
	workspace := &FileInfo{}
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return result, &FileError{Msg: err.Error()}
	}
	err = json.Unmarshal(b, workspace)
	if err != nil {
		return result, &FileError{Msg: err.Error()}
	}

	for _, component := range workspace.Children {
		result = append(result, component.ScmInfo.ItemId)
	}

	return result, nil
}

type File struct {
	client  *Client
	url     string
	etag    string
	info    FileInfo
	reading io.ReadCloser
}

type FileInfo struct {
	Name      string
	Directory bool
	Children  []FileInfo
	ScmInfo   ScmInfo `json:"RTCSCM"`
}

type ScmInfo struct {
	ComponentId string
	ItemId      string
	StateId     string
}

type FileError struct {
	Msg        string
	StatusCode int
	Body       string
}

func (fe *FileError) Error() string {
	return fe.Msg
}

func Open(client *Client, ccmBaseUrl string, workspaceId string, componentId string, p string) (*File, error) {
	f := &File{}
	f.client = client
	f.url = path.Join(ccmBaseUrl, "/service/com.ibm.team.filesystem.service.jazzhub.IOrionFilesystem/pa/_", workspaceId, componentId, p)
	f.url = strings.Replace(f.url, ":/", "://", 1)

	request, err := http.NewRequest("GET", f.url, nil)
	if err != nil {
		return nil, &FileError{Msg: err.Error()}
	}

	resp, err := client.Do(request)
	if err != nil {
		return nil, &FileError{Msg: err.Error()}
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := ioutil.ReadAll(resp.Body)
		body := string(b)
		// The service returns 500 instead of 404
		if resp.StatusCode == 500 && strings.Contains(body, "Failed to resolve path:") {
			return nil, &FileError{Msg: "Not Found", StatusCode: 404, Body: body}
		}
		return nil, &FileError{Msg: resp.Status, StatusCode: resp.StatusCode, Body: body}
	}
	info := &FileInfo{}
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, &FileError{Msg: err.Error()}
	}
	err = json.Unmarshal(b, info)
	if err != nil {
		return nil, &FileError{Msg: err.Error()}
	}

	f.info = *info

	// The etag returned from the server may have the form W/"c <compSyncTime> ...".
	// We want the sync time.
	etag := resp.Header.Get("ETag")
	etagComponents := strings.Split(etag, "\"")
	etag = etagComponents[1]
	etagComponents = strings.Split(etag, " ")
	etag = etagComponents[1]

	f.etag = etag

	return f, nil
}

func Create(client *Client, ccmBaseUrl string, workspaceId string, componentId, p string) (*File, error) {
	f := &File{}
	f.client = client

	f.url = path.Join(ccmBaseUrl, "/service/com.ibm.team.filesystem.service.jazzhub.IOrionFilesystem/pa/_", workspaceId, componentId, p)
	f.url = strings.Replace(f.url, ":/", "://", 1)

	parentPath := path.Dir(p)
	fileName := path.Base(p)

	createUrl := path.Join(ccmBaseUrl, "/service/com.ibm.team.filesystem.service.jazzhub.IOrionFilesystem/pa/_", workspaceId, componentId, parentPath) + "?op=createFile&name=" + fileName
	createUrl = strings.Replace(createUrl, ":/", "://", 1)

	request, err := http.NewRequest("POST", createUrl, nil)
	if err != nil {
		return nil, &FileError{Msg: err.Error()}
	}

	resp, err := client.Do(request)
	if err != nil {
		return nil, &FileError{Msg: err.Error()}
	}

	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := ioutil.ReadAll(resp.Body)
		body := string(b)
		// The service returns 500 instead of 404
		if resp.StatusCode == 500 && strings.Contains(body, "Failed to resolve path:") {
			return nil, &FileError{Msg: "Not Found", StatusCode: 404, Body: body}
		}
		return nil, &FileError{Msg: resp.Status, StatusCode: resp.StatusCode, Body: body}
	}
	info := &FileInfo{}
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, &FileError{Msg: err.Error()}
	}
	err = json.Unmarshal(b, info)
	if err != nil {
		return nil, &FileError{Msg: err.Error()}
	}

	f.info = *info

	// The etag returned from the server may have the form W/"c <compSyncTime> ...".
	// We want the sync time.
	etag := resp.Header.Get("ETag")
	etagComponents := strings.Split(etag, "\"")
	etag = etagComponents[1]
	etagComponents = strings.Split(etag, " ")
	etag = etagComponents[1]

	f.etag = etag

	return f, nil
}

func Mkdir(client *Client, ccmBaseUrl string, workspaceId string, componentId, p string) (*File, error) {
	f := &File{}
	f.client = client

	f.url = path.Join(ccmBaseUrl, "/service/com.ibm.team.filesystem.service.jazzhub.IOrionFilesystem/pa/_", workspaceId, componentId, p)
	f.url = strings.Replace(f.url, ":/", "://", 1)

	parentPath := path.Dir(p)
	fileName := path.Base(p)

	createUrl := path.Join(ccmBaseUrl, "/service/com.ibm.team.filesystem.service.jazzhub.IOrionFilesystem/pa/_", workspaceId, componentId, parentPath) + "?op=createFolder&name=" + fileName
	createUrl = strings.Replace(createUrl, ":/", "://", 1)

	request, err := http.NewRequest("POST", createUrl, nil)
	if err != nil {
		return nil, &FileError{Msg: err.Error()}
	}

	resp, err := client.Do(request)
	if err != nil {
		return nil, &FileError{Msg: err.Error()}
	}

	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := ioutil.ReadAll(resp.Body)
		body := string(b)
		// The service returns 500 instead of 404
		if resp.StatusCode == 500 && strings.Contains(body, "Failed to resolve path:") {
			return nil, &FileError{Msg: "Not Found", StatusCode: 404, Body: body}
		}
		return nil, &FileError{Msg: resp.Status, StatusCode: resp.StatusCode, Body: body}
	}
	info := &FileInfo{}
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, &FileError{Msg: err.Error()}
	}
	err = json.Unmarshal(b, info)
	if err != nil {
		return nil, &FileError{Msg: err.Error()}
	}

	f.info = *info

	// The etag returned from the server may have the form W/"c <compSyncTime> ...".
	// We want the sync time.
	etag := resp.Header.Get("ETag")
	etagComponents := strings.Split(etag, "\"")
	etag = etagComponents[1]
	etagComponents = strings.Split(etag, " ")
	etag = etagComponents[1]

	f.etag = etag

	return f, nil
}

func (f *File) Read(p []byte) (int, error) {
	if f.reading == nil {
		request, err := http.NewRequest("GET", f.url+"?op=readContent", nil)
		if err != nil {
			return 0, &FileError{Msg: err.Error()}
		}

		resp, err := f.client.Do(request)
		if err != nil {
			return 0, &FileError{Msg: err.Error()}
		}

		if resp.StatusCode != 200 {
			b, _ := ioutil.ReadAll(resp.Body)
			body := string(b)

			defer resp.Body.Close()

			// The service returns 500 instead of 404
			if resp.StatusCode == 500 && strings.Contains(body, "Failed to resolve path:") {
				return 0, &FileError{Msg: "Not Found", StatusCode: 404, Body: body}
			}
			return 0, &FileError{Msg: resp.Status, StatusCode: resp.StatusCode, Body: body}
		}

		f.reading = resp.Body
	}

	return f.reading.Read(p)
}

func (f *File) Write(contents io.Reader) error {
	request, err := http.NewRequest("POST", f.url+"?opt=writeContent", contents)
	if err != nil {
		return &FileError{Msg: err.Error()}
	}

	resp, err := f.client.Do(request)
	if err != nil {
		return &FileError{Msg: err.Error()}
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := ioutil.ReadAll(resp.Body)
		body := string(b)
		// The service returns 500 instead of 404
		if resp.StatusCode == 500 && strings.Contains(body, "Failed to resolve path:") {
			return &FileError{Msg: "Not Found", StatusCode: 404, Body: body}
		}
		return &FileError{Msg: resp.Status, StatusCode: resp.StatusCode, Body: body}
	}

	return nil
}

func (f *File) Close() error {
	if f.reading != nil {
		toClose := f.reading
		f.reading = nil

		return toClose.Close()
	}

	return nil
}

type WalkFunc func(path string, file File) error

type walkData struct {
	client       *Client
	ccmBaseUrl   string
	workspaceId  string
	componentId  string
	wf           WalkFunc
	startingEtag string
	path         string
	queue        chan walkData
	workTracker  chan bool
}

func Walk(client *Client, ccmBaseUrl string, workspaceId string, componentId string, wf WalkFunc) error {
	// Walk doesn't callback for the component root
	root, err := Open(client, ccmBaseUrl, workspaceId, componentId, "/")
	if err != nil {
		return err
	}

	// We track the etag through this whole process to make sure that the configuration
	//  doesn't change in the middle.
	startingEtag := root.etag

	walkDataQueue := make(chan walkData)
	workTracker := make(chan bool)
	finished := make(chan bool)

	numGoRoutines := 10

	var firstError error = nil
	errMutex := &sync.Mutex{}

	go func() {
		work := 0

		for {
			workAdded := <-workTracker
			if workAdded {
				work += 1
			} else {
				work -= 1

				if work == 0 {
					// Send everyone (calling goroutine plus all helpers) the signal that they are finished
					for i := 0; i < numGoRoutines+1; i++ {
						finished <- true
					}
					return
				}
			}
		}
	}()

	for i := 0; i < numGoRoutines; i++ {
		go func() {
			for {
				select {
				case data := <-walkDataQueue:

					err := internalWalk(data)
					workTracker <- false

					if err != nil {
						errMutex.Lock()
						if firstError == nil {
							firstError = err
						}
						errMutex.Unlock()
					}
				case <-finished:
					return
				}
			}
		}()
	}

	workTracker <- true

	for _, childInfo := range root.info.Children {
		p := childInfo.Name

		childData := walkData{
			client:       client,
			ccmBaseUrl:   ccmBaseUrl,
			workspaceId:  workspaceId,
			componentId:  componentId,
			wf:           wf,
			startingEtag: startingEtag,
			path:         p,
			queue:        walkDataQueue,
			workTracker:  workTracker,
		}

		// Try to push this child on the queue, otherwise simply recurse
		//  if nobody is listening.
		workTracker <- true
		select {
		case walkDataQueue <- childData:
		default:
			err = internalWalk(childData)
			workTracker <- false

			if err != nil {
				errMutex.Lock()
				if firstError == nil {
					firstError = err
				}
				errMutex.Unlock()
				break
			}
		}
	}

	workTracker <- false
	<-finished

	errMutex.Lock()
	retVal := firstError
	errMutex.Unlock()

	return retVal
}

func internalWalk(data walkData) error {
	f, err := Open(data.client, data.ccmBaseUrl, data.workspaceId, data.componentId, data.path)
	if err != nil {
		return err
	}

	err = data.wf(data.path, *f)
	if err != nil {
		return err
	}

	for _, childInfo := range f.info.Children {
		p := filepath.Join(data.path, childInfo.Name)

		childData := walkData{
			client:       data.client,
			ccmBaseUrl:   data.ccmBaseUrl,
			workspaceId:  data.workspaceId,
			componentId:  data.componentId,
			wf:           data.wf,
			startingEtag: data.startingEtag,
			path:         p,
			queue:        data.queue,
			workTracker:  data.workTracker,
		}

		data.workTracker <- true
		select {
		case data.queue <- childData:
		default:
			err = internalWalk(childData)
			data.workTracker <- false

			if err != nil {
				return err
			}
		}
	}

	return nil
}
