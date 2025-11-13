// SPDX-License-Identifier: GPL-3.0-or-later

package nagios

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/config"
)

type directoryWatcher struct {
	cfg      config.DirectoryConfig
	log      *logger.Logger
	watcher  *fsnotify.Watcher
	cancel   context.CancelFunc
	debounce time.Duration
	wg       sync.WaitGroup
}

func newDirectoryWatcher(ctx context.Context, log *logger.Logger, cfg config.DirectoryConfig, debounce time.Duration, onChange func()) (*directoryWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	watchCtx, cancel := context.WithCancel(ctx)
	if debounce <= 0 {
		debounce = 250 * time.Millisecond
	}
	dw := &directoryWatcher{
		cfg:      cfg,
		log:      log,
		watcher:  watcher,
		cancel:   cancel,
		debounce: debounce,
	}
	if err := dw.addInitialPaths(); err != nil {
		watcher.Close()
		cancel()
		return nil, err
	}
	dw.wg.Add(1)
	go dw.run(watchCtx, onChange)
	return dw, nil
}

func (dw *directoryWatcher) addInitialPaths() error {
	root := dw.cfg.Path
	if root == "" {
		return errors.New("empty directory path")
	}
	info, err := os.Stat(root)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("directory watcher requires a directory path")
	}
	if !dw.cfg.Recursive {
		return dw.watcher.Add(root)
	}
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		return dw.watcher.Add(path)
	})
}

func (dw *directoryWatcher) run(ctx context.Context, onChange func()) {
	defer dw.wg.Done()
	interval := dw.debounce
	if interval <= 0 {
		interval = 250 * time.Millisecond
	}
	debounce := time.NewTimer(time.Hour)
	debounce.Stop()
	debouncing := false
	trigger := func() {
		debouncing = false
		onChange()
	}
	defer debounce.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-dw.watcher.Events:
			if !ok {
				return
			}
			if dw.shouldTrigger(event) {
				if dw.cfg.Recursive {
					dw.tryWatchNewDir(event)
				}
				if !debouncing {
					debouncing = true
					debounce.Reset(interval)
				} else {
					debounce.Reset(interval)
				}
			}
		case err := <-dw.watcher.Errors:
			if err != nil {
				dw.log.Errorf("nagios directory watcher (%s): %v", dw.cfg.Path, err)
			}
		case <-debounce.C:
			trigger()
		}
	}
}

func (dw *directoryWatcher) shouldTrigger(event fsnotify.Event) bool {
	if event.Has(fsnotify.Chmod) && event.Op^fsnotify.Chmod == 0 {
		return false
	}
	return event.Has(fsnotify.Create) || event.Has(fsnotify.Write) || event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename)
}

func (dw *directoryWatcher) tryWatchNewDir(event fsnotify.Event) {
	if !event.Has(fsnotify.Create) {
		return
	}
	info, err := os.Stat(event.Name)
	if err != nil || !info.IsDir() {
		return
	}
	if err := dw.watcher.Add(event.Name); err != nil {
		dw.log.Debugf("nagios watcher: unable to add %s: %v", event.Name, err)
	}
}

func (dw *directoryWatcher) Close() {
	if dw.cancel != nil {
		dw.cancel()
	}
	_ = dw.watcher.Close()
	dw.wg.Wait()
}
