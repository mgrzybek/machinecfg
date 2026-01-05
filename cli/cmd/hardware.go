/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"log/slog"
	"os"

	"github.com/netbox-community/go-netbox/v4"
	"github.com/spf13/cobra"

	tinkerbellKubeObjects "github.com/tinkerbell/tink/api/v1alpha1"

	"machinecfg/pkg/tinkerbell"
)

type HardwareConfigurationArgs struct {
	Template *string
}

// hardwareCmd represents the hardware command
var hardwareCmd = &cobra.Command{
	Use:   "hardware",
	Short: "Read devices from Netbox and create Hardware objects",
	Long: `Read DCIM or Virtualisation sections to get the designated devices.

Only Primary and OOB addresses are used to provision machines.`,
	Run: func(cmd *cobra.Command, args []string) {
		configureLogger(cmd)
		rootArguments := processRootArgs(cmd)
		hardwareArguments := processHardwareArgs(cmd)

		k8sClient, err := getK8sClient()
		if err != nil {
			if !dirExists(rootArguments.OutputDirectory) {
				slog.Error("output-directory does not exist and no k8s configuration found")
				os.Exit(1)
			}
		}

		ctx := context.Background()
		client := netbox.NewAPIClientFor(rootArguments.Endpoint, rootArguments.Token)

		hardwares, _ := tinkerbell.CreateHardwares(client, ctx, rootArguments.Filters)

		for _, h := range hardwares {
			if k8sClient == nil {
				printYAMLFile(&h, rootArguments, hardwareArguments)
			} else {
				err = k8sClient.Create(ctx, &h)
				if err != nil {
					slog.Error(err.Error())
				}
			}
		}
	},
}

func init() {
	tinkerbellCmd.AddCommand(hardwareCmd)
	hardwareCmd.Flags().String("template", "", "The custom template to use to create Hardwares")
}

func processHardwareArgs(cmd *cobra.Command) *HardwareConfigurationArgs {
	var templatePtr *string

	template, _ := cmd.Flags().GetString("template")
	if len(template) > 0 {
		templatePtr = &template
		if !fileExists(template) {
			os.Exit(1)
		}
	}

	return &HardwareConfigurationArgs{
		Template: templatePtr,
	}
}

func printYAMLFile(h *tinkerbellKubeObjects.Hardware, rootArguments *ConfigurationArgs, hardwareArguments *HardwareConfigurationArgs) {
	outputFileDescriptor, err := createFileDescriptor(rootArguments.OutputDirectory, h.Spec.Metadata.Instance.Hostname, "yaml")
	if err != nil {
		slog.Error("hardwareCmd", "message", err.Error())
	} else {
		defer outputFileDescriptor.Close()
		if hardwareArguments.Template == nil {
			tinkerbell.PrintDefaultYAML(h, outputFileDescriptor)
		} else {
			tinkerbell.PrintExternalYAML(h, "hardware.yaml.tmpl", outputFileDescriptor)
		}
	}
}
