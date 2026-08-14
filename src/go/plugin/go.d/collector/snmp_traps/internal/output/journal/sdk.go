// SPDX-License-Identifier: GPL-3.0-or-later

package journal

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	sdkjournal "github.com/netdata/systemd-journal-sdk/go/journal"

	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/hostidentity"
)

var (
	errMissingJournalHost = errors.New("missing journal host provider")
	errNilEntry           = errors.New("nil trap entry")
	errMissingJobName     = errors.New("missing job name")
	errMissingSourceIP    = errors.New("missing source IP")
	errNegativeTimestamp  = errors.New("negative timestamp")
	errMissingTrapOID     = errors.New("missing trap OID for trap report")
)

const (
	netdataLogDirEnv     = "NETDATA_LOG_DIR"
	defaultNetdataLogDir = "/var/log/netdata"
)

type Config struct {
	MaxSize     uint64
	MaxDuration time.Duration
	RotateSize  uint64
	RotateDur   time.Duration
}

type sdkWriter struct {
	mu                  sync.Mutex
	log                 *sdkjournal.Log
	host                hostidentity.Provider
	binaryEncodedFields atomic.Uint64
}

func Root(jobName string) string {
	// Caller must validate jobName first; it becomes a filesystem path segment.
	return filepath.Join(BaseRoot(), jobName)
}

func BaseRoot() string {
	return filepath.Join(netdataLogDir(), "traps")
}

func netdataLogDir() string {
	if dir := strings.TrimSpace(os.Getenv(netdataLogDirEnv)); dir != "" {
		return filepath.Clean(dir)
	}
	if dir := strings.TrimSpace(buildinfo.LogDir); dir != "" {
		return filepath.Clean(dir)
	}
	return defaultNetdataLogDir
}

func ValidateLogRoot() error {
	logDir := netdataLogDir()
	info, err := os.Stat(logDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("Netdata log directory %s does not exist; direct SNMP trap journals require a usable Netdata log directory", logDir)
		}
		return fmt.Errorf("stat Netdata log directory %s: %w", logDir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("Netdata log path %s is not a directory", logDir)
	}
	return nil
}

func newSDKWriter(dir string, cfg Config, host hostidentity.Provider) (*sdkWriter, error) {
	if host == nil {
		return nil, errMissingJournalHost
	}

	logCfg := sdkjournal.LogConfig{
		Source: "snmp-traps",
		Options: sdkjournal.Options{
			MachineID:   host.MachineID(),
			BootID:      host.BootID(),
			Compact:     true,
			Compression: sdkjournal.CompressionNone,
			Seal:        nil,
		},
		RotationPolicy:  rotationPolicyFromConfig(cfg),
		RetentionPolicy: retentionPolicyFromConfig(cfg),
		OpenMode:        sdkjournal.LogOpenEager,
		IdentityMode:    sdkjournal.LogIdentityStrict,
	}

	log, err := sdkjournal.NewLog(dir, logCfg)
	if err != nil {
		return nil, fmt.Errorf("open journal log %s: %w", dir, err)
	}

	return &sdkWriter{
		log:  log,
		host: host,
	}, nil
}

func rotationPolicyFromConfig(cfg Config) sdkjournal.RotationPolicy {
	policy := sdkjournal.RotationPolicy{}
	if cfg.RotateSize > 0 {
		policy = policy.WithMaxFileSize(cfg.RotateSize)
	}
	if cfg.RotateDur > 0 {
		policy = policy.WithMaxDuration(cfg.RotateDur)
	}
	return policy
}

func retentionPolicyFromConfig(cfg Config) sdkjournal.RetentionPolicy {
	policy := sdkjournal.RetentionPolicy{}
	if cfg.MaxSize > 0 {
		policy = policy.WithMaxBytes(cfg.MaxSize)
	}
	if cfg.MaxDuration > 0 {
		policy = policy.WithMaxAge(cfg.MaxDuration)
	}
	return policy
}

func (w *sdkWriter) writeRaw(payloads [][]byte, binaryEncodedFields int, realtimeUsec, monotonicUsec int64) error {
	if realtimeUsec < 0 || monotonicUsec < 0 {
		return errNegativeTimestamp
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if w.log == nil {
		return sdkjournal.ErrWriterClosed
	}
	err := w.log.AppendRaw(payloads, sdkjournal.EntryOptions{
		RealtimeUsec:     uint64(realtimeUsec),
		RealtimeUsecSet:  true,
		MonotonicUsec:    uint64(monotonicUsec),
		MonotonicUsecSet: true,
		BootID:           w.host.BootID(),
	})
	if err != nil {
		return err
	}
	if binaryEncodedFields > 0 {
		w.binaryEncodedFields.Add(uint64(binaryEncodedFields))
	}
	return nil
}

func (w *sdkWriter) binaryFieldCount() uint64 {
	return w.binaryEncodedFields.Load()
}

func (w *sdkWriter) sweepRetention() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.log == nil {
		return sdkjournal.ErrWriterClosed
	}
	return w.log.EnforceRetention()
}

func (w *sdkWriter) sync() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.log == nil {
		return sdkjournal.ErrWriterClosed
	}
	return w.log.Sync()
}

func (w *sdkWriter) close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.log == nil {
		return nil
	}
	err := w.log.Close()
	w.log = nil
	return err
}
