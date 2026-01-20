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
	"k8s.io/apimachinery/pkg/api/errors"

	"machinecfg/pkg/tinkerbell"
)

type HardwareConfigurationArgs struct {
	Template                  *string
	EmbedIgnitionAsVendorData bool
	EmbeddedIgnitionVariant   *string
}

// hardwareCmd represents the hardware command
var hardwareCmd = &cobra.Command{
	Use:   "hardware",
	Short: "Read devices from Netbox and manage Hardware objects",
	Long: `Read DCIM or Virtualisation sections to get the designated devices.

Only Primary and OOB addresses are used to provision machines.

When device is in an active on staged status, it is created.
When device is in offline or planned status, it is deleted.`,
	Run: func(cmd *cobra.Command, args []string) {
		var successCounter int
		var failureCounter int

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

		hardwares, _ := tinkerbell.CreateHardwares(client, ctx, rootArguments.Filters, hardwareArguments.EmbeddedIgnitionVariant)
		for _, h := range hardwares {
			if k8sClient == nil {
				printYAMLFile(&h, rootArguments, hardwareArguments)
			} else {
				err = k8sClient.Create(ctx, &h)
				if err != nil {
					if !errors.IsAlreadyExists(err) {
						slog.Error("hardwareCmd", "message", err.Error(), "namespace", h.Namespace, "device", h.Name)
						failureCounter = failureCounter + 1
					} else {
						successCounter = successCounter + 1
					}
				} else {
					successCounter = successCounter + 1
				}

				slog.Info("hardwareCmd", "message", "creations", "success_nb", successCounter, "failure_nb", failureCounter)
			}
		}

		if k8sClient != nil {
			successCounter = 0
			failureCounter = 0

			hardwares, _ := tinkerbell.CreateHardwaresToPrune(client, ctx, rootArguments.Filters)
			for _, h := range hardwares {
				err = k8sClient.Delete(ctx, &h)
				if err != nil {
					slog.Error("hardwareCmd", "message", err.Error(), "namespace", h.Namespace, "device", h.Name)
					failureCounter = failureCounter + 1
				} else {
					successCounter = successCounter + 1
				}
			}

			slog.Info("hardwareCmd", "message", "deletions", "success_nb", successCounter, "failure_nb", failureCounter)
		}

	},
}

func init() {
	tinkerbellCmd.AddCommand(hardwareCmd)
	hardwareCmd.Flags().String("template", "", "The custom template to use to create Hardwares")
	hardwareCmd.Flags().Bool("embed-ignition-as-vendor-data", false, "Generates ignition data and write them in .specs.vendorData")
	hardwareCmd.Flags().String("embedded-ignition-variant", "flatcar", "Provides which ignition variant to produce")
}

func processHardwareArgs(cmd *cobra.Command) *HardwareConfigurationArgs {
	var templatePtr, embeddedIgnitionVariant *string

	template, _ := cmd.Flags().GetString("template")
	if len(template) > 0 {
		templatePtr = &template
		if !fileExists(template) {
			os.Exit(1)
		}
	}

	embedIgnition, _ := cmd.Flags().GetBool("embed-ignition-as-vendor-data")
	if embedIgnition {
		buffer, _ := cmd.Flags().GetString("embedded-ignition-variant")
		embeddedIgnitionVariant = &buffer
	}

	return &HardwareConfigurationArgs{
		Template:                  templatePtr,
		EmbedIgnitionAsVendorData: embedIgnition,
		EmbeddedIgnitionVariant:   embeddedIgnitionVariant,
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
