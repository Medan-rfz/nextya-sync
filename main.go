package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"nextya-sync/clients"
	"nextya-sync/processor"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cfgFile string
	rootCmd = &cobra.Command{
		Use:   "nextya-sync",
		Short: "Synchronization tool for Nextcloud and Yandex Disk",
		Long: `nextya-sync is a CLI tool for synchronizing files between 
Nextcloud and Yandex Disk cloud storage services.

It supports various operations like listing files, uploading, downloading, 
and full synchronization between the two platforms.`,
		Run: process,
	}
)

func init() {
	cobra.OnInitialize(initConfig)

	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.nextya-sync.yaml)")

	// Yandex Disk flags
	rootCmd.Flags().StringP("yandex-token", "y", "", "Yandex Disk OAuth token")
	rootCmd.Flags().StringP("yandex-target-path", "t", "disk:/nextcloud", "Target path in Yandex Disk for synchronization")

	// Nextcloud flags
	rootCmd.Flags().StringP("nextcloud-url", "u", "", "Nextcloud server URL")
	rootCmd.Flags().StringP("nextcloud-username", "n", "", "Nextcloud username")
	rootCmd.Flags().StringP("nextcloud-password", "p", "", "Nextcloud password")
	rootCmd.Flags().StringSliceP("nextcloud-paths", "s", []string{"/"}, "List of paths to sync from Nextcloud (comma-separated)")

	// Bind flags to viper
	viper.BindPFlag("yandex.token", rootCmd.Flags().Lookup("yandex-token"))
	viper.BindPFlag("yandex.target_path", rootCmd.Flags().Lookup("yandex-target-path"))
	viper.BindPFlag("nextcloud.url", rootCmd.Flags().Lookup("nextcloud-url"))
	viper.BindPFlag("nextcloud.username", rootCmd.Flags().Lookup("nextcloud-username"))
	viper.BindPFlag("nextcloud.password", rootCmd.Flags().Lookup("nextcloud-password"))
	viper.BindPFlag("nextcloud.sync_paths", rootCmd.Flags().Lookup("nextcloud-paths"))

	// Bind environment variables
	viper.BindEnv("yandex.token", "YANDEX_TOKEN")
	viper.BindEnv("yandex.target_path", "YANDEX_TARGET_PATH")
	viper.BindEnv("nextcloud.url", "NEXTCLOUD_URL")
	viper.BindEnv("nextcloud.username", "NEXTCLOUD_USERNAME")
	viper.BindEnv("nextcloud.password", "NEXTCLOUD_PASSWORD")
	viper.BindEnv("nextcloud.sync_paths", "NEXTCLOUD_SYNC_PATHS")
}

func initConfig() {
	if cfgFile != "" {
		// Use specified config file
		viper.SetConfigFile(cfgFile)
	} else {
		// Look for config file in home directory
		home, err := os.UserHomeDir()
		cobra.CheckErr(err)

		// Look for config in home directory with name ".nextya-sync" (without extension)
		viper.AddConfigPath(home)
		viper.AddConfigPath(".")
		viper.SetConfigType("yaml")
		viper.SetConfigName(".nextya-sync")
	}

	viper.AutomaticEnv() // read environment variables

	// If config file is found, read it
	if err := viper.ReadInConfig(); err == nil {
		fmt.Fprintf(os.Stderr, "Using config file: %s\n", viper.ConfigFileUsed())
	}
}

func validation() {
	// Validate required flags
	if viper.GetString("yandex.token") == "" {
		log.Fatal("❌ Yandex token is required")
	}
	if viper.GetString("nextcloud.url") == "" {
		log.Fatal("❌ Nextcloud URL is required")
	}
	if viper.GetString("nextcloud.username") == "" {
		log.Fatal("❌ Nextcloud username is required")
	}
	if viper.GetString("nextcloud.password") == "" {
		log.Fatal("❌ Nextcloud password is required")
	}

	yndxTargetPath := viper.GetString("yandex.target_path")
	if yndxTargetPath == "/" || yndxTargetPath == "disk:/" {
		log.Fatal("❌ Forbidden: Yandex target path is set to root, this may overwrite existing files")
	}
}

func process(cmd *cobra.Command, args []string) {
	ctx := context.Background()

	validation()

	nextcloudClient := clients.NewNextcloudClient(
		viper.GetString("nextcloud.url"),
		viper.GetString("nextcloud.username"),
		viper.GetString("nextcloud.password"),
	)
	if err := nextcloudClient.Authenticate(ctx); err != nil {
		log.Fatalf("❌ Failed to authenticate with Nextcloud: %v", err)
	}

	yandexClient := clients.NewYandexDiskClient(viper.GetString("yandex.token"))
	if err := yandexClient.Authenticate(ctx); err != nil {
		log.Fatalf("❌ Failed to authenticate with Yandex Disk: %v", err)
	}

	proc := processor.NewProcessor(&processor.Dependencies{
		YandexClient:    yandexClient,
		NextcloudClient: nextcloudClient,
	})

	log.Fatalln(proc.Main(ctx, processor.Config{
		YandexTargetPath:   viper.GetString("yandex.target_path"),
		NextcloudSyncPaths: viper.GetStringSlice("nextcloud.sync_paths"),
	}))
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
