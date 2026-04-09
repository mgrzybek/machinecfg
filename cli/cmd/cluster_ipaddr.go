/*
Copyright © 2025 Mathieu Grzybek <github@grzybek.fr>
SPDX-License-Identifier: GPL-3.0-or-later
*/
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/netbox-community/go-netbox/v4"
	"github.com/spf13/cobra"

	"machinecfg/pkg/cluster"
)

// ipaddrCmd is the parent command for cluster IP address subcommands.
var ipaddrCmd = &cobra.Command{
	Use:   "ipaddr",
	Short: "Inspect cluster IP addresses",
	Long:  `Commands for listing and reconciling IP addresses advertised by Cilium for Kubernetes clusters.`,
}

// ipaddrShowCmd represents the "cluster ipaddr show" command
var ipaddrShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show Cilium LB-IPAM addresses and their NetBox IPAM status",
	Long: `For each KamajiControlPlane object in the given namespace (optionally filtered
by --clusters), read the annotation io.cilium/lb-ipam-ips from
spec.network.serviceAnnotations and check whether each IP is known to NetBox IPAM.

Output columns:
  CLUSTER          name of the KamajiControlPlane / CAPI cluster
  IP-ADDRESS       IP address advertised via Cilium LB-IPAM
  NETBOX-ASSIGNED  whether the IP exists in NetBox IPAM
  NETBOX-STATUS    NetBox IPAM status (active, reserved, deprecated, …)`,
	Run: func(cmd *cobra.Command, args []string) {
		configureLogger(cmd)

		rootArguments := processRootArgs(cmd, false, false)

		namespace := getNamespace(cmd)
		output, _ := cmd.Flags().GetString("output")
		kubeconfig, _ := cmd.Flags().GetString("kubeconfig")

		k8sClient, err := getK8sClient(kubeconfig)
		if err != nil {
			fatalExit("no k8s configuration found", "func", "ipaddrShowCmd", "error", err.Error())
		}

		ctx := context.Background()
		netboxClient := netbox.NewAPIClientFor(rootArguments.Endpoint, rootArguments.Token)

		rows, err := cluster.ShowIPAddresses(
			k8sClient,
			ctx,
			namespace,
			netboxClient,
			rootArguments.Filters.Clusters,
		)
		if err != nil {
			fatalExit("failed to show IP addresses", "func", "ipaddrShowCmd", "error", err.Error())
		}

		if output == "json" {
			jsonData, err := json.Marshal(rows)
			if err != nil {
				fatalExit("failed to marshal json", "func", "ipaddrShowCmd", "error", err.Error())
			}
			fmt.Println(string(jsonData))
		} else {
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
			_, _ = fmt.Fprintln(w, "CLUSTER\tIP-ADDRESS\tNETBOX-ASSIGNED\tNETBOX-STATUS")
			for _, r := range rows {
				fmt.Fprintf(w, "%s\t%s\t%t\t%s\n",
					r.ClusterName, r.IPAddress, r.NetBoxAssigned, r.NetBoxStatus)
			}
			_ = w.Flush()
		}
	},
}

func init() {
	clusterCmd.AddCommand(ipaddrCmd)
	ipaddrCmd.AddCommand(ipaddrShowCmd)
	ipaddrShowCmd.Flags().String("output", "table", "Output format: table or json")
}
