// SPDX-License-Identifier: GPL-3.0-or-later

// Alternative UI for Netdata
//
// This is a lightweight metrics aggregation server that can receive metrics
// from multiple nodes or applications and display them in a unified dashboard.
//
// Features:
// - Receive metrics via HTTP POST from any app/node
// - Real-time WebSocket updates
// - Multi-node visualization
// - Configurable retention
// - Simple REST API
//
// Usage:
//   alternative-ui [options]
//
// Options:
//   -addr      Listen address (default: :19998)
//   -retention Data retention duration (default: 1h)
//   -api-key   API key for push authentication (optional)

package main

import (
	"embed"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/netdata/netdata/src/alternative-ui/metrics"
	"github.com/netdata/netdata/src/alternative-ui/server"
)

//go:embed web/*
var webFS embed.FS

var (
	version = "1.0.0"
)

func main() {
	// Parse command line flags
	addr := flag.String("addr", ":19998", "Listen address")
	retention := flag.Duration("retention", 1*time.Hour, "Data retention duration")
	maxPoints := flag.Int("max-points", 3600, "Maximum data points per dimension")
	apiKey := flag.String("api-key", "", "API key for push authentication (optional)")
	showVersion := flag.Bool("version", false, "Show version")
	flag.Parse()

	if *showVersion {
		log.Printf("Netdata Alternative UI v%s", version)
		os.Exit(0)
	}

	log.Printf("Netdata Alternative UI v%s", version)
	log.Printf("Configuration:")
	log.Printf("  Listen address: %s", *addr)
	log.Printf("  Data retention: %s", *retention)
	log.Printf("  Max points: %d", *maxPoints)
	if *apiKey != "" {
		log.Printf("  API key: configured")
	} else {
		log.Printf("  API key: not configured (push endpoint is open)")
	}

	// Create metrics store
	store := metrics.NewStore(*retention, *maxPoints)

	// Create and start HTTP server
	srv := server.NewServer(store, *addr, *apiKey, webFS)

	// Handle shutdown gracefully
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Shutting down...")
		os.Exit(0)
	}()

	// Start server
	if err := srv.Start(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
