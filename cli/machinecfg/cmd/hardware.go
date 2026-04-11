/*
Copyright © 2025 Mathieu Grzybek <github@grzybek.fr>
SPDX-License-Identifier: GPL-3.0-or-later
*/
package cmd

import (
	"github.com/spf13/cobra"
)

// hardwareCmd represents the tinkerbell command
var hardwareCmd = &cobra.Command{
	Use:   "hardware",
	Short: "Manage Tinkerbell Hardware objects",
	Long:  ``,
}

func init() {
	tinkerbellCmd.AddCommand(hardwareCmd)
}
