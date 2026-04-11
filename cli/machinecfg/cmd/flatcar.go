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

// flatcarCmd represents the flatcar command
var flatcarCmd = &cobra.Command{
	Use:   "flatcar",
	Short: "Read devices from Netbox and create Ignition files using Butane (Flatcar variant)",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		configureLogger(cmd)
		rootArguments := processRootArgs(cmd, true)

		ctx := context.Background()
		client := netbox.NewAPIClientFor(rootArguments.Endpoint, rootArguments.Token)

		flatcars, err := butane.CreateFlatcars(client, ctx, rootArguments.Filters)
		if err != nil {
			fatalExit("failed to create flatcar configs", "func", "flatcarCmd", "error", err.Error())
		}

		for _, f := range flatcars {
			writeFlatcarIgnition(&f, rootArguments.OutputDirectory)
		}
	},
}

func writeFlatcarIgnition(f *butane.Flatcar, outputDirectory string) {
	if outputDirectory == "" {
		butane.PrintFlatcarIgnitionFile(&f.Config, os.Stdout)
		return
	}
	fd, err := createFileDescriptor(outputDirectory, f.Hostname, "ign")
	if err != nil {
		slog.Error("failed to create output file", "func", "writeFlatcarIgnition", "error", err.Error())
		return
	}
	defer func() { _ = fd.Close() }()
	butane.PrintFlatcarIgnitionFile(&f.Config, fd)
}

func init() {
	butaneCmd.AddCommand(flatcarCmd)
}
