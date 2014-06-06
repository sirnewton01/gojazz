package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Printf("No subcommand provided. Available subcommands: 'load', 'status'\n")
		return
	}

	switch os.Args[1] {
	case "load":
		loadOp()
	case "status":
		statusOp()
	default:
		fmt.Printf("Invalid subcommand '%v'. Available subcommands: 'load', 'status'\n", os.Args[1])
	}
}