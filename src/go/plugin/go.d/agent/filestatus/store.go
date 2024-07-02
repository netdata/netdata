// SPDX-License-Identifier: GPL-3.0-or-later

package filestatus

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/confgroup"
)

func LoadStore(path string) (*Store, error) {
	var s Store

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	return &s, json.NewDecoder(f).Decode(&s.items)
}

type Store struct {
	mux   sync.Mutex
	items map[string]map[string]string // [module][name:hash]status
}

func (s *Store) Contains(cfg confgroup.Config, statuses ...string) bool {
	status, ok := s.lookup(cfg)
	if !ok {
		return false
	}

	return slices.Contains(statuses, status)
}

func (s *Store) lookup(cfg confgroup.Config) (string, bool) {
	s.mux.Lock()
	defer s.mux.Unlock()

	jobs, ok := s.items[cfg.Module()]
	if !ok {
		return "", false
	}

	status, ok := jobs[storeJobKey(cfg)]

	return status, ok
}

func (s *Store) add(cfg confgroup.Config, status string) {
	s.mux.Lock()
	defer s.mux.Unlock()

	if s.items == nil {
		s.items = make(map[string]map[string]string)
	}

	if s.items[cfg.Module()] == nil {
		s.items[cfg.Module()] = make(map[string]string)
	}

	s.items[cfg.Module()][storeJobKey(cfg)] = status
}

func (s *Store) remove(cfg confgroup.Config) {
	s.mux.Lock()
	defer s.mux.Unlock()

	delete(s.items[cfg.Module()], storeJobKey(cfg))

	if len(s.items[cfg.Module()]) == 0 {
		delete(s.items, cfg.Module())
	}
}

func (s *Store) bytes() ([]byte, error) {
	s.mux.Lock()
	defer s.mux.Unlock()

	return json.MarshalIndent(s.items, "", " ")
}

func storeJobKey(cfg confgroup.Config) string {
	return fmt.Sprintf("%s:%d", cfg.Name(), cfg.Hash())
}
