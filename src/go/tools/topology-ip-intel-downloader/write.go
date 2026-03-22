// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"time"

	"github.com/maxmind/mmdbwriter"
	"github.com/maxmind/mmdbwriter/mmdbtype"
	"go4.org/netipx"
)

type generationMetadata struct {
	GeneratedAt string `json:"generated_at"`
	GeneratedBy string `json:"generated_by"`
	Source      struct {
		Provider string                `json:"provider"`
		Combined *generationDatasetRef `json:"combined,omitempty"`
		Asn      *generationDatasetRef `json:"asn,omitempty"`
		Country  *generationDatasetRef `json:"country,omitempty"`
	} `json:"source"`
	Counts struct {
		AsnRanges     int `json:"asn_ranges"`
		CountryRanges int `json:"country_ranges"`
	} `json:"counts"`
	Policy struct {
		LocalhostCIDRs   []string `json:"localhost_cidrs"`
		PrivateCIDRs     []string `json:"private_cidrs"`
		InterestingCIDRs []string `json:"interesting_cidrs"`
	} `json:"policy"`
	Output struct {
		AsnFile     string `json:"asn_file"`
		CountryFile string `json:"country_file"`
		MetadataFile string `json:"metadata_file"`
	} `json:"output"`
}

type generationDatasetRef struct {
	URL         string `json:"url,omitempty"`
	Path        string `json:"path,omitempty"`
	Format      string `json:"format,omitempty"`
	Compression string `json:"compression,omitempty"`
}

func writeOutputs(cfg config, asnRanges []asnRange, countryRanges []countryRange) error {
	if err := os.MkdirAll(cfg.output.directory, 0o755); err != nil {
		return fmt.Errorf("failed to create output dir %s: %w", cfg.output.directory, err)
	}

	classes, err := classifyRanges(cfg.policy)
	if err != nil {
		return err
	}

	asnPath := filepath.Join(cfg.output.directory, cfg.output.asnFile)
	countryPath := filepath.Join(cfg.output.directory, cfg.output.countryFile)
	metadataPath := filepath.Join(cfg.output.directory, cfg.output.metadataFile)

	if err := writeAsnDatabase(asnPath, asnRanges, classes); err != nil {
		return fmt.Errorf("asn database generation failed: %w", err)
	}
	if err := writeCountryDatabase(countryPath, countryRanges, classes); err != nil {
		return fmt.Errorf("country database generation failed: %w", err)
	}

	md := generationMetadata{}
	md.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	md.GeneratedBy = "topology-ip-intel-downloader"
	md.Source = metadataSource(cfg.source)
	md.Counts.AsnRanges = len(asnRanges)
	md.Counts.CountryRanges = len(countryRanges)
	md.Policy.LocalhostCIDRs = append([]string{}, cfg.policy.localhostCIDRs...)
	md.Policy.PrivateCIDRs = append([]string{}, cfg.policy.privateCIDRs...)
	md.Policy.InterestingCIDRs = append([]string{}, cfg.policy.interestingCIDRs...)
	md.Output.AsnFile = cfg.output.asnFile
	md.Output.CountryFile = cfg.output.countryFile
	md.Output.MetadataFile = cfg.output.metadataFile

	blob, err := json.MarshalIndent(md, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode metadata json: %w", err)
	}
	if err := atomicWriteFile(metadataPath, blob, 0o644); err != nil {
		return err
	}
	return nil
}

func metadataDatasetSpec(spec datasetSpec) *generationDatasetRef {
	if spec.url == "" && spec.path == "" && spec.format == "" && spec.compression == "" {
		return nil
	}
	return &generationDatasetRef{
		URL:         spec.url,
		Path:        spec.path,
		Format:      spec.format,
		Compression: spec.compression,
	}
}

func metadataSource(src sourceConfig) struct {
	Provider string                `json:"provider"`
	Combined *generationDatasetRef `json:"combined,omitempty"`
	Asn      *generationDatasetRef `json:"asn,omitempty"`
	Country  *generationDatasetRef `json:"country,omitempty"`
} {
	md := struct {
		Provider string                `json:"provider"`
		Combined *generationDatasetRef `json:"combined,omitempty"`
		Asn      *generationDatasetRef `json:"asn,omitempty"`
		Country  *generationDatasetRef `json:"country,omitempty"`
	}{
		Provider: src.provider,
	}

	switch src.provider {
	case providerIPToASN:
		md.Combined = metadataDatasetSpec(src.combined)
	case providerDBIP:
		md.Asn = metadataDatasetSpec(src.asn)
		md.Country = metadataDatasetSpec(src.country)
	case providerCustom:
		combinedConfigured := src.combined.url != "" || src.combined.path != ""
		asnConfigured := src.asn.url != "" || src.asn.path != ""
		countryConfigured := src.country.url != "" || src.country.path != ""

		if combinedConfigured {
			md.Combined = metadataDatasetSpec(src.combined)
		}
		if asnConfigured {
			md.Asn = metadataDatasetSpec(src.asn)
		}
		if countryConfigured {
			md.Country = metadataDatasetSpec(src.country)
		}
	}

	return md
}

