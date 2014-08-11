package main

import (
	"crypto/sha1"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type mode int

const (
	stageFolder  = ".jazzstage"
	backupFolder = ".jazzbackup"

	STAGE mode = iota
	BACKUP
	NO_COPY
)

func statusDefaults() {
	fmt.Printf("gojazz status [options]\n")
	flag.PrintDefaults()
}

type status struct {
	Added    map[string]bool
	Modified map[string]bool
	Deleted  map[string]bool

	metaData *metaData

	sandboxPath string
	copyPath    string
}

func newStatus(sandboxPath string, m mode) *status {
	status := &status{}
	status.Added = make(map[string]bool)
	status.Modified = make(map[string]bool)
	status.Deleted = make(map[string]bool)

	status.sandboxPath = sandboxPath

	if m == STAGE {
		status.copyPath = filepath.Join(status.sandboxPath, stageFolder)
	} else if m == BACKUP {
		status.copyPath = filepath.Join(status.sandboxPath, backupFolder)
	}

	return status
}

func (status *status) unchanged() bool {
	return len(status.Added) == 0 && len(status.Modified) == 0 && len(status.Deleted) == 0
}

func (status *status) String() string {
	// TODO hit the server and find the current name of this workspace
	//result := status.metaData.workspaceName + "\n"

	result := ""

	if status.metaData.isstream {
		result = result + "Type: Stream\n"
	} else {
		result = result + "Type: Repository Workspace\n"
	}

	nochanges := true

	for k, _ := range status.Added {
		result = result + k + " (Added)\n"
		nochanges = false
	}

	for k, _ := range status.Modified {
		result = result + k + " (Modified)\n"
		nochanges = false
	}

	for k, _ := range status.Deleted {
		result = result + k + " (Deleted)\n"
		nochanges = false
	}

	if nochanges {
		result = result + "No local changes\n"
	}

	return result
}

func statusOp() {
	sandboxPath := flag.String("sandbox", "", "Location of the sandbox to load the files")
	flag.Usage = statusDefaults
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
	status, err := scmStatus(*sandboxPath, NO_COPY)

	if err != nil {
		panic(err)
	}

	fmt.Printf("%v", status)
}

func scmStatus(sandboxPath string, m mode) (*status, error) {
	// Load up existing metadata and prepare fresh metadata
	oldMetaData := newMetaData()
	// If the load fails, it's not a problem, just empty
	err := oldMetaData.load(filepath.Join(sandboxPath, metadataFileName))

	if err != nil {
		return nil, simpleWarning("Not a sandbox")
	}

	status := newStatus(sandboxPath, m)
	status.metaData = oldMetaData

	// Delete any existing staging area
	if m == STAGE {
		err = os.RemoveAll(status.copyPath)
		if err != nil {
			panic(err)
		}
	}

	// Walk the current directory structure looking for changed items
	err = filepath.Walk(sandboxPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip the metadata, staging and backup directories
		if path == sandboxPath || filepath.Base(path) == metadataFileName || strings.Contains(path, stageFolder) || strings.Contains(path, backupFolder) {
			return nil
		}

		// Skip binary directories
		if strings.HasSuffix(path, "/bin") {
			return filepath.SkipDir
		}

		meta, ok := oldMetaData.get(path, sandboxPath)

		// Metadata doesn't exist for this file, so it must be added
		if !ok {
			status.fileAdded(path, sandboxPath)
			return nil
		}

		if !info.IsDir() {
			// Check the modified time
			if meta.LastModified != info.ModTime().Unix() {
				// Different sizes mean that the file has changed for sure
				if meta.Size != info.Size() {
					status.fileModified(meta, path, sandboxPath)
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

					newHash := base64.StdEncoding.EncodeToString(hash.Sum(nil))

					if meta.Hash != newHash {
						status.fileModified(meta, path, sandboxPath)
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
	for path, meta := range oldMetaData.pathMap {
		fullpath := filepath.Join(sandboxPath, path)
		_, err := os.Stat(fullpath)
		if err != nil {
			status.fileDeleted(meta, fullpath, sandboxPath)
		}
	}

	return status, nil
}

func (status *status) calcCopyPath(path string) string {
	if status.copyPath == "" {
		return ""
	}

	relpath, err := filepath.Rel(status.sandboxPath, path)

	if err != nil {
		panic(err)
	}

	return filepath.Join(status.copyPath, relpath)
}

func (status *status) fileAdded(path string, sandboxPath string) {
	rel, err := filepath.Rel(sandboxPath, path)
	if err != nil {
		panic(err)
	}

	s, err := os.Stat(path)
	if err != nil {
		panic(err)
	}

	// Should we ignore this new file?
	// Check for signs that it isn't source code
	//  -Really big file (>1MB)
	//  -Control characters
	//  -File extension (e.g. .exe, .dll, .so)
	//  -Temporary files left by editors (e.g. *~, *.ext.swp)
	if !s.IsDir() {
		base := filepath.Base(path)
		if strings.HasSuffix(base, ".exe") || strings.HasSuffix(base, ".dll") || strings.HasSuffix(base, ".so") {
			return
		}

		if strings.HasSuffix(base, "~") || strings.HasSuffix(base, ".ext.swp") {
			return
		}

		if s.Size() > 1024^2 {
			return
		}

		origFile, err := os.Open(path)
		if err != nil {
			panic(err)
		}
		buffer := make([]byte, 1024, 1024)
		for n, _ := origFile.Read(buffer); n > 0; {
			for i := 0; i < n; i++ {
				if buffer[i] < 32 && buffer[i] != '\r' && buffer[i] != 'n' {
					origFile.Close()
					return
				}
			}
		}
		origFile.Close()
	}

	status.Added[rel] = true
	copyPath := status.calcCopyPath(path)

	if copyPath != "" {
		if s.IsDir() {
			os.MkdirAll(copyPath, 0700)
		} else {
			os.MkdirAll(filepath.Dir(copyPath), 0700)

			stagedFile, err := os.Create(copyPath)
			if err != nil {
				panic(err)
			}
			defer stagedFile.Close()
			origFile, err := os.Open(path)
			if err != nil {
				panic(err)
			}
			defer origFile.Close()

			_, err = io.Copy(stagedFile, origFile)
			if err != nil {
				panic(err)
			}
		}
	}
}

func (status *status) fileModified(meta metaObject, path string, sandboxPath string) {
	rel, err := filepath.Rel(sandboxPath, path)
	if err != nil {
		panic(err)
	}

	status.Modified[rel] = true
	copyPath := status.calcCopyPath(path)

	if copyPath != "" {
		s, err := os.Stat(path)

		if err != nil {
			panic(err)
		}

		if s.IsDir() {
			os.MkdirAll(copyPath, 0700)
		} else {
			os.MkdirAll(filepath.Dir(copyPath), 0700)

			stagedFile, err := os.Create(copyPath)
			if err != nil {
				panic(err)
			}
			defer stagedFile.Close()
			origFile, err := os.Open(path)
			if err != nil {
				panic(err)
			}
			defer origFile.Close()

			_, err = io.Copy(stagedFile, origFile)
			if err != nil {
				panic(err)
			}
		}
	}
}

func (status *status) fileDeleted(meta metaObject, path string, sandboxPath string) {
	rel, err := filepath.Rel(sandboxPath, path)
	if err != nil {
		panic(err)
	}

	status.Deleted[rel] = true
}
