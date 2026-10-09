// SPDX-License-Identifier: GPL-3.0-or-later

package collector

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/netdata/netdata/go/plugins/plugin/framework/filelock"
	"github.com/netdata/netdata/go/plugins/plugin/java/protocol"
)

type attempt struct {
	Process protocol.Process `json:"process"`
	Token   string           `json:"token"`
	Status  string           `json:"status"`
	Detail  string           `json:"detail"`
}

type persistentState struct {
	Version  int                `json:"version"`
	BootID   string             `json:"boot_id"`
	Port     uint16             `json:"port"`
	Attempts map[string]attempt `json:"attempts"`
}

type journal struct {
	dir   string
	lock  *filelock.Locker
	state persistentState
}

func openJournal(dir, bootID string) (*journal, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	// Sync the parent even when another candidate created this directory.
	if err := syncDirectory(filepath.Dir(dir)); err != nil {
		return nil, err
	}
	j := &journal{dir: dir, lock: filelock.New(dir)}
	locked, err := j.lock.Lock("java")
	if err != nil {
		return nil, err
	}
	if !locked {
		return nil, fmt.Errorf("another java.plugin owns the attachment state")
	}
	ok := false
	defer func() {
		if !ok {
			j.close()
		}
	}()
	f, err := os.Open(filepath.Join(dir, "state.json"))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if err == nil {
		defer f.Close()
		decoder := json.NewDecoder(io.LimitReader(f, 16<<20))
		if err := decoder.Decode(&j.state); err != nil {
			return nil, fmt.Errorf("read Java attachment state: %w", err)
		}
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			return nil, fmt.Errorf("trailing Java attachment state")
		}
		if err := j.state.validate(); err != nil {
			return nil, err
		}
	}
	if j.state.BootID != bootID {
		j.state = persistentState{Version: 1, BootID: bootID, Attempts: make(map[string]attempt)}
	}
	ok = true
	return j, nil
}

func (s persistentState) validate() error {
	if s.Version != 1 || s.BootID == "" || s.Port == 0 || s.Attempts == nil {
		return fmt.Errorf("invalid Java attachment state")
	}
	tokens := make(map[string]bool)
	for key, a := range s.Attempts {
		token, err := hex.DecodeString(a.Token)
		if err != nil || len(token) != 32 || tokens[a.Token] || key != a.Process.Instance() || a.Process.BootID != s.BootID || a.Process.Application == "" || a.Process.PID <= 0 || a.Process.StartTime == 0 {
			return fmt.Errorf("invalid Java attachment identity or credential")
		}
		if a.Status != protocol.Unknown && a.Status != protocol.Attached && a.Status != protocol.Blocked {
			return fmt.Errorf("invalid Java attachment outcome")
		}
		tokens[a.Token] = true
	}
	return nil
}

func (j *journal) save() error {
	if err := j.state.validate(); err != nil {
		return err
	}
	data, err := json.Marshal(j.state)
	if err != nil {
		return err
	}
	if len(data) >= 16<<20 {
		return fmt.Errorf("Java attachment state exceeds 16 MiB")
	}
	f, err := os.CreateTemp(j.dir, ".state-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err := os.Rename(f.Name(), filepath.Join(j.dir, "state.json")); err != nil {
		return err
	}
	return syncDirectory(j.dir)
}

func (j *journal) close() { j.lock.UnlockAll() }

func syncDirectory(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
