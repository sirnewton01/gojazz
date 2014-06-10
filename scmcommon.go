package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
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

func fetchFSObject(client *Client, request *http.Request) *FSObject {
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
	fsObject := &FSObject{}
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
