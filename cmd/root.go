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
	rootCmd.PersistentFlags().StringP("log-level", "", "", "Log level ’development’ (default) or ’production’")
	rootCmd.PersistentFlags().StringP("netbox-token", "", "", "Token used to call Netbox API")
	rootCmd.PersistentFlags().StringP("netbox-endpoint", "", "", "URL of the API")
	rootCmd.PersistentFlags().StringP("output-directory", "", "", "Where to write the result")
	rootCmd.PersistentFlags().StringP("site", "", "", "Site to extract data from")
	rootCmd.PersistentFlags().StringP("tenant", "", "", "Tenant to extract data from")
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
	outputDirectory, _ := cmd.Flags().GetString("output-directory")
	site, _ := cmd.Flags().GetString("site")
	tenant, _ := cmd.Flags().GetString("tenant")
	token, _ := cmd.Flags().GetString("netbox-token")

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
		OutputDirectory: outputDirectory,
		Site:            site,
		Tenant:          tenant,
		Token:           token,
	}
}
