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

	"machinecfg/pkg/tinkerbell"
)

// syncStatusCmd represents the "tinkerbell hardware sync-status" command
var syncStatusCmd = &cobra.Command{
	Use:   "sync-status",
	Short: "Transition NetBox devices from staged to active based on Hardware provisioned annotation",
	Long: `List Tinkerbell Hardware objects in a Kubernetes namespace and, for each one
whose annotation v1alpha1.tinkerbell.org/provisioned is "true", transition the
corresponding NetBox device from staged to active.

The Hardware object must carry a netbox-device-id label (set automatically by
machinecfg tinkerbell hardware sync) for the transition to succeed.`,
	Run: func(cmd *cobra.Command, args []string) {
		configureLogger(cmd)

		rootArguments := processRootArgs(cmd, false)

		namespace := getNamespace(cmd)
		output, _ := cmd.Flags().GetString("output")
		kubeconfig, _ := cmd.Flags().GetString("kubeconfig")

		k8sClient, err := getK8sClient(kubeconfig)
		if err != nil {
			fatalExit("no k8s configuration found", "func", "syncStatusCmd", "error", err.Error())
		}

		ctx := context.Background()
		netboxClient := netbox.NewAPIClientFor(rootArguments.Endpoint, rootArguments.Token)

		results, err := tinkerbell.SyncStatus(k8sClient, ctx, namespace, netboxClient)
		if err != nil {
			fatalExit("failed to sync status", "func", "syncStatusCmd", "error", err.Error())
		}

		if output == "json" {
			jsonData, err := json.Marshal(results)
			if err != nil {
				fatalExit("failed to marshal json", "func", "syncStatusCmd", "error", err.Error())
			}
			fmt.Println(string(jsonData))
		} else {
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
			_, _ = fmt.Fprintln(w, "HOSTNAME\tDEVICE-ID\tUPDATED\tERROR")
			for _, r := range results {
				_, _ = fmt.Fprintf(w, "%s\t%d\t%t\t%s\n", r.Hostname, r.DeviceID, r.Updated, r.Error)
			}
			_ = w.Flush()
		}
	},
}

func init() {
	hardwareCmd.AddCommand(syncStatusCmd)
	syncStatusCmd.Flags().String("output", "table", "Output format: table or json")
}
