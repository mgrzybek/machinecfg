/*
Copyright © 2025 Mathieu Grzybek <github@grzybek.fr>
SPDX-License-Identifier: GPL-3.0-or-later
*/
package cmd

import (
	"github.com/spf13/cobra"
)

// butaneCmd represents the butane command
var butaneCmd = &cobra.Command{
	Use:     "butane",
	Aliases: []string{"ignition"},
	Short:   "Manage butane / ignition configurations",
	Long:    ``,
}

func init() {
	rootCmd.AddCommand(butaneCmd)
}
