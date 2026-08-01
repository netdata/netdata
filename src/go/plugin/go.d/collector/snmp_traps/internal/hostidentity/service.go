// SPDX-License-Identifier: GPL-3.0-or-later

package hostidentity

import (
	"fmt"
	"sync"

	sdkjournal "github.com/netdata/systemd-journal-sdk/go/journal"
	"github.com/netdata/systemd-journal-sdk/go/journalhost"
)

type Provider interface {
	MachineID() sdkjournal.UUID
	BootID() sdkjournal.UUID
	MonotonicUsec() uint64
}

type LoadConfig struct {
	StateDir             string
	HostFilesystemPrefix string
}

type ConfigSource func() LoadConfig

type Loader func(LoadConfig) (Provider, error)

type Service struct {
	configSource ConfigSource
	loader       Loader

	cached struct {
		once     sync.Once
		provider Provider
		err      error
	}
}

func New(configSource ConfigSource) *Service {
	return NewWithLoader(configSource, load)
}

func NewWithLoader(configSource ConfigSource, loader Loader) *Service {
	if configSource == nil {
		panic("hostidentity: nil config source")
	}
	if loader == nil {
		panic("hostidentity: nil loader")
	}
	return &Service{configSource: configSource, loader: loader}
}

func (s *Service) FreshJournal() (Provider, error) {
	if s == nil {
		return nil, fmt.Errorf("hostidentity: nil service")
	}
	cfg := s.configSource()
	provider, err := s.loader(cfg)
	if err != nil {
		return nil, fmt.Errorf("load local journal host identity using state directory %s: %w", cfg.StateDir, err)
	}
	return provider, nil
}

func (s *Service) CachedFallback() (Provider, error) {
	if s == nil {
		return nil, fmt.Errorf("hostidentity: nil service")
	}
	s.cached.once.Do(func() {
		s.cached.provider, s.cached.err = s.loader(s.configSource())
	})
	return s.cached.provider, s.cached.err
}

func load(cfg LoadConfig) (Provider, error) {
	return journalhost.Load(journalhost.LoadOptions{
		StateDir:             cfg.StateDir,
		HostFilesystemPrefix: cfg.HostFilesystemPrefix,
	})
}
