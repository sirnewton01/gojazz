package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/howeyc/gopass"
)

func syncOp() {
	sandboxPath := flag.String("sandbox", "", "Location of the sandbox to sync the files")
	flag.Parse()

	if *sandboxPath == "" {
		path, err := os.Getwd()
		if err != nil {
			panic(err)
		}

		path = findSandbox(path)
		sandboxPath = &path
	}

	status, err := scmStatus(*sandboxPath, STAGE)
	if err != nil {
		fmt.Printf("%v\n", err.Error())
		return
	}

	if status.metaData.isstream {
		fmt.Printf("Sync is for repository workspaces, use load instead to update your loaded stream.\n")
		return
	}

	fmt.Printf("Password: ")
	password := string(gopass.GetPasswd())

	client, err := NewClient(status.metaData.userId, password)
	if err != nil {
		panic(err)
	}

	scmCheckin(client, status, *sandboxPath)
	scmLoad(client, status.metaData.ccmBaseUrl, status.metaData.projectName, status.metaData.workspaceId, status.metaData.isstream, status.metaData.userId, *sandboxPath, status)
}
