// SPDX-License-Identifier: GPL-3.0-or-later

package snmpsd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

func (d *Discoverer) loadFileStatus() {
	d.status = newDiscoveryStatus()

	filename := statusFileName()
	if filename == "" {
		return
	}

	f, err := os.Open(filename)
	if err != nil {
		d.Warningf("failed to open status file %s: %v", filename, err)
		return
	}
	defer func() { _ = f.Close() }()

	if err := json.NewDecoder(f).Decode(d.status); err != nil {
		d.Warningf("failed to parse status file %s: %v", filename, err)
		return
	}

	d.Infof("loaded status file: last discovery=%s", d.status.LastDiscoveryTime)
}

func statusFileName() string {
	v := os.Getenv("NETDATA_LIB_DIR")
	if v == "" {
		return ""
	}
	return filepath.Join(v, "god-sd-snmp-status.json")
}

func newDiscoveryStatus() *discoveryStatus {
	return &discoveryStatus{
		Networks: make(map[string]map[string]*discoveredDevice),
	}
}

type (
	discoveryStatus struct {
		updated           atomic.Bool
		mux               sync.RWMutex
		Networks          map[string]map[string]*discoveredDevice `json:"networks"`
		LastDiscoveryTime time.Time                               `json:"last_discovery_time"`
		ConfigHash        uint64                                  `json:"config_hash"`
	}
	discoveredDevice struct {
		DiscoverTime time.Time `json:"discover_time"`
		SysInfo      SysInfo   `json:"sysinfo"`
	}
)

func (s *discoveryStatus) Bytes() ([]byte, error) {
	s.mux.RLock()
	defer s.mux.RUnlock()

	return json.MarshalIndent(s, "", " ")
}

func (s *discoveryStatus) get(sub subnet, ip string) *discoveredDevice {
	s.mux.RLock()
	defer s.mux.RUnlock()

	devices, ok := s.Networks[subKey(sub)]
	if !ok {
		return nil
	}
	return devices[ip]
}

func (s *discoveryStatus) put(sub subnet, ip string, dev *discoveredDevice) {
	s.mux.Lock()
	defer s.mux.Unlock()

	devices, ok := s.Networks[subKey(sub)]
	if !ok {
		devices = make(map[string]*discoveredDevice)
		s.Networks[subKey(sub)] = devices
	}
	devices[ip] = dev
}

func (s *discoveryStatus) del(sub subnet, ip string) {
	s.mux.Lock()
	defer s.mux.Unlock()

	if devices, ok := s.Networks[subKey(sub)]; ok {
		delete(devices, ip)
	}
}

func subKey(s subnet) string {
	return fmt.Sprintf("%s:%s", s.str, s.credential.Name)
}
