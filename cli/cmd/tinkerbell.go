/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"github.com/spf13/cobra"
)

// tinkerbellCmd represents the tinkerbell command
var tinkerbellCmd = &cobra.Command{
	Use:   "tinkerbell",
	Short: "Manage Tinkerbell objects",
	Long:  ``,
}

func init() {
	rootCmd.AddCommand(tinkerbellCmd)
}
