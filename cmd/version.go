package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("ibmdocs %s (commit: %s, built: %s)\n", version, gitCommit, buildDate)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
