// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

var engineBootsDirBase string

const (
	maxSnmpEngineBoots = 2147483647
	maxSnmpEngineTime  = uint32(4294967295)
)

func engineBootsDir(jobName string) string {
	return filepath.Join(engineBootsBaseDir(), jobName)
}

func engineBootsBaseDir() string {
	if engineBootsDirBase != "" {
		return engineBootsDirBase
	}
	return filepath.Join(netdataLibDir(), "snmp-trap")
}

func engineBootsPath(jobName string) string {
	return filepath.Join(engineBootsDir(jobName), "engine-boots")
}

func engineStatePathExistsChecked(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		parent := filepath.Dir(path)
		info, parentErr := os.Stat(parent)
		if parentErr != nil {
			if os.IsNotExist(parentErr) {
				return false, nil
			}
			return false, parentErr
		}
		if !info.IsDir() {
			return false, fmt.Errorf("%s is not a directory", parent)
		}
		return false, nil
	}
	return false, err
}

func cleanupCreatedEngineState(jobName string, removeEngineBoots, removeLocalEngineID, removeDir bool) {
	if removeEngineBoots {
		_ = os.Remove(engineBootsPath(jobName))
		_ = os.Remove(engineBootsPath(jobName) + ".tmp")
	}
	if removeLocalEngineID {
		_ = os.Remove(localEngineIDPath(jobName))
		_ = os.Remove(localEngineIDPath(jobName) + ".tmp")
	}
	if removeDir {
		_ = os.Remove(engineBootsDir(jobName))
	}
}

type EngineBoots struct {
	mu        sync.Mutex
	path      string
	value     int64
	startedAt time.Time
	valid     bool
}

func NewEngineBoots(jobName string) (*EngineBoots, error) {
	dir := engineBootsDir(jobName)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("engine-boots: create directory %s: %w", dir, err)
	}

	eb := &EngineBoots{path: engineBootsPath(jobName), startedAt: time.Now()}
	if err := eb.init(); err != nil {
		return nil, err
	}
	return eb, nil
}

func (eb *EngineBoots) init() error {
	data, err := os.ReadFile(eb.path)
	if err != nil {
		if os.IsNotExist(err) {
			eb.value = 1
			eb.valid = true
			return eb.persist()
		}
		return fmt.Errorf("engine-boots: read %s: %w", eb.path, err)
	}

	s := strings.TrimSpace(string(data))
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return fmt.Errorf("engine-boots: parse value in %s: %w", eb.path, err)
	}
	if v < 1 {
		return fmt.Errorf("engine-boots: value %d in %s must be >= 1", v, eb.path)
	}
	if v >= maxSnmpEngineBoots {
		return fmt.Errorf("engine-boots: value %d in %s reached RFC3414 maximum %d", v, eb.path, maxSnmpEngineBoots)
	}

	eb.value = v + 1
	eb.valid = true
	return eb.persist()
}

func (eb *EngineBoots) persist() error {
	return persistEngineStateFile("engine-boots", eb.path, fmt.Appendf(nil, "%d\n", eb.value))
}

func (eb *EngineBoots) Value() int64 {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	return eb.value
}

func (eb *EngineBoots) EngineTime() uint32 {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	return eb.engineTimeLocked()
}

func (eb *EngineBoots) Snapshot() (int64, uint32) {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	return eb.value, eb.engineTimeLocked()
}

func (eb *EngineBoots) engineTimeLocked() uint32 {
	if eb.startedAt.IsZero() {
		return 0
	}
	elapsed := time.Since(eb.startedAt)
	if elapsed <= 0 {
		return 0
	}
	if elapsed.Seconds() > float64(maxSnmpEngineTime) {
		return maxSnmpEngineTime
	}
	return uint32(elapsed / time.Second)
}

func localEngineIDPath(jobName string) string {
	return filepath.Join(engineBootsDir(jobName), "local-engine-id")
}

type LocalEngineID struct {
	mu    sync.Mutex
	path  string
	value []byte
	valid bool
}

func NewLocalEngineID(jobName string, configuredHex string) (*LocalEngineID, error) {
	dir := engineBootsDir(jobName)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("local-engine-id: create directory %s: %w", dir, err)
	}

	lid := &LocalEngineID{path: localEngineIDPath(jobName)}
	if err := lid.init(configuredHex); err != nil {
		return nil, err
	}
	return lid, nil
}

