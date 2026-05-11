// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var errTarMMDBNotFound = errors.New("tar payload has no mmdb member")

type downloader struct {
	client    *http.Client
	userAgent string
}

func newDownloader(httpCfg httpConfig) *downloader {
	return &downloader{
		client:    &http.Client{Timeout: httpCfg.timeout},
		userAgent: strings.TrimSpace(httpCfg.userAgent),
	}
}

func (d *downloader) readDataset(source sourceEntry) (generationDatasetRef, []byte, error) {
	resolved, err := d.resolveSource(source)
	if err != nil {
		return generationDatasetRef{}, nil, err
	}

	raw, err := d.readRaw(resolved.fetchPath, resolved.fetchURL)
	if err != nil {
		return generationDatasetRef{}, nil, err
	}
	payload, err := decodePayloadForSource(source, raw)
	if err != nil {
		return generationDatasetRef{}, nil, err
	}
	return resolved.ref, payload, nil
}

type resolvedSource struct {
	ref       generationDatasetRef
	fetchURL  string
	fetchPath string
}

func (d *downloader) resolveSource(source sourceEntry) (resolvedSource, error) {
	spec, ok := builtInSource(source.provider, source.artifact)
	if !ok {
		return resolvedSource{}, fmt.Errorf(
			"unsupported provider/artifact %q/%q",
			source.provider,
			source.artifact,
		)
	}

	ref := generationDatasetRef{
		Name:     source.name,
		Family:   source.family,
		Provider: source.provider,
		Artifact: source.artifact,
		Format:   source.format,
	}

	switch {
	case source.path != "":
		path, err := expandEnvPlaceholders(source.path)
		if err != nil {
			return resolvedSource{}, err
		}
		ref.Source = "path"
		ref.Path = path
		return resolvedSource{
			ref:       ref,
			fetchPath: path,
		}, nil
	case source.url != "":
		fetchURL, err := expandEnvPlaceholders(source.url)
		if err != nil {
			return resolvedSource{}, err
		}
		ref.Source = "url"
		ref.URL = sanitizeURLForMetadata(source.url)
		return resolvedSource{
			ref:      ref,
			fetchURL: fetchURL,
		}, nil
	case spec.directURL != "":
		fetchURL, err := expandEnvPlaceholders(spec.directURL)
		if err != nil {
			return resolvedSource{}, err
		}
		ref.Source = "builtin"
		ref.URL = sanitizeURLForMetadata(spec.directURL)
		return resolvedSource{
			ref:      ref,
			fetchURL: fetchURL,
		}, nil
	case spec.pageURL != "":
		var (
			resolvedURL string
			err         error
		)
		if source.provider == providerCAIDA && source.artifact == artifactCAIDAPrefix2AS {
			resolvedURL, err = d.resolveCAIDAPrefix2ASURL(spec.pageURL)
		} else {
			resolvedURL, err = d.resolveDBIPArtifactURL(spec.pageURL, source.artifact, source.format)
		}
		if err != nil {
			return resolvedSource{}, err
		}
		ref.Source = "builtin"
		ref.DownloadPage = sanitizeURLForMetadata(spec.pageURL)
		ref.ResolvedURL = sanitizeURLForMetadata(resolvedURL)
		return resolvedSource{
			ref:      ref,
			fetchURL: resolvedURL,
		}, nil
	default:
		return resolvedSource{}, fmt.Errorf(
			"provider/artifact %q/%q has no usable locator",
			source.provider,
			source.artifact,
		)
	}
}

func (d *downloader) resolveDBIPArtifactURL(pageURL, artifact, format string) (string, error) {
	page, err := d.readHTTP(pageURL)
	if err != nil {
		return "", fmt.Errorf("failed to fetch DB-IP landing page %s: %w", pageURL, err)
	}

	ext := format
	if format == formatTSV {
		return "", fmt.Errorf("dbip artifact %q does not support format %q", artifact, format)
	}
	pattern := fmt.Sprintf(
		`https://download\.db-ip\.com/free/dbip-%s-\d{4}-\d{2}\.%s\.gz`,
		regexp.QuoteMeta(strings.ToLower(strings.TrimSpace(artifact))),
		regexp.QuoteMeta(ext),
	)
	re := regexp.MustCompile(pattern)
	match := re.Find(page)
	if len(match) == 0 {
		return "", fmt.Errorf(
			"failed to resolve DB-IP %s/%s download from %s",
			artifact,
			format,
			pageURL,
		)
	}
	return string(match), nil
}

func (d *downloader) resolveCAIDAPrefix2ASURL(logURL string) (string, error) {
	page, err := d.readHTTP(logURL)
	if err != nil {
		return "", fmt.Errorf("failed to fetch CAIDA prefix2as creation log %s: %w", redactURLForDisplay(logURL), err)
	}

	type caidaCandidate struct {
		path      string
		timestamp int64
		hasTime   bool
	}

	candidates := make([]caidaCandidate, 0)
	for _, line := range strings.Split(string(page), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) == 0 {
			continue
		}
		candidate := fields[len(fields)-1]
		if strings.HasSuffix(candidate, ".pfx2as.gz") {
			entry := caidaCandidate{path: candidate}
			if len(fields) >= 2 {
				if timestamp, err := strconv.ParseInt(fields[len(fields)-2], 10, 64); err == nil {
					entry.timestamp = timestamp
					entry.hasTime = true
				}
			}
			candidates = append(candidates, entry)
		}
	}
	if len(candidates) == 0 {
		return "", fmt.Errorf("failed to resolve latest CAIDA prefix2as download from %s", redactURLForDisplay(logURL))
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].hasTime != candidates[j].hasTime {
			return !candidates[i].hasTime
		}
		if candidates[i].timestamp != candidates[j].timestamp {
			return candidates[i].timestamp < candidates[j].timestamp
		}
		return candidates[i].path < candidates[j].path
	})
	latest := candidates[len(candidates)-1].path

	base, err := url.Parse(logURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse CAIDA prefix2as creation log URL %s: %w", redactURLForDisplay(logURL), err)
	}
	base.RawQuery = ""
	base.Fragment = ""
	if !strings.HasSuffix(base.Path, "/") {
		base.Path = path.Dir(base.Path) + "/"
	}
	ref, err := url.Parse(latest)
	if err != nil {
		return "", fmt.Errorf("failed to parse CAIDA prefix2as candidate %q: %w", latest, err)
	}
	return base.ResolveReference(ref).String(), nil
}

