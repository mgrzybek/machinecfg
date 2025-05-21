/*
Copyright © 2025 Mathieu GRZYBEK
*/
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// server6Cmd represents the dhcp6Server command
var server6Cmd = &cobra.Command{
	Use:   "server6",
	Short: "Creates a DHCPv6 server configuration",
	Long: `This command creates a DHCPv6 server configuration using:
* the MAC addresses from the database
* the desired ranges
`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("keaServer6 called")
	},
}

func init() {
	keaCmd.AddCommand(server6Cmd)
}
