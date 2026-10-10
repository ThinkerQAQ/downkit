package main

import (
	"fmt"
	"os"

	"github.com/ThinkerQAQ/downkit/devcontrol"
	"github.com/thinkerqaq/devtool/sdk/project"
)

func main() {
	provider := &devcontrol.Provider{}
	if err := project.Serve(provider); err != nil {
		fmt.Fprintln(os.Stderr, "project.downkit:", err)
		os.Exit(1)
	}
}
