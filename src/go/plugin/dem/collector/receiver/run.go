// SPDX-License-Identifier: GPL-3.0-or-later

package receiver

import (
	"context"
	"errors"
	"io"
	stdlog "log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/httpapi"
	rumregistry "github.com/netdata/netdata/go/plugins/plugin/dem/rum/registry"
)

// Run owns the socket and joins every admitted handler before returning. Init
// and Check never bind, including native DynCfg test/update preflight.
func (c *Collector) Run(ctx context.Context, ready func()) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	ln, err := net.Listen("tcp", c.Listen)
	if err != nil {
		return err
	}
	domain := httpapi.New(&c.Receiver, c.registry, c.geo)
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
	publication := c.registry.PublishReceiver(
		rumregistry.Availability{
			Serving:   true,
			Listen:    ln.Addr().String(),
			PublicURL: c.PublicURL,
			GeoIP:     c.geo.Status(),
		},
	)
	c.publicationMu.Lock()
	c.publication = publication
	c.publicationMu.Unlock()
	workerCtx, stopWorker := context.WithCancel(ctx)
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		ticker := time.NewTicker(geoIPRefreshInterval)
		defer ticker.Stop()
		c.runGeoIP(workerCtx, ticker.C)
	}()
	ready()
	var serveErr error
	exited := false
	select {
	case <-ctx.Done():
	case serveErr = <-serving:
		exited = true
	}
	publication.Close()
	stopWorker()
	<-workerDone
	c.publicationMu.Lock()
	c.publication = nil
	c.publicationMu.Unlock()
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

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (w *responseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *responseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
