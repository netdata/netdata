// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/netdata/netdata/go/plugins/logger"
)

const (
	profileWatcherRefreshEvery = time.Minute
	profileWatcherEventSettle  = 200 * time.Millisecond
)

type profileWatcher struct {
	log          *logger.Logger
	dirs         []string
	refreshEvery time.Duration
	eventSettle  time.Duration
	newWatcher   func() (*fsnotify.Watcher, error)
	reload       func() error
	markDirty    func()
	fingerprint  func([]string) (string, error)

	watcher         *fsnotify.Watcher
	watches         map[string]struct{}
	cancel          context.CancelFunc
	done            chan struct{}
	lastFingerprint string
}

func newUserProfileWatcher(dirs []string) *profileWatcher {
	return &profileWatcher{
		log: logger.With(
			slog.String("collector", "snmp_traps"),
			slog.String("component", "profile_watcher"),
		),
		dirs:         cleanProfileWatcherDirs(dirs),
		refreshEvery: profileWatcherRefreshEvery,
		eventSettle:  profileWatcherEventSettle,
		newWatcher:   fsnotify.NewWatcher,
		reload:       ReloadUserProfileCache,
		markDirty:    MarkProfileCacheDirty,
		fingerprint:  fingerprintUserProfileFiles,
	}
}

func cleanProfileWatcherDirs(dirs []string) []string {
	seen := make(map[string]struct{}, len(dirs))
	clean := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		dir = filepath.Clean(dir)
		if _, ok := seen[dir]; ok {
			continue
		}
		seen[dir] = struct{}{}
		clean = append(clean, dir)
	}
	return clean
}

func (w *profileWatcher) Start() error {
	if len(w.dirs) == 0 {
		return nil
	}
	fsw, err := w.newWatcher()
	if err != nil {
		w.log.Warningf("SNMP trap user profile watcher initialization failed; falling back to periodic profile scans: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	w.done = make(chan struct{})
	w.watcher = fsw
	w.watches = make(map[string]struct{})

	go w.run(ctx)
	return nil
}

func (w *profileWatcher) Stop() {
	if w.cancel == nil {
		return
	}
	w.cancel()
	if w.done != nil {
		<-w.done
	}
	w.cancel = nil
	w.done = nil
}

func (w *profileWatcher) run(ctx context.Context) {
	defer close(w.done)
	if w.watcher != nil {
		defer w.watcher.Close()
	}

	var events <-chan fsnotify.Event
	var watchErrors <-chan error
	if w.watcher != nil {
		events = w.watcher.Events
		watchErrors = w.watcher.Errors
	}

	w.addWatches()
	if fp, err := w.fingerprint(w.dirs); err != nil {
		w.handleFingerprintFailure(err)
	} else {
		w.lastFingerprint = fp
	}

	ticker := time.NewTicker(w.refreshEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.refresh(ctx)
		case event, ok := <-events:
			if !ok {
				return
			}
			if !w.shouldRefreshForEvent(event) {
				continue
			}
			w.forgetRemovedWatch(event)
			sleepWithContext(ctx, w.eventSettle)
			w.refresh(ctx)
		case err, ok := <-watchErrors:
			if !ok {
				return
			}
			if err != nil {
				w.log.Warningf("SNMP trap user profile watcher error: %v", err)
			}
		}
	}
}

func (w *profileWatcher) refresh(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	default:
	}

	w.addWatches()
	fp, err := w.fingerprint(w.dirs)
	if err != nil {
		w.handleFingerprintFailure(err)
		return
	}
	if fp == w.lastFingerprint {
		return
	}
	w.markDirty()

	if err := w.reload(); err != nil {
		if !errors.Is(err, errNoActiveProfileJobs) {
			w.log.Warningf("SNMP trap user profile reload failed: %v", err)
		}
		return
	}
	w.lastFingerprint = fp
	w.log.Infof("SNMP trap user profiles reloaded")
}

func (w *profileWatcher) handleFingerprintFailure(err error) {
	w.log.Warningf("SNMP trap user profile fingerprint failed: %v", err)
	w.markDirty()
	if reloadErr := w.reload(); reloadErr != nil && !errors.Is(reloadErr, errNoActiveProfileJobs) {
		w.log.Warningf("SNMP trap user profile reload failed: %v", reloadErr)
	}
}

func (w *profileWatcher) addWatches() {
	if w.watcher == nil {
		return
	}

	for _, dir := range w.dirs {
		_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				if errors.Is(err, os.ErrNotExist) || errors.Is(err, os.ErrPermission) {
					return nil
				}
				w.log.Warningf("SNMP trap user profile watcher scan '%s': %v", path, err)
				return nil
			}
			if d.IsDir() {
				w.addWatch(path)
			}
			return nil
		})
	}
}

func (w *profileWatcher) addWatch(path string) {
	if path == "" || path == "." {
		return
	}
	path = filepath.Clean(path)
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		delete(w.watches, path)
		return
	}
	if _, ok := w.watches[path]; ok {
		return
	}
	if err := w.watcher.Add(path); err != nil {
		w.log.Warningf("SNMP trap user profile watcher cannot watch '%s': %v", path, err)
		delete(w.watches, path)
		return
	}
	w.watches[path] = struct{}{}
}

func (w *profileWatcher) forgetRemovedWatch(event fsnotify.Event) {
	if !event.Has(fsnotify.Remove) && !event.Has(fsnotify.Rename) {
		return
	}
	delete(w.watches, filepath.Clean(event.Name))
}

func (w *profileWatcher) shouldRefreshForEvent(event fsnotify.Event) bool {
	if event.Name == "" {
		return false
	}
	if !event.Has(fsnotify.Create) &&
		!event.Has(fsnotify.Write) &&
		!event.Has(fsnotify.Remove) &&
		!event.Has(fsnotify.Rename) &&
		!event.Has(fsnotify.Chmod) {
		return false
	}

	path := filepath.Clean(event.Name)
	for _, dir := range w.dirs {
		if path == dir || strings.HasPrefix(path, dir+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}

func fingerprintUserProfileFiles(dirs []string) (string, error) {
	type entry struct {
		path    string
		size    int64
		modTime int64
		mode    fs.FileMode
	}

	var entries []entry
	for _, dir := range cleanProfileWatcherDirs(dirs) {
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				if path == dir && errors.Is(err, os.ErrNotExist) {
					return nil
				}
				return err
			}
			if d.IsDir() || !isProfileFileName(d.Name()) {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					return nil
				}
				return err
			}
			if info.IsDir() {
				return nil
			}
			entries = append(entries, entry{
				path:    filepath.Clean(path),
				size:    info.Size(),
				modTime: info.ModTime().UnixNano(),
				mode:    info.Mode(),
			})
			return nil
		})
		if err != nil {
			return "", err
		}
	}

	slices.SortFunc(entries, func(a, b entry) int {
		return strings.Compare(a.path, b.path)
	})

	h := sha256.New()
	for _, e := range entries {
		_, _ = fmt.Fprintf(h, "%s\x00%d\x00%d\x00%s\x00", e.path, e.size, e.modTime, e.mode.String())
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func sleepWithContext(ctx context.Context, d time.Duration) {
	if d <= 0 {
		return
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}
