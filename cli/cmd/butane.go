/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>

*/
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// butaneCmd represents the butane command
var butaneCmd = &cobra.Command{
	Use:   "butane",
	Short: "Manage butane / ignition configurations",
	Long: ``,
}

func init() {
	rootCmd.AddCommand(butaneCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// butaneCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// butaneCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
