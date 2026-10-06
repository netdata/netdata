// SPDX-License-Identifier: GPL-3.0-or-later
// Package workerprovider receives a native Collector configuration from the
// parent. EOF on the same pipe requests graceful Collector shutdown.
package workerprovider

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"sync"

	"go.opentelemetry.io/collector/confmap"
)

type provider struct{ input io.ReadCloser }

func NewFactory() confmap.ProviderFactory {
	return confmap.NewProviderFactory(
		func(confmap.ProviderSettings) confmap.Provider { return &provider{input: os.Stdin} },
	)
}
func (*provider) Scheme() string { return "worker" }
func (p *provider) Retrieve(ctx context.Context, uri string, watcher confmap.WatcherFunc) (*confmap.Retrieved, error) {
	if uri != "worker:stdin" {
		return nil, errors.New("supported URI: worker:stdin")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	reader := bufio.NewReader(p.input)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		return nil, errors.New("read worker configuration: complete JSON line required")
	}
	var config map[string]any
	if err := json.Unmarshal(line, &config); err != nil {
		return nil, errors.New("invalid worker configuration JSON")
	}
	escapeLiterals(config)
	stopped, done := make(chan struct{}), make(chan struct{})
	var once sync.Once
	go func() {
		defer close(done)
		// This channel has no commands after configuration; loss of the parent (or
		// its explicit close) ends the Collector. Unexpected data also ends it.
		_, _ = reader.ReadByte()
		select {
		case <-stopped:
			return
		default:
		}
		if watcher != nil {
			watcher(&confmap.ChangeEvent{Error: errors.New("worker parent pipe closed")})
		}
	}()
	return confmap.NewRetrieved(config, confmap.WithRetrievedClose(func(ctx context.Context) error {
		once.Do(func() { close(stopped); _ = p.input.Close() })
		select {
		case <-done:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}))
}
func (p *provider) Shutdown(context.Context) error {
	err := p.input.Close()
	if errors.Is(err, os.ErrClosed) {
		return nil
	}
	return err
}

// Curated job values are literal strings, not Collector provider expressions.
func escapeLiterals(value any) any {
	switch v := value.(type) {
	case string:
		return strings.ReplaceAll(v, "$", "$$")
	case map[string]any:
		for key, item := range v {
			v[key] = escapeLiterals(item)
		}
	case []any:
		for i, item := range v {
			v[i] = escapeLiterals(item)
		}
	}
	return value
}
