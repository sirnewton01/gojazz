package main

import (
	"encoding/gob"
	"os"
	"path/filepath"
)

// TODO convert into a smaller object
type MetaObject struct {
	Path        string
	ItemId      string
	StateId     string
	LasModified int64
	Size        int64
	Hash        string
}

type MetaData struct {
	pathMap       map[string]MetaObject
	componentEtag map[string]string

	inited    bool
	storeMeta chan MetaObject
	sync      chan int
}

func NewMetaData() *MetaData {
	metadata := &MetaData{}

	metadata.pathMap = make(map[string]MetaObject)
	metadata.componentEtag = make(map[string]string)

	metadata.inited = false

	return metadata
}

func (metadata *MetaData) Load(path string) error {
	file, err := os.Open(path)
	if err == nil {
		decoder := gob.NewDecoder(file)
		err = decoder.Decode(&metadata.pathMap)
		err = decoder.Decode(&metadata.componentEtag)
	}

	return err
}

func (metadata *MetaData) Save(path string) error {
	if metadata.inited {
		// Synchronize first and then write out the metadata
		metadata.sync <- 1
	}

	file, err := os.Create(path)
	if err == nil {
		encoder := gob.NewEncoder(file)
		err = encoder.Encode(&metadata.pathMap)
		err = encoder.Encode(&metadata.componentEtag)
	}

	return err
}

func (metadata *MetaData) InitConcurrentWrite() {
	metadata.storeMeta = make(chan MetaObject)
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

func (metadata *MetaData) Put(obj MetaObject, sandboxpath string) {
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

func (metadata *MetaData) Get(path string, sandboxpath string) (MetaObject, bool) {
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
