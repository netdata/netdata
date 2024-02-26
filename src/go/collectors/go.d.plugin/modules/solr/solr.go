// SPDX-License-Identifier: GPL-3.0-or-later

package solr

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/netdata/netdata/go/go.d.plugin/pkg/web"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("solr", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
	})
}

const (
	defaultURL         = "http://127.0.0.1:8983"
	defaultHTTPTimeout = time.Second
)

const (
	minSupportedVersion   = 6.4
	coresHandlersURLPath  = "/solr/admin/metrics"
	coresHandlersURLQuery = "group=core&prefix=UPDATE,QUERY&wt=json"
	infoSystemURLPath     = "/solr/admin/info/system"
	infoSystemURLQuery    = "wt=json"
)

type infoSystem struct {
	Lucene struct {
		Version string `json:"solr-spec-version"`
	}
}

// New creates Solr with default values
func New() *Solr {
	config := Config{
		HTTP: web.HTTP{
			Request: web.Request{
				URL: defaultURL,
			},
			Client: web.Client{
				Timeout: web.Duration{Duration: defaultHTTPTimeout},
			},
		},
	}
	return &Solr{
		Config: config,
		cores:  make(map[string]bool),
	}
}

// Config is the Solr module configuration.
type Config struct {
	web.HTTP `yaml:",inline"`
}

// Solr solr module
type Solr struct {
	module.Base
	Config `yaml:",inline"`

	cores   map[string]bool
	client  *http.Client
	version float64
	charts  *Charts
}

func (s *Solr) doRequest(req *http.Request) (*http.Response, error) {
	return s.client.Do(req)
}

// Cleanup makes cleanup
func (Solr) Cleanup() {}

// Init makes initialization
func (s *Solr) Init() bool {
	if s.URL == "" {
		s.Error("URL not set")
		return false
	}

	client, err := web.NewHTTPClient(s.Client)
	if err != nil {
		s.Error(err)
		return false
	}

	s.client = client
	return true
}

// Check makes check
func (s *Solr) Check() bool {
	if err := s.getVersion(); err != nil {
		s.Error(err)
		return false
	}

	if s.version < minSupportedVersion {
		s.Errorf("unsupported Solr version : %.1f", s.version)
		return false
	}

	return true
}

// Charts creates Charts
func (s *Solr) Charts() *Charts {
	s.charts = &Charts{}

	return s.charts
}

// Collect collects metrics
func (s *Solr) Collect() map[string]int64 {
	req, err := createRequest(s.Request, coresHandlersURLPath, coresHandlersURLQuery)
	if err != nil {
		s.Errorf("error on creating http request : %v", err)
		return nil
	}

	resp, err := s.doRequest(req)
	if err != nil {
		s.Errorf("error on request to %s : %s", req.URL, err)
		return nil
	}
	defer closeBody(resp)

	if resp.StatusCode != http.StatusOK {
		s.Errorf("%s returned HTTP status %d", req.URL, resp.StatusCode)
		return nil
	}

	metrics, err := s.parse(resp)
	if err != nil {
		s.Errorf("error on parse response from %s : %s", req.URL, err)
		return nil
	}

	return metrics
}

func (s *Solr) getVersion() error {
	req, err := createRequest(s.Request, infoSystemURLPath, infoSystemURLQuery)
	if err != nil {
		return fmt.Errorf("error on creating http request : %v", err)
	}

	resp, err := s.doRequest(req)
	if err != nil {
		return fmt.Errorf("error on request to %s : %s", req.URL, err)
	}
	defer closeBody(resp)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s returned HTTP status %d", req.URL, resp.StatusCode)
	}

	var info infoSystem

	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return fmt.Errorf("error on decode response from %s : %s", req.URL, err)
	}

	var idx int

	if idx = strings.LastIndex(info.Lucene.Version, "."); idx == -1 {
		return fmt.Errorf("error on parsing version '%s': bad format", info.Lucene.Version)
	}

	if s.version, err = strconv.ParseFloat(info.Lucene.Version[:idx], 64); err != nil {
		return fmt.Errorf("error on parsing version '%s' :  %s", info.Lucene.Version, err)
	}

	return nil
}

func createRequest(req web.Request, urlPath, urlQuery string) (*http.Request, error) {
	r := req.Copy()
	u, err := url.Parse(r.URL)
	if err != nil {
		return nil, err
	}

	u.Path = urlPath
	u.RawQuery = urlQuery
	r.URL = u.String()
	return web.NewHTTPRequest(r)
}

func closeBody(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
}
