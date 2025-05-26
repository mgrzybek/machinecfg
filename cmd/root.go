/*
Copyright © 2025 Mathieu GRZYBEK
*/
package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "machinecfg",
	Short: "Creates machines’ configurations to use with matchbox",
	Long:  ``,
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringP("log-level", "l", "", "Log level ’development’ (default) or ’production’")
	rootCmd.PersistentFlags().StringP("netbox-token", "t", "", "Token used to call Netbox API")
	rootCmd.PersistentFlags().StringP("netbox-endpoint", "e", "", "URL of the API")
	rootCmd.PersistentFlags().StringP("output", "o", "", "Where to write the result (default 'console')")
}
