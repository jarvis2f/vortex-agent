package agent

import (
	"github.com/jarvis2f/vortex-agent/cmd"
	"github.com/spf13/cobra"
)

var agentCmd = &cobra.Command{
	Use:  "agent",
	Long: `manage agent`,
}

func init() {
	cmd.RootCmd.AddCommand(agentCmd)
}
