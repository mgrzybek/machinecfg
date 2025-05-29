/*
Copyright © 2025 Mathieu GRZYBEK
*/
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// dhcp4Cmd represents the dhcp4 command
var keaCmd = &cobra.Command{
	Use:   "kea",
	Short: "Creates a DHCPv4 and v6 configurations",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("kea called")
	},
}

func init() {
	rootCmd.AddCommand(keaCmd)
}
