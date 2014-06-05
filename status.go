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
)

type Status struct {
	Added    map[string]bool
	Modified map[string]bool
	Deleted  map[string]bool
	metaData *MetaData
}

func NewStatus() *Status {
	status := &Status{}
	status.Added = make(map[string]bool)
	status.Modified = make(map[string]bool)
	status.Deleted = make(map[string]bool)

	return status
}

func (status *Status) unchanged() bool {
	return len(status.Added) == 0 && len(status.Modified) == 0 && len(status.Deleted) == 0
}

func (status *Status) String() string {
	result := ""

	for k, _ := range status.Added {
		result = result + k + " (Added)\n"
	}

	for k, _ := range status.Modified {
		result = result + k + " (Modified)\n"
	}

	for k, _ := range status.Deleted {
		result = result + k + " (Deleted)\n"
	}

	if result == "" {
		result = "No local changes\n"
	}

	return result
}

func statusOp() {
	os.Args = os.Args[1:]
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

	fmt.Printf("Status of %v...\n", *sandboxPath)
	status, err := scmStatus(*sandboxPath)
	if err == nil {
		fmt.Printf("%v", status)
	} else {
		fmt.Printf("%v", err.Error())
	}
}

func scmStatus(sandboxPath string) (*Status, error) {
	// Load up existing metadata and prepare fresh metadata
	oldMetaData := NewMetaData()
	// If the load fails, it's not a problem, just empty
	err := oldMetaData.Load(filepath.Join(sandboxPath, ".jazzmeta"))

	if err != nil {
		return nil, errors.New("Not a sandbox")
	}

	status := NewStatus()
	status.metaData = oldMetaData

	// Walk the current directory structure looking for Added and Modified items
	err = filepath.Walk(sandboxPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip the metadata
		if path == sandboxPath || strings.HasSuffix(path, ".jazzmeta") {
			return nil
		}

		meta, ok := oldMetaData.Get(path, sandboxPath)

		// Metadata doesn't exist for this file, so it must be added
		if !ok {
			status.Added[path] = true
			return nil
		}

		if !info.IsDir() {
			// Check the modified time
			if meta.LasModified != info.ModTime().Unix() {
				// Different sizes mean that the file has changed for sure
				if meta.Size != info.Size() {
					status.Modified[path] = true
				} else {
					// Check the hashes
					file, err := os.Open(path)
					if err != nil {
						return err
					}

					hash := sha1.New()
					_, err = io.Copy(hash, file)

					if err != nil {
						return err
					}

					if meta.Hash != base64.StdEncoding.EncodeToString(hash.Sum(nil)) {
						status.Modified[path] = true
					}
				}
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	// Walk the metadata to find any items that don't exist
	for path, _ := range oldMetaData.pathMap {
		_, err := os.Stat(filepath.Join(sandboxPath, path))
		if err != nil {
			status.Deleted[path] = true
		}
	}

	return status, nil
}
