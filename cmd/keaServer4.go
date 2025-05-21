/*
Copyright © 2025 Mathieu GRZYBEK
*/
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// server4Cmd represents the server command
var server4Cmd = &cobra.Command{
	Use:   "server4",
	Short: "Creates a DHCPv4 server configuration",
	Long: `This command creates a DHCPv4 server configuration using:
* the MAC addresses from the database
* the desired ranges
`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("keaServer4 called")
	},
}

func init() {
	keaCmd.AddCommand(server4Cmd)
}
