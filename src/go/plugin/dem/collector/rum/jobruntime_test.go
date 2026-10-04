// SPDX-License-Identifier: GPL-3.0-or-later
package rum_test

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/receiver"
	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/rum"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/agg"
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
	site.Name = "shop"
	site.DisplayName = "Shop"
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
	site.Name = "shop"
	site.AllowedOrigins = []string{"https://example.org"}
	site.OTLP.TLSCA = "synthetic-ca.pem"
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, site.Init(ctx), context.Canceled)
}

func TestIndependentSitesSurviveReceiverReplacementAndRetirement(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	hub := runtimehub.New()
	startSite := func(name string) func() {
		c := rum.New(rum.Dependencies{
			Hub:     hub,
			History: db,
		})
		c.Name, c.AllowedOrigins, c.OTLP.Enabled = name, []string{"https://example.org"}, "no"
		_, _, stop := startJob(t, "rum", name, c)
		return stop
	}
	stopAlpha, stopBeta := startSite("alpha"), startSite("beta")
	startReceiver := func() func() {
		c := receiver.New(hub)
		c.Listen = "127.0.0.1:0"
		_, _, stop := startJob(t, "receiver", "receiver", c)
		return stop
	}
	stopReceiver := startReceiver()
	send := func(site, page string, status int) {
		body := `{"meta":{"page":{"url":"https://example.org` + page + `"},"session":{"id":"same-session"},"browser":{"name":"Chrome"}}}`
		req, err := http.NewRequestWithContext(
			ctx,
			http.MethodPost,
			"http://"+hub.Availability().Listen+"/rum/"+site+"/collect",
			strings.NewReader(body),
		)
		require.NoError(t, err)
		req.Header.Set("Origin", "https://example.org")
		req.Header.Set("User-Agent", "Mozilla/5.0 Chrome/124.0.0.0 Safari/537.36")
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
		require.Equal(t, status, resp.StatusCode)
	}
	generations := make(map[string]string)
	for _, name := range []string{"alpha", "beta"} {
		send(name, "/first", http.StatusAccepted)
		data, _, release, ok := hub.AcquireSite(name)
		require.True(t, ok)
		generations[name] = data.Generation
		snapshot := data.Aggregator.Snapshot()
		release()
		require.EqualValues(t, 1, snapshot.Counters[agg.CounterPageviews])
		require.Equal(t, 1, snapshot.ActiveSessions)
	}
	stopReceiver()
	stopNext := startReceiver()
	for _, name := range []string{"alpha", "beta"} {
		data, _, release, ok := hub.AcquireSite(name)
		require.True(t, ok)
		generation := data.Generation
		release()
		require.Equal(t, generations[name], generation)
	}
	stopAlpha()
	send("alpha", "/retired", http.StatusNotFound)
	send("beta", "/second", http.StatusAccepted)
	stopBeta()
	stopNext()
	for name, want := range map[string]uint64{"alpha": 1, "beta": 2} {
		rows, err := db.QueryRumSessions(ctx, name, 0, time.Now().Unix()+10, 10)
		require.NoError(t, err)
		require.Len(t, rows, 1)
		require.Equal(t, "same-session", rows[0].SessionID)
		require.EqualValues(t, want, rows[0].Pageviews)
	}
}

func TestOversizedChunkedUploadRespondsPromptlyAndDisablesKeepAlive(t *testing.T) {
	hub := runtimehub.New()
	db, err := store.Open(context.Background(), "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	site := rum.New(rum.Dependencies{Hub: hub, History: db})
	site.Name = "shop"
	site.AllowedOrigins = []string{"https://example.org"}
	site.OTLP.Enabled = "no"
	startJob(t, "rum", "shop", site)
	listener := receiver.New(hub)
	listener.Listen = "127.0.0.1:0"
	listener.MaxBodyBytes = 32
	startJob(t, "receiver", "receiver", listener)
	for _, complete := range []bool{false, true} {
		t.Run(fmt.Sprintf("complete=%v", complete), func(t *testing.T) {
			conn, err := net.Dial("tcp", hub.Availability().Listen)
			require.NoError(t, err)
			defer conn.Close()
			require.NoError(t, conn.SetDeadline(time.Now().Add(2*time.Second)))
			body := strings.Repeat("x", 64)
			request := "POST /rum/shop/collect HTTP/1.1\r\nHost: example.org\r\nOrigin: https://example.org\r\nUser-Agent: Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36\r\nTransfer-Encoding: chunked\r\n\r\n"
			request += fmt.Sprintf("%x\r\n%s\r\n", len(body), body)
			if complete {
				request += "0\r\n\r\n"
			}
			_, err = io.WriteString(conn, request)
			require.NoError(t, err)
			response, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: http.MethodPost})
			require.NoError(t, err, "oversize response must not wait for the terminating chunk")
			defer response.Body.Close()
			assert.Equal(t, http.StatusRequestEntityTooLarge, response.StatusCode)
			assert.True(t, response.Close, "an oversized upload must not leave this connection reusable")
		})
	}
}
