// SPDX-License-Identifier: GPL-3.0-or-later

package meraki

import (
	"net/http"
	"sync"
	"time"
)

// Meraki's steady-state organization budget is 10 requests/second, shared with
// other applications. Pace actual requests, including redirect and page hops.
const requestSpacing = time.Second / 10

type pacedTransport struct {
	base      http.RoundTripper
	mu        sync.Mutex
	lastStart time.Time
}

func (p *pacedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	p.mu.Lock()
	if delay := time.Until(p.lastStart.Add(requestSpacing)); delay > 0 {
		timer := time.NewTimer(delay)
		select {
		case <-req.Context().Done():
			timer.Stop()
			p.mu.Unlock()
			return nil, req.Context().Err()
		case <-timer.C:
		}
	}
	if err := req.Context().Err(); err != nil {
		p.mu.Unlock()
		return nil, err
	}
	p.lastStart = time.Now()
	p.mu.Unlock()
	return p.base.RoundTrip(req)
}

func (p *pacedTransport) CloseIdleConnections() {
	if closer, ok := p.base.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}
