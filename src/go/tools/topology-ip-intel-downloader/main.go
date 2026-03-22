// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	var configPath string
	var outputDir string

	flag.StringVar(&configPath, "config", "", "path to topology ip intelligence yaml config")
	flag.StringVar(&outputDir, "output-dir", "", "override output directory")
	flag.Parse()

	cfg, loadedPath, err := loadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "topology-ip-intel-downloader: failed to load config: %v\n", err)
		os.Exit(1)
	}
	if outputDir = strings.TrimSpace(outputDir); outputDir != "" {
		cfg.output.directory = outputDir
	}
	if err := cfg.validate(); err != nil {
		fmt.Fprintf(os.Stderr, "topology-ip-intel-downloader: invalid config: %v\n", err)
		os.Exit(1)
	}

	dl := newDownloader(cfg.http)
	asnRanges, countryRanges, err := loadRanges(cfg, dl)
	if err != nil {
		fmt.Fprintf(os.Stderr, "topology-ip-intel-downloader: failed to download/parse source data: %v\n", err)
		os.Exit(1)
	}

	if err := writeOutputs(cfg, asnRanges, countryRanges); err != nil {
		fmt.Fprintf(os.Stderr, "topology-ip-intel-downloader: failed to write outputs: %v\n", err)
		os.Exit(1)
	}

	asnPath := filepath.Join(cfg.output.directory, cfg.output.asnFile)
	countryPath := filepath.Join(cfg.output.directory, cfg.output.countryFile)
	metadataPath := filepath.Join(cfg.output.directory, cfg.output.metadataFile)
	if loadedPath != "" {
		fmt.Printf("updated ip intelligence databases using config %s\n", loadedPath)
	} else {
		fmt.Printf("updated ip intelligence databases using built-in defaults\n")
	}
	fmt.Printf("asn_mmdb=%s\n", asnPath)
	fmt.Printf("country_mmdb=%s\n", countryPath)
	fmt.Printf("metadata=%s\n", metadataPath)
	fmt.Printf("asn_ranges=%d country_ranges=%d\n", len(asnRanges), len(countryRanges))
}
