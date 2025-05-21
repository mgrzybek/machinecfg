/*
Copyright © 2025 Mathieu GRZYBEK
*/
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// netboxCmd represents the netbox command
var netboxCmd = &cobra.Command{
	Use:   "netbox",
	Short: "Interact with Netbox CMDB",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("netbox called")
	},
}

func init() {
	rootCmd.AddCommand(netboxCmd)
}
