// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

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
	payload, err := decodePayload(raw)
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
		ref.Source = "path"
		ref.Path = source.path
		return resolvedSource{
			ref:       ref,
			fetchPath: source.path,
		}, nil
	case source.url != "":
		ref.Source = "url"
		ref.URL = source.url
		return resolvedSource{
			ref:      ref,
			fetchURL: source.url,
		}, nil
	case spec.directURL != "":
		ref.Source = "builtin"
		ref.URL = spec.directURL
		return resolvedSource{
			ref:      ref,
			fetchURL: spec.directURL,
		}, nil
	case spec.pageURL != "":
		resolvedURL, err := d.resolveDBIPArtifactURL(spec.pageURL, source.artifact, source.format)
		if err != nil {
			return resolvedSource{}, err
		}
		ref.Source = "builtin"
		ref.DownloadPage = spec.pageURL
		ref.ResolvedURL = resolvedURL
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
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build request %s: %w", rawURL, err)
	}
	req.Header.Set("User-Agent", d.userAgent)

	start := time.Now()
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch %s: %w", rawURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch %s: unexpected status %d", rawURL, resp.StatusCode)
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", rawURL, err)
	}
	_ = start
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
