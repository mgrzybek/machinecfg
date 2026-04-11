/*
Copyright © 2025 Mathieu Grzybek <github@grzybek.fr>
SPDX-License-Identifier: GPL-3.0-or-later
*/
package cmd

import (
	"github.com/spf13/cobra"
)

// tinkerbellCmd represents the tinkerbell command
var tinkerbellCmd = &cobra.Command{
	Use:   "tinkerbell",
	Short: "Manage Tinkerbell objects",
	Long:  ``,
}

func init() {
	rootCmd.AddCommand(tinkerbellCmd)
}
