package main

import (
	"encoding/gob"
	"os"
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
	storeMeta     chan MetaObject
	sync          chan int
}

func NewMetaData() *MetaData {
	metadata := &MetaData{}

	metadata.pathMap = make(map[string]MetaObject)
	metadata.componentEtag = make(map[string]string)
	metadata.storeMeta = make(chan MetaObject)
	metadata.sync = make(chan int)

	go func() {
		// TODO shutdown
		for {
			select {
			case data := <-metadata.storeMeta:
				metadata.pathMap[data.Path] = data
			case <-metadata.sync:

			}
		}
	}()

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
	// Synchronize first and then write out the metadata
	metadata.sync <- 1

	file, err := os.Create(path)
	if err == nil {
		encoder := gob.NewEncoder(file)
		err = encoder.Encode(&metadata.pathMap)
		err = encoder.Encode(&metadata.componentEtag)
	}

	return err
}

func (metadata *MetaData) Put(obj MetaObject) {
	metadata.storeMeta <- obj
}

func (metadata *MetaData) Get(path string) MetaObject {
	return metadata.pathMap[path]
}
