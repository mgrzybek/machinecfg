/*
Copyright © 2025 Mathieu Grzybek <github@grzybek.fr>
SPDX-License-Identifier: GPL-3.0-or-later
*/
package cmd

import (
	"github.com/spf13/cobra"
)

// clusterCmd is the parent command for all cluster-related subcommands.
var clusterCmd = &cobra.Command{
	Use:   "cluster",
	Short: "Manage Kubernetes clusters",
	Long:  `Commands for inspecting and reconciling Kubernetes cluster state against NetBox.`,
}

func init() {
	rootCmd.AddCommand(clusterCmd)
}
