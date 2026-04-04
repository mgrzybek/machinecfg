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
	"strings"
	"text/tabwriter"

	"github.com/netbox-community/go-netbox/v4"
	"github.com/spf13/cobra"

	"machinecfg/pkg/cluster"
)

// clusterShowCmd represents the "cluster show" command
var clusterShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show Kubernetes clusters with their NetBox record and K8s readiness",
	Long: `List all NetBox virtualization clusters (filtered by --clusters when provided)
and display for each one:

  - the NetBox status
  - the CAPI cluster readiness (true / false / unknown / empty if not found)
  - the control-plane host from the CAPI Cluster spec
  - the list of DCIM devices assigned to the cluster in NetBox

Missing data on either side is displayed as an empty field so that gaps
between the two systems are immediately visible.`,
	Run: func(cmd *cobra.Command, args []string) {
		configureLogger(cmd)

		rootArguments := processRootArgs(cmd, false, false)

		namespace, _ := cmd.Flags().GetString("namespace")
		output, _ := cmd.Flags().GetString("output")
		kubeconfig, _ := cmd.Flags().GetString("kubeconfig")

		k8sClient, err := getK8sClient(kubeconfig)
		if err != nil {
			slog.Error("no k8s configuration found", "func", "clusterShowCmd", "error", err.Error())
			os.Exit(1)
		}

		ctx := context.Background()
		netboxClient := netbox.NewAPIClientFor(rootArguments.Endpoint, rootArguments.Token)

		rows, err := cluster.Show(
			k8sClient,
			ctx,
			namespace,
			netboxClient,
			rootArguments.Filters.Clusters,
		)
		if err != nil {
			slog.Error("failed to show clusters", "func", "clusterShowCmd", "error", err.Error())
			os.Exit(1)
		}

		if output == "json" {
			jsonData, err := json.Marshal(rows)
			if err != nil {
				slog.Error("failed to marshal json", "func", "clusterShowCmd", "error", err.Error())
				os.Exit(1)
			}
			fmt.Println(string(jsonData))
		} else {
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
			fmt.Fprintln(w, "NAME\tNETBOX-STATUS\tCAPI-READY\tCONTROL-PLANE\tDEVICE-COUNT\tDEVICES")
			for _, r := range rows {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%s\n",
					r.Name,
					r.NetBoxStatus,
					r.CAPIReady,
					r.ControlPlaneHost,
					r.DeviceCount,
					strings.Join(r.Devices, ", "),
				)
			}
			w.Flush()
		}
	},
}

func init() {
	clusterCmd.AddCommand(clusterShowCmd)
	clusterShowCmd.Flags().String("namespace", "", "Kubernetes namespace where CAPI Cluster objects are stored")
	clusterShowCmd.MarkFlagRequired("namespace")
	clusterShowCmd.Flags().String("output", "table", "Output format: table or json")
}
