/*
Copyright © 2025 Mathieu GRZYBEK
*/
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// matchboxGroupCmd represents the matchboxGroup command
var matchboxGroupCmd = &cobra.Command{
	Use:   "group",
	Short: "Creates a matchbox group",
	Long: `This subcommand creates a json object according to the given input.

* If no args are given, a generic installation group pointing to the
  flatcar-install profile is given.

* If a profile ID and a serial number are given, the profile and selector
  attributes are set. The os-install boolean is then used to add the tuple
  {"os": "installed"} into the selector.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("matchboxGroup called")
	},
}

func init() {
	matchboxGroupCmd.Flags().Bool("os-install", false, "Is it an on-disk installation process running? (default: ’false’)")
	matchboxGroupCmd.Flags().String("profile-id", "", "Pre-configured profile to apply")
	matchboxGroupCmd.Flags().String("serial", "", "Serial number of the machine")
	matchboxCmd.AddCommand(matchboxGroupCmd)
}
