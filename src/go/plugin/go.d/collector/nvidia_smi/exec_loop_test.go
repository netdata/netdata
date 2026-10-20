// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !windows

package nvidia_smi

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/streamexec/streamexectest"
)

// TestMain doubles as the fake nvidia-smi; see streamexectest.
func TestMain(m *testing.M) {
	streamexectest.RunIfFake()
	os.Exit(m.Run())
}

// newLoopCollector returns a loop-mode collector whose nvidia-smi is the test binary, started through a pass-through
// nd-run. Init is skipped: it requires a root-owned binary.
func newLoopCollector(t *testing.T, updateEvery int, timeout time.Duration) (*Collector, *streamexectest.Fake) {
	t.Helper()
	fake := streamexectest.NewFake(t)
	collr := New()
	collr.UpdateEvery = updateEvery
	collr.Timeout = confopt.Duration(timeout)
	exec, err := newNvidiaSmiLoopExec(fake.Binary, collr.Config, nil)
	require.NoError(t, err)
	collr.exec = exec
	t.Cleanup(func() { collr.Cleanup(context.Background()) })
	return collr, fake
}

// goCheck runs Check in the background, since it waits for the first sample.
func goCheck(collr *Collector) <-chan error {
	done := make(chan error, 1)
	go func() { done <- collr.Check(context.Background()) }()
	return done
}

func waitCheck(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("Check did not return")
		return nil
	}
}

// sendSample makes the fake print data as nvidia-smi's loop does: every line terminated, including the last.
func sendSample(t *testing.T, conn *streamexectest.Conn, data []byte) {
	t.Helper()
	conn.Raw(t, strings.TrimSuffix(string(data), "\n")+"\n")
}

// collectFromMock returns what a collector reading data through the mock exec collects.
func collectFromMock(t *testing.T, data []byte) map[string]int64 {
	t.Helper()
	collr := New()
	collr.exec = &mockNvidiaSmi{
		gpuInfo: data,
	}
	mx := collr.Collect(context.Background())
	require.NotEmpty(t, mx)
	return mx
}

func TestLoopModeCollects(t *testing.T) {
	collr, fake := newLoopCollector(t, 10, 10*time.Second)
	done := goCheck(collr)
	conn := fake.Accept(t)
	assert.Equal(t, []string{"-q", "-x", "-l", "5"}, conn.Args)

	sendSample(t, conn, dataXMLRTX3060) // the XML header and doctype precede the sample document
	require.NoError(t, waitCheck(t, done))
	assert.Equal(t, collectFromMock(t, dataXMLRTX3060), collr.Collect(context.Background()))

	collr.Cleanup(context.Background())
	conn.RequireExited(t) // nvidia-smi is gone once Cleanup returns
}

func TestLoopModeCheckFailures(t *testing.T) {
	for name, tc := range map[string]struct {
		timeout time.Duration
		act     func(*testing.T, *streamexectest.Conn)
		wantErr string
		minWait time.Duration // Check fails no sooner than this
		maxWait time.Duration // and sooner than this
	}{
		"nvidia-smi exits before a sample": {
			timeout: 10 * time.Second,
			act:     func(t *testing.T, conn *streamexectest.Conn) { conn.Exit(t, 9) },
			wantErr: "nvidia-smi exited: exit status 9",
			maxWait: 3 * time.Second, // long before the timeout
		},
		"no sample within the timeout": {
			timeout: time.Second,
			act:     func(*testing.T, *streamexectest.Conn) {},
			wantErr: "wait for the first nvidia-smi record: context deadline exceeded",
			minWait: time.Second,
			maxWait: 1500 * time.Millisecond,
		},
	} {
		t.Run(name, func(t *testing.T) {
			collr, fake := newLoopCollector(t, 10, tc.timeout)
			began := time.Now()
			done := goCheck(collr)
			tc.act(t, fake.Accept(t))

			require.EqualError(t, waitCheck(t, done), tc.wantErr)
			elapsed := time.Since(began)
			assert.GreaterOrEqual(t, elapsed, tc.minWait)
			assert.Less(t, elapsed, tc.maxWait, "a failure waits for nothing else")
			fake.RequireNoStart(t, 300*time.Millisecond)
		})
	}
}

