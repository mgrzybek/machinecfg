/*
Copyright © 2025 Mathieu GRZYBEK
*/
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// matchboxProfileCmd represents the matchboxProfile command
var matchboxProfileCmd = &cobra.Command{
	Use:   "profile",
	Short: "Creates a matchbox profile",
	Long: `This subcommand generates a profile according to the given arguments:

* If profile is set to ’install’, then a generic configuration is given.

* If profile is set to ’live’ or ’disk’, then a hostname must be provided too. This will
  generate a dedicated one.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("matchboxProfile called")
	},
}

func init() {
	matchboxProfileCmd.Flags().String("profile", "", "Pre-configured profile to apply (’disk’, ’install’ or ’live’)")
	matchboxCmd.AddCommand(matchboxProfileCmd)
}
