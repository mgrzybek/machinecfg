/*
Copyright © 2025 Mathieu Grzybek <github@grzybek.fr>
SPDX-License-Identifier: GPL-3.0-or-later
*/
package cmd

import (
	"github.com/spf13/cobra"
)

// talosCmd represents the talos command
var talosCmd = &cobra.Command{
	Use:   "talos",
	Short: "Manage Talos Linux",
	Long:  ``,
}

func init() {
	rootCmd.AddCommand(talosCmd)
}
