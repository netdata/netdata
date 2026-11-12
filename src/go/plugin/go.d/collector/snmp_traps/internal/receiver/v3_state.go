// SPDX-License-Identifier: GPL-3.0-or-later

package receiver

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/netdata/netdata/go/plugins/pkg/safefile"
)

const maxSnmpEngineBoots = 2147483647

type engineStatePaths struct {
	dir           string
	engineBoots   string
	localEngineID string
}

func newEngineStatePaths(root, jobName string) engineStatePaths {
	dir := filepath.Join(root, jobName)
	return engineStatePaths{
		dir:           dir,
		engineBoots:   filepath.Join(dir, "engine-boots"),
		localEngineID: filepath.Join(dir, "local-engine-id"),
	}
}

func engineStatePathExistsChecked(path string) (bool, error) {
	info, err := os.Lstat(path)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return false, fmt.Errorf("%s is a symbolic link", path)
		}
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
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

func cleanupCreatedEngineState(paths engineStatePaths, removeEngineBoots, removeLocalEngineID, removeDir bool) {
	if removeEngineBoots {
		_ = os.Remove(paths.engineBoots)
	}
	if removeLocalEngineID {
		_ = os.Remove(paths.localEngineID)
	}
	if removeDir {
		_ = os.Remove(paths.dir)
	}
}

// engineBoots is the job's persisted snmpEngineBoots, raised at every start
// (RFC 3414 section 2.2.2); the receiver counts the engine time.
type engineBoots struct {
	mu    sync.Mutex
	path  string
	value int64
	valid bool
}

func newEngineBoots(paths engineStatePaths) (*engineBoots, error) {
	if err := safefile.EnsureDir(paths.dir, safefile.Mode{
		Perm: 0750,
	}); err != nil {
		return nil, fmt.Errorf("engine-boots: create directory %s: %w", paths.dir, err)
	}

	eb := &engineBoots{
		path: paths.engineBoots,
	}
	if err := eb.init(); err != nil {
		return nil, err
	}
	return eb, nil
}

func (eb *engineBoots) init() error {
	data, err := safefile.ReadNoFollow(eb.path, safefile.MaxSize)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			eb.value = 1
			eb.valid = true
			return eb.persist()
		}
		return fmt.Errorf("engine-boots: read %s: %w", eb.path, err)
	}

	s := strings.TrimSpace(string(data))
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return fmt.Errorf("engine-boots: invalid value in %s", eb.path)
	}
	if v < 1 {
		return fmt.Errorf("engine-boots: invalid value in %s: must be >= 1", eb.path)
	}
	if v >= maxSnmpEngineBoots {
		return fmt.Errorf("engine-boots: invalid value in %s: reached RFC3414 maximum", eb.path)
	}

	eb.value = v + 1
	eb.valid = true
	return eb.persist()
}

func (eb *engineBoots) persist() error {
	return persistEngineStateFile("engine-boots", eb.path, fmt.Appendf(nil, "%d\n", eb.value))
}

func (eb *engineBoots) bootsValue() int64 {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	return eb.value
}

type localEngineID struct {
	mu    sync.Mutex
	path  string
	value []byte
	valid bool
}

func newLocalEngineID(paths engineStatePaths, configuredHex string) (*localEngineID, error) {
	if err := safefile.EnsureDir(paths.dir, safefile.Mode{
		Perm: 0750,
	}); err != nil {
		return nil, fmt.Errorf("local-engine-id: create directory %s: %w", paths.dir, err)
	}

	lid := &localEngineID{
		path: paths.localEngineID,
	}
	if err := lid.init(configuredHex); err != nil {
		return nil, err
	}
	return lid, nil
}

func (lid *localEngineID) init(configuredHex string) error {
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

	data, err := safefile.ReadNoFollow(lid.path, safefile.MaxSize)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return lid.generate()
		}
		return fmt.Errorf("local-engine-id: read %s: %w", lid.path, err)
	}

	hexStr := strings.TrimSpace(string(data))
	raw, err := parseEngineIDHex(hexStr)
	if err != nil {
		return fmt.Errorf("local-engine-id: invalid value in %s", lid.path)
	}
	lid.value = raw
	lid.valid = true
	return nil
}

func (lid *localEngineID) generate() error {
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

func (lid *localEngineID) persist() error {
	hexStr := hex.EncodeToString(lid.value)
	return persistEngineStateFile("local-engine-id", lid.path, []byte(hexStr+"\n"))
}

func persistEngineStateFile(label, path string, data []byte) error {
	if err := safefile.Replace(path, data, safefile.Mode{
		Perm: 0640,
	}); err != nil {
		return fmt.Errorf("%s: persist %s: %w", label, path, err)
	}
	if err := safefile.SyncDir(filepath.Dir(path)); err != nil {
		return fmt.Errorf("%s: sync directory: %w", label, err)
	}
	return nil
}

func (lid *localEngineID) hexString() string {
	lid.mu.Lock()
	defer lid.mu.Unlock()
	if !lid.valid {
		return ""
	}
	return hex.EncodeToString(lid.value)
}

func (lid *localEngineID) bytes() []byte {
	lid.mu.Lock()
	defer lid.mu.Unlock()
	if !lid.valid {
		return nil
	}
	out := make([]byte, len(lid.value))
	copy(out, lid.value)
	return out
}
