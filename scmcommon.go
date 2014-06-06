package main

import (
	"os"
	"path/filepath"
	"strings"
)

func findSandbox(startingPath string) (path string) {
	_, err := os.Stat(startingPath)
	if err != nil {
		return startingPath
	}

	path = startingPath
	path = filepath.Clean(path)

	for path != "." && !strings.HasSuffix(path, "/") {
		_, err = os.Stat(filepath.Join(path, metadataFileName))
		if err == nil {
			return path
		}

		path = filepath.Dir(path)
	}

	return startingPath
}
