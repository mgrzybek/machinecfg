/*
Copyright © 2025 Mathieu GRZYBEK
*/
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// butaneCmd represents the butane command
var butaneCmd = &cobra.Command{
	Use:   "butane",
	Short: "Creates a butane-based YAML document",
	Long: `The command generates a YAML document according to the Flatcar v1.1.0 specification.
https://github.com/coreos/butane/blob/main/docs/config-flatcar-v1_1.md

The available profiles are:

* install: the machine runs an in-memory version of the system. A installation
  script is run to persist the targetted deployment.

* live: the machine runs the final configuration.
`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("butane called")
	},
}

func init() {
	butaneCmd.Flags().String("profile", "p", "Pre-configured profile to apply ’install’, ’live’")
	rootCmd.AddCommand(butaneCmd)
}
