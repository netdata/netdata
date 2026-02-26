// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/filepersister"
)

func statusFileName(dir, pluginName string) string {
	name := strings.ReplaceAll(pluginName, ".", "")
	return filepath.Join(dir, fmt.Sprintf("%s-jobs-statuses.json", name))
}

func (m *Manager) loadFileStatus() {
	m.fileStatus = newFileStatus()

	if !m.runModePolicy.UseFileStatusPersistence || m.varLibDir == "" {
		return
	}

	s, err := loadFileStatus(statusFileName(m.varLibDir, m.pluginName))
	if err != nil {
		m.Warningf("failed to load state file: %v", err)
		return
	}
	m.fileStatus = s
}

func (m *Manager) runFileStatusPersistence() {
	if !m.runModePolicy.UseFileStatusPersistence || m.varLibDir == "" {
		return
	}

	p := filepersister.New(statusFileName(m.varLibDir, m.pluginName))

	p.Run(m.ctx, m.fileStatus)
}

func loadFileStatus(path string) (*fileStatus, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	s := newFileStatus()

	return s, json.NewDecoder(f).Decode(&s.items)
}

func newFileStatus() *fileStatus {
	return &fileStatus{
		items: make(map[string]map[string]string),
		ch:    make(chan struct{}, 1),
	}
}

type fileStatus struct {
	mux   sync.Mutex
	items map[string]map[string]string // [module][name:hash]status
	ch    chan struct{}
}

func (s *fileStatus) Bytes() ([]byte, error) {
	s.mux.Lock()
	defer s.mux.Unlock()

	return json.MarshalIndent(s.items, "", " ")
}

func (s *fileStatus) Updated() <-chan struct{} {
	return s.ch
}

func (s *fileStatus) contains(cfg confgroup.Config, statuses ...string) bool {
	status, ok := s.lookup(cfg)
	if !ok {
		return false
	}

	return slices.Contains(statuses, status)
}

func (s *fileStatus) lookup(cfg confgroup.Config) (string, bool) {
	s.mux.Lock()
	defer s.mux.Unlock()

	jobs, ok := s.items[cfg.Module()]
	if !ok {
		return "", false
	}

	status, ok := jobs[s.jobKey(cfg)]

	return status, ok
}

func (s *fileStatus) add(cfg confgroup.Config, status string) {
	s.mux.Lock()
	defer s.mux.Unlock()

	defer s.setUpdated()

	if s.items == nil {
		s.items = make(map[string]map[string]string)
	}

	if s.items[cfg.Module()] == nil {
		s.items[cfg.Module()] = make(map[string]string)
	}

	s.items[cfg.Module()][s.jobKey(cfg)] = status

	select {
	case s.ch <- struct{}{}:
	default:
	}
}

func (s *fileStatus) remove(cfg confgroup.Config) {
	s.mux.Lock()
	defer s.mux.Unlock()

	defer s.setUpdated()

	delete(s.items[cfg.Module()], s.jobKey(cfg))

	if len(s.items[cfg.Module()]) == 0 {
		delete(s.items, cfg.Module())
	}
}
func (s *fileStatus) setUpdated() {
	select {
	case s.ch <- struct{}{}:
	default:
	}
}

func (s *fileStatus) jobKey(cfg confgroup.Config) string {
	return fmt.Sprintf("%s:%d", cfg.Name(), cfg.Hash())
}
