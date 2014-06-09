package main

import (
	"encoding/gob"
	"os"
	"path/filepath"
	"strings"
)

const (
	metadataFileName = ".jazzmeta"
)

// TODO convert into a smaller object
type metaObject struct {
	Path        string
	ItemId      string
	StateId     string
	LasModified int64
	Size        int64
	Hash        string
	ComponentId string
}

type metaData struct {
	pathMap       map[string]metaObject
	componentEtag map[string]string
	workspaceName string
	isstream      bool
	workspaceId   string
	projectUrl    string
	userId        string

	inited    bool
	storeMeta chan metaObject
	sync      chan int
}

func newMetaData() *metaData {
	metadata := &metaData{}

	metadata.pathMap = make(map[string]metaObject)
	metadata.componentEtag = make(map[string]string)

	metadata.inited = false

	return metadata
}

func (metadata *metaData) load(path string) error {
	file, err := os.Open(path)
	if err == nil {
		decoder := gob.NewDecoder(file)
		err = decoder.Decode(&metadata.workspaceName)
		err = decoder.Decode(&metadata.isstream)
		err = decoder.Decode(&metadata.workspaceId)
		err = decoder.Decode(&metadata.projectUrl)
		err = decoder.Decode(&metadata.userId)
		err = decoder.Decode(&metadata.pathMap)
		err = decoder.Decode(&metadata.componentEtag)
	}

	return err
}

func (metadata *metaData) save(path string) error {
	if metadata.inited {
		// Synchronize first and then write out the metadata
		metadata.sync <- 1
	}

	file, err := os.Create(path)
	if err == nil {
		encoder := gob.NewEncoder(file)
		err = encoder.Encode(&metadata.workspaceName)
		err = encoder.Encode(&metadata.isstream)
		err = encoder.Encode(&metadata.workspaceId)
		err = encoder.Encode(&metadata.projectUrl)
		err = encoder.Encode(&metadata.userId)
		err = encoder.Encode(&metadata.pathMap)
		err = encoder.Encode(&metadata.componentEtag)
	}

	return err
}

func (metadata *metaData) initConcurrentWrite() {
	metadata.storeMeta = make(chan metaObject)
	metadata.sync = make(chan int)

	metadata.inited = true

	go func() {
		for {
			select {
			case data := <-metadata.storeMeta:
				metadata.pathMap[data.Path] = data
			case <-metadata.sync:
				// Shutdown after synchronizing
				metadata.inited = false
				return
			}
		}
	}()
}

func (metadata *metaData) put(obj metaObject, sandboxpath string) {
	if !metadata.inited {
		panic("Metadata is not initialized for concurrent write, call InitConcurentWrite() first.")
	}

	// Reduce the path of the metadata object using the sandbox path
	//  this will dramatically decrease the size of the metadata
	relpath, err := filepath.Rel(sandboxpath, obj.Path)

	if err != nil {
		panic(err)
	}

	obj.Path = relpath

	metadata.storeMeta <- obj
}

func (metadata *metaData) get(path string, sandboxpath string) (metaObject, bool) {
	// All metadata lookups are based on relative path
	relpath, err := filepath.Rel(sandboxpath, path)

	if err != nil {
		panic(err)
	}

	meta, hit := metadata.pathMap[relpath]

	// This may be an zero (empty) metadata object (ie. a miss on the metadata map)
	//   Don't do any manipulation of the path
	if hit {
		meta.Path = filepath.Join(sandboxpath, meta.Path)
	}

	return meta, hit
}

func (metadata *metaData) projectId() string {
	projectUrlParts := strings.Split(metadata.projectUrl, "/")
	return projectUrlParts[len(projectUrlParts)-1]
}