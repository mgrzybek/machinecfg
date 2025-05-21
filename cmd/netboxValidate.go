/*
Copyright © 2025 Mathieu GRZYBEK
*/
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// netboxValidateCmd represents the netboxValidate command
var netboxValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Check prerequisites",
	Long: `Some Netbox resources and objects must be present for machinecfg to
work well. These include:
- IP addresses on business interfaces
- Special tagged IP addresses
- Other attributes…`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("netboxValidate called")
	},
}

func init() {
	netboxCmd.AddCommand(netboxValidateCmd)
}
