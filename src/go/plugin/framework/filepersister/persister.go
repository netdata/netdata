// SPDX-License-Identifier: GPL-3.0-or-later

package filepersister

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
)

type Data interface {
	Bytes() ([]byte, error)
	Updated() <-chan struct{}
}

func Save(path string, data interface{ Bytes() ([]byte, error) }) {
	if path == "" {
		return
	}
	New(path).flush(data)
}

func New(path string) *Persister {
	return &Persister{
		Logger: logger.New().With(
			slog.String("component", "file persister"),
			slog.String("file", path),
		),
		FlushEvery: time.Minute * 1,
		filepath:   path,
		flushCh:    make(chan struct{}, 1),
	}
}

type Persister struct {
	*logger.Logger

	FlushEvery time.Duration

	data     Data
	filepath string
	flushCh  chan struct{}
}

func (p *Persister) Run(ctx context.Context, data Data) {
	p.Info("instance is started")
	defer func() { p.Info("instance is stopped") }()

	p.data = data

	tk := time.NewTicker(p.FlushEvery)
	defer tk.Stop()
	defer p.flush(p.data)

	for {
		select {
		case <-ctx.Done():
			return
		case <-p.data.Updated():
			p.triggerFlush()
		case <-tk.C:
			p.tryFlush()
		}
	}
}

func (p *Persister) triggerFlush() {
	select {
	case p.flushCh <- struct{}{}:
	default:
		// already has a pending flush
	}
}

func (p *Persister) tryFlush() {
	select {
	case <-p.flushCh:
		p.flush(p.data)
	default:
		// no pending flush
	}
}

func (p *Persister) flush(data interface{ Bytes() ([]byte, error) }) {
	bs, err := data.Bytes()
	if err != nil {
		p.Debugf("failed to marshal data: %v", err)
		return
	}

	_ = os.WriteFile(p.filepath, bs, 0644)

	p.Debug("file persisted successfully")
}
