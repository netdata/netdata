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

func (d *downloader) readDataset(spec datasetSpec) ([]byte, error) {
	raw, err := d.readRaw(spec)
	if err != nil {
		return nil, err
	}
	compression := strings.ToLower(strings.TrimSpace(spec.compression))
	if compression == "" {
		compression = compressionAuto
	}
	return decodePayload(raw, compression)
}

func (d *downloader) readRaw(spec datasetSpec) ([]byte, error) {
	if path := strings.TrimSpace(spec.path); path != "" {
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read %s: %w", path, err)
		}
		return content, nil
	}

	url := strings.TrimSpace(spec.url)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build request %s: %w", url, err)
	}
	req.Header.Set("User-Agent", d.userAgent)

	start := time.Now()
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch %s: unexpected status %d", url, resp.StatusCode)
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", url, err)
	}
	_ = start
	return content, nil
}

func decodePayload(raw []byte, compression string) ([]byte, error) {
	switch compression {
	case compressionNone:
		return raw, nil
	case compressionGzip:
		return decodeGzip(raw)
	case compressionZip:
		return decodeZip(raw)
	case compressionAuto:
		if len(raw) >= 2 && raw[0] == 0x1f && raw[1] == 0x8b {
			return decodeGzip(raw)
		}
		if len(raw) >= 4 && bytes.Equal(raw[:4], []byte{'P', 'K', 0x03, 0x04}) {
			return decodeZip(raw)
		}
		return raw, nil
	default:
		return nil, fmt.Errorf("unsupported compression mode %q", compression)
	}
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
