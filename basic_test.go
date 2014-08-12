package main

import (
	"flag"
	"io/ioutil"
	"os"
	"path/filepath"
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

	// Verify that specific files show up
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

	// Make adds and mods to the files
	filesToModify := []string{
		"README.md", "project.json", "bigFile.txt", ".jazzignore",
		".cfignore", "folder", "filename(with)[chars$]^that.must-be-escaped",
		"filename(with)[chars$]^that.must-be-escaped/test.java",
		"folder/file1.txt", "folder/file2.jsp", "folder/file3.jar",
		"folder/filename(with)[chars$]^that.must-be-escaped",
	}
	for _, file := range filesToModify {
		path := filepath.Join(sandbox1, file)
		s, _ := os.Stat(path)
		if s == nil {
			t.Error("File not found in sandbox: %v", path)
		}

		if s.IsDir() {
			deleteMe, err := os.Create(filepath.Join(path, "deleteMe.txt"))
			if err != nil {
				panic(err)
			}
			defer deleteMe.Close()
			_, err = deleteMe.Write([]byte("test contents"))
			if err != nil {
				panic(err)
			}

			err = os.Mkdir(filepath.Join(path, "deleteMe"), 0700)
			if err != nil {
				panic(err)
			}
		} else {
			modFile, err := os.OpenFile(path, os.O_WRONLY, 0)
			if err != nil {
				panic(err)
			}
			defer modFile.Close()

			_, err = modFile.Write([]byte("new contents123"))
			if err != nil {
				panic(err)
			}
		}
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
	for _, file := range filesToModify {
		path := filepath.Join(sandbox1, backupFolder, file)
		s, _ := os.Stat(path)
		if s == nil {
			t.Fatalf("File not found in backup: %v", path)
		}

		if s.IsDir() {
			s, _ := os.Stat(filepath.Join(path, "deleteMe"))
			if s == nil {
				t.Error("File not found in backup: %v", filepath.Join(path, "deleteMe"))
			}

			s, _ = os.Stat(filepath.Join(path, "deleteMe.txt"))
			if s == nil {
				t.Error("File not found in backup: %v", filepath.Join(path, "deleteMe.txt"))
			}
		}
	}
}

func TestAlternateStreamLoad(t *testing.T) {
	sandbox1, err := ioutil.TempDir(os.TempDir(), "gojazz-test")
	if err != nil {
		panic(err)
	}

	defer os.RemoveAll(sandbox1)

	t.Logf("Loading test project into %v\n", sandbox1)
	os.Args = []string{"load", "sirnewton | gojazz-test", "-stream=Alternate Stream", "-sandbox=" + sandbox1}
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	loadOp()

	// Verify that specific files show up
	filesToCheck := []string{
		"project.json", "bigFile.txt", ".jazzignore",
		".cfignore", "folder", "filename(with)[chars$]^that.must-be-escaped", "bin",
		"bin/mybinary.so", "filename(with)[chars$]^that.must-be-escaped/test.java",
		"folder/file.exe", "folder/file2.jsp", "folder/file3.jar",
		"folder/filename(with)[chars$]^that.must-be-escaped",
		"alternateFile.txt", "alternateFolder", "alternateFolder/anotherAlternateFolder",
		"alternateFolder/anotherAlternateFile.txt",
	}
	for _, file := range filesToCheck {
		p := filepath.Join(sandbox1, file)
		s, _ := os.Stat(p)
		if s == nil {
			t.Error("File not found in sandbox: %v", p)
		}
	}
}

func TestEmptyStreamLoad(t *testing.T) {
	sandbox1, err := ioutil.TempDir(os.TempDir(), "gojazz-test")
	if err != nil {
		panic(err)
	}

	defer os.RemoveAll(sandbox1)

	t.Logf("Loading test project into %v\n", sandbox1)
	os.Args = []string{"load", "sirnewton | gojazz-test", "-stream=Empty Stream", "-sandbox=" + sandbox1}
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	loadOp()

	// Verify that only the jazzMeta file is created in the sandbox
	f, err := os.Open(sandbox1)
	if err != nil {
		panic(err)
	}

	names, err := f.Readdirnames(-1)
	if err != nil {
		panic(err)
	}

	if len(names) != 1 {
		t.Fatalf("Expected 1 file but found %v files", len(names))
	}

	if names[0] != metadataFileName {
		t.Fatalf("Expected only the metadata file but found %v", names[0])
	}
}

func TestSwitchStreams(t *testing.T) {
	sandbox1, err := ioutil.TempDir(os.TempDir(), "gojazz-test")
	if err != nil {
		panic(err)
	}

	defer os.RemoveAll(sandbox1)

	t.Logf("Loading test project into %v\n", sandbox1)
	os.Args = []string{"load", "sirnewton | gojazz-test", "-sandbox=" + sandbox1}
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	loadOp()

	t.Logf("Loading test project into %v\n", sandbox1)
	os.Args = []string{"load", "sirnewton | gojazz-test", "-stream=Alternate Stream", "-sandbox=" + sandbox1}
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	loadOp()

	// Verify that specific files show up
	filesToCheck := []string{
		"project.json", "bigFile.txt", ".jazzignore",
		".cfignore", "folder", "filename(with)[chars$]^that.must-be-escaped", "bin",
		"bin/mybinary.so", "filename(with)[chars$]^that.must-be-escaped/test.java",
		"folder/file.exe", "folder/file2.jsp", "folder/file3.jar",
		"folder/filename(with)[chars$]^that.must-be-escaped",
		"alternateFile.txt", "alternateFolder", "alternateFolder/anotherAlternateFolder",
		"alternateFolder/anotherAlternateFile.txt",
	}
	for _, file := range filesToCheck {
		p := filepath.Join(sandbox1, file)
		s, _ := os.Stat(p)
		if s == nil {
			t.Error("File not found in sandbox: %v", p)
		}
	}

	// Verify that the file from the other stream no longer exists
	filesToCheck = []string{
		"folder/file1.txt", "README.md",
	}
	for _, file := range filesToCheck {
		p := filepath.Join(sandbox1, file)
		s, _ := os.Stat(p)
		if s != nil {
			t.Error("File from the wrong stream found in sandbox: %v", p)
		}
	}

	// Verify that there are no backups since nothing was modified
	s, _ := os.Stat(filepath.Join(sandbox1, backupFolder))
	if s != nil {
		t.Fatalf("Found a backup folder even though no changes were made.")
	}
}
