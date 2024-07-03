// SPDX-License-Identifier: GPL-3.0-or-later

package file

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/confgroup"

	"github.com/fsnotify/fsnotify"
)

type (
	Watcher struct {
		*logger.Logger

		paths        []string
		reg          confgroup.Registry
		watcher      *fsnotify.Watcher
		cache        cache
		refreshEvery time.Duration
	}
	cache map[string]time.Time
)

func (c cache) lookup(path string) (time.Time, bool) { v, ok := c[path]; return v, ok }
func (c cache) has(path string) bool                 { _, ok := c.lookup(path); return ok }
func (c cache) remove(path string)                   { delete(c, path) }
func (c cache) put(path string, modTime time.Time)   { c[path] = modTime }

func NewWatcher(reg confgroup.Registry, paths []string) *Watcher {
	d := &Watcher{
		Logger:       log,
		paths:        paths,
		reg:          reg,
		watcher:      nil,
		cache:        make(cache),
		refreshEvery: time.Minute,
	}
	return d
}

func (w *Watcher) String() string {
	return w.Name()
}

func (w *Watcher) Name() string {
	return "file watcher"
}

func (w *Watcher) Run(ctx context.Context, in chan<- []*confgroup.Group) {
	w.Info("instance is started")
	defer func() { w.Info("instance is stopped") }()

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		w.Errorf("fsnotify watcher initialization: %v", err)
		return
	}

	w.watcher = watcher
	defer w.stop()
	w.refresh(ctx, in)

	tk := time.NewTicker(w.refreshEvery)
	defer tk.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-tk.C:
			w.refresh(ctx, in)
		case event := <-w.watcher.Events:
			// TODO: check if event.Has will do
			if event.Name == "" || isChmodOnly(event) || !w.fileMatches(event.Name) {
				break
			}
			if event.Has(fsnotify.Create) && w.cache.has(event.Name) {
				// vim "backupcopy=no" case, already collected after Rename event.
				break
			}
			if event.Has(fsnotify.Rename) {
				// It is common to modify files using vim.
				// When writing to a file a backup is made. "backupcopy" option tells how it's done.
				// Default is "no": rename the file and write a new one.
				// This is cheap attempt to not send empty group for the old file.
				time.Sleep(time.Millisecond * 100)
			}
			w.refresh(ctx, in)
		case err := <-w.watcher.Errors:
			if err != nil {
				w.Warningf("watch: %v", err)
			}
		}
	}
}

func (w *Watcher) fileMatches(file string) bool {
	for _, pattern := range w.paths {
		if ok, _ := filepath.Match(pattern, file); ok {
			return true
		}
	}
	return false
}

func (w *Watcher) listFiles() (files []string) {
	for _, pattern := range w.paths {
		if matches, err := filepath.Glob(pattern); err == nil {
			files = append(files, matches...)
		}
	}
	return files
}

func (w *Watcher) refresh(ctx context.Context, in chan<- []*confgroup.Group) {
	select {
	case <-ctx.Done():
		return
	default:
	}
	var groups []*confgroup.Group
	seen := make(map[string]bool)

	for _, file := range w.listFiles() {
		fi, err := os.Lstat(file)
		if err != nil {
			w.Warningf("lstat '%s': %v", file, err)
			continue
		}

		if !fi.Mode().IsRegular() {
			continue
		}

		seen[file] = true
		if v, ok := w.cache.lookup(file); ok && v.Equal(fi.ModTime()) {
			continue
		}
		w.cache.put(file, fi.ModTime())

		if group, err := parse(w.reg, file); err != nil {
			w.Warningf("parse '%s': %v", file, err)
		} else if group == nil {
			groups = append(groups, &confgroup.Group{Source: file})
		} else {
			for _, cfg := range group.Configs {
				cfg.SetProvider("file watcher")
				cfg.SetSourceType(configSourceType(file))
				cfg.SetSource(fmt.Sprintf("discoverer=file_watcher,file=%s", file))
			}
			groups = append(groups, group)
		}
	}

	for name := range w.cache {
		if seen[name] {
			continue
		}
		w.cache.remove(name)
		groups = append(groups, &confgroup.Group{Source: name})
	}

	send(ctx, in, groups)

	w.watchDirs()
}

func (w *Watcher) watchDirs() {
	for _, path := range w.paths {
		if idx := strings.LastIndex(path, "/"); idx > -1 {
			path = path[:idx]
		} else {
			path = "./"
		}
		if err := w.watcher.Add(path); err != nil {
			w.Errorf("start watching '%s': %v", path, err)
		}
	}
}

func (w *Watcher) stop() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// closing the watcher deadlocks unless all events and errors are drained.
	go func() {
		for {
			select {
			case <-w.watcher.Errors:
			case <-w.watcher.Events:
			case <-ctx.Done():
				return
			}
		}
	}()

	_ = w.watcher.Close()
}

func isChmodOnly(event fsnotify.Event) bool {
	return event.Op^fsnotify.Chmod == 0
}

func send(ctx context.Context, in chan<- []*confgroup.Group, groups []*confgroup.Group) {
	if len(groups) == 0 {
		return
	}
	select {
	case <-ctx.Done():
	case in <- groups:
	}
}
