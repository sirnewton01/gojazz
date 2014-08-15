package main

import (
	"flag"
	"fmt"
	"net/url"
	"os"

	"github.com/howeyc/gopass"
)

func syncDefaults() {
	fmt.Printf("gojazz sync [options]\n")
	flag.PrintDefaults()
}

func syncOp() {
	sandboxPath := flag.String("sandbox", "", "Location of the sandbox to sync the files")
	force := flag.Bool("force", false, "Don't prompt for anything. Clobber files when necessary.")
	flag.Usage = syncDefaults
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
		panic(err)
	}

	if status.metaData.isstream {
		panic(simpleWarning("Sync is for repository workspaces, use load instead to incrementally update your loaded stream."))
	}

	if password == "" {
		fmt.Printf("Password: ")
		password = string(gopass.GetPasswd())
	}

	client, err := NewClient(status.metaData.userId, password)
	if err != nil {
		panic(err)
	}

	scmCheckin(client, status, *sandboxPath)

	// Clear out all of the changes in the status before performing the load
	status.Added = make(map[string]bool)
	status.Modified = make(map[string]bool)
	status.Deleted = make(map[string]bool)

	scmLoad(client, status.metaData.ccmBaseUrl, status.metaData.projectName, status.metaData.workspaceId, status.metaData.isstream, status.metaData.userId, *sandboxPath, status, *force)

	// Force a load/reload of the jazzhub sandbox to avoid out of sync when
	//  looking at the changes page
	err = loadWorkspace(client, status.metaData.projectName, status.metaData.workspaceId)
	if err != nil {
		panic(err)
	}
	fmt.Println("Visit the following URL to work with your changes, deliver them to the rest of the team and more:")
	redirect := fmt.Sprintf(jazzHubBaseUrl + "/code/jazzui/changes.html#" + "/code/jazz/Changes/_/file/" + client.GetJazzId() + "-OrionContent/" + status.metaData.projectName)
	fmt.Printf("https://login.jazz.net/psso/proxy/jazzlogin?redirect_uri=%v\n", url.QueryEscape(redirect))
}
