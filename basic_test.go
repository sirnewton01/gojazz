package main

import (
	"flag"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBasicStreamLoad(t *testing.T) {
	sandbox1, err := ioutil.TempDir(os.TempDir(), "gojazz-test")
	if err != nil {
		panic(err)
	}

	defer os.RemoveAll(sandbox1)

	t.Logf("Loading test project into %v\n", sandbox1)
	os.Args = []string{"load", "cbmcgee | test22", "-sandbox=" + sandbox1}
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	loadOp()
}

func TestLoadAndClobberChanges(t *testing.T) {
	sandbox1, err := ioutil.TempDir(os.TempDir(), "gojazz-test")
	if err != nil {
		panic(err)
	}

	defer os.RemoveAll(sandbox1)

	t.Logf("Loading test project into %v\n", sandbox1)
	os.Args = []string{"load", "cbmcgee | test22", "-sandbox=" + sandbox1}
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	loadOp()

	// Make adds and mods to the files
	err = filepath.Walk(sandbox1, func(path string, fi os.FileInfo, err error) error {
		if strings.Contains(path, "deleteMe") || strings.Contains(path, "jazz") {
			return nil
		}

		if fi.IsDir() {
			deleteMe, err := os.Create(filepath.Join(path, "deleteMe.txt"))
			if err != nil {
				return err
			}
			defer deleteMe.Close()
			_, err = deleteMe.Write([]byte("test contents"))
			if err != nil {
				return err
			}

			err = os.Mkdir(filepath.Join(path, "deleteMe"), 0700)
			if err != nil {
				return err
			}
		} else {
			modFile, err := os.Open(path)
			if err != nil {
				return err
			}
			defer modFile.Close()

			modFile.Write([]byte("new contents"))
		}
		return nil
	})

	if err != nil {
		t.Fatalf("%v", err.Error())
	}

	os.Args = []string{"load", "-sandbox=" + sandbox1}
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	loadOp()

	status, err := scmStatus(sandbox1, NO_COPY)
	if err != nil {
		t.Fatalf("%v", err.Error())
	}
	if !status.unchanged() {
		t.Fail()
	}
}
