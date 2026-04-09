/*
Copyright © 2025 Mathieu Grzybek <github@grzybek.fr>
SPDX-License-Identifier: GPL-3.0-or-later
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

		configs, err := talos.CreateTalosConfigs(client, ctx, rootArguments.Filters)
		if err != nil {
			fatalExit("failed to create talos configs", "func", "machineconfigCmd", "error", err.Error())
		}

		for _, c := range configs {
			writeTalosConfig(&c, rootArguments.OutputDirectory)
		}
	},
}

func writeTalosConfig(c *talos.Talos, outputDirectory string) {
	if outputDirectory == "" {
		talos.PrintYAMLFile(c.Config, os.Stdout)
		return
	}
	fd, err := createFileDescriptor(outputDirectory, c.Hostname, ".patch.yaml")
	if err != nil {
		slog.Error("failed to create output file", "func", "writeTalosConfig", "error", err.Error())
		return
	}
	defer func() { _ = fd.Close() }()
	talos.PrintYAMLFile(c.Config, fd)
}

func init() {
	talosCmd.AddCommand(machineconfigCmd)
}
