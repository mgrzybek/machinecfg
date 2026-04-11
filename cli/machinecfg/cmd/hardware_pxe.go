/*
Copyright © 2025 Mathieu Grzybek <github@grzybek.fr>
SPDX-License-Identifier: GPL-3.0-or-later
*/
package cmd

import (
	"context"

	"github.com/spf13/cobra"

	"machinecfg/pkg/tinkerbell"
)

// pxeAllowCmd represents the pxe-allow command
var pxeAllowCmd = &cobra.Command{
	Use:     "pxe-allow",
	Aliases: []string{"allow-pxe"},
	Short:   "Set AllowPXE=true on Hardware objects",
	Long: `Set AllowPXE=true on Tinkerbell Hardware objects in a given Kubernetes namespace.

If --hostname is not provided, all Hardware objects in the namespace are updated.
The command stops at the first error encountered.`,
	Run: func(cmd *cobra.Command, args []string) {
		configureLogger(cmd)

		namespace := getNamespace(cmd)
		hostname, _ := cmd.Flags().GetString("hostname")
		kubeconfig, _ := cmd.Flags().GetString("kubeconfig")

		k8sClient, err := getK8sClient(kubeconfig)
		if err != nil {
			fatalExit("no k8s configuration found", "func", "pxeAllowCmd", "error", err.Error())
		}

		ctx := context.Background()

		if err := tinkerbell.AllowPXE(k8sClient, ctx, namespace, hostname); err != nil {
			fatalExit("failed to set AllowPXE", "func", "pxeAllowCmd", "error", err.Error())
		}
	},
}

// pxeDenyCmd represents the pxe-deny command
var pxeDenyCmd = &cobra.Command{
	Use:     "pxe-deny",
	Aliases: []string{"deny-pxe"},
	Short:   "Set AllowPXE=false on Hardware objects",
	Long: `Set AllowPXE=false on Tinkerbell Hardware objects in a given Kubernetes namespace.

If --hostname is not provided, all Hardware objects in the namespace are updated.
The command stops at the first error encountered.`,
	Run: func(cmd *cobra.Command, args []string) {
		configureLogger(cmd)

		kubeconfig, _ := cmd.Flags().GetString("kubeconfig")
		k8sClient, err := getK8sClient(kubeconfig)
		if err != nil {
			fatalExit("no k8s configuration found", "func", "pxeDenyCmd", "error", err.Error())
		}

		_ = k8sClient
	},
}

func init() {
	hardwareCmd.AddCommand(pxeAllowCmd)
	pxeAllowCmd.Flags().String("hostname", "", "Name of the Hardware object to update (optional, all objects if omitted)")

	hardwareCmd.AddCommand(pxeDenyCmd)
	pxeDenyCmd.Flags().String("hostname", "", "The hostname to target")
}
