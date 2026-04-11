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

	"machinecfg/pkg/butane"
)

// fcosCmd represents the fcos command
var fcosCmd = &cobra.Command{
	Use:   "fcos",
	Short: "Read devices from Netbox and create Ignition files using Butane (fcos variant)",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		configureLogger(cmd)
		rootArguments := processRootArgs(cmd, true)

		ctx := context.Background()
		client := netbox.NewAPIClientFor(rootArguments.Endpoint, rootArguments.Token)

		fcoss, err := butane.CreateFCOSs(client, ctx, rootArguments.Filters)
		if err != nil {
			fatalExit("failed to create fcos configs", "func", "fcosCmd", "error", err.Error())
		}

		for _, f := range fcoss {
			writeFCOSIgnition(&f, rootArguments.OutputDirectory)
		}
	},
}

func writeFCOSIgnition(f *butane.FCOS, outputDirectory string) {
	if outputDirectory == "" {
		butane.PrintFCOSIgnitionFile(&f.Config, os.Stdout)
		return
	}
	fd, err := createFileDescriptor(outputDirectory, f.Hostname, "ign")
	if err != nil {
		slog.Error("failed to create output file", "func", "writeFCOSIgnition", "error", err.Error())
		return
	}
	defer func() { _ = fd.Close() }()
	butane.PrintFCOSIgnitionFile(&f.Config, fd)
}

func init() {
	butaneCmd.AddCommand(fcosCmd)
}
