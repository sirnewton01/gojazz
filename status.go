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

// Convenience call to scan the entire sandbox for changes.
func scmStatus(sandboxPath string, m mode) (*status, error) {
	return scmStatusSelectively(sandboxPath, nil, m)
}

// Search the local directory for changes. The scanRoots is an array of full paths
// (ie not relative) within the sandboxPath. scanRoots may be 'nil', in which case
// we search from the sandboxPath.
func scmStatusSelectively(sandboxPath string, scanRoots *[]string, m mode) (*status, error) {
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

	if scanRoots == nil {
		scanRoots = &[]string{sandboxPath}
	}

	// Walk the current directory structure looking for changed items
	for _, scanPath := range *scanRoots {
		err = filepath.Walk(scanPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				// One of the errors we can receive here is that the file no longer
				// exists, which is perfectly reasonable. Continue processing anyway.
				return nil
			}

			if path == sandboxPath {
				return nil
			}

			ignored, err := IsIgnored(path)
			if err != nil {
				return err
			}

			if ignored && info.IsDir() {
				return filepath.SkipDir
			} else if ignored {
				return nil
			}

			meta, ok := oldMetaData.get(path, sandboxPath)

			// Metadata doesn't exist for this file, so it must be added
			if !ok {
				status.fileAdded(path, sandboxPath)
				return nil
			}

			if !info.IsDir() {
				// The modified time is not a good enough check to see if the
				//  file is modified or not.
				// Different sizes mean that the file has changed for sure
				if meta.Size != info.Size() {
					status.fileModified(meta, path, sandboxPath)
				} else {
					// Check the hashes
					file, err := os.Open(path)
					if err != nil {
						return err
					}
					defer file.Close()

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

			return nil
		})

		if err != nil {
			return nil, err
		}
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

func IsIgnored(path string) (bool, error) {
	// Should we ignore changes to this file?
	// Check for signs that it isn't source code
	//  -Really big file (>10MB)
	//  -File extension (e.g. .exe, .dll, .so)
	//  -Temporary files left by editors (e.g. *~, *.ext.swp)

	base := filepath.Base(path)

	// Skip the metadata, staging and backup directories
	if base == metadataFileName || strings.Contains(path, stageFolder) || strings.Contains(path, backupFolder) {
		return true, nil
	}

	// Skip bin directories
	if strings.HasSuffix(path, "/bin") || strings.Contains(path, "/bin/") {
		return true, nil
	}

	if strings.HasSuffix(base, ".exe") || strings.HasSuffix(base, ".dll") || strings.HasSuffix(base, ".so") {
		return true, nil
	}

	if strings.HasSuffix(base, "~") || strings.HasSuffix(base, ".ext.swp") {
		return true, nil
	}

	s, err := os.Stat(path)
	if err != nil {
		return false, err
	}

	if s.Size() > 10*1024*1024 {
		return true, nil
	}

	return false, nil
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
