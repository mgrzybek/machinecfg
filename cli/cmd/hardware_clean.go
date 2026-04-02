/*
Copyright © 2025 Mathieu Grzybek <github@grzybek.fr>
SPDX-License-Identifier: GPL-3.0-or-later
*/
package cmd

import (
	"context"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"machinecfg/pkg/tinkerbell"
)

// cleanUserdataCmd represents the "tinkerbell hardware clean-userdata" command
var cleanUserdataCmd = &cobra.Command{
	Use:   "clean-userdata",
	Short: "Wipe the userData field of Hardware objects",
	Long: `Set the userData field to nil on Tinkerbell Hardware objects in a given Kubernetes namespace.

This is useful after a reprovisioning cycle to force fresh user-data on next boot.

If --hostname is not provided, all Hardware objects in the namespace are updated.`,
	Run: func(cmd *cobra.Command, args []string) {
		configureLogger(cmd)

		namespace, _ := cmd.Flags().GetString("namespace")
		hostname, _ := cmd.Flags().GetString("hostname")
		kubeconfig, _ := cmd.Flags().GetString("kubeconfig")

		k8sClient, err := getK8sClient(kubeconfig)
		if err != nil {
			slog.Error("no k8s configuration found", "func", "cleanUserdataCmd", "error", err.Error())
			os.Exit(1)
		}

		ctx := context.Background()

		if err := tinkerbell.CleanUserData(k8sClient, ctx, namespace, hostname); err != nil {
			slog.Error("failed to clean userData", "func", "cleanUserdataCmd", "error", err.Error())
			os.Exit(1)
		}
	},
}

// cleanVendordataCmd represents the "tinkerbell hardware clean-vendordata" command
var cleanVendordataCmd = &cobra.Command{
	Use:   "clean-vendordata",
	Short: "Wipe the vendorData field of Hardware objects",
	Long: `Set the vendorData field to nil on Tinkerbell Hardware objects in a given Kubernetes namespace.

This forces a fresh embedded Ignition config on next provisioning.

If --hostname is not provided, all Hardware objects in the namespace are updated.`,
	Run: func(cmd *cobra.Command, args []string) {
		configureLogger(cmd)

		namespace, _ := cmd.Flags().GetString("namespace")
		hostname, _ := cmd.Flags().GetString("hostname")
		kubeconfig, _ := cmd.Flags().GetString("kubeconfig")

		k8sClient, err := getK8sClient(kubeconfig)
		if err != nil {
			slog.Error("no k8s configuration found", "func", "cleanVendordataCmd", "error", err.Error())
			os.Exit(1)
		}

		ctx := context.Background()

		if err := tinkerbell.CleanVendorData(k8sClient, ctx, namespace, hostname); err != nil {
			slog.Error("failed to clean vendorData", "func", "cleanVendordataCmd", "error", err.Error())
			os.Exit(1)
		}
	},
}

func init() {
	hardwareCmd.AddCommand(cleanUserdataCmd)
	cleanUserdataCmd.Flags().String("namespace", "", "Kubernetes namespace containing the Hardware objects")
	cleanUserdataCmd.MarkFlagRequired("namespace")
	cleanUserdataCmd.Flags().String("hostname", "", "Name of the Hardware object to clean (optional, all objects if omitted)")

	hardwareCmd.AddCommand(cleanVendordataCmd)
	cleanVendordataCmd.Flags().String("namespace", "", "Kubernetes namespace containing the Hardware objects")
	cleanVendordataCmd.MarkFlagRequired("namespace")
	cleanVendordataCmd.Flags().String("hostname", "", "Name of the Hardware object to clean (optional, all objects if omitted)")
}
