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
	os.Args = []string{"load", "sirnewton | gojazz-test", "-sandbox=" + sandbox1}
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	loadOp()

	// Verify that certain specific files show up

	filesToCheck := []string{
		"README.md", "project.json", "bigFile.txt", ".jazzignore",
		".cfignore", "folder", "filename(with)[chars$]^that.must-be-escaped", "bin",
		"bin/mybinary.so", "filename(with)[chars$]^that.must-be-escaped/test.java",
		"folder/file.exe", "folder/file1.txt", "folder/file2.jsp", "folder/file3.jar",
		"folder/filename(with)[chars$]^that.must-be-escaped",
	}
	for _, file := range filesToCheck {
		p := filepath.Join(sandbox1, file)
		s, _ := os.Stat(p)
		if s == nil {
			t.Error("File not found in sandbox: %v", p)
		}
	}
}

func TestStreamLoadOnExistingFiles(t *testing.T) {
	sandbox1, err := ioutil.TempDir(os.TempDir(), "gojazz-test")
	if err != nil {
		panic(err)
	}

	deletemePath := filepath.Join(sandbox1, "deleteme.txt")
	deleteme, err := os.Create(deletemePath)
	if err != nil {
		panic(err)
	}
	defer deleteme.Close()
	_, err = deleteme.Write([]byte("delete this"))
	if err != nil {
		panic(err)
	}
	deleteme.Close()

	defer os.RemoveAll(sandbox1)

	t.Logf("Loading test project into %v\n", sandbox1)
	os.Args = []string{"load", "sirnewton | gojazz-test", "-sandbox=" + sandbox1, "-force=true"}
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	loadOp()

	s, _ := os.Stat(deletemePath)
	if s != nil {
		t.Fail()
	}

	// TODO maybe someday we should back up what was in the sandbox before the first load
	// Check if the deleteme is backed up in the backup folder
	//s, _ = os.Stat(filepath.Join(sandbox1, backupFolder, "deleteme.txt"))
	//if s == nil {
	//	t.Fail()
	//}
}

func TestLoadAndClobberChanges(t *testing.T) {
	sandbox1, err := ioutil.TempDir(os.TempDir(), "gojazz-test")
	if err != nil {
		panic(err)
	}

	defer os.RemoveAll(sandbox1)

	t.Logf("Loading test project into %v\n", sandbox1)
	os.Args = []string{"load", "sirnewton | gojazz-test", "-sandbox=" + sandbox1}
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	loadOp()

	numFileModifiedAdded := 0

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

			numFileModifiedAdded += 1

			err = os.Mkdir(filepath.Join(path, "deleteMe"), 0700)
			if err != nil {
				return err
			}

			numFileModifiedAdded += 1
		} else if !strings.Contains(path, "deleteMe") {
			modFile, err := os.Open(path)
			if err != nil {
				return err
			}
			defer modFile.Close()

			_, err = modFile.Write([]byte("new contents"))
			if err != nil {
				return err
			}

			numFileModifiedAdded += 1
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

	// Check that all of the files and folders made their way into the backup
	numFileBackedUp := 0
	err = filepath.Walk(filepath.Join(sandbox1, backupFolder), func(path string, fi os.FileInfo, err error) error {
		if filepath.Base(path) == backupFolder {
			return nil
		}

		numFileBackedUp += 1

		return nil
	})

	if numFileBackedUp != numFileModifiedAdded {
		t.Errorf("Expected %v files in the backup and there were %v\n", numFileModifiedAdded, numFileBackedUp)
	}
}
