/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"log/slog"
	"os"
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
	rootCmd.PersistentFlags().StringP("log-level", "", "", "Log level ’development’ (default) or ’production’")
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
	case "production":
		handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
		logger := slog.New(handler)
		slog.SetDefault(logger)
	default:
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}
}

func processRootArgs(cmd *cobra.Command) *ConfigurationArgs {
	fatalError := false
	endpoint, _ := cmd.Flags().GetString("netbox-endpoint")
	token, _ := cmd.Flags().GetString("netbox-token")
	outputDirectory, _ := cmd.Flags().GetString("output-directory")

	regions, _ := cmd.Flags().GetString("regions")
	sites, _ := cmd.Flags().GetString("sites")
	locations, _ := cmd.Flags().GetString("locations")
	//racks, _ := cmd.Flags().GetString("racks")
	tenants, _ := cmd.Flags().GetString("tenants")
	roles, _ := cmd.Flags().GetString("roles")
	virtualization, _ := cmd.Flags().GetBool("virtualization")
	clusters, _ := cmd.Flags().GetString("clusters")

	if endpoint == "" {
		slog.Error("endpoint option must be given")
		fatalError = true
	}

	if len(token) != 40 {
		slog.Error("token option must be valid")
		fatalError = true
	}

	if len(outputDirectory) == 0 {
		slog.Error("output-directory must be given")
		fatalError = true
	}

	/*
		if regions == "" {
			slog.Error("regions option must be given")
			fatalError = true
		}
	*/

	if sites == "" {
		slog.Error("sites option must be given")
		fatalError = true
	}

	/*
		if locations == "" {
			slog.Error("locations option must be given")
			fatalError = true
		}

		if tenants == "" {
			slog.Error("tenants option must be given")
			fatalError = true
		}
	*/

	if roles == "" {
		slog.Error("roles option must be given")
		fatalError = true
	}

	if fatalError {
		os.Exit(1)
	}

	return &ConfigurationArgs{
		Endpoint:        endpoint,
		Token:           token,
		OutputDirectory: outputDirectory,

		Filters: common.DeviceFilters{
			Regions:   strings.Split(regions, ","),
			Sites:     strings.Split(sites, ","),
			Locations: strings.Split(locations, ","),
			//Racks:
			Tenants:        strings.Split(tenants, ","),
			Roles:          strings.Split(roles, ","),
			Virtualisation: virtualization,
			Clusters:       strings.Split(clusters, ","),
		},
	}
}

func dirExists(path string) bool {
	dirStats, err := os.Stat(path)
	if err == nil {
		return true
	} else {
		slog.Error("dirExists", "message", err.Error())
	}
	if os.IsNotExist(err) {
		return false
	}
	if dirStats.IsDir() {
		return true
	}
	return false
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	if err == nil {
		return true
	} else {
		slog.Error("fileExists", "message", err.Error())
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
		slog.Debug("getk8sClient", "message", "In-Cluster failed, trying Out-Cluster configuration...")

		kubeconfig := getKubeconfigPath()

		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("cannot find any kubeconfig: %w", err)
		} else {
			slog.Info("getK8sClient", "message", "Out-Cluster configuration found", "kubeconfig", kubeconfig)
		}
	} else {
		slog.Info("getK8sClient", "message", "In-Cluster configuration found")
	}

	scheme = runtime.NewScheme()

	err = tinkerbellKubeObjects.AddToScheme(scheme)
	if err != nil {
		slog.Error("getK8sClient", "message", err.Error())
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
