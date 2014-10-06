package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	"runtime/debug"
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

func main() {
	if len(os.Args) < 2 {
		fmt.Printf("No subcommand provided. Available subcommands: 'load', 'status', 'sync', 'build' and 'login'\n")
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
				fmt.Printf("Error: Unauthorized. Use the login command to set your credentials.\n")
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
	case "login":
		os.Args = os.Args[1:]
		loginOp()
	case "build":
		os.Args = os.Args[1:]
		buildOp()
	case "autosync":
		os.Args = os.Args[1:]
		autosyncOp()
	default:
		fmt.Printf("Invalid subcommand '%v'. Available subcommands: 'load', 'status', 'sync', 'autosync', 'build' and 'login'\n", os.Args[1])
	}
}
