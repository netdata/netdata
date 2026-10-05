// SPDX-License-Identifier: GPL-3.0-or-later

package netdataprovider

import (
	"context"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/framework/functions"
	"github.com/netdata/netdata/otel-facade-poc/internal/control"
	"go.opentelemetry.io/collector/confmap"
)

func TestOneShotWatchAndRetrievalGap(t *testing.T) {
	reader, writer := io.Pipe()
	defer writer.Close()
	c, err := control.New(reader, io.Discard, "127.0.0.1:4317", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p := &provider{controller: c}
	t.Cleanup(func() { _ = p.Shutdown(context.Background()) })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var calls atomic.Int32
	watched := make(chan struct{}, 1)
	first, err := p.Retrieve(ctx, "netdata:local", func(*confmap.ChangeEvent) {
		calls.Add(1)
		watched <- struct{}{}
	})
	if err != nil {
		t.Fatal(err)
	}
	call := func(args []string, payload string) {
		t.Helper()
		if err := c.HandleCall(ctx, functions.Call{UID: "watch", Method: "config", Args: args, HasPayload: payload != "", ContentType: "application/json", Payload: []byte(payload)}); err != nil {
			t.Fatal(err)
		}
	}
	call([]string{"otel-poc:hostmetrics", "add", "host"}, `{}`)
	call([]string{"otel-poc:hostmetrics:host", "enable"}, "")
	select {
	case <-watched:
	case <-ctx.Done():
		t.Fatal("watcher did not observe enable")
	}
	// Mutations cannot block behind another callback while Collector reloads.
	for _, interval := range []string{"3s", "4s", "5s"} {
		call([]string{"otel-poc:hostmetrics:host", "update"}, `{"interval":"`+interval+`"}`)
	}
	if err := first.Close(ctx); err != nil {
		t.Fatal(err)
	}
	call([]string{"otel-poc:hostmetrics:host", "update"}, `{"interval":"6s"}`)
	second, err := p.Retrieve(ctx, "netdata:local", func(*confmap.ChangeEvent) { calls.Add(1) })
	if err != nil {
		t.Fatal(err)
	}
	conf, err := second.AsConf()
	if err != nil {
		t.Fatal(err)
	}
	if got := conf.Get("receivers::host_metrics/hostmetrics_host::collection_interval"); got != "6s" {
		t.Fatalf("retrieval missed latest update: %v", got)
	}
	if err := second.Close(ctx); err != nil {
		t.Fatal(err)
	}
	call([]string{"otel-poc:hostmetrics:host", "disable"}, "")
	if calls.Load() != 1 {
		t.Fatalf("expected exactly one callback, got %d", calls.Load())
	}
}

func TestEOFLatchedAcrossWatchGap(t *testing.T) {
	reader, writer := io.Pipe()
	c, err := control.New(reader, io.Discard, "127.0.0.1:4317", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p := &provider{controller: c}
	t.Cleanup(func() { _ = p.Shutdown(context.Background()) })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	retrieved, err := p.Retrieve(ctx, "netdata:local", func(*confmap.ChangeEvent) {})
	if err != nil {
		t.Fatal(err)
	}
	_, _, terminal, _ := c.Snapshot()
	if err := retrieved.Close(ctx); err != nil {
		t.Fatal(err)
	}
	_ = writer.Close()
	select {
	case <-terminal:
	case <-ctx.Done():
		t.Fatal("EOF not observed")
	}
	if _, err := p.Retrieve(ctx, "netdata:local", func(*confmap.ChangeEvent) {}); err == nil {
		t.Fatal("EOF lost between retrievals")
	}
}
