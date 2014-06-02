package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Printf("No subcommand provided. Available subcommands are 'load'\n")
		return
	}

	switch os.Args[1] {
	case "load":
		loadOp()
	default:
		fmt.Printf("Invalid subcommand '%v'. Options are 'load'\n", os.Args[1])
	}
}
