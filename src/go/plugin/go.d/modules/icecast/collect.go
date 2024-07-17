// SPDX-License-Identifier: GPL-3.0-or-later

package icecast

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

type Source struct {
	ServerName  string `json:"server_name"`
	StreamStart string `json:"stream_start"`
	Listeners   int64  `json:"listeners"`
}

type Icestats struct {
	Source []Source `json:"source"`
}

type Data struct {
	Icestats Icestats `json:"icestats"`
}

type sourceWrapper struct {
	Source json.RawMessage `json:"source"`
}

const (
	urlPathServerStats = "/status-json.xsl" // https://icecast.org/docs/icecast-trunk/server_stats/
)

func (ic *Icecast) collect() (map[string]int64, error) {
	mx := make(map[string]int64)

	if err := ic.collectServerStats(mx); err != nil {
		return nil, err
	}

	return mx, nil
}

func (ic *Icecast) collectServerStats(mx map[string]int64) error {
	stats, err := ic.queryServerStats()
	if err != nil {
		return err
	}

	seen := make(map[string]bool)

	// if len(stats.Icestats.Source) == 0 {
	// 	return fmt.Errorf("no active icecast sources found")
	// }

	for _, source := range stats.Icestats.Source {

		if source.ServerName != "" && source.StreamStart != "" {
			key := source.ServerName

			seen[key] = true

			if _, ok := ic.seenSources[key]; !ok {
				ic.seenSources[key] = &source
				ic.addSourceChart(&source)
			}

			mx[source.ServerName+"_listeners"] = source.Listeners
		}

	}

	for _, source := range stats.Icestats.Source {

		if !seen[source.ServerName] {
			delete(ic.seenSources, source.ServerName)
			ic.removeSourceChart(&source)
		}

	}

	return nil
}

func (ic *Icecast) queryServerStats() (*Data, error) {
	req, err := web.NewHTTPRequestWithPath(ic.Request, urlPathServerStats)
	if err != nil {
		return nil, err
	}

	var statsWrapper struct {
		Icestats *sourceWrapper `json:"icestats"`
	}

	if err := ic.doOKDecode(req, &statsWrapper); err != nil {
		return nil, err
	}

	if statsWrapper.Icestats == nil {
		return nil, fmt.Errorf("unexpected response: icestats field is nil")
	}

	if len(statsWrapper.Icestats.Source) == 0 {
		return nil, fmt.Errorf("unexpected response: no sources found")
	}

	var icestats Icestats

	if statsWrapper.Icestats.Source[0] == '{' {
		// Single object case
		var singleSource Source
		if err := json.Unmarshal(statsWrapper.Icestats.Source, &singleSource); err != nil {
			return nil, err
		}
		icestats.Source = []Source{singleSource}
	} else {
		// Array case
		if err := json.Unmarshal(statsWrapper.Icestats.Source, &icestats.Source); err != nil {
			return nil, err
		}
	}

	for _, source := range icestats.Source {
		if source.ServerName == "" || source.StreamStart == "" {
			return nil, fmt.Errorf("invalid JSON response: missing required fields in source")
		}
	}

	return &Data{Icestats: icestats}, nil
}

func (ic *Icecast) doOKDecode(req *http.Request, in interface{}) error {
	resp, err := ic.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error on HTTP request '%s': %v", req.URL, err)
	}
	defer closeBody(resp)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("'%s' returned HTTP status code: %d", req.URL, resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(in); err != nil {
		return fmt.Errorf("error on decoding response from '%s': %v", req.URL, err)
	}
	return nil
}

func closeBody(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
}
