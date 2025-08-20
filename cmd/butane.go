/*
Copyright © 2025 Mathieu GRZYBEK
*/
package cmd

import (
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"machinecfg/internal/core"
	"machinecfg/internal/core/domain"
	"machinecfg/internal/input"
	"machinecfg/internal/output"
)

// butaneCmd represents the butane command
var butaneCmd = &cobra.Command{
	Use:   "butane",
	Short: "Creates a butane-based YAML document",
	Long: `The command generates a YAML document according to the Flatcar v1.1.0 specification.
https://github.com/coreos/butane/blob/main/docs/config-flatcar-v1_1.md

The available profiles are:

* install: the machine runs an in-memory version of the system. A installation
  script is run to persist the targetted deployment.

* live: the machine runs the final configuration.
`,
	Run: func(cmd *cobra.Command, args []string) {
		var machines []domain.MachineInfo

		configureLogger(cmd)

		cmdRootArgs := processRootArgs(cmd)
		cmdButaneARgs := processButaneArgs(cmd)

		cmdb, err := input.NewNetbox(cmdRootArgs)

		if err != nil {
			slog.Error(err.Error())
			os.Exit(1)
		}

		if cmdRootArgs.VirtualMachines {
			machines = cmdb.GetVirtualMachines()
		} else {
			machines = cmdb.GetMachines()
		}

		output, err := output.NewDirectory(cmdRootArgs.OutputDirectory)
		if err != nil {
			slog.Error(err.Error())
			os.Exit(1)
		}

		engine := core.NewEngine(cmdRootArgs, cmdButaneARgs.Template, output)
		engine.PrintYAMLTemplates(machines)

	},
}

func init() {
	butaneCmd.Flags().String("profile", "p", "Pre-configured profile to apply ’install’, ’live’")
	butaneCmd.Flags().String("template", "", "Go-Template file to use to generate YAML")
	rootCmd.AddCommand(butaneCmd)
}

func processButaneArgs(cmd *cobra.Command) *domain.ButaneArgs {
	profile, _ := cmd.Flags().GetString("profile")
	template, _ := cmd.Flags().GetString("template")

	stat, err := os.Stat(template)
	if os.IsNotExist(err) || stat.Size() == 0 {
		slog.Error("option template must exist")
		os.Exit(1)
	}

	return &domain.ButaneArgs{
		Profile:  profile,
		Template: template,
	}
}
