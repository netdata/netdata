// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	var configPath string
	var outputDir string
	var asnFlags familyFlagValues
	var geoFlags familyFlagValues
	var noASN bool
	var noGeo bool

	asnFlags.family = sourceFamilyASN
	geoFlags.family = sourceFamilyGeo

	flag.StringVar(&configPath, "config", "", "path to topology IP intelligence YAML config")
	flag.StringVar(&outputDir, "output-dir", "", "override output directory")
	flag.Var(&asnFlags, "asn", "repeatable ASN source token: provider:artifact[@format]")
	flag.Var(&geoFlags, "geo", "repeatable GEO source token: provider:artifact[@format]")
	flag.BoolVar(&noASN, "no-asn", false, "disable ASN output and remove any stale ASN file")
	flag.BoolVar(&noGeo, "no-geo", false, "disable GEO output and remove any stale GEO file")
	flag.Parse()

	cfg, loadedPath, err := loadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "topology-ip-intel-downloader: failed to load config: %v\n", err)
		os.Exit(1)
	}
	if outputDir = strings.TrimSpace(outputDir); outputDir != "" {
		cfg.output.directory = outputDir
	}

	cfg.sources = applyFamilyOverride(cfg.sources, sourceFamilyASN, asnFlags.sources, noASN)
	cfg.sources = applyFamilyOverride(cfg.sources, sourceFamilyGeo, geoFlags.sources, noGeo)

	if err := cfg.normalizeAndValidate(); err != nil {
		fmt.Fprintf(os.Stderr, "topology-ip-intel-downloader: invalid config: %v\n", err)
		os.Exit(1)
	}

	printExecutionPlan(cfg)

	dl := newDownloader(cfg.http)
	asnRanges, geoRanges, sourceRefs, err := loadRanges(cfg, dl)
	if err != nil {
		fmt.Fprintf(os.Stderr, "topology-ip-intel-downloader: failed to download/parse source data: %v\n", err)
		os.Exit(1)
	}

	if err := writeOutputs(cfg, asnRanges, geoRanges, sourceRefs); err != nil {
		fmt.Fprintf(os.Stderr, "topology-ip-intel-downloader: failed to write outputs: %v\n", err)
		os.Exit(1)
	}

	asnPath := filepath.Join(cfg.output.directory, cfg.output.asnFile)
	geoPath := filepath.Join(cfg.output.directory, cfg.output.geoFile)
	metadataPath := filepath.Join(cfg.output.directory, cfg.output.metadataFile)
	if loadedPath != "" {
		fmt.Printf("updated IP intelligence databases using config %s\n", loadedPath)
	} else {
		fmt.Printf("updated IP intelligence databases using built-in defaults\n")
	}
	if cfg.hasFamily(sourceFamilyASN) {
		fmt.Printf("asn_mmdb=%s\n", asnPath)
	} else {
		fmt.Printf("asn_mmdb=disabled\n")
	}
	if cfg.hasFamily(sourceFamilyGeo) {
		fmt.Printf("geo_mmdb=%s\n", geoPath)
	} else {
		fmt.Printf("geo_mmdb=disabled\n")
	}
	fmt.Printf("metadata=%s\n", metadataPath)
	fmt.Printf("asn_ranges=%d geo_ranges=%d\n", len(asnRanges), len(geoRanges))
}

func applyFamilyOverride(
	current []sourceEntry,
	family string,
	override []sourceEntry,
	disable bool,
) []sourceEntry {
	if !disable && len(override) == 0 {
		return current
	}

	out := make([]sourceEntry, 0, len(current)+len(override))
	for _, source := range current {
		if source.family != family {
			out = append(out, source)
		}
	}
	if disable {
		return out
	}
	out = append(out, override...)
	return out
}

func printExecutionPlan(cfg config) {
	fmt.Printf("effective source plan:\n")
	printFamilyPlan(sourceFamilyASN, cfg.familySources(sourceFamilyASN))
	printFamilyPlan(sourceFamilyGeo, cfg.familySources(sourceFamilyGeo))
	fmt.Printf("output actions:\n")
	if cfg.hasFamily(sourceFamilyASN) {
		fmt.Printf("- write %s\n", cfg.output.asnFile)
	} else {
		fmt.Printf("- remove %s\n", cfg.output.asnFile)
	}
	if cfg.hasFamily(sourceFamilyGeo) {
		fmt.Printf("- write %s\n", cfg.output.geoFile)
	} else {
		fmt.Printf("- remove %s\n", cfg.output.geoFile)
	}
	fmt.Printf("- write %s\n", cfg.output.metadataFile)
}

func printFamilyPlan(family string, sources []sourceEntry) {
	label := strings.ToUpper(family)
	if len(sources) == 0 {
		fmt.Printf("%s sources: none\n", label)
		return
	}
	fmt.Printf("%s sources (first wins):\n", label)
	for i, source := range sources {
		fmt.Printf("- %d. %s\n", i+1, sourceDisplayName(source))
	}
}

func sourceDisplayName(source sourceEntry) string {
	base := fmt.Sprintf("%s:%s", source.provider, source.artifact)
	if source.format != "" {
		base = fmt.Sprintf("%s@%s", base, source.format)
	}
	if source.path != "" {
		return fmt.Sprintf("%s path=%s", base, source.path)
	}
	if source.url != "" {
		return fmt.Sprintf("%s url=%s", base, redactURLForDisplay(source.url))
	}
	return base
}

func redactURLForDisplay(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return "<redacted>"
	}
	return fmt.Sprintf("%s://%s/<redacted>", parsed.Scheme, parsed.Host)
}
