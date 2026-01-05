/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"github.com/spf13/cobra"
)

// talosCmd represents the talos command
var talosCmd = &cobra.Command{
	Use:   "talos",
	Short: "Manage Talos Linux",
	Long:  ``,
}

func init() {
	rootCmd.AddCommand(talosCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// talosCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// talosCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
