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

		fcoss, _ := butane.CreateFCOSs(client, ctx, rootArguments.Filters)

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
		slog.Error("fcosCmd", "message", err.Error())
		return
	}
	defer fd.Close()
	butane.PrintFCOSIgnitionFile(&f.Config, fd)
}

func init() {
	butaneCmd.AddCommand(fcosCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// fcosCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// fcosCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
