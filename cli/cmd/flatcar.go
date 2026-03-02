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
			slog.Error("flatcarCmd", "message", err.Error())
			os.Exit(1)
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
		slog.Error("flatcarCmd", "message", err.Error())
		return
	}
	defer fd.Close()
	butane.PrintFlatcarIgnitionFile(&f.Config, fd)
}

func init() {
	butaneCmd.AddCommand(flatcarCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// flatcarCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// flatcarCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
