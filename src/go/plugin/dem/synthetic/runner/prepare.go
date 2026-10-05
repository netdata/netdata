// SPDX-License-Identifier: GPL-3.0-or-later
package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

var packagePins = map[string]string{
	"@playwright/test": "1.63.0",
	"playwright":       "1.63.0",
	"playwright-core":  "1.63.0",
	"lighthouse":       "13.4.1",
	"chrome-launcher":  "1.2.1",
}

func runtimeAllowed() error {
	if runtime.GOOS != "linux" || (runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64") {
		return errors.New("DEM browser execution requires prepared Linux amd64 or arm64 runtime")
	}
	if os.Geteuid() == 0 {
		return errors.New(
			"DEM browser execution requires the plugin to run as an unprivileged user; root RUM operation does not enable browser jobs",
		)
	}
	return nil
}

func (e *Engine) checkFiles(kind synthetic.Kind) error {
	if kind != synthetic.Journey && kind != synthetic.Lighthouse {
		return errors.New("unsupported synthetic kind")
	}
	for name, value := range map[string]string{"node_path": e.config.NodePath, "dependencies_path": e.config.DependenciesPath, "browser_path": e.config.BrowserPath, "assets_path": e.config.AssetsPath} {
		if !filepath.IsAbs(value) {
			return fmt.Errorf("prepared %s must be an absolute path", name)
		}
	}
	for name, value := range map[string]string{"node": e.config.NodePath, "browser": e.config.BrowserPath} {
		info, err := os.Stat(value)
		if err != nil {
			return fmt.Errorf("prepared %s: %w", name, err)
		}
		if !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
			return fmt.Errorf("prepared %s is not an executable regular file", name)
		}
	}
	for _, name := range []string{"run.mjs", "reporter.cjs", "protocol.cjs", "lighthouse.mjs"} {
		f, err := os.Open(filepath.Join(e.config.AssetsPath, name))
		if err != nil {
			return fmt.Errorf("prepared runner asset %s: %w", name, err)
		}
		info, statErr := f.Stat()
		err = errors.Join(statErr, f.Close())
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("prepared runner asset %s is not regular", name)
		}
	}
	for name, pin := range packagePins {
		raw, err := os.ReadFile(filepath.Join(e.config.DependenciesPath, "node_modules", name, "package.json"))
		if err != nil {
			return fmt.Errorf("prepared %s package: %w", name, err)
		}
		var p struct {
			Version string `json:"version"`
		}
		if err = json.Unmarshal(raw, &p); err != nil {
			return fmt.Errorf("prepared %s package metadata: %w", name, err)
		}
		if p.Version != pin {
			return fmt.Errorf("prepared %s must be version %s", name, pin)
		}
	}
	return nil
}

// Check verifies the prepared runtime using bounded supervised version probes.
// It never loads the configured workflow or launches a browser session.
func (e *Engine) Check(ctx context.Context, kind synthetic.Kind) error {
	if e.isPoisoned() {
		return ndexec.ErrTreeNotDrained
	}
	if err := e.platform(); err != nil {
		return err
	}
	if err := e.checkFiles(kind); err != nil {
		return err
	}
	checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	node, err := e.probe(
		checkCtx,
		[]string{
			"-e",
			"process.stdout.write(JSON.stringify({version:process.versions.node,uid:process.getuid(),gid:process.getgid()}))",
		},
	)
	if err != nil {
		return fmt.Errorf("prepared Node check: %w", err)
	}
	var actual struct {
		Version string `json:"version"`
		UID     int    `json:"uid"`
		GID     int    `json:"gid"`
	}
	if err = json.Unmarshal(node, &actual); err != nil {
		return fmt.Errorf("invalid prepared Node version response")
	}
	var major int
	if _, err = fmt.Sscanf(actual.Version, "%d.", &major); err != nil || major < 24 {
		return errors.New("prepared Node must be version 24 or newer")
	}
	if actual.UID == 0 || actual.UID != os.Geteuid() || actual.GID != os.Getegid() {
		return errors.New("nd-run must preserve the plugin's unprivileged identity for private run directories")
	}
	// execFileSync is confined to a version query. It runs under the same tree
	// supervisor; cancellation still drains any descendant it creates.
	script := "require('node:child_process').execFileSync(process.argv[1],['--version'],{stdio:['ignore',1,2]})"
	browser, err := e.probe(checkCtx, []string{"-e", script, e.config.BrowserPath})
	if err != nil {
		return fmt.Errorf("prepared Chromium check: %w", err)
	}
	if !regexp.MustCompile(`\b153\.0\.8010\.12\b`).Match(browser) {
		return errors.New("prepared full Chromium must be version 153.0.8010.12 (Playwright revision 1243)")
	}
	return nil
}

func (e *Engine) probe(ctx context.Context, args []string) ([]byte, error) {
	var output []byte
	var parseErr error
	var diagnostic string
	result, err := e.executeProcess(ctx, args, nil, func(reader io.Reader) error {
		output, parseErr = io.ReadAll(io.LimitReader(reader, 4097))
		if len(output) > 4096 {
			parseErr = errors.New("runtime version response exceeds 4096 bytes")
		}
		return parseErr
	}, func(reader io.Reader) {
		raw, _ := io.ReadAll(io.LimitReader(reader, 2001))
		diagnostic = clip(string(raw))
		_, _ = io.Copy(io.Discard, reader)
	})
	if err != nil {
		return nil, err
	}
	if !result.Drained {
		return nil, ndexec.ErrTreeNotDrained
	}
	if parseErr != nil {
		return nil, parseErr
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if result.Err != nil {
		return nil, fmt.Errorf("version probe failed: %w (%s)", result.Err, diagnostic)
	}
	return output, nil
}
