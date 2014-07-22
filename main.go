package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"runtime/debug"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Printf("No subcommand provided. Available subcommands: 'load', 'status'\n")
		return
	}

	// Error handling and log file dump routine
	defer func() {
		r := recover()

		if r == nil {
			return
		}

		fmt.Printf("ERROR: %v\n", r)
		logfile, err := ioutil.TempFile("", "gojazz-log")
		if err == nil {
			fmt.Printf("Writing detailed log to %v\n", logfile.Name())
			logfile.Write(debug.Stack())
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
		fmt.Printf("Invalid subcommand '%v'. Available subcommands: 'load', 'status'\n", os.Args[1])
	}
}
