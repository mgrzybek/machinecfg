/*
Copyright © 2025 Mathieu GRZYBEK
*/
package cmd

import (
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"machinecfg/internal/core/domain"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "machinecfg",
	Short: "Creates machines’ configurations to use with matchbox",
	Long:  ``,
}

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
	rootCmd.PersistentFlags().StringP("region", "", "", "Region to extract data from")
	rootCmd.PersistentFlags().StringP("site", "", "", "Site to extract data from")
	rootCmd.PersistentFlags().StringP("location", "", "", "Location to extract data from")
	rootCmd.PersistentFlags().StringP("rack", "", "", "Apply a filter on the given rack")

	// Usage filters
	rootCmd.PersistentFlags().StringP("tenant", "", "", "Tenant to extract data from")
	rootCmd.PersistentFlags().StringP("role", "", "", "Apply a filter on the given role")
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

func processRootArgs(cmd *cobra.Command) *domain.ConfigurationArgs {
	fatalError := false
	endpoint, _ := cmd.Flags().GetString("netbox-endpoint")
	token, _ := cmd.Flags().GetString("netbox-token")
	outputDirectory, _ := cmd.Flags().GetString("output-directory")

	region, _ := cmd.Flags().GetString("region")
	site, _ := cmd.Flags().GetString("site")
	location, _ := cmd.Flags().GetString("location")
	rack, _ := cmd.Flags().GetString("rack")
	tenant, _ := cmd.Flags().GetString("tenant")
	role, _ := cmd.Flags().GetString("role")

	if endpoint == "" {
		slog.Error("endpoint option must be given")
		fatalError = true
	}

	if len(token) != 40 {
		slog.Error("token option must be valid")
		fatalError = true
	}

	if fatalError {
		os.Exit(1)
	}

	return &domain.ConfigurationArgs{
		Endpoint:        endpoint,
		Token:           token,
		OutputDirectory: outputDirectory,
		Region:          region,
		Site:            site,
		Location:        location,
		Rack:            rack,
		Tenant:          tenant,
		Role:            role,
	}
}
