package main

import (
	"bufio"
	"fmt"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/howeyc/gopass"
)

const (
	gojazzDataDir   = ".gojazz"
	credentialsFile = "credentials.txt"
)

func loginOp() {
	usr, err := user.Current()
	if err != nil {
		panic(err)
	}

	credentialsFilePath := filepath.Join(usr.HomeDir, gojazzDataDir, credentialsFile)

	// Remove existing credentials first
	err = os.RemoveAll(credentialsFilePath)
	if err != nil {
		panic(err)
	}

	userId, password, err := getCredentials()
	if err != nil {
		panic(err)
	}

	// Test the credentials by retrieving a page
	client, err := NewClient(userId, password)
	if err != nil {
		panic(err)
	}

	request, err := http.NewRequest("GET", "https://hub.jazz.net/invitations", nil)
	if err != nil {
		panic(err)
	}

	_, err = client.Do(request)
	if err != nil {
		jazzErr, ok := err.(*JazzError)
		if ok && jazzErr.StatusCode == 401 {
			fmt.Printf("Not logged in, check your credentials and try again.\n")
			return
		}
		panic(err)
	}

	fmt.Printf("Logged in\n")
	err = storeCredentials(userId, password)
	if err != nil {
		panic(err)
	}
}

func isLoggedIn() bool {
	usr, err := user.Current()
	if err != nil {
		return false
	}

	gojazzDir := filepath.Join(usr.HomeDir, gojazzDataDir)
	credentialFilePath := filepath.Join(gojazzDir, credentialsFile)

	s, _ := os.Stat(credentialFilePath)

	return s != nil
}

func getCredentials() (string, string, error) {
	// Is there a credentials file in the user's home directory?
	usr, err := user.Current()
	if err != nil {
		return "", "", err
	}

	gojazzDir := filepath.Join(usr.HomeDir, gojazzDataDir)
	credentialFilePath := filepath.Join(gojazzDir, credentialsFile)

	f, err := os.Open(credentialFilePath)
	if err != nil {
		fmt.Printf("We need your credentials for this operation.")
		fmt.Printf("You can avoid this prompt next time by using the 'gojazz login' command.\n")
		fmt.Println()

		// No credentials file, we need to prompt
		fmt.Printf("User ID: ")
		reader := bufio.NewReader(os.Stdin)
		userId, _ := reader.ReadString('\n')
		userId = strings.TrimSpace(userId)

		fmt.Printf("Password: ")
		password := string(gopass.GetPasswd())

		return userId, password, nil
	}

	reader := bufio.NewReader(f)
	userId, err := reader.ReadString('\n')
	if err != nil {
		return "", "", err
	}
	userId = strings.TrimSpace(userId)

	password, _ := reader.ReadString('\n')
	password = strings.TrimSpace(password)

	return userId, password, nil
}

func storeCredentials(userId string, password string) error {
	// Is there a credentials file in the user's home directory?
	usr, err := user.Current()
	if err != nil {
		return err
	}

	gojazzDir := filepath.Join(usr.HomeDir, gojazzDataDir)
	credentialFilePath := filepath.Join(gojazzDir, credentialsFile)

	s, _ := os.Stat(gojazzDir)
	if s == nil {
		err = os.Mkdir(gojazzDir, 0700)
		if err != nil {
			return err
		}
	}

	credentialFile, err := os.Create(credentialFilePath)
	if err != nil {
		return err
	}
	credentialFile.Close()

	err = os.Chmod(credentialFilePath, 0600)
	if err != nil {
		return err
	}

	// Open the file again with appropriate permissions
	credentialFile, err = os.OpenFile(credentialFilePath, os.O_WRONLY, 0600)
	if err != nil {
		return err
	}

	_, err = credentialFile.WriteString(userId + "\n")
	_, err = credentialFile.WriteString(password)
	if err != nil {
		return err
	}
	credentialFile.Close()

	fmt.Printf("You credentials have been stored in %v and protected using operating system permissions.\n", credentialFilePath)

	return nil
}
