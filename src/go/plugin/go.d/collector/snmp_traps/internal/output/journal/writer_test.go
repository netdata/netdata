// SPDX-License-Identifier: GPL-3.0-or-later

package journal

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/journaltest"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/model"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/output"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriterConcurrentWriteCloseDoesNotPanic(t *testing.T) {
	for range 50 {
		writer := newWriter(nil, Options{QueueCapacity: 2})
		require.NoError(t, writer.Start())
		start := make(chan struct{})
		done := make(chan struct{})

		var wg sync.WaitGroup
		for range 8 {
			wg.Go(func() {
				<-start
				for {
					err := writer.Write(&model.TrapEntry{JobName: "local", Message: "trap"})
					if errors.Is(err, output.ErrClosed) {
						return
					}
					if err != nil && !errors.Is(err, output.ErrQueueFull) {
						return
					}
				}
			})
		}

		close(start)
		time.Sleep(time.Millisecond)
		require.NoError(t, writer.Close())

		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("writer goroutines did not stop after Close")
		}
	}
}

func TestWriterPreparedLifecycle(t *testing.T) {
	writer := newWriter(nil, Options{QueueCapacity: 1})
	require.ErrorIs(t, writer.Write(&model.TrapEntry{}), output.ErrNotStarted)
	require.ErrorIs(t, writer.Flush(), output.ErrNotStarted)
	require.NoError(t, writer.Close())
	require.ErrorIs(t, writer.Start(), output.ErrClosed)
}

func TestRetentionSweepInterval(t *testing.T) {
	tests := map[string]struct {
		cfg  Config
		want time.Duration
	}{
		"no retention": {
			cfg:  Config{},
			want: 0,
		},
		"size only": {
			cfg:  Config{MaxSize: DefaultMaxSize},
			want: maxRetentionSweepInterval,
		},
		"duration below minimum": {
			cfg:  Config{MaxDuration: time.Second},
			want: minRetentionSweepInterval,
		},
		"duration capped": {
			cfg:  Config{MaxDuration: 4 * maxRetentionSweepInterval},
			want: maxRetentionSweepInterval,
		},
		"rotation duration caps size only": {
			cfg:  Config{MaxSize: DefaultMaxSize, RotateDur: 5 * time.Minute},
			want: 5 * time.Minute,
		},
	}

	assert.Zero(t, journalRetentionSweepInterval(nil))
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, journalRetentionSweepInterval(&sdkWriter{cfg: tc.cfg}))
		})
	}
}

func TestWriterTickerFlushesWithoutCountTrigger(t *testing.T) {
	journaltest.RequireJournalctl(t)

	sdk, err := newTestSDKWriter(t.TempDir(), Config{RotateSize: 200 * bytesPerMB})
	require.NoError(t, err)
	writer := newWriter(sdk, Options{QueueCapacity: 1 << 10})
	writer.flushInterval = 100 * time.Millisecond
	require.NoError(t, writer.Start())

	const want = 5
	for range want {
		require.NoError(t, writer.Write(testTrapEntry()))
	}

	journalDir := writer.Directory()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var count int
	for ctx.Err() == nil {
		count = journalctlRowCount(ctx, journalDir, "TRAP_CATEGORY=security", "TRAP_OID")
		if count >= want {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	require.GreaterOrEqualf(t, count, want, "ticker did not flush %d entries without an explicit Flush", want)
	require.NoError(t, writer.Close())

	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()
	require.GreaterOrEqual(t, journalctlRowCount(ctx2, journalDir, "TRAP_CATEGORY=security", "TRAP_OID"), want)
}

func testTrapEntry() *model.TrapEntry {
	return &model.TrapEntry{
		JobName:               "local",
		ReportType:            model.ReportTypeTrap,
		ReceivedRealtimeUsec:  time.Now().UnixMicro(),
		ReceivedMonotonicUsec: 1000,
		TrapOID:               "1.3.6.1.6.3.1.1.5.1",
		Category:              "security",
		Severity:              "warning",
		Message:               "trap",
		SourceIP:              "192.0.2.1",
		SourceUDPPeer:         "192.0.2.1:162",
		PduType:               model.PduTypeTrap,
		SnmpVersion:           model.SnmpVersionV2c,
	}
}

func journalctlRowCount(ctx context.Context, dir, match, field string) int {
	out, _ := exec.CommandContext(ctx, "journalctl", "--directory="+dir, match, "-o", "json", "--no-pager").CombinedOutput()
	return strings.Count(string(out), field)
}
