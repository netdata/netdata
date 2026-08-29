// SPDX-License-Identifier: GPL-3.0-or-later

package redfishsd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"time"
	"unicode/utf8"
)

const (
	statusVersion  = 1
	maxStatusBytes = 4 << 20
)

type discoveryStatus struct {
	Version    int                    `json:"version"`
	ConfigHash string                 `json:"config_hash"`
	Endpoints  map[string]statusEntry `json:"endpoints"`
}

type statusEntry struct {
	ValidatedAt time.Time `json:"validated_at"`
}

func newDiscoveryStatus(configHash string) *discoveryStatus {
	return &discoveryStatus{
		Version: statusVersion, ConfigHash: configHash,
		Endpoints: make(map[string]statusEntry),
	}
}

func statusFileName(cacheKey string) string {
	root := os.Getenv("NETDATA_LIB_DIR")
	if root == "" || len(cacheKey) < 32 {
		return ""
	}
	return filepath.Join(root, "god-sd-redfish-"+cacheKey[:32]+".json")
}

func loadStatus(filename, configHash string) (*discoveryStatus, error) {
	if filename == "" {
		return newDiscoveryStatus(configHash), nil
	}
	info, err := os.Lstat(filename)
	if errors.Is(err, os.ErrNotExist) {
		return newDiscoveryStatus(configHash), nil
	}
	if err != nil {
		return newDiscoveryStatus(configHash), err
	}
	if err := validateStatusFileInfo(info); err != nil {
		return newDiscoveryStatus(configHash), err
	}
	file, err := os.Open(filename)
	if err != nil {
		return newDiscoveryStatus(configHash), err
	}
	defer func() { _ = file.Close() }()
	openedInfo, err := file.Stat()
	if err != nil {
		return newDiscoveryStatus(configHash), errors.New("Redfish discovery status changed during validation")
	}
	if err := validateOpenedStatusFile(info, openedInfo); err != nil {
		return newDiscoveryStatus(configHash), err
	}
	var status discoveryStatus
	decoder := json.NewDecoder(io.LimitReader(file, maxStatusBytes+1))
	if err := decoder.Decode(&status); err != nil {
		return newDiscoveryStatus(configHash), err
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return newDiscoveryStatus(configHash), err
	}
	if status.Version != statusVersion || status.ConfigHash != configHash {
		return newDiscoveryStatus(configHash), nil
	}
	if status.Endpoints == nil {
		status.Endpoints = make(map[string]statusEntry)
	}
	return &status, nil
}

func validateOpenedStatusFile(pathInfo, openedInfo os.FileInfo) error {
	if pathInfo == nil || openedInfo == nil || !os.SameFile(pathInfo, openedInfo) {
		return errors.New("Redfish discovery status changed during validation")
	}
	return validateStatusFileInfo(openedInfo)
}

func validateStatusFileInfo(info os.FileInfo) error {
	if info == nil || !info.Mode().IsRegular() {
		return errors.New("Redfish discovery status is not a regular file")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("Redfish discovery status permissions %04o are too broad", info.Mode().Perm())
	}
	if info.Size() <= 0 || info.Size() > maxStatusBytes {
		return fmt.Errorf("Redfish discovery status size must be between 1 and %d bytes", maxStatusBytes)
	}
	return nil
}

func saveStatus(filename string, status *discoveryStatus) error {
	if filename == "" || status == nil {
		return nil
	}
	payload, err := marshalStatus(status)
	if err != nil {
		return err
	}
	if len(payload) > maxStatusBytes {
		return fmt.Errorf("Redfish discovery status exceeds %d bytes", maxStatusBytes)
	}
	dir := filepath.Dir(filename)
	temp, err := os.CreateTemp(dir, ".god-sd-redfish-status-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	remove := true
	defer func() {
		_ = temp.Close()
		if remove {
			_ = os.Remove(tempName)
		}
	}()
	if err := temp.Chmod(0o600); err != nil {
		return err
	}
	if _, err := temp.Write(payload); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempName, filename); err != nil {
		return err
	}
	remove = false
	if runtime.GOOS == "windows" {
		return nil
	}
	directory, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer func() { _ = directory.Close() }()
	return directory.Sync()
}

func marshalStatus(status *discoveryStatus) ([]byte, error) {
	const suffix = "}}\n"
	prefix := strconv.AppendInt([]byte("{\"version\":"), int64(status.Version), 10)
	prefix = append(prefix, ",\"config_hash\":"...)
	configHashSize := jsonStringEncodedSize(status.ConfigHash)
	if configHashSize > maxStatusBytes-len(prefix)-len(",\"endpoints\":{")-len(suffix) {
		return nil, statusTooLargeError()
	}
	encodedConfigHash, err := json.Marshal(status.ConfigHash)
	if err != nil {
		return nil, err
	}
	prefix = append(prefix, encodedConfigHash...)
	prefix = append(prefix, ",\"endpoints\":{"...)

	type encodedEntry struct {
		key   string
		name  []byte
		value []byte
	}
	entries := make([]encodedEntry, 0, min(len(status.Endpoints), 1_024))
	total := len(prefix) + len(suffix)
	for key, entry := range status.Endpoints {
		encodedValue, err := json.Marshal(entry)
		if err != nil {
			return nil, err
		}
		separator := 0
		if len(entries) > 0 {
			separator = 1
		}
		keySize := jsonStringEncodedSize(key)
		entrySize := separator + keySize + 1 + len(encodedValue)
		if entrySize > maxStatusBytes-total {
			return nil, statusTooLargeError()
		}
		encodedKey, err := json.Marshal(key)
		if err != nil {
			return nil, err
		}
		if len(encodedKey) != keySize {
			return nil, errors.New("Redfish discovery status JSON string size mismatch")
		}
		entries = append(entries, encodedEntry{key: key, name: encodedKey, value: encodedValue})
		total += entrySize
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].key < entries[j].key })

	var buffer bytes.Buffer
	buffer.Grow(total)
	buffer.Write(prefix)
	for index, entry := range entries {
		if index > 0 {
			buffer.WriteByte(',')
		}
		buffer.Write(entry.name)
		buffer.WriteByte(':')
		buffer.Write(entry.value)
	}
	buffer.WriteString(suffix)
	return buffer.Bytes(), nil
}

func statusTooLargeError() error {
	return fmt.Errorf("Redfish discovery status exceeds %d bytes", maxStatusBytes)
}

func jsonStringEncodedSize(value string) int {
	size := 2 // Opening and closing quotes.
	for index := 0; index < len(value); {
		current := value[index]
		if current < utf8.RuneSelf {
			index++
			switch current {
			case '\\', '"', '\b', '\f', '\n', '\r', '\t':
				size += 2
			case '<', '>', '&':
				size += 6
			default:
				if current < 0x20 {
					size += 6
				} else {
					size++
				}
			}
			continue
		}
		r, width := utf8.DecodeRuneInString(value[index:])
		index += width
		switch {
		case (r == utf8.RuneError && width == 1) || r == '\u2028' || r == '\u2029':
			size += 6
		default:
			size += width
		}
	}
	return size
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("trailing JSON value")
		}
		return err
	}
	return nil
}
