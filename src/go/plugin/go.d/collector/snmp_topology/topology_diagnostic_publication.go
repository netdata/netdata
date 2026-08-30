// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
	"github.com/netdata/netdata/go/plugins/pkg/pluginconfig"
)

const topologyDiagnosticArchiveFailureLogKey = "snmp_topology:diagnostic-archive"

func topologyDiagnosticArchivePath(varLibDir string) string {
	return filepath.Join(varLibDir, "snmp-topology", "diagnostics", "latest.zst")
}

func defaultTopologyDiagnosticArchivePath() string {
	varLibDir := strings.TrimSpace(pluginconfig.VarLibDir())
	if varLibDir == "" {
		varLibDir = strings.TrimSpace(buildinfo.VarLibDir)
	}
	if varLibDir == "" {
		varLibDir = buildinfo.DefaultVarLibDir
	}
	return topologyDiagnosticArchivePath(filepath.Clean(varLibDir))
}

func publishTopologyDiagnosticArchiveFile(path string, diagnostics topologyDiagnostics) error {
	return writeTopologyDiagnosticArchiveFile(path, diagnostics, replaceTopologyDiagnosticArchiveFile)
}

func writeTopologyDiagnosticArchiveFile(
	path string,
	diagnostics topologyDiagnostics,
	replace func(string, string) error,
) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create SNMP topology diagnostic archive directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("secure SNMP topology diagnostic archive directory: %w", err)
	}

	tempPath := path + ".tmp"
	if err := os.Remove(tempPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove stale SNMP topology diagnostic archive temporary file: %w", err)
	}

	file, err := os.OpenFile(tempPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create temporary SNMP topology diagnostic archive: %w", err)
	}
	keepTemp := true
	defer func() {
		if keepTemp {
			_ = os.Remove(tempPath)
		}
	}()

	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return fmt.Errorf("secure temporary SNMP topology diagnostic archive: %w", err)
	}
	if err := writeTopologyDiagnosticArchive(file, diagnostics); err != nil {
		_ = file.Close()
		return fmt.Errorf("write temporary SNMP topology diagnostic archive: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close temporary SNMP topology diagnostic archive: %w", err)
	}
	if err := replace(tempPath, path); err != nil {
		return fmt.Errorf("replace SNMP topology diagnostic archive: %w", err)
	}
	keepTemp = false
	return nil
}

func replaceTopologyDiagnosticArchiveFile(from, to string) error {
	return os.Rename(from, to)
}

func runTopologyDiagnosticArchivePublisher(ctx context.Context, ticks <-chan time.Time, publish func()) {
	if ctx.Err() != nil {
		return
	}
	publish()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticks:
			if ctx.Err() != nil {
				return
			}
			publish()
		}
	}
}

func (c *Collector) diagnosticArchiveEvery() time.Duration {
	return max(c.deviceCheckEvery(), c.refreshEvery())
}

func (c *Collector) runTopologyDiagnosticArchivePublisher(ctx context.Context) {
	ticker := time.NewTicker(c.diagnosticArchiveEvery())
	defer ticker.Stop()
	runTopologyDiagnosticArchivePublisher(ctx, ticker.C, c.publishTopologyDiagnosticArchiveRecovering)
}

func (c *Collector) publishTopologyDiagnosticArchiveRecovering() {
	defer func() {
		if recovered := recover(); recovered != nil {
			c.Limit(topologyDiagnosticArchiveFailureLogKey, 1, topologyRefreshWarningEvery).
				Warningf("failed to publish SNMP topology diagnostic archive: panic: %v", recovered)
		}
	}()
	if c.publishDiagnosticArchiveFile == nil {
		return
	}
	if err := c.publishDiagnosticArchiveFile(c.diagnosticArchivePath, c.acquireTopologyDiagnostics()); err != nil {
		c.Limit(topologyDiagnosticArchiveFailureLogKey, 1, topologyRefreshWarningEvery).
			Warningf("failed to publish SNMP topology diagnostic archive: %v", err)
	}
}
