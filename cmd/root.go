package cmd

import (
	"fmt"
	"github.com/jarvis2f/vortex-agent/agent"
	"os"

	"github.com/spf13/cobra"
)

var RootCmd = &cobra.Command{
	Use:  "vortex",
	Long: `vortex is a forward proxy tool`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		agent.Version = cmd.Root().Version
		agent.Config = cmd.Flag("config").Value.String()
		agent.Dir = cmd.Flag("dir").Value.String()
		logLevel := cmd.Flag("log-level").Value.String()
		logDest := cmd.Flag("log-dest").Value.String()

		if err := agent.ValidateLogLevel(logLevel); err != nil {
			return err
		}

		if err := agent.ValidateLogDest(logDest); err != nil {
			return err
		}
		if logLevel != "" && logDest != "" {
			agent.InitLog(logLevel, logDest)
		}
		return nil
	},
	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		agent.SyncLog()
	},
}

func Execute(version string) {
	RootCmd.Version = version
	err := RootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	RootCmd.PersistentFlags().StringP("config", "C", "", "config file")
	RootCmd.PersistentFlags().String("log-level", "info", fmt.Sprintf("log level. Available levels: %v", agent.LogLevels))
	RootCmd.PersistentFlags().String("log-dest", "console", fmt.Sprintf("log destination. Available destinations: %v", agent.LogDestinations))
	RootCmd.PersistentFlags().String("dir", "", "agent dir, script will be saved in this dir")
}
