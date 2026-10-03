// SPDX-License-Identifier: GPL-3.0-or-later

package receiver

import (
	"context"
	"crypto/tls"
	_ "embed"
	"errors"
	"fmt"
	"io"
	stdlog "log"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/pkg/tlscfg"
	"github.com/netdata/netdata/go/plugins/plugin/dem/config"
	"github.com/netdata/netdata/go/plugins/plugin/dem/geoip"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/ingest"
	"github.com/netdata/netdata/go/plugins/plugin/dem/runtimehub"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

//go:embed charts.yaml
var charts string

//go:embed config_schema.json
var schema string

type Config struct {
	UpdateEvery   int `yaml:"update_every,omitempty" json:"update_every"`
	config.RumCfg `    yaml:",inline"                json:""`
}
type Collector struct {
	collectorapi.Base
	Config    `yaml:",inline" json:""`
	hub       *runtimehub.Hub
	store     metrix.CollectorStore
	tlsConfig *tls.Config
	geo       *geoip.Resolver
	requests  [3]atomic.Uint64
}

func Creator(hub *runtimehub.Hub) collectorapi.Creator {
	return collectorapi.Creator{
		Defaults: collectorapi.Defaults{
			UpdateEvery: 10,
		},
		InstancePolicy:  collectorapi.InstancePolicySingle,
		CreateV2:        func() collectorapi.CollectorV2 { return New(hub) },
		Config:          func() any { return &Config{} },
		JobConfigSchema: schema,
		StoreFirst:      true,
	}
}
func New(hub *runtimehub.Hub) *Collector {
	return &Collector{
		Config: Config{
			RumCfg: config.RumCfg{
				Listen:       "127.0.0.1:19938",
				MaxBodyBytes: 262144,
				RateLimit: config.RumRateLimit{
					PerIPPerMin:   120,
					PerSitePerSec: 500,
				},
			},
		},
		hub:   hub,
		store: metrix.NewCollectorStore(),
	}
}
func (c *Collector) Configuration() any                 { return c.Config }
func (c *Collector) MetricStore() metrix.CollectorStore { return c.store }
func (c *Collector) ChartTemplateYAML() string          { return charts }
func (c *Collector) Init(ctx context.Context) error {
	if c.hub == nil {
		return errors.New("missing runtime routes")
	}
	if _, _, err := net.SplitHostPort(c.Listen); err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	if c.MaxBodyBytes <= 0 || c.RateLimit.PerIPPerMin <= 0 || c.RateLimit.PerSitePerSec <= 0 {
		return errors.New("body and rate limits must be positive")
	}
	for _, p := range c.TrustedProxies {
		if _, err := netip.ParsePrefix(p); err != nil {
			if _, err := netip.ParseAddr(p); err != nil {
				return fmt.Errorf("trusted proxy %q: %w", p, err)
			}
		}
	}
	if c.PublicURL != "" {
		u, err := url.Parse(c.PublicURL)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil ||
			u.RawQuery != "" ||
			u.Fragment != "" {
			return errors.New("public_url must be an http(s) base URL without credentials, query or fragment")
		}
	}
	if (c.TLSCert == "") != (c.TLSKey == "") {
		return errors.New("tls_cert and tls_key must be set together")
	}
	if c.TLSCert != "" {
		tlsConfig, err := tlscfg.NewTLSConfig(ctx, tlscfg.TLSConfig{
			TLSCert: c.TLSCert,
			TLSKey:  c.TLSKey,
		})
		if err != nil {
			return fmt.Errorf("listener TLS: %w", err)
		}
		tlsConfig.MinVersion = tls.VersionTLS12
		c.tlsConfig = tlsConfig
	}
	resolver, err := geoip.Open(c.GeoIPDB)
	if err != nil && !errors.Is(err, geoip.ErrNotFound) {
		c.Warningf("GeoIP unavailable: %v", err)
	}
	c.geo = resolver
	return nil
}
func (c *Collector) Check(context.Context) error {
	if c.hub == nil {
		return errors.New("missing runtime routes")
	}
	return nil
}

// Run owns the socket and joins every admitted handler before returning. Init
// and Check never bind, including native DynCfg test/update preflight.
func (c *Collector) Run(ctx context.Context, ready func()) error {
	ln, err := net.Listen("tcp", c.Listen)
	if err != nil {
		return err
	}
	var geo ingest.CountryResolver
	if c.geo != nil {
		geo = c.geo
	}
	domain := ingest.New(&c.RumCfg, c.hub, geo)
	handler := domain.Handler()
	var mu sync.Mutex
	var active sync.WaitGroup
	accepting := true
	srv := &http.Server{
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
		ErrorLog: stdlog.New(
			io.Discard,
			"",
			0,
		),
		BaseContext: func(net.Listener) context.Context { return ctx },
		TLSConfig:   c.tlsConfig,
	}
	srv.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		if !accepting {
			mu.Unlock()
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		active.Add(1)
		mu.Unlock()
		defer active.Done()
		interrupted := make(chan struct{})
		stop := context.AfterFunc(r.Context(), func() {
			defer close(interrupted)
			_ = http.NewResponseController(w).SetReadDeadline(time.Now())
			_ = r.Body.Close()
		})
		defer func() {
			if !stop() {
				<-interrupted
			}
		}()
		rw := &responseWriter{
			ResponseWriter: w,
			status:         200,
		}
		handler.ServeHTTP(rw, r)
		switch {
		case rw.status < 400:
			c.requests[0].Add(1)
		case rw.status < 500:
			c.requests[1].Add(1)
		default:
			c.requests[2].Add(1)
		}
	})
	serving := make(chan error, 1)
	go func() {
		if c.tlsConfig != nil {
			serving <- srv.ServeTLS(ln, "", "")
		} else {
			serving <- srv.Serve(ln)
		}
	}()
	revoke := c.hub.PublishReceiver(
		runtimehub.Availability{
			Serving:   true,
			Listen:    ln.Addr().String(),
			TLS:       c.tlsConfig != nil,
			PublicURL: c.PublicURL,
		},
	)
	ready()
	var serveErr error
	exited := false
	select {
	case <-ctx.Done():
	case serveErr = <-serving:
		exited = true
	}
	revoke()
	mu.Lock()
	accepting = false
	mu.Unlock()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	shutdownErr := srv.Shutdown(shutdownCtx)
	cancel()
	if shutdownErr != nil {
		_ = srv.Close()
	}
	active.Wait()
	if !exited {
		serveErr = <-serving
	}
	if errors.Is(serveErr, http.ErrServerClosed) {
		return nil
	}
	return serveErr
}
func (c *Collector) Collect(context.Context) error {
	m := c.store.Write().SnapshotMeter("")
	state := "unavailable"
	if c.hub.Availability().Serving {
		state = "serving"
	}
	m.StateSet("receiver_state", metrix.WithStateSetStates("serving", "unavailable"), metrix.WithStateSetMode(metrix.ModeEnum)).
		Enable(state)
	for i, name := range []string{"http_success", "http_client_errors", "http_server_errors"} {
		m.Counter(name).ObserveTotal(float64(c.requests[i].Load()))
	}
	return nil
}
func (c *Collector) Cleanup(context.Context) {
	if c.geo != nil {
		c.geo.Close()
		c.geo = nil
	}
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (w *responseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *responseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
