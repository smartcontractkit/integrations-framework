package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"

	"github.com/smartcontractkit/chainlink-testing-framework/parrot"
)

const (
	envPort      = "PARROT_PORT"
	envLogLevel  = "PARROT_LOG_LEVEL"
	envJSON      = "PARROT_JSON"
	envRecorders = "PARROT_RECORDERS"
	envHost      = "PARROT_HOST"
)

func main() {
	var (
		port      int
		debug     bool
		trace     bool
		silent    bool
		logLevel  string
		json      bool
		recorders []string
		host      string
	)

	preRun := func(cmd *cobra.Command, args []string) {
		// Check environment variables if flags are not set
		if !cmd.Flags().Changed("port") {
			if envPort, err := strconv.Atoi(os.Getenv(envPort)); err == nil {
				port = envPort
			}
		}
		if !cmd.Flags().Changed("host") {
			if addr := os.Getenv(envHost); addr != "" {
				host = addr
			}
		}
		if !cmd.Flags().Changed("debug") &&
			!cmd.Flags().Changed("trace") &&
			!cmd.Flags().Changed("silent") &&
			!cmd.Flags().Changed("logLevel") {
			if lvl := os.Getenv(envLogLevel); lvl != "" {
				logLevel = lvl
			}
		}
		if !cmd.Flags().Changed("json") {
			json = os.Getenv(envJSON) == "true"
		}
		if !cmd.Flags().Changed("recorders") {
			if r := os.Getenv(envRecorders); r != "" {
				recorders = strings.Split(r, ",")
			}
		}
	}

	rootCmd := &cobra.Command{
		Use:    "parrot",
		Short:  "A server that can register and parrot back dynamic requests",
		PreRun: preRun,
		RunE: func(cmd *cobra.Command, args []string) error {
			options := []parrot.ServerOption{parrot.WithPort(port), parrot.WithHost(host)}
			zerologLevel := zerolog.InfoLevel
			if debug {
				zerologLevel = zerolog.DebugLevel
			}
			if trace {
				zerologLevel = zerolog.TraceLevel
			}
			if silent {
				zerologLevel = zerolog.Disabled
			}
			if logLevel != "" {
				parsedLevel, err := zerolog.ParseLevel(logLevel)
				if err != nil {
					return err
				}
				zerologLevel = parsedLevel
			}
			options = append(options, parrot.WithLogLevel(zerologLevel))
			if json {
				options = append(options, parrot.WithJSONLogs())
			}
			options = append(options, parrot.WithRecorders(recorders...))

			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()

			p, err := parrot.NewServer(options...)
			if err != nil {
				return err
			}

			c := make(chan os.Signal, 1)
			signal.Notify(c, os.Interrupt, syscall.SIGTERM)
			<-c
			err = p.Shutdown(ctx)
			if err != nil {
				log.Error().Err(err).Msg("Error putting parrot to sleep")
			}
			return nil
		},
	}

	rootCmd.PersistentFlags().IntVarP(&port, "port", "p", 0, fmt.Sprintf("Port to run the parrot on (env: %s)", envPort))
	rootCmd.Flags().BoolVarP(&debug, "debug", "d", false, "Enable debug output")
	rootCmd.Flags().BoolVarP(&trace, "trace", "t", false, "Enable trace and debug output")
	rootCmd.Flags().BoolVarP(&silent, "silent", "s", false, "Disable all output")
	rootCmd.Flags().StringVarP(&logLevel, "logLevel", "l", "", fmt.Sprintf("Set the log level (env: %s)", envLogLevel))
	rootCmd.Flags().BoolVarP(&json, "json", "j", false, fmt.Sprintf("Output logs in JSON format (env: %s)", envJSON))
	rootCmd.Flags().StringSliceVarP(&recorders, "recorders", "r", nil, fmt.Sprintf("Existing recorders to use (env: %s)", envRecorders))
	rootCmd.Flags().StringVar(&host, "host", "localhost", fmt.Sprintf("Host to run the parrot on. (env: %s)", envHost))

	healthCheckCmd := &cobra.Command{
		Use:    "health",
		Short:  "Check if the parrot server is healthy",
		PreRun: preRun,
		Run: func(cmd *cobra.Command, args []string) {
			resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", port))
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			if resp.StatusCode != 200 {
				fmt.Printf("Health check failed with status code %d\n", resp.StatusCode)
				os.Exit(1)
			}
			fmt.Println("Parrot is healthy!")
		},
		SilenceUsage: true,
	}

	rootCmd.AddCommand(healthCheckCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Printf("Error running parrot:\n%s\n", err)
		os.Exit(1)
	}
}
