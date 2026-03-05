/*
Copyright © 2025 Mathieu Grzybek <github@grzybek.fr>
SPDX-License-Identifier: GPL-3.0-or-later
*/
package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	tinkerbellKubeObjects "github.com/tinkerbell/tink/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"machinecfg/pkg/common"
)

type ConfigurationArgs struct {
	Endpoint string
	Token    string

	OutputDirectory string

	Filters common.DeviceFilters
}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "machinecfg",
	Short: "",
	Long:  ``,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	// Generic options
	rootCmd.PersistentFlags().StringP("log-level", "", "", "Log level ‘production’ (default) or ‘development’")
	rootCmd.PersistentFlags().StringP("netbox-token", "", "", "Token used to call Netbox API")
	rootCmd.PersistentFlags().StringP("netbox-endpoint", "", "", "URL of the API")
	rootCmd.PersistentFlags().StringP("output-directory", "", "", "Where to write the result")

	// Location filters
	rootCmd.PersistentFlags().StringP("regions", "", "", "Regions to extract data from")
	rootCmd.PersistentFlags().StringP("sites", "", "", "Sites to extract data from")
	rootCmd.PersistentFlags().StringP("locations", "", "", "Locations to extract data from")
	rootCmd.PersistentFlags().StringP("racks", "", "", "Apply a filter on the given racks")
	rootCmd.PersistentFlags().StringP("clusters", "", "", "Apply a filter on the given clusters")

	// Usage filters
	rootCmd.PersistentFlags().StringP("tenants", "", "", "Tenants to extract data from")
	rootCmd.PersistentFlags().StringP("roles", "", "", "Apply a filter on the given roles")

	// Physical or virtual devices?
	rootCmd.PersistentFlags().BoolP("virtualization", "", false, "Use the virtual machines' inventory instead of the physical devices")
}

func configureLogger(cmd *cobra.Command) {
	logLevel, _ := cmd.Flags().GetString("log-level")

	switch strings.ToLower(logLevel) {
	case "debug", "development":
		slog.SetLogLoggerLevel(slog.LevelDebug)
	default:
		handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
		logger := slog.New(handler)
		slog.SetDefault(logger)
	}
}

func processRootArgs(cmd *cobra.Command, requireOutputDirectory bool) *ConfigurationArgs {
	fatalError := false
	endpoint, _ := cmd.Flags().GetString("netbox-endpoint")
	token, _ := cmd.Flags().GetString("netbox-token")
	outputDirectory, _ := cmd.Flags().GetString("output-directory")

	regions, _ := cmd.Flags().GetString("regions")
	sites, _ := cmd.Flags().GetString("sites")
	locations, _ := cmd.Flags().GetString("locations")
	racksStr, _ := cmd.Flags().GetString("racks")
	tenants, _ := cmd.Flags().GetString("tenants")
	roles, _ := cmd.Flags().GetString("roles")
	virtualization, _ := cmd.Flags().GetBool("virtualization")
	clusters, _ := cmd.Flags().GetString("clusters")

	if endpoint == "" {
		slog.Error("endpoint option is required", "func", "processRootArgs")
		fatalError = true
	}

	if len(token) != 40 {
		slog.Error("token option is invalid", "func", "processRootArgs")
		fatalError = true
	}

	if requireOutputDirectory {
		if len(outputDirectory) == 0 {
			slog.Error("output-directory is required", "func", "processRootArgs")
			fatalError = true
		}
	}

	if sites == "" {
		slog.Error("sites option is required", "func", "processRootArgs")
		fatalError = true
	}

	if roles == "" {
		slog.Error("roles option is required", "func", "processRootArgs")
		fatalError = true
	}

	var rackIDs []int32
	for _, s := range strings.Split(racksStr, ",") {
		if s != "" {
			id, parseErr := strconv.ParseInt(s, 10, 32)
			if parseErr != nil {
				slog.Error("invalid rack id", "func", "processRootArgs", "value", s, "error", parseErr.Error())
				fatalError = true
			} else {
				rackIDs = append(rackIDs, int32(id))
			}
		}
	}

	if fatalError {
		os.Exit(1)
	}

	return &ConfigurationArgs{
		Endpoint:        endpoint,
		Token:           token,
		OutputDirectory: outputDirectory,

		Filters: common.DeviceFilters{
			Regions:        strings.Split(regions, ","),
			Sites:          strings.Split(sites, ","),
			Locations:      strings.Split(locations, ","),
			Racks:          rackIDs,
			Tenants:        strings.Split(tenants, ","),
			Roles:          strings.Split(roles, ","),
			Virtualisation: virtualization,
			Clusters:       strings.Split(clusters, ","),
		},
	}
}

func dirExists(path string) bool {
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	slog.Error("failed to stat path", "func", "dirExists", "error", err.Error())
	return false
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	if err == nil {
		return true
	} else {
		slog.Error("failed to stat path", "func", "fileExists", "error", err.Error())
	}
	if os.IsNotExist(err) {
		return false
	}
	return false
}

func createFileDescriptor(dirPath, filename, extension string) (*os.File, error) {
	outputPath := fmt.Sprintf("%s/%s.%s", dirPath, filename, extension)
	return os.Create(outputPath)
}

func getK8sClient() (client.Client, error) {
	var config *rest.Config
	var scheme *runtime.Scheme
	var err error

	config, err = rest.InClusterConfig()
	if err != nil {
		slog.Debug("in-cluster config failed, trying out-of-cluster", "func", "getK8sClient")

		kubeconfig := getKubeconfigPath()

		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("cannot find any kubeconfig: %w", err)
		} else {
			slog.Debug("out-of-cluster configuration found", "func", "getK8sClient", "kubeconfig", kubeconfig)
		}
	} else {
		slog.Debug("in-cluster configuration found", "func", "getK8sClient")
	}

	scheme = runtime.NewScheme()

	err = tinkerbellKubeObjects.AddToScheme(scheme)
	if err != nil {
		slog.Error("failed to add tinkerbell scheme", "func", "getK8sClient", "error", err.Error())
		return nil, err
	}

	return client.New(config, client.Options{Scheme: scheme})
}

func getKubeconfigPath() string {
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		kubeconfig = os.Getenv("HOME") + "/.kube/config"
	}

	return kubeconfig
}
