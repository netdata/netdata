// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

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
)

var (
	errInvalidBootID     = errors.New("invalid boot ID")
	errInvalidMachineID  = errors.New("invalid machine ID")
	errMissingBootID     = errors.New("missing boot ID")
	errMissingMachineID  = errors.New("missing machine ID")
	errNilEntry          = errors.New("nil TrapEntry")
	errMissingJobName    = errors.New("missing job name")
	errMissingSourceIP   = errors.New("missing source IP")
	errNegativeTimestamp = errors.New("negative timestamp")
	errMissingTrapOID    = errors.New("missing trap OID for trap report")
)

var persistentSystemdJournalRoot = "/var/log/journal"

type JournalField = sdkjournal.Field

type JournalConfig struct {
	MaxSize     uint64
	MaxDuration time.Duration
	RotateSize  uint64
	RotateDur   time.Duration
}

type JournalWriter struct {
	mu                  sync.Mutex
	log                 *sdkjournal.Log
	cfg                 JournalConfig
	journalDir          string
	activePath          string
	binaryEncodedFields uint64
}

func journalRoot(jobName string) string {
	// Caller must validate jobName first; it becomes a filesystem path segment.
	return filepath.Join(journalBaseRoot(), jobName)
}

func journalBaseRoot() string {
	return filepath.Join(persistentSystemdJournalRoot, "netdata", "snmp-traps")
}

func validatePersistentJournalRoot() error {
	info, err := os.Stat(persistentSystemdJournalRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("persistent systemd journal directory %s does not exist; enable persistent journald storage before enabling direct SNMP trap journals", persistentSystemdJournalRoot)
		}
		return fmt.Errorf("stat persistent systemd journal directory %s: %w", persistentSystemdJournalRoot, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("persistent systemd journal path %s is not a directory", persistentSystemdJournalRoot)
	}
	return nil
}

func NewJournalWriter(dir string, cfg JournalConfig) (*JournalWriter, error) {
	machineID, err := loadJournalUUID("/etc/machine-id", errMissingMachineID, errInvalidMachineID)
	if err != nil {
		return nil, fmt.Errorf("machine ID: %w", err)
	}
	bootID, err := loadJournalUUID("/proc/sys/kernel/random/boot_id", errMissingBootID, errInvalidBootID)
	if err != nil {
		return nil, fmt.Errorf("boot ID: %w", err)
	}

	logCfg := sdkjournal.LogConfig{
		Source: "snmp-traps",
		Options: sdkjournal.Options{
			MachineID:   machineID,
			BootID:      bootID,
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

	return &JournalWriter{
		log:        log,
		cfg:        cfg,
		journalDir: log.JournalDirectory(),
		activePath: log.ActivePath(),
	}, nil
}

func loadJournalUUID(path string, missingErr, invalidErr error) (sdkjournal.UUID, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return sdkjournal.UUID{}, missingErr
		}
		return sdkjournal.UUID{}, fmt.Errorf("%w: %v", missingErr, err)
	}
	value := strings.TrimSpace(string(content))
	if value == "" {
		return sdkjournal.UUID{}, missingErr
	}
	id, err := sdkjournal.ParseUUID(value)
	if err != nil {
		return sdkjournal.UUID{}, fmt.Errorf("%w: %v", invalidErr, err)
	}
	return id, nil
}

func rotationPolicyFromConfig(cfg JournalConfig) sdkjournal.RotationPolicy {
	policy := sdkjournal.RotationPolicy{}
	if cfg.RotateSize > 0 {
		policy = policy.WithMaxFileSize(cfg.RotateSize)
	}
	if cfg.RotateDur > 0 {
		policy = policy.WithMaxDuration(cfg.RotateDur)
	}
	return policy
}

func retentionPolicyFromConfig(cfg JournalConfig) sdkjournal.RetentionPolicy {
	policy := sdkjournal.RetentionPolicy{}
	if cfg.MaxSize > 0 {
		policy = policy.WithMaxBytes(cfg.MaxSize)
	}
	if cfg.MaxDuration > 0 {
		policy = policy.WithMaxAge(cfg.MaxDuration)
	}
	return policy
}

func (w *JournalWriter) WriteEntry(fields []JournalField, realtimeUsec, monotonicUsec int64) error {
	if realtimeUsec < 0 || monotonicUsec < 0 {
		return errNegativeTimestamp
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if w.log == nil {
		return sdkjournal.ErrWriterClosed
	}
	// The SDK writes length-delimited journal DATA objects. Unsafe values cannot
	// inject fields, but we count them for the self-metrics surface.
	count := binaryEncodedFieldCount(fields)
	err := w.log.Append(fields, sdkjournal.EntryOptions{
		RealtimeUsec:  uint64(realtimeUsec),
		MonotonicUsec: uint64(monotonicUsec),
	})
	if err != nil {
		return err
	}
	if count > 0 {
		atomic.AddUint64(&w.binaryEncodedFields, uint64(count))
	}
	if activePath := w.log.ActivePath(); activePath != "" {
		w.activePath = activePath
	}
	return nil
}

func (w *JournalWriter) WriteRawEntry(payloads [][]byte, binaryEncodedFields int, realtimeUsec, monotonicUsec int64) error {
	if realtimeUsec < 0 || monotonicUsec < 0 {
		return errNegativeTimestamp
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if w.log == nil {
		return sdkjournal.ErrWriterClosed
	}
	err := w.log.AppendRaw(payloads, sdkjournal.EntryOptions{
		RealtimeUsec:  uint64(realtimeUsec),
		MonotonicUsec: uint64(monotonicUsec),
	})
	if err != nil {
		return err
	}
	if binaryEncodedFields > 0 {
		atomic.AddUint64(&w.binaryEncodedFields, uint64(binaryEncodedFields))
	}
	if activePath := w.log.ActivePath(); activePath != "" {
		w.activePath = activePath
	}
	return nil
}

func (w *JournalWriter) BinaryEncodedFields() uint64 {
	return atomic.LoadUint64(&w.binaryEncodedFields)
}

func (w *JournalWriter) SweepRetention() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.log == nil {
		return sdkjournal.ErrWriterClosed
	}
	return w.log.EnforceRetention()
}

func (w *JournalWriter) Sync() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.log == nil {
		return sdkjournal.ErrWriterClosed
	}
	return w.log.Sync()
}

func (w *JournalWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.log == nil {
		return nil
	}
	err := w.log.Close()
	w.log = nil
	return err
}

func (w *JournalWriter) JournalDirectory() string {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.journalDir
}

func (w *JournalWriter) ActivePath() string {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.log != nil {
		w.activePath = w.log.ActivePath()
	}
	return w.activePath
}
