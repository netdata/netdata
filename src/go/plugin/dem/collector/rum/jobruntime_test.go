// SPDX-License-Identifier: GPL-3.0-or-later
package rum_test

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/receiver"
	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/rum"
	"github.com/netdata/netdata/go/plugins/plugin/dem/runtimehub"
	"github.com/netdata/netdata/go/plugins/plugin/dem/store"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/jobruntime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type output struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (o *output) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.buf.Write(p)
}
func (o *output) String() string { o.mu.Lock(); defer o.mu.Unlock(); return o.buf.String() }
func startJob(t *testing.T, module, name string, c collectorapi.CollectorV2) (*jobruntime.JobV2, *output, func()) {
	t.Helper()
	out := &output{}
	job := jobruntime.NewJobV2(
		jobruntime.JobV2Config{
			PluginName:  "dem",
			ModuleName:  module,
			Name:        name,
			FullName:    module + "_" + name,
			Module:      c,
			Out:         out,
			UpdateEvery: 1,
			StoreFirst:  true,
		},
	)
	require.NoError(t, job.AutoDetectionManaged(context.Background()))
	run := jobruntime.NewManagedRun(context.Background(), nil)
	done := make(chan struct{})
	go func() { job.StartManaged(run); close(done) }()
	var once sync.Once
	stop := func() {
		once.Do(func() {
			job.Stop()
			select {
			case <-done:
			case <-time.After(8 * time.Second):
				t.Error("native job did not stop")
			}
			job.Cleanup()
			assert.Nil(t, run.Failure())
		})
	}
	t.Cleanup(stop)
	select {
	case <-run.StartupDone():
	case <-time.After(3 * time.Second):
		t.Fatal("native job did not become ready")
	}
	require.NoError(t, run.StartupErr())
	require.True(t, run.Running())
	return job, out, stop
}
func tickUntil(t *testing.T, job *jobruntime.JobV2, out *output, needle string) {
	t.Helper()
	for i := 1; i <= 300; i++ {
		job.Tick(i)
		if strings.Contains(out.String(), needle) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("protocol never contained %q:\n%s", needle, out.String())
}

// Receiver reload must preserve an independent site's measurements and history.
func TestNativeJobsIngressReplacementAndHistory(t *testing.T) {
	hub := runtimehub.New()
	db, err := store.Open(context.Background(), "")
	require.NoError(t, err)
	defer db.Close()
	site := rum.New(rum.Dependencies{
		Hub:     hub,
		History: db,
	})
	site.Key = "shop"
	site.Name = "Shop"
	site.AllowedOrigins = []string{"https://example.org"}
	site.OTLP.Enabled = "no"
	siteJob, siteOut, stopSite := startJob(t, "rum", "shop", site)
	tickUntil(t, siteJob, siteOut, "SET 'unavailable' = 1")
	assert.NotContains(t, siteOut.String(), "rum.lcp", "no browser measurement has been made")
	newReceiver := func() (*jobruntime.JobV2, *output, func()) {
		c := receiver.New(hub)
		c.Listen = "127.0.0.1:0"
		return startJob(t, "receiver", "receiver", c)
	}
	recvJob, recvOut, stopReceiver := newReceiver()
	tickUntil(t, recvJob, recvOut, "SET 'serving' = 1")
	send := func(path string) {
		body := `{"meta":{"page":{"url":"https://example.org` + path + `"},"session":{"id":"browser"},"browser":{"name":"Chrome"}},"measurements":[{"type":"web-vitals","values":{"lcp":3000,"cls":0.12}}]}`
		req, err := http.NewRequest(
			"POST",
			"http://"+hub.Availability().Listen+"/rum/shop/collect",
			strings.NewReader(body),
		)
		require.NoError(t, err)
		req.Header.Set("Origin", "https://example.org")
		req.Header.Set("User-Agent", "Mozilla/5.0 Chrome/124.0.0.0 Safari/537.36")
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, resp.Body)
		require.Equal(t, http.StatusAccepted, resp.StatusCode)
	}
	send("/products")
	tickUntil(t, siteJob, siteOut, "SET 'p75' = 3000")
	assert.Contains(t, siteOut.String(), "CLABEL '_collect_job' 'shop'")
	assert.Contains(t, siteOut.String(), "CLABEL 'display_name' 'Shop'")
	assert.Contains(t, siteOut.String(), "SET 'p75' = 0.12")
	data, _, release, ok := hub.AcquireSite("shop")
	require.True(t, ok)
	generation := data.Generation
	release()
	oldAddr := hub.Availability().Listen
	stopReceiver()
	assert.False(t, hub.Availability().Serving)
	// The retired receiver released the socket as well as its handler leases.
	ln, err := net.Listen("tcp", oldAddr)
	require.NoError(t, err)
	require.NoError(t, ln.Close())
	data, _, release, ok = hub.AcquireSite("shop")
	require.True(t, ok)
	assert.Equal(t, generation, data.Generation)
	release()
	_, _, stopNext := newReceiver()
	send("/checkout")
	stopNext()
	stopSite()
	rows, err := db.QueryRumSessions(context.Background(), "shop", 0, time.Now().Unix()+10, 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.EqualValues(t, 2, rows[0].Pageviews)
	_, _, _, ok = hub.AcquireSite("shop")
	assert.False(t, ok)
}

func TestCancelledOTLPPreparationHonorsCaller(t *testing.T) {
	db, err := store.Open(context.Background(), "")
	require.NoError(t, err)
	defer db.Close()
	site := rum.New(rum.Dependencies{
		Hub:     runtimehub.New(),
		History: db,
	})
	site.Key = "shop"
	site.AllowedOrigins = []string{"https://example.org"}
	site.OTLP.TLSCA = "synthetic-ca.pem"
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, site.Init(ctx), context.Canceled)
}