func writeAsnDatabase(path string, ranges []asnRange, classes []classification) error {
	writer, err := mmdbwriter.New(mmdbwriter.Options{
		DatabaseType:            "Netdata-Topology-ASN",
		Description:             map[string]string{"en": "Netdata topology ASN mapping"},
		IPVersion:               6,
		RecordSize:              28,
		DisableIPv4Aliasing:     true,
		IncludeReservedNetworks: true,
	})
	if err != nil {
		return fmt.Errorf("failed to create ASN MMDB writer: %w", err)
	}

	for idx, rec := range ranges {
		record := mmdbtype.Map{}
		if rec.asn != 0 {
			record["autonomous_system_number"] = mmdbtype.Uint32(rec.asn)
		}
		if rec.org != "" {
			record["autonomous_system_organization"] = mmdbtype.String(rec.org)
		}
		if err := insertRange(writer, rec.start, rec.end, record); err != nil {
			return fmt.Errorf("asn range %d (%s-%s): %w", idx, rec.start, rec.end, err)
		}
	}

	if err := applyClassifications(writer, classes); err != nil {
		return err
	}
	return writeMMDBAtomic(path, writer)
}

func writeCountryDatabase(path string, ranges []countryRange, classes []classification) error {
	writer, err := mmdbwriter.New(mmdbwriter.Options{
		DatabaseType:            "Netdata-Topology-Country",
		Description:             map[string]string{"en": "Netdata topology country mapping"},
		IPVersion:               6,
		RecordSize:              28,
		DisableIPv4Aliasing:     true,
		IncludeReservedNetworks: true,
	})
	if err != nil {
		return fmt.Errorf("failed to create country MMDB writer: %w", err)
	}

	for idx, rec := range ranges {
		record := mmdbtype.Map{
			"country": mmdbtype.Map{
				"iso_code": mmdbtype.String(rec.country),
			},
		}
		if err := insertRange(writer, rec.start, rec.end, record); err != nil {
			return fmt.Errorf("country range %d (%s-%s): %w", idx, rec.start, rec.end, err)
		}
	}

	if err := applyClassifications(writer, classes); err != nil {
		return err
	}
	return writeMMDBAtomic(path, writer)
}

func applyClassifications(writer *mmdbwriter.Tree, classes []classification) error {
	for _, classSet := range classes {
		record := mmdbtype.Map{
			"netdata": mmdbtype.Map{
				"ip_class":         mmdbtype.String(classSet.class),
				"track_individual": mmdbtype.Bool(trackIndividual(classSet.class)),
			},
		}
		for _, prefix := range classSet.prefixes {
			ipRange := netipx.RangeOfPrefix(prefix)
			start := addrToNetIP(ipRange.From())
			end := addrToNetIP(ipRange.To())
			if err := writer.InsertRange(start, end, record); err != nil {
				return fmt.Errorf("failed to apply class %s for %s: %w", classSet.class, prefix, err)
			}
		}
	}
	return nil
}

func insertRange(writer *mmdbwriter.Tree, start, end netip.Addr, value mmdbtype.DataType) error {
	if value == nil {
		value = mmdbtype.Map{}
	}
	if err := writer.InsertRange(addrToNetIP(start), addrToNetIP(end), value); err != nil {
		return err
	}
	return nil
}

func addrToNetIP(addr netip.Addr) net.IP {
	if addr.Is4() {
		a := addr.As4()
		return net.IPv4(a[0], a[1], a[2], a[3])
	}
	a := addr.As16()
	return net.IP(a[:])
}

func writeMMDBAtomic(path string, writer *mmdbwriter.Tree) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-topology-ip-intel-*.mmdb")
	if err != nil {
		return fmt.Errorf("failed to create temporary file in %s: %w", dir, err)
	}
	tmpPath := tmp.Name()

	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}

	if _, err := writer.WriteTo(tmp); err != nil {
		cleanup()
		return fmt.Errorf("failed to write temporary MMDB %s: %w", tmpPath, err)
	}
	if err := tmp.Chmod(0o644); err != nil {
		cleanup()
		return fmt.Errorf("failed to chmod temporary MMDB %s: %w", tmpPath, err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("failed to close temporary MMDB %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		cleanup()
		return fmt.Errorf("failed to atomically replace %s: %w", path, err)
	}
	return nil
}

func atomicWriteFile(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-topology-ip-intel-*.json")
	if err != nil {
		return fmt.Errorf("failed to create temporary file in %s: %w", dir, err)
	}
	tmpPath := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}
	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return fmt.Errorf("failed to write temporary file %s: %w", tmpPath, err)
	}
	if err := tmp.Chmod(mode); err != nil {
		cleanup()
		return fmt.Errorf("failed to chmod temporary file %s: %w", tmpPath, err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("failed to close temporary file %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		cleanup()
		return fmt.Errorf("failed to atomically replace %s: %w", path, err)
	}
	return nil
}
