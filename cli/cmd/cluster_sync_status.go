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

// clusterSyncStatusCmd represents the "cluster sync-status" command
var clusterSyncStatusCmd = &cobra.Command{
	Use:   "sync-status",
	Short: "Reconcile Kubernetes cluster state back into NetBox",
	Long: `For each NetBox virtualization cluster (optionally filtered by --clusters),
look up the corresponding CAPI Cluster object in Kubernetes and ensure that
the following NetBox records exist:

  - an FHRP group named after the cluster (protocol: other)
  - a ServiceTemplate "Kubernetes endpoint" (TCP, port from controlPlaneEndpoint)
  - a Service attached to that FHRP group

Existing records are left untouched (idempotent).`,
	Run: func(cmd *cobra.Command, args []string) {
		configureLogger(cmd)

		rootArguments := processRootArgs(cmd, false, false)

		namespace := getNamespace(cmd)
		output, _ := cmd.Flags().GetString("output")
		kubeconfig, _ := cmd.Flags().GetString("kubeconfig")

		k8sClient, err := getK8sClient(kubeconfig)
		if err != nil {
			fatalExit("no k8s configuration found", "func", "clusterSyncStatusCmd", "error", err.Error())
		}

		ctx := context.Background()
		netboxClient := netbox.NewAPIClientFor(rootArguments.Endpoint, rootArguments.Token)

		results, err := cluster.SyncStatus(
			k8sClient,
			ctx,
			namespace,
			netboxClient,
			rootArguments.Filters.Clusters,
		)
		if err != nil {
			fatalExit("failed to sync cluster status", "func", "clusterSyncStatusCmd", "error", err.Error())
		}

		if output == "json" {
			jsonData, err := json.Marshal(results)
			if err != nil {
				fatalExit("failed to marshal json", "func", "clusterSyncStatusCmd", "error", err.Error())
			}
			fmt.Println(string(jsonData))
		} else {
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
			_, _ = fmt.Fprintln(w, "CLUSTER\tFHRP-GROUP-ID\tIP-ADDRESS-ID\tSERVICE-ID\tUPDATED\tERROR")
			for _, r := range results {
				fmt.Fprintf(w, "%s\t%d\t%d\t%d\t%t\t%s\n",
					r.ClusterName, r.FHRPGroupID, r.IPAddressID, r.ServiceID, r.Updated, r.Error)
			}
			_ = w.Flush()
		}
	},
}

func init() {
	clusterCmd.AddCommand(clusterSyncStatusCmd)
	clusterSyncStatusCmd.Flags().String("output", "table", "Output format: table or json")
}
