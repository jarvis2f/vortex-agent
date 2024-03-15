package main

import (
	"github.com/jarvis2f/vortex-agent/cmd"
	_ "github.com/jarvis2f/vortex-agent/cmd/agent"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	cmd.Execute(version)
}
