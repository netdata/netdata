// SPDX-License-Identifier: GPL-3.0-or-later

package diagnostics

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
)

var log = logger.New()

// Probe fetches this site's bootstrap and records whether this receiver answers.
func (s *State) Probe(ctx context.Context, client *http.Client, base string) Reach {
	r := Reach{
		State:     ReachOK,
		Base:      base,
		CheckedAt: time.Now(),
	}
	if err := fetchOwnBootstrap(ctx, client, s.config.Name, base); err != nil {
		r.State, r.Error = ReachFailed, err.Error()
	}
	s.mu.Lock()
	prev, seen := s.result, s.hasResult
	s.result, s.hasResult = r, true
	s.mu.Unlock()
	if !seen || prev.State != r.State || prev.Base != r.Base {
		if r.State == ReachOK {
			log.Noticef("rum: site %s public URL %s reachable", s.config.Name, base)
		} else {
			log.Warningf("rum: site %s public URL %s unreachable: %s", s.config.Name, base, r.Error)
		}
	}
	return r
}

func fetchOwnBootstrap(ctx context.Context, client *http.Client, key, base string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+"/rum/"+key+".js", nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return err
	}
	quotedKey, _ := json.Marshal(key)
	if !strings.Contains(string(body), "faro-web-sdk") || !strings.Contains(string(body), string(quotedKey)) {
		return fmt.Errorf("answered, but not by this collector")
	}
	return nil
}

// RunReachability probes this site immediately and then every interval.
// Without a configured or learned public address, reachability stays unknown.
func (s *State) RunReachability(ctx context.Context, every time.Duration, base func() string, client *http.Client) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		publicBase := strings.TrimRight(base(), "/")
		if publicBase == "" {
			publicBase = s.ObservedBase()
		}
		if publicBase != "" {
			s.Probe(ctx, client, publicBase)
			s.recordSnippet(CheckSnippet(ctx, client, s.config, publicBase))
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