func TestLoopModeLateSampleIsAGapAndRestarts(t *testing.T) {
	// A sample stays current for one loop interval (1s) plus the timeout (0.5s).
	collr, fake := newLoopCollector(t, 1, 500*time.Millisecond)
	done := goCheck(collr)
	conn := fake.Accept(t)
	assert.Equal(t, []string{"-q", "-x", "-l", "1"}, conn.Args)
	sendSample(t, conn, dataXMLRTX3060)
	require.NoError(t, waitCheck(t, done))
	require.NotEmpty(t, collr.Collect(context.Background()))

	// nvidia-smi keeps running but sends nothing more: the sample is not republished, and nvidia-smi is replaced.
	require.Eventually(t, func() bool { return collr.Collect(context.Background()) == nil }, 3*time.Second,
		20*time.Millisecond)
	replacement := fake.Accept(t)
	conn.RequireExited(t)

	sendSample(t, replacement, dataXMLTeslaP100)
	require.Eventually(t, func() bool { return collr.Collect(context.Background()) != nil }, 3*time.Second,
		20*time.Millisecond)
	assert.Equal(t, collectFromMock(t, dataXMLTeslaP100), collr.Collect(context.Background()))
}

func TestLoopModeSlowSampleStaysCurrent(t *testing.T) {
	// A query may take up to the timeout: a sample arriving 1.5s after the previous one, within the loop interval (1s)
	// plus the timeout (1s), keeps the charts current and nvidia-smi running.
	collr, fake := newLoopCollector(t, 1, time.Second)
	done := goCheck(collr)
	conn := fake.Accept(t)
	sendSample(t, conn, dataXMLRTX3060)
	require.NoError(t, waitCheck(t, done))

	for range 2 {
		deadline := time.Now().Add(1500 * time.Millisecond)
		for time.Now().Before(deadline) {
			require.NotNil(t, collr.Collect(context.Background()), "a slow but timely sample left a gap")
			time.Sleep(50 * time.Millisecond)
		}
		sendSample(t, conn, dataXMLRTX3060)
	}
	fake.RequireNoStart(t, 100*time.Millisecond)
}

func TestLoopTiming(t *testing.T) {
	// The loop interval is the data collection interval, at most 5 seconds; a sample stays current for one loop
	// interval plus the timeout.
	for name, tc := range map[string]struct {
		updateEvery  int
		timeout      time.Duration
		wantInterval int
		wantFreshFor time.Duration
	}{
		"fast collection":     {updateEvery: 1, timeout: 500 * time.Millisecond, wantInterval: 1, wantFreshFor: 1500 * time.Millisecond},
		"interval at the cap": {updateEvery: 5, timeout: 10 * time.Second, wantInterval: 5, wantFreshFor: 15 * time.Second},
		"default collection":  {updateEvery: 10, timeout: 10 * time.Second, wantInterval: 5, wantFreshFor: 15 * time.Second},
		"slow collection":     {updateEvery: 60, timeout: 2 * time.Second, wantInterval: 5, wantFreshFor: 7 * time.Second},
	} {
		t.Run(name, func(t *testing.T) {
			interval, freshFor := loopTiming(tc.updateEvery, tc.timeout)
			assert.Equal(t, tc.wantInterval, interval)
			assert.Equal(t, tc.wantFreshFor, freshFor)
		})
	}
}

func TestLogDecoder(t *testing.T) {
	dec := &logDecoder{}
	var got []string
	for _, line := range []string{
		`<?xml version="1.0" ?>`,
		"</nvidia_smi_log>", // an end tag outside a document is not a sample
		"<nvidia_smi_log>",
		"    <timestamp>1</timestamp>",
		"</nvidia_smi_log>",
		"",
		"<nvidia_smi_log>",
		"    <timestamp>partial</timestamp>",
		"<nvidia_smi_log>", // a new document discards an unfinished one
		"    <timestamp>2</timestamp>",
		"</nvidia_smi_log>",
	} {
		if sample, ok := dec.Decode([]byte(line)); ok {
			got = append(got, string(sample))
		}
	}
	assert.Equal(t, []string{
		"<nvidia_smi_log>\n    <timestamp>1</timestamp>\n</nvidia_smi_log>",
		"<nvidia_smi_log>\n    <timestamp>2</timestamp>\n</nvidia_smi_log>",
	}, got)
}
