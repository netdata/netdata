// SPDX-License-Identifier: GPL-3.0-or-later

// Package healthgen owns disposable, per-site health configuration below DEM's
// exclusive state directory. The installer exposes this directory to health.d.
package healthgen

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
	"github.com/netdata/netdata/go/plugins/plugin/dem/config"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

const (
	reloadTimeout = 3 * time.Second
	retryInterval = 10 * time.Second
)

var siteKey = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)
var ownedFile = regexp.MustCompile(`^rum\.[A-Za-z0-9][A-Za-z0-9_-]*\.conf$`)

// Options supplies side effects and presentation policy. Nil Reload uses the
// installed netdatacli; tests can replace it without contacting an Agent.
type Options struct {
	Debug   bool
	Reload  func(context.Context) error
	OnError func(error)
	Redact  func(string) string
}

type effects struct {
	debug   bool
	reload  func(context.Context) error
	onError func(error)
	retry   time.Duration
}

func newEffects(opts Options) effects {
	reload := opts.Reload
	if reload == nil {
		reload = reloadHealth
	}
	return effects{
		debug:   opts.Debug,
		reload:  reload,
		onError: opts.OnError,
		retry:   retryInterval,
	}
}

func (e effects) report(err error) {
	if err != nil && e.onError != nil {
		e.onError(err)
	}
}

func (e effects) reloadBounded(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, reloadTimeout)
	defer cancel()
	if err := e.reload(ctx); err != nil {
		return fmt.Errorf("reload DEM health configuration: %w", err)
	}
	return nil
}

func reloadHealth(ctx context.Context) error {
	cmd := ndexec.UnprivilegedCommandContextWithPreservedEnv(
		ctx,
		filepath.Join(buildinfo.NetdataBinDir, "netdatacli"),
		"reload-health",
	)
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	return cmd.Run()
}

// Site owns one exact output file for the lifetime of an active site job.
// Construct during validation; Run performs mutations after job admission.
type Site struct {
	effects
	path    string
	content []byte
	enabled bool
}

// NewSite snapshots the policy and metadata; subsequent caller changes do not
// alter this generation. dir must be DEM's exclusive generated-health directory.
func NewSite(dir string, site config.RumSite, opts Options) (*Site, error) {
	// Supported filesystems limit a filename to 255 bytes; rum. + .conf uses nine.
	if len(site.Key) > 246 {
		return nil, errors.New("RUM site key exceeds the 246-byte generated health filename limit")
	}
	if !siteKey.MatchString(site.Key) {
		return nil, fmt.Errorf("invalid RUM site key %q", site.Key)
	}
	if dir == "" && !opts.Debug {
		return nil, errors.New("DEM health directory is empty")
	}
	content, err := renderSite(site, opts.Redact)
	if err != nil {
		return nil, err
	}
	return &Site{
		effects: newEffects(opts),
		path:    filepath.Join(dir, "rum."+site.Key+".conf"),
		content: content,
		enabled: site.Alerts.IsEnabled(),
	}, nil
}

// Run retries failed writes/reloads without blocking ingestion. It must finish
// before a replacement generation may own the same site file.
func (s *Site) Run(ctx context.Context) {
	if s.debug {
		<-ctx.Done()
		return
	}
	defer func() {
		s.report(removeFile(s.path))
		// The job context is cancelled; retirement gets its own fixed budget.
		s.report(s.reloadBounded(context.Background()))
	}()
	for ctx.Err() == nil {
		err := s.sync()
		if err == nil {
			err = s.reloadBounded(ctx)
		}
		if err == nil {
			<-ctx.Done()
			return
		}
		s.report(err)
		if !waitRetry(ctx, s.retry) {
			return
		}
	}
}

func (s *Site) sync() error {
	if !s.enabled {
		return removeFile(s.path)
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0750); err != nil {
		return fmt.Errorf("create DEM health directory: %w", err)
	}
	// Never follow an unexpected symlink when comparing existing output.
	if info, err := os.Lstat(s.path); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("DEM health output is not a regular file: %s", s.path)
		}
		if current, err := os.ReadFile(s.path); err == nil && string(current) == string(s.content) {
			return nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(s.path), ".rum-*")
	if err != nil {
		return fmt.Errorf("create DEM health temporary file: %w", err)
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err := f.Chmod(0640); err != nil {
		return err
	}
	if _, err := f.Write(s.content); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), s.path)
}

func removeFile(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("DEM health output is not a regular file: %s", path)
	}
	return os.Remove(path)
}

func waitRetry(ctx context.Context, interval time.Duration) bool {
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
