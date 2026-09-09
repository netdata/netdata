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
	GeneratedAt string                 `json:"generated_at"`
	GeneratedBy string                 `json:"generated_by"`
	Sources     []generationDatasetRef `json:"sources"`
	Counts      struct {
		AsnRanges int `json:"asn_ranges"`
		GeoRanges int `json:"geo_ranges"`
	} `json:"counts"`
	Policy struct {
		LocalhostCIDRs   []string `json:"localhost_cidrs"`
		PrivateCIDRs     []string `json:"private_cidrs"`
		InterestingCIDRs []string `json:"interesting_cidrs"`
	} `json:"policy"`
	Output struct {
		AsnFile      string `json:"asn_file,omitempty"`
		GeoFile      string `json:"geo_file,omitempty"`
		MetadataFile string `json:"metadata_file"`
	} `json:"output"`
}

type generationDatasetRef struct {
	Name         string `json:"name,omitempty"`
	Family       string `json:"family"`
	Provider     string `json:"provider,omitempty"`
	Artifact     string `json:"artifact,omitempty"`
	Source       string `json:"source,omitempty"`
	Format       string `json:"format,omitempty"`
	URL          string `json:"url,omitempty"`
	Path         string `json:"path,omitempty"`
	DownloadPage string `json:"download_page,omitempty"`
	ResolvedURL  string `json:"resolved_url,omitempty"`
}

func writeOutputs(
	cfg config,
	asnRanges []asnRange,
	geoRanges []geoRange,
	sources []generationDatasetRef,
) error {
	if err := os.MkdirAll(cfg.output.directory, 0o755); err != nil {
		return fmt.Errorf("failed to create output dir %s: %w", cfg.output.directory, err)
	}

	classes, err := classifyRanges(cfg.policy)
	if err != nil {
		return err
	}

	asnPath := filepath.Join(cfg.output.directory, cfg.output.asnFile)
	geoPath := filepath.Join(cfg.output.directory, cfg.output.geoFile)
	metadataPath := filepath.Join(cfg.output.directory, cfg.output.metadataFile)

	stageDir, err := os.MkdirTemp(cfg.output.directory, ".tmp-topology-ip-intel-stage-*")
	if err != nil {
		return fmt.Errorf("failed to create staging dir in %s: %w", cfg.output.directory, err)
	}
	defer os.RemoveAll(stageDir)

	stagedASNPath := filepath.Join(stageDir, cfg.output.asnFile)
	stagedGeoPath := filepath.Join(stageDir, cfg.output.geoFile)
	stagedMetadataPath := filepath.Join(stageDir, cfg.output.metadataFile)

	asnEnabled := cfg.hasFamily(sourceFamilyASN)
	geoEnabled := cfg.hasFamily(sourceFamilyGeo)

	if asnEnabled {
		if err := writeAsnDatabase(stagedASNPath, asnRanges, classes); err != nil {
			return fmt.Errorf("asn database generation failed: %w", err)
		}
	}
	if geoEnabled {
		if err := writeGeoDatabase(stagedGeoPath, geoRanges, classes); err != nil {
			return fmt.Errorf("geo database generation failed: %w", err)
		}
	}

	md := generationMetadata{}
	md.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	md.GeneratedBy = "topology-ip-intel-downloader"
	md.Sources = append([]generationDatasetRef(nil), sources...)
	md.Counts.AsnRanges = len(asnRanges)
	md.Counts.GeoRanges = len(geoRanges)
	md.Policy.LocalhostCIDRs = append([]string{}, cfg.policy.localhostCIDRs...)
	md.Policy.PrivateCIDRs = append([]string{}, cfg.policy.privateCIDRs...)
	md.Policy.InterestingCIDRs = append([]string{}, cfg.policy.interestingCIDRs...)
	if asnEnabled {
		md.Output.AsnFile = cfg.output.asnFile
	}
	if geoEnabled {
		md.Output.GeoFile = cfg.output.geoFile
	}
	md.Output.MetadataFile = cfg.output.metadataFile

	blob, err := json.MarshalIndent(md, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode metadata json: %w", err)
	}
	if err := os.WriteFile(stagedMetadataPath, blob, 0o644); err != nil {
		return fmt.Errorf("failed to write staged metadata %s: %w", stagedMetadataPath, err)
	}

	if asnEnabled {
		if err := renameFileAtomic(stagedASNPath, asnPath); err != nil {
			return err
		}
	} else if err := removeIfExists(asnPath); err != nil {
		return err
	}

	if geoEnabled {
		if err := renameFileAtomic(stagedGeoPath, geoPath); err != nil {
			return err
		}
	} else if err := removeIfExists(geoPath); err != nil {
		return err
	}

	if err := renameFileAtomic(stagedMetadataPath, metadataPath); err != nil {
		return err
	}
	return nil
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

func writeGeoDatabase(path string, ranges []geoRange, classes []classification) error {
	writer, err := mmdbwriter.New(mmdbwriter.Options{
		DatabaseType:            "Netdata-Topology-GEO",
		Description:             map[string]string{"en": "Netdata topology geographic mapping"},
		IPVersion:               6,
		RecordSize:              28,
		DisableIPv4Aliasing:     true,
		IncludeReservedNetworks: true,
	})
	if err != nil {
		return fmt.Errorf("failed to create GEO MMDB writer: %w", err)
	}

	for idx, rec := range ranges {
		record := mmdbtype.Map{}
		if rec.country != "" {
			record["country"] = mmdbtype.Map{
				"iso_code": mmdbtype.String(rec.country),
			}
		}
		if rec.city != "" {
			record["city"] = mmdbtype.Map{
				"names": mmdbtype.Map{
					"en": mmdbtype.String(rec.city),
				},
			}
		}
		if rec.state != "" {
			record["region"] = mmdbtype.String(rec.state)
			record["subdivisions"] = mmdbtype.Slice{
				mmdbtype.Map{
					"names": mmdbtype.Map{
						"en": mmdbtype.String(rec.state),
					},
				},
			}
		}
		if rec.hasLocation {
			record["location"] = mmdbtype.Map{
				"latitude":  mmdbtype.Float64(rec.latitude),
				"longitude": mmdbtype.Float64(rec.longitude),
			}
		}
		if err := insertRange(writer, rec.start, rec.end, record); err != nil {
			return fmt.Errorf("geo range %d (%s-%s): %w", idx, rec.start, rec.end, err)
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

func removeIfExists(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove stale output %s: %w", path, err)
	}
	return nil
}

func renameFileAtomic(from, to string) error {
	if err := os.Rename(from, to); err != nil {
		return fmt.Errorf("failed to atomically replace %s: %w", to, err)
	}
	return nil
}
