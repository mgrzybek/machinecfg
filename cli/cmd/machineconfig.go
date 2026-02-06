/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"log/slog"
	"os"

	"github.com/netbox-community/go-netbox/v4"
	"github.com/spf13/cobra"

	"machinecfg/pkg/talos"
)

// machineconfigCmd represents the machineconfig command
var machineconfigCmd = &cobra.Command{
	Use:   "machineconfig",
	Short: "Reads devices from Netbox and creates MachineConfig file ",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		configureLogger(cmd)
		rootArguments := processRootArgs(cmd, true)

		ctx := context.Background()
		client := netbox.NewAPIClientFor(rootArguments.Endpoint, rootArguments.Token)

		configs, _ := talos.CreateTalosConfigs(client, ctx, rootArguments.Filters)

		var outputFileDescriptor *os.File
		var err error

		for _, c := range configs {
			if rootArguments.OutputDirectory == "" {
				outputFileDescriptor = os.Stdout
			} else {
				outputFileDescriptor, err = createFileDescriptor(rootArguments.OutputDirectory, c.Hostname, ".patch.yaml")
			}
			defer outputFileDescriptor.Close()
			if err != nil {
				slog.Error("machineconfigCmd", "message", err.Error())
			} else {
				talos.PrintYAMLFile(c.Config, outputFileDescriptor)
			}
		}
	},
}

func init() {
	talosCmd.AddCommand(machineconfigCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// machineconfigCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// machineconfigCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