func (lid *LocalEngineID) init(configuredHex string) error {
	configuredHex = strings.TrimSpace(configuredHex)
	if configuredHex != "" {
		raw, err := parseEngineIDHex(configuredHex)
		if err != nil {
			return fmt.Errorf("local-engine-id: configured value: %w", err)
		}
		lid.value = raw
		lid.valid = true
		return lid.persist()
	}

	data, err := os.ReadFile(lid.path)
	if err != nil {
		if os.IsNotExist(err) {
			return lid.generate()
		}
		return fmt.Errorf("local-engine-id: read %s: %w", lid.path, err)
	}

	hexStr := strings.TrimSpace(string(data))
	raw, err := parseEngineIDHex(hexStr)
	if err != nil {
		return fmt.Errorf("local-engine-id: persisted value in %s: %w", lid.path, err)
	}
	lid.value = raw
	lid.valid = true
	return nil
}

func (lid *LocalEngineID) generate() error {
	raw := make([]byte, 12)
	if _, err := rand.Read(raw); err != nil {
		return fmt.Errorf("local-engine-id: generate random bytes: %w", err)
	}
	raw[0] &^= 0x80
	if isAllByte(raw, 0x00) {
		raw[len(raw)-1] = 0x01
	}
	lid.value = raw
	lid.valid = true
	return lid.persist()
}

func isAllByte(b []byte, value byte) bool {
	for _, v := range b {
		if v != value {
			return false
		}
	}
	return len(b) > 0
}

func (lid *LocalEngineID) persist() error {
	hexStr := hex.EncodeToString(lid.value)
	return persistEngineStateFile("local-engine-id", lid.path, []byte(hexStr+"\n"))
}

func persistEngineStateFile(label, path string, data []byte) error {
	tmpPath := path + ".tmp"

	if err := os.WriteFile(tmpPath, data, 0640); err != nil {
		return fmt.Errorf("%s: write %s: %w", label, tmpPath, err)
	}

	f, err := os.OpenFile(tmpPath, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("%s: open temp for fsync %s: %w", label, tmpPath, err)
	}
	syncErr := f.Sync()
	closeErr := f.Close()
	if syncErr != nil {
		return fmt.Errorf("%s: fsync %s: %w", label, tmpPath, syncErr)
	}
	if closeErr != nil {
		return fmt.Errorf("%s: close %s after fsync: %w", label, tmpPath, closeErr)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("%s: rename %s -> %s: %w", label, tmpPath, path, err)
	}
	// Windows does not support opening directories for fsync.
	if runtime.GOOS == "windows" {
		return nil
	}
	dirPath := filepath.Dir(path)
	dir, err := os.Open(dirPath)
	if err != nil {
		return fmt.Errorf("%s: open directory for fsync %s: %w", label, dirPath, err)
	}
	syncErr = dir.Sync()
	closeErr = dir.Close()
	if syncErr != nil {
		return fmt.Errorf("%s: fsync directory %s: %w", label, dirPath, syncErr)
	}
	if closeErr != nil {
		return fmt.Errorf("%s: close directory %s after fsync: %w", label, dirPath, closeErr)
	}
	return nil
}

func (lid *LocalEngineID) Hex() string {
	lid.mu.Lock()
	defer lid.mu.Unlock()
	if !lid.valid {
		return ""
	}
	return hex.EncodeToString(lid.value)
}

func (lid *LocalEngineID) Bytes() []byte {
	lid.mu.Lock()
	defer lid.mu.Unlock()
	if !lid.valid {
		return nil
	}
	out := make([]byte, len(lid.value))
	copy(out, lid.value)
	return out
}

func (lid *LocalEngineID) EqualRaw(raw string) bool {
	// GoSNMP stores authoritative engine IDs as strings containing raw bytes.
	// Compare byte-by-byte without converting either side through hex.
	lid.mu.Lock()
	defer lid.mu.Unlock()
	if !lid.valid || len(lid.value) != len(raw) {
		return false
	}
	for i, v := range lid.value {
		if v != raw[i] {
			return false
		}
	}
	return true
}
