//go:build cgo
// +build cgo

package as400

// SPDX-License-Identifier: GPL-3.0-or-later

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type dumpContext struct {
	baseDir    string
	queriesDir string
	rowsDir    string
	metricsDir string
	metaDir    string

	mu              sync.Mutex
	seq             int
	metricSeq       int
	metadataWritten bool
}

func newDumpContext(baseDir string, cfg *Config) (*dumpContext, error) {
	dirs := []string{
		filepath.Join(baseDir, "queries"),
		filepath.Join(baseDir, "rows"),
		filepath.Join(baseDir, "metrics"),
		filepath.Join(baseDir, "meta"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("creating dump directory %s: %w", dir, err)
		}
	}

	dc := &dumpContext{
		baseDir:    baseDir,
		queriesDir: dirs[0],
		rowsDir:    dirs[1],
		metricsDir: dirs[2],
		metaDir:    dirs[3],
	}

	if err := dc.writeMetadata(cfg); err != nil {
		return nil, err
	}

	return dc, nil
}

func (d *dumpContext) writeMetadata(cfg *Config) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.metadataWritten {
		return nil
	}
	safeCfg := *cfg
	safeCfg.Password = ""
	payload := struct {
		GeneratedAt time.Time `json:"generated_at"`
		Config      Config    `json:"config"`
	}{
		GeneratedAt: time.Now(),
		Config:      safeCfg,
	}
	path := filepath.Join(d.metaDir, "config.json")
	if err := writePrettyJSON(path, payload); err != nil {
		return err
	}
	d.metadataWritten = true
	return nil
}

func (d *dumpContext) recordQuery(query string, columns []string, rows [][]string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.seq++
	name := fmt.Sprintf("query-%04d", d.seq)
	_ = os.WriteFile(filepath.Join(d.queriesDir, name+".sql"), []byte(query), 0o644)

	rowObjs := make([]map[string]string, 0, len(rows))
	for _, row := range rows {
		obj := make(map[string]string, len(columns))
		for idx, col := range columns {
			if idx < len(row) {
				obj[col] = row[idx]
			} else {
				obj[col] = ""
			}
		}
		rowObjs = append(rowObjs, obj)
	}
	payload := struct {
		Columns []string            `json:"columns"`
		Rows    []map[string]string `json:"rows"`
	}{
		Columns: columns,
		Rows:    rowObjs,
	}
	_ = writePrettyJSON(filepath.Join(d.rowsDir, name+".json"), payload)
}

func (d *dumpContext) recordMetrics(metrics map[string]int64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.metricSeq++
	filename := fmt.Sprintf("metrics-%04d.json", d.metricSeq)
	copyMetrics := make(map[string]int64, len(metrics))
	for k, v := range metrics {
		copyMetrics[k] = v
	}
	payload := struct {
		GeneratedAt time.Time        `json:"generated_at"`
		Metrics     map[string]int64 `json:"metrics"`
	}{
		GeneratedAt: time.Now(),
		Metrics:     copyMetrics,
	}
	_ = writePrettyJSON(filepath.Join(d.metricsDir, filename), payload)
}

func writePrettyJSON(path string, payload any) error {
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