func expandEnvPlaceholders(raw string) (string, error) {
	missing := map[string]struct{}{}
	expanded := os.Expand(raw, func(name string) string {
		value, ok := os.LookupEnv(name)
		if !ok || strings.TrimSpace(value) == "" {
			missing[name] = struct{}{}
			return ""
		}
		return value
	})
	if len(missing) == 0 {
		return expanded, nil
	}
	names := make([]string, 0, len(missing))
	for name := range missing {
		names = append(names, name)
	}
	sort.Strings(names)
	return "", fmt.Errorf("missing environment variable(s): %s", strings.Join(names, ", "))
}

func sanitizeURLForMetadata(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return "<redacted>"
	}
	parsed.User = nil
	if parsed.RawQuery != "" {
		parsed.RawQuery = "redacted"
	}
	parsed.Fragment = ""
	return parsed.String()
}

func (d *downloader) readRaw(path, rawURL string) ([]byte, error) {
	if path != "" {
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read %s: %w", path, err)
		}
		return content, nil
	}
	return d.readHTTP(rawURL)
}

func (d *downloader) readHTTP(rawURL string) ([]byte, error) {
	displayURL := redactURLForDisplay(rawURL)
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build request %s: %w", displayURL, err)
	}
	req.Header.Set("User-Agent", d.userAgent)

	start := time.Now()
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch %s: %w", displayURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch %s: unexpected status %d", displayURL, resp.StatusCode)
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", displayURL, err)
	}
	_ = start
	return content, nil
}

func decodePayloadForSource(source sourceEntry, raw []byte) ([]byte, error) {
	switch {
	case source.provider == providerMaxMind && source.artifact == artifactMaxMindGeoLite2ASN:
		return decodeMaxMindASNPayload(raw)
	case source.provider == providerMaxMind && source.artifact == artifactMaxMindGeoLite2Country:
		return raw, nil
	case source.provider == providerIP2Location && source.artifact == artifactIP2LocationCountryLite:
		return raw, nil
	case source.provider == providerIPDeny && source.artifact == artifactIPDenyCountryZones:
		return raw, nil
	case source.provider == providerIPIP && source.artifact == artifactIPIPCountry:
		return raw, nil
	default:
		return decodePayload(raw)
	}
}

func decodeMaxMindASNPayload(raw []byte) ([]byte, error) {
	if len(raw) < 2 || raw[0] != 0x1f || raw[1] != 0x8b {
		return raw, nil
	}
	content, err := decodeGzip(raw)
	if err != nil {
		return nil, err
	}
	mmdb, err := extractMMDBFromTar(content)
	if err == nil {
		return mmdb, nil
	}
	if !errors.Is(err, errTarMMDBNotFound) {
		return nil, fmt.Errorf("failed to extract MaxMind ASN MMDB from tar payload: %w", err)
	}
	return content, nil
}

func decodePayload(raw []byte) ([]byte, error) {
	if len(raw) >= 2 && raw[0] == 0x1f && raw[1] == 0x8b {
		return decodeGzip(raw)
	}
	if len(raw) >= 4 && bytes.Equal(raw[:4], []byte{'P', 'K', 0x03, 0x04}) {
		return decodeZip(raw)
	}
	return raw, nil
}

func decodeGzip(raw []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("failed to open gzip payload: %w", err)
	}
	defer reader.Close()

	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read gzip payload: %w", err)
	}
	return content, nil
}

func decodeZip(raw []byte) ([]byte, error) {
	archive, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil, fmt.Errorf("failed to open zip payload: %w", err)
	}
	for _, f := range archive.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("failed to open zip member %s: %w", f.Name, err)
		}
		defer rc.Close()
		content, err := io.ReadAll(rc)
		if err != nil {
			return nil, fmt.Errorf("failed to read zip member %s: %w", f.Name, err)
		}
		return content, nil
	}
	return nil, fmt.Errorf("zip payload has no regular files")
}

func extractMMDBFromTar(raw []byte) ([]byte, error) {
	tr := tar.NewReader(bytes.NewReader(raw))
	for {
		header, err := tr.Next()
		if err == io.EOF {
			return nil, errTarMMDBNotFound
		}
		if err != nil {
			if !looksLikeTarPayload(raw) {
				return nil, errTarMMDBNotFound
			}
			return nil, err
		}
		if header.Typeflag != tar.TypeReg || !strings.HasSuffix(strings.ToLower(header.Name), ".mmdb") {
			continue
		}
		content, err := io.ReadAll(tr)
		if err != nil {
			return nil, fmt.Errorf("failed to read tar member %s: %w", header.Name, err)
		}
		return content, nil
	}
}

func looksLikeTarPayload(raw []byte) bool {
	return len(raw) >= 512 && bytes.Equal(raw[257:262], []byte("ustar"))
}
