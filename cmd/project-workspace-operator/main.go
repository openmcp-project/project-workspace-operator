package main

import (
	"fmt"
	"os"

	"github.com/openmcp-project/project-workspace-operator/cmd/project-workspace-operator/app"
)

func main() {
	cmd := app.NewPlatformServiceProjectWorkspaceCommand()

	if err := cmd.Execute(); err != nil {
		fmt.Print(err)
		os.Exit(1)
	}
}
