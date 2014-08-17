package main

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/howeyc/gopass"
)

type JazzError struct {
	Msg        string
	StatusCode int
	Details    string
	Log        bool
}

func (jError *JazzError) Error() string {
	return jError.Msg
}

func errorFromResponse(response *http.Response) *JazzError {
	b, _ := ioutil.ReadAll(response.Body)
	requestString := response.Request.Method + ": " + response.Request.URL.String() + "\n"
	return &JazzError{Msg: response.Status, StatusCode: response.StatusCode, Details: requestString + string(b), Log: response.StatusCode > 499}
}

func simpleWarning(msg string) *JazzError {
	return &JazzError{Msg: msg, Log: false}
}

func getCredentials() (string, string, error) {
	// Is there a credentials file in the user's home directory?
	usr, err := user.Current()
	if err != nil {
		return "", "", err
	}

	gojazzDir := filepath.Join(usr.HomeDir, ".gojazz")
	credentialFilePath := filepath.Join(gojazzDir, "credentials.txt")

	f, err := os.Open(credentialFilePath)
	if err != nil {
		// No credentials file, we need to prompt
		fmt.Printf("User ID: ")
		reader := bufio.NewReader(os.Stdin)
		userId, _ := reader.ReadString('\n')
		userId = strings.TrimSpace(userId)

		fmt.Printf("Password: ")
		password := string(gopass.GetPasswd())

		fmt.Printf("Would you like to save these credentials for next time?\n")
		fmt.Printf("They will be saved into %v and protected with OS permissions.\n", credentialFilePath)
		fmt.Printf("[Y/n]")
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(answer)

		if answer == "" || strings.ToLower(answer) == "y" {
			err = os.Mkdir(gojazzDir, 0700)
			if err != nil {
				return "", "", err
			}

			credentialFile, err := os.Create(credentialFilePath)
			if err != nil {
				return "", "", err
			}
			credentialFile.Close()

			err = os.Chmod(credentialFilePath, 0600)
			if err != nil {
				return "", "", err
			}

			// Open the file again with appropriate permissions
			credentialFile, err = os.OpenFile(credentialFilePath, os.O_WRONLY, 0600)
			if err != nil {
				return "", "", err
			}

			_, err = credentialFile.WriteString(userId + "\n")
			_, err = credentialFile.WriteString(password)
			if err != nil {
				return "", "", err
			}
			credentialFile.Close()
		}

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

func main() {
	if len(os.Args) < 2 {
		fmt.Printf("No subcommand provided. Available subcommands: 'load', 'status', 'sync'\n")
		return
	}

	// Error handling and log file dump routine
	defer func() {
		r := recover()

		if r == nil {
			// Normal exit
			return
		}

		jazzError, ok := r.(*JazzError)
		if ok {
			// First, check to see if it a well known status code
			if jazzError.StatusCode == 401 {
				fmt.Printf("Error: Unauthorized. Check your credentials and try again.\n")
				return
			}

			if jazzError.StatusCode == 403 {
				fmt.Printf("Error: Forbidden. You are not allowed access.\n")
				return
			}

			if jazzError.StatusCode == 404 {
				fmt.Printf("Error: Not Found. Check the name and spelling and try again.\n")
				return
			}

			if jazzError.Log {
				fmt.Printf("ERROR: %v\n", jazzError.Msg)
				logfile, err := ioutil.TempFile("", "gojazz-log")
				if err == nil {
					fmt.Printf("Writing details of this problem to %v\n", logfile.Name())
					logfile.Write([]byte(fmt.Sprintf("ERROR: %v\n", r)))
					logfile.Write([]byte(fmt.Sprintf("DETAILS: %v\n", jazzError.Details)))
					logfile.Write(debug.Stack())
				}
			} else {
				fmt.Printf("%v\n", jazzError.Msg)
			}
		} else {
			fmt.Printf("ERROR: %v\n", r)
			logfile, err := ioutil.TempFile("", "gojazz-log")
			if err == nil {
				fmt.Printf("Writing details of this problem to %v\n", logfile.Name())
				logfile.Write([]byte(fmt.Sprintf("ERROR: %v\n", r)))
				logfile.Write(debug.Stack())
			}
		}
	}()

	switch os.Args[1] {
	case "load":
		os.Args = os.Args[1:]
		loadOp()
	case "status":
		os.Args = os.Args[1:]
		statusOp()
	case "checkin":
		os.Args = os.Args[1:]
		checkinOp()
	case "sync":
		os.Args = os.Args[1:]
		syncOp()
	default:
		fmt.Printf("Invalid subcommand '%v'. Available subcommands: 'load', 'status', 'sync'\n", os.Args[1])
	}
}
