// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import "context"

// ScrapeStream implements [Prometheus]. See the interface for the ordering and
// callback contract. onHelp may be nil when per-family HELP is not needed.
func (p *prometheus) ScrapeStream(ctx context.Context, onHelp func(name, help string), onSample func(Sample) error) error {
	p.buf.Reset()

	if err := p.fetch(ctx, p.buf); err != nil {
		return err
	}

	return p.parser.parseToStream(p.buf.Bytes(), onHelp, onSample)
}
