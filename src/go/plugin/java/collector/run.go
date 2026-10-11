// SPDX-License-Identifier: GPL-3.0-or-later

package collector

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	collectorv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	"google.golang.org/protobuf/proto"
)

func (c *Collector) run(parent context.Context, ready func()) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	if c.isTerminal() {
		return fmt.Errorf("interactive Java attachment is disabled; run java.plugin through Netdata")
	}
	boot, err := os.ReadFile(filepath.Join(c.runtime.ProcDir, "sys/kernel/random/boot_id"))
	if err != nil {
		return err
	}
	bootID := strings.TrimSpace(string(boot))
	if bootID == "" {
		return fmt.Errorf("empty Linux boot identity")
	}
	journal, err := openJournal(c.runtime.StateDir, bootID)
	if err != nil {
		return err
	}
	defer journal.close()
	listener, err := net.Listen("tcp4", "127.0.0.1:"+strconv.Itoa(int(journal.state.Port)))
	if err != nil {
		return fmt.Errorf("bind persisted Java receiver: %w", err)
	}
	defer listener.Close()
	if journal.state.Port == 0 {
		journal.state.Port = uint16(listener.Addr().(*net.TCPAddr).Port)
		if err := journal.save(); err != nil {
			return err
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/metrics", c.receiver())
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second}
	workers := make(chan error, 2)
	go func() { workers <- server.Serve(listener) }()
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			if err := c.scan(ctx, journal); err != nil {
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

func (c *Collector) receiver() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("x-netdata-java-token")
		hash := sha256.Sum256([]byte(token))
		c.mu.Lock()
		admitted, ok := c.credentials[hash]
		c.mu.Unlock()
		if len(token) != 64 || !ok {
			http.Error(w, "unrecognized Java credential", http.StatusUnauthorized)
			return
		}

		if r.Header.Get("Content-Type") != "application/x-protobuf" {
			http.Error(w, "protobuf required", http.StatusUnsupportedMediaType)
			return
		}
		var body io.Reader = http.MaxBytesReader(w, r.Body, 8<<20)
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
		// Bound both compressed and decompressed exports before protobuf decoding.
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
		if !matchesAdmission(&req, admitted) {
			http.Error(w, "Java resource does not match credential", http.StatusForbidden)
			return
		}
		c.mu.Lock()
		current, stillAdmitted := c.credentials[hash]
		if !stillAdmitted || current != admitted {
			c.mu.Unlock()
			http.Error(w, "Java credential retired", http.StatusUnauthorized)
			return
		}
		result, ingestErr := c.ingress.Ingest(&req, c.now())
		c.mu.Unlock()
		response := &collectorv1.ExportMetricsServiceResponse{}
		if ingestErr != nil {
			response.PartialSuccess = &collectorv1.ExportMetricsPartialSuccess{RejectedDataPoints: int64(result.Rejected), ErrorMessage: fmt.Sprint(ingestErr)}
		}
		data, _ = proto.Marshal(response)
		w.Header().Set("Content-Type", "application/x-protobuf")
		_, _ = w.Write(data)
	}
}

func matchesAdmission(req *collectorv1.ExportMetricsServiceRequest, expected admission) bool {
	for _, resource := range req.ResourceMetrics {
		values := make(map[string]string)
		for _, a := range resource.GetResource().GetAttributes() {
			if a.Key != "service.name" && a.Key != "service.instance.id" {
				continue
			}
			if _, exists := values[a.Key]; exists {
				return false
			}
			values[a.Key] = a.GetValue().GetStringValue()
		}
		if values["service.name"] != expected.application || values["service.instance.id"] != expected.instance {
			return false
		}
	}
	return true
}
