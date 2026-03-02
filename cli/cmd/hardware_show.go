/*
Copyright © 2025 Mathieu Grzybek <github@grzybek.fr>
SPDX-License-Identifier: GPL-3.0-or-later
*/
package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"machinecfg/pkg/tinkerbell"
)

// showCmd represents the "tinkerbell hardware show" command
var showCmd = &cobra.Command{
	Use:   "show",
	Short: "Show Hardware objects and their PXE/Workflow status",
	Long: `List Tinkerbell Hardware objects in a Kubernetes namespace and display
their PXE and Workflow boot settings.

If --hostname is not provided, all Hardware objects in the namespace are listed.`,
	Run: func(cmd *cobra.Command, args []string) {
		configureLogger(cmd)

		namespace, _ := cmd.Flags().GetString("namespace")
		hostname, _ := cmd.Flags().GetString("hostname")

		k8sClient, err := getK8sClient()
		if err != nil {
			slog.Error("no k8s configuration found", "func", "showCmd", "error", err.Error())
			os.Exit(1)
		}

		ctx := context.Background()

		hardwares, err := tinkerbell.GetHardwares(k8sClient, ctx, namespace, hostname)
		if err != nil {
			slog.Error("failed to get Hardware objects", "func", "showCmd", "error", err.Error())
			os.Exit(1)
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "HOSTNAME\tSTATUS\tALLOW-PXE\tWORKFLOW")

		for _, hw := range hardwares {
			status := hw.Labels["status"]
			for _, iface := range hw.Spec.Interfaces {
				allowPXE := iface.Netboot != nil && iface.Netboot.AllowPXE != nil && *iface.Netboot.AllowPXE
				workflow := iface.Netboot != nil && iface.Netboot.AllowWorkflow != nil && *iface.Netboot.AllowWorkflow
				fmt.Fprintf(w, "%s\t%s\t%t\t%t\n", hw.Name, status, allowPXE, workflow)
			}
		}

		w.Flush()
	},
}

func init() {
	hardwareCmd.AddCommand(showCmd)
	showCmd.Flags().String("namespace", "", "Kubernetes namespace containing the Hardware objects")
	showCmd.MarkFlagRequired("namespace")
	showCmd.Flags().String("hostname", "", "Name of the Hardware object to show (optional, all objects if omitted)")
}
