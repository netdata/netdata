// SPDX-License-Identifier: GPL-3.0-or-later

package healthgen

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Recover removes abandoned RUM outputs before jobs start. It never traverses
// directories or removes symlinks, unrelated files, or another alert namespace.
func Recover(dir string, debug bool) error {
	if debug {
		return nil
	}
	if dir == "" {
		return errors.New("DEM health directory is empty")
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read DEM health recovery directory: %w", err)
	}
	var errs []error
	for _, entry := range entries {
		if !ownedFile.MatchString(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if !info.Mode().IsRegular() {
			continue
		}
		if err := os.Remove(filepath.Join(dir, entry.Name())); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Recovery separates synchronous file recovery from the process-owned reload
// retry. Prepare must be called before job admission; Run never deletes files.
type Recovery struct {
	effects
	dir string
}

func NewRecovery(dir string, opts Options) *Recovery {
	return &Recovery{
		effects: newEffects(opts),
		dir:     dir,
	}
}

func (r *Recovery) Prepare() error { return Recover(r.dir, r.debug) }

func (r *Recovery) Run(ctx context.Context) {
	if r.debug {
		<-ctx.Done()
		return
	}
	// Always reload, even if Prepare found no files: a previous process may
	// have died after deleting files but before asking the Agent to reload.
	for ctx.Err() == nil {
		if err := r.reloadBounded(ctx); err != nil {
			r.report(err)
			if !waitRetry(ctx, r.retry) {
				return
			}
			continue
		}
		<-ctx.Done()
		return
	}
}
