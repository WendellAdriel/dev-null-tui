package main

import (
	"fmt"
	"os"

	"github.com/WendellAdriel/dev-null-cli/internal/cli"
)

func main() {
	if err := cli.Run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "devnull:", err)
		os.Exit(1)
	}
}
