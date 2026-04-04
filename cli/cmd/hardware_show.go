/*
Copyright © 2025 Mathieu Grzybek <github@grzybek.fr>
SPDX-License-Identifier: GPL-3.0-or-later
*/
package cmd

import (
	"context"
	"encoding/json"
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

		namespace := getNamespace(cmd)
		hostname, _ := cmd.Flags().GetString("hostname")
		output, _ := cmd.Flags().GetString("output")

		kubeconfig, _ := cmd.Flags().GetString("kubeconfig")
		k8sClient, err := getK8sClient(kubeconfig)
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

		if output == "json" {
			type hwRow struct {
				Hostname string `json:"hostname"`
				Status   string `json:"status"`
				AllowPXE bool   `json:"allow-pxe"`
				Workflow bool   `json:"workflow"`
				Cluster  string `json:"cluster,omitempty"`
			}
			var rows []hwRow
			for _, hw := range hardwares {
				status := hw.Labels["status"]
				cluster := tinkerbell.GetClusterName(k8sClient, ctx, &hw)
				for _, iface := range hw.Spec.Interfaces {
					allowPXE := iface.Netboot != nil && iface.Netboot.AllowPXE != nil && *iface.Netboot.AllowPXE
					workflow := iface.Netboot != nil && iface.Netboot.AllowWorkflow != nil && *iface.Netboot.AllowWorkflow
					rows = append(rows, hwRow{Hostname: hw.Name, Status: status, AllowPXE: allowPXE, Workflow: workflow, Cluster: cluster})
				}
			}
			jsonData, err := json.Marshal(rows)
			if err != nil {
				slog.Error("failed to marshal json", "func", "showCmd", "error", err.Error())
				os.Exit(1)
			}
			fmt.Println(string(jsonData))
		} else {
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
			fmt.Fprintln(w, "HOSTNAME\tSTATUS\tALLOW-PXE\tWORKFLOW\tCLUSTER")

			for _, hw := range hardwares {
				status := hw.Labels["status"]
				cluster := tinkerbell.GetClusterName(k8sClient, ctx, &hw)
				for _, iface := range hw.Spec.Interfaces {
					allowPXE := iface.Netboot != nil && iface.Netboot.AllowPXE != nil && *iface.Netboot.AllowPXE
					workflow := iface.Netboot != nil && iface.Netboot.AllowWorkflow != nil && *iface.Netboot.AllowWorkflow
					fmt.Fprintf(w, "%s\t%s\t%t\t%t\t%s\n", hw.Name, status, allowPXE, workflow, cluster)
				}
			}

			w.Flush()
		}
	},
}

func init() {
	hardwareCmd.AddCommand(showCmd)
	showCmd.Flags().String("hostname", "", "Name of the Hardware object to show (optional, all objects if omitted)")
	showCmd.Flags().String("output", "table", "Output format: table or json")
}
