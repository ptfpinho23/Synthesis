package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/synthesis/orchestrator/pkg/runtime"
	"github.com/synthesis/orchestrator/pkg/runtime/containerd"
	"github.com/synthesis/orchestrator/pkg/server"
)

var (
	cfgFile   string
	serverCfg *server.Config
)

func main() {
	var rootCmd = &cobra.Command{
		Use:   "synthesis-server",
		Short: "Synthesis lightweight container orchestrator server",
		Long:  "A lightweight container orchestration platform - like Kubernetes, but tinier. Compatible with Kubernetes workload manifests (Pods, Deployments, StatefulSets).",
	}

	var startCmd = &cobra.Command{
		Use:   "start",
		Short: "Start the synthesis orchestrator server",
		Run:   runServer,
	}

	var versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Synthesis Server v0.1.0")
			fmt.Println("Compatible with Kubernetes Pod, Deployment, and StatefulSet manifests")
		},
	}

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./synthesis.yaml)")

	startCmd.Flags().String("listen-addr", ":8080", "Server listen address")
	startCmd.Flags().String("runtime", "containerd", "Container runtime to use (containerd)")
	startCmd.Flags().String("runtime-socket", "", "Container runtime socket path")
	startCmd.Flags().String("data-dir", "./data", "Data directory for persistent storage")
	startCmd.Flags().Bool("debug", false, "Enable debug logging")

	viper.BindPFlag("server.listen_addr", startCmd.Flags().Lookup("listen-addr"))
	viper.BindPFlag("runtime.type", startCmd.Flags().Lookup("runtime"))
	viper.BindPFlag("runtime.socket_path", startCmd.Flags().Lookup("runtime-socket"))
	viper.BindPFlag("server.data_dir", startCmd.Flags().Lookup("data-dir"))
	viper.BindPFlag("server.debug", startCmd.Flags().Lookup("debug"))

	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(versionCmd)

	cobra.OnInitialize(initConfig)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		viper.SetConfigName("synthesis")
		viper.SetConfigType("yaml")
		viper.AddConfigPath(".")
		viper.AddConfigPath("$HOME/.synthesis")
		viper.AddConfigPath("/etc/synthesis")
	}

	viper.SetDefault("server.listen_addr", ":8080")
	viper.SetDefault("server.debug", false)
	viper.SetDefault("server.data_dir", "./data")
	viper.SetDefault("runtime.type", "containerd")
	viper.SetDefault("runtime.socket_path", "/run/containerd/containerd.sock")
	viper.SetDefault("runtime.timeout", 30)
	viper.SetDefault("runtime.default_network", "synthesis")

	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err == nil {
		fmt.Println("Using config file:", viper.ConfigFileUsed())
	}

	serverCfg = &server.Config{
		ListenAddr: viper.GetString("server.listen_addr"),
		Debug:      viper.GetBool("server.debug"),
		DataDir:    viper.GetString("server.data_dir"),
		Runtime: runtime.RuntimeConfig{
			SocketPath:     viper.GetString("runtime.socket_path"),
			APIVersion:     viper.GetString("runtime.api_version"),
			Timeout:        viper.GetInt("runtime.timeout"),
			DefaultNetwork: viper.GetString("runtime.default_network"),
			DefaultLabels: map[string]string{
				"managed-by": "synthesis",
			},
		},
	}
}

func runServer(cmd *cobra.Command, args []string) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var containerRuntime runtime.ContainerRuntime
	var err error

	switch viper.GetString("runtime.type") {
	case "containerd":
		containerRuntime, err = containerd.NewContainerdRuntime(&serverCfg.Runtime)
		if err != nil {
			log.Fatalf("Failed to create containerd runtime: %v", err)
		}
	default:
		log.Fatalf("Unsupported runtime type: %s (only containerd is supported)", viper.GetString("runtime.type"))
	}

	if err := containerRuntime.HealthCheck(ctx); err != nil {
		log.Fatalf("Container runtime health check failed: %v", err)
	}

	log.Printf("Container runtime (%s) initialized successfully", viper.GetString("runtime.type"))

	srv, err := server.NewServer(serverCfg, containerRuntime)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	router := mux.NewRouter()
	srv.SetupRoutes(router)

	httpServer := &http.Server{
		Addr:         serverCfg.ListenAddr,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("Starting Synthesis server on %s", serverCfg.ListenAddr)
		log.Printf("Synthesis is ready to accept Kubernetes manifests (Pod, Deployment, StatefulSet)")
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	go srv.StartControllers(ctx)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down server...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	cancel()

	log.Println("Server stopped")
} 