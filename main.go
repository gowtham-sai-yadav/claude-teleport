package main

import (
	"fmt"
	"os"

	"github.com/gowtham-sai-yadav/claude-teleport/internal/cli"
)

func main() {
	if err := cli.Run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
