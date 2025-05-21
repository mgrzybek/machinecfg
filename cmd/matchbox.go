/*
Copyright © 2025 Mathieu GRZYBEK
*/
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// matchboxCmd represents the matchbox command
var matchboxCmd = &cobra.Command{
	Use:   "matchbox",
	Short: "Interact with Matchbox service",
	Long: ``,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("matchbox called")
	},
}

func init() {
	rootCmd.AddCommand(matchboxCmd)
}
