// SPDX-License-Identifier: GPL-3.0-or-later

package collector

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	collectorv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func (c *Collector) run(parent context.Context, ready func()) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	listener, err := net.Listen("tcp", c.runtime.Listen)
	if err != nil {
		return err
	}
	defer listener.Close()
	journal, err := os.OpenFile(filepath.Join(c.runtime.StateDir, "attempts.jsonl"), os.O_CREATE|os.O_RDWR|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	defer journal.Close()
	known, err := readAttempts(journal)
	if err != nil {
		return err
	}
	recording, err := os.OpenFile(filepath.Join(c.runtime.StateDir, "metrics.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	defer recording.Close()
	var recordMu sync.Mutex
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/metrics", c.receiver(recording, &recordMu))
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second}
	workers := make(chan error, 2)
	go func() { workers <- server.Serve(listener) }()
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			if err := c.scan(ctx, journal, known); err != nil {
				workers <- err
				return
			}
			select {
			case <-ctx.Done():
				workers <- ctx.Err()
				return
			case <-ticker.C:
			}
		}
	}()
	ready()
	remaining := 2
	select {
	case <-parent.Done():
		err = nil
	case err = <-workers:
		remaining--
	}
	cancel()
	shutdown, stop := context.WithTimeout(context.Background(), 20*time.Second)
	defer stop()
	if closeErr := server.Shutdown(shutdown); closeErr != nil {
		_ = server.Close()
		if err == nil {
			err = closeErr
		}
	}
	for ; remaining > 0; remaining-- {
		<-workers
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (c *Collector) receiver(recording io.Writer, recordMu *sync.Mutex) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/x-protobuf" {
			http.Error(w, "protobuf required", http.StatusUnsupportedMediaType)
			return
		}
		var body io.Reader = r.Body
		if encoding := r.Header.Get("Content-Encoding"); encoding != "" {
			if encoding != "gzip" {
				http.Error(w, "unsupported encoding", http.StatusUnsupportedMediaType)
				return
			}
			gz, err := gzip.NewReader(body)
			if err != nil {
				http.Error(w, "invalid gzip", http.StatusBadRequest)
				return
			}
			defer gz.Close()
			body = gz
		}
		// Bound decompressed fixture exports; this is a lab transport, not a public receiver.
		const maxRequest = 8 << 20
		data, err := io.ReadAll(io.LimitReader(body, maxRequest+1))
		if err != nil || len(data) > maxRequest {
			http.Error(w, "invalid or oversized payload", http.StatusBadRequest)
			return
		}
		var req collectorv1.ExportMetricsServiceRequest
		if err := proto.Unmarshal(data, &req); err != nil {
			http.Error(w, "invalid protobuf", http.StatusBadRequest)
			return
		}
		encoded, err := protojson.Marshal(&req)
		if err != nil {
			http.Error(w, "recording encode failed", http.StatusInternalServerError)
			return
		}
		recordMu.Lock()
		_, err = recording.Write(append(encoded, '\n'))
		recordMu.Unlock()
		if err != nil {
			http.Error(w, "recording failed", http.StatusInternalServerError)
			return
		}
		result, ingestErr := c.ingress.Ingest(&req, c.now())
		response := &collectorv1.ExportMetricsServiceResponse{}
		if ingestErr != nil {
			response.PartialSuccess = &collectorv1.ExportMetricsPartialSuccess{RejectedDataPoints: int64(result.Rejected), ErrorMessage: fmt.Sprint(ingestErr)}
		}
		data, _ = proto.Marshal(response)
		w.Header().Set("Content-Type", "application/x-protobuf")
		_, _ = w.Write(data)
	}
}
