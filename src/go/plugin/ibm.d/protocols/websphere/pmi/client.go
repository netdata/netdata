// Package pmi provides a reusable PerfServlet (PMI) client used by WebSphere collectors.
// SPDX-License-Identifier: GPL-3.0-or-later

package pmi

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/web"
)

// Config captures the options required to communicate with the PerfServlet endpoint.
type Config struct {
	// URL is the full PerfServlet endpoint (e.g. https://host:9443/wasPerfTool/servlet/perfservlet).
	URL string

	// StatsType controls the PMI statistics level (basic, extended, all, custom).
	StatsType string

	// HTTPConfig mirrors go.d's HTTP configuration for TLS/auth proxy handling.
	HTTPConfig web.HTTPConfig
}

// Client handles HTTP transport and XML decoding for PMI snapshots.
type Client struct {
	cfg        Config
	httpClient *http.Client
	baseURL    string
}

// NewClient validates the configuration and prepares an HTTP client.
func NewClient(cfg Config) (*Client, error) {
	if time.Duration(cfg.HTTPConfig.ClientConfig.Timeout) <= 0 {
		cfg.HTTPConfig.ClientConfig.Timeout = confopt.Duration(5 * time.Second)
	}

	httpClient, err := web.NewHTTPClient(cfg.HTTPConfig.ClientConfig)
	if err != nil {
		return nil, fmt.Errorf("pmi protocol: creating http client failed: %w", err)
	}

	return NewClientWithHTTP(cfg, httpClient)
}

// NewClientWithHTTP allows supplying a custom *http.Client, primarily for tests.
func NewClientWithHTTP(cfg Config, httpClient *http.Client) (*Client, error) {
	if httpClient == nil {
		return nil, errors.New("pmi protocol: http client is required")
	}

	normalizedCfg, baseURL, err := normalizeConfig(cfg)
	if err != nil {
		return nil, err
	}

	return &Client{
		cfg:        normalizedCfg,
		httpClient: httpClient,
		baseURL:    baseURL,
	}, nil
}

func normalizeConfig(cfg Config) (Config, string, error) {
	trimmedURL := strings.TrimSpace(cfg.URL)
	if trimmedURL == "" {
		return cfg, "", errors.New("pmi protocol: url is required")
	}

	parsedURL, err := url.Parse(trimmedURL)
	if err != nil {
		return cfg, "", fmt.Errorf("pmi protocol: invalid url: %w", err)
	}
	if parsedURL.Path == "" || parsedURL.Path == "/" {
		return cfg, "", errors.New("pmi protocol: url must include the PerfServlet path (e.g., /wasPerfTool/servlet/perfservlet)")
	}

	statsType := strings.ToLower(strings.TrimSpace(cfg.StatsType))
	switch statsType {
	case "", "extended":
		statsType = "extended"
	case "basic", "all", "custom":
		// accepted as-is
	default:
		statsType = "extended"
	}
	cfg.StatsType = statsType

	return cfg, parsedURL.String(), nil
}

// Close releases resources held by the underlying HTTP client.
func (c *Client) Close() {
	if c.httpClient != nil {
		c.httpClient.CloseIdleConnections()
	}
}

// Fetch retrieves a PMI snapshot and normalises the data tree.
func (c *Client) Fetch(ctx context.Context) (*Snapshot, error) {
	if c.httpClient == nil {
		return nil, errors.New("pmi protocol: client not initialised")
	}

	reqURL := c.baseURL + "?stats=" + url.QueryEscape(c.cfg.StatsType)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("pmi protocol: creating request failed: %w", err)
	}

	if c.cfg.HTTPConfig.Username != "" || c.cfg.HTTPConfig.Password != "" {
		req.SetBasicAuth(c.cfg.HTTPConfig.Username, c.cfg.HTTPConfig.Password)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("pmi protocol: request cancelled: %w", ctxErr)
		}
		return nil, fmt.Errorf("pmi protocol: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("pmi protocol: unexpected status %d: %s", resp.StatusCode, string(body))
	}

	decoder := xml.NewDecoder(resp.Body)

	type decodeResult struct {
		snapshot *Snapshot
		err      error
	}
	resultCh := make(chan decodeResult, 1)

	go func() {
		var snapshot Snapshot
		err := decoder.Decode(&snapshot)
		resultCh <- decodeResult{snapshot: &snapshot, err: err}
	}()

	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("pmi protocol: decoding cancelled: %w", ctx.Err())
	case res := <-resultCh:
		if res.err != nil {
			return nil, fmt.Errorf("pmi protocol: decoding failed: %w", res.err)
		}
		res.snapshot.Normalize()
		return res.snapshot, nil
	}
}

// Snapshot represents a full PMI response tree.
type Snapshot struct {
	XMLName        xml.Name `xml:"PerformanceMonitor"`
	ResponseStatus string   `xml:"responseStatus,attr"`
	Version        string   `xml:"version,attr"`
	Nodes          []Node   `xml:"Node"`
	Stats          []Stat   `xml:"Stat"`
}

// Normalize ensures backward-compatible pointers and propagates paths within the tree.
func (s *Snapshot) Normalize() {
	for i := range s.Nodes {
		s.Nodes[i].normalize()
	}
	for i := range s.Stats {
		s.Stats[i].normalize("", s.Stats[i].Name)
	}
}

// Node represents a PMI node element.
type Node struct {
	XMLName xml.Name `xml:"Node"`
	Name    string   `xml:"name,attr"`
	Servers []Server `xml:"Server"`
}

func (n *Node) normalize() {
	for i := range n.Servers {
		n.Servers[i].normalize(n.Name)
	}
}

// Server gathers stats for a specific WebSphere server instance.
type Server struct {
	XMLName xml.Name `xml:"Server"`
	Name    string   `xml:"name,attr"`
	Stats   []Stat   `xml:"Stat"`
}

func (s *Server) normalize(nodeName string) {
	base := s.Name
	if nodeName != "" {
		base = nodeName + "/" + base
	}
	for i := range s.Stats {
		s.Stats[i].normalize(base, s.Stats[i].Name)
	}
}

// Stat represents an individual PMI statistic entry.
type Stat struct {
	XMLName                xml.Name           `xml:"Stat"`
	Name                   string             `xml:"name,attr"`
	Type                   string             `xml:"type,attr"`
	ID                     string             `xml:"id,attr"`
	Path                   string             `xml:"path,attr"`
	Value                  *Value             `xml:"Value"`
	CountStatistics        []CountStatistic   `xml:"CountStatistic"`
	TimeStatistics         []TimeStatistic    `xml:"TimeStatistic"`
	RangeStatistics        []RangeStatistic   `xml:"RangeStatistic"`
	BoundedRangeStatistics []BoundedRangeStat `xml:"BoundedRangeStatistic"`
	DoubleStatistics       []DoubleStatistic  `xml:"DoubleStatistic"`
	AverageStatistics      []AverageStatistic `xml:"AverageStatistic"`
	SubStats               []Stat             `xml:"Stat"`

	CountStatistic        *CountStatistic   `xml:"-"`
	TimeStatistic         *TimeStatistic    `xml:"-"`
	RangeStatistic        *RangeStatistic   `xml:"-"`
	BoundedRangeStatistic *BoundedRangeStat `xml:"-"`
	DoubleStatistic       *DoubleStatistic  `xml:"-"`
	AverageStatistic      *AverageStatistic `xml:"-"`
}

func (s *Stat) normalize(parentPath, fallbackName string) {
	if len(s.CountStatistics) > 0 {
		s.CountStatistic = &s.CountStatistics[0]
	}
	if len(s.TimeStatistics) > 0 {
		s.TimeStatistic = &s.TimeStatistics[0]
	}
	if len(s.RangeStatistics) > 0 {
		s.RangeStatistic = &s.RangeStatistics[0]
	}
	if len(s.BoundedRangeStatistics) > 0 {
		s.BoundedRangeStatistic = &s.BoundedRangeStatistics[0]
	}
	if len(s.DoubleStatistics) > 0 {
		s.DoubleStatistic = &s.DoubleStatistics[0]
	}
	if len(s.AverageStatistics) > 0 {
		s.AverageStatistic = &s.AverageStatistics[0]
	}

	name := s.Name
	if name == "" {
		name = fallbackName
	}
	if parentPath == "" {
		s.Path = name
	} else if name != "" {
		s.Path = parentPath + "/" + name
	} else {
		s.Path = parentPath
	}

	for i := range s.SubStats {
		s.SubStats[i].normalize(s.Path, s.SubStats[i].Name)
	}
}

// Value captures the textual value for simple statistics.
type Value struct {
	Value string `xml:",chardata"`
}

// CountStatistic represents PMI count metrics.
type CountStatistic struct {
	Name  string `xml:"name,attr"`
	Count string `xml:"count,attr"`
	Unit  string `xml:"unit,attr"`
}

// TimeStatistic captures PMI time-based measurements.
type TimeStatistic struct {
	Name      string `xml:"name,attr"`
	Count     string `xml:"count,attr"`
	Total     string `xml:"total,attr"`
	TotalTime string `xml:"totalTime,attr"`
	Mean      string `xml:"mean,attr"`
	Min       string `xml:"min,attr"`
	Max       string `xml:"max,attr"`
	Unit      string `xml:"unit,attr"`
}

// RangeStatistic describes PMI range values.
type RangeStatistic struct {
	Name          string `xml:"name,attr"`
	Current       string `xml:"value,attr"`
	Integral      string `xml:"integral,attr"`
	Mean          string `xml:"mean,attr"`
	HighWaterMark string `xml:"highWaterMark,attr"`
	LowWaterMark  string `xml:"lowWaterMark,attr"`
	Unit          string `xml:"unit,attr"`
}

// BoundedRangeStat includes range metrics with explicit bounds.
type BoundedRangeStat struct {
	Name          string `xml:"name,attr"`
	Current       string `xml:"value,attr"`
	Integral      string `xml:"integral,attr"`
	Mean          string `xml:"mean,attr"`
	LowerBound    string `xml:"lowerBound,attr"`
	UpperBound    string `xml:"upperBound,attr"`
	HighWaterMark string `xml:"highWaterMark,attr"`
	LowWaterMark  string `xml:"lowWaterMark,attr"`
	Unit          string `xml:"unit,attr"`
}

// DoubleStatistic captures PMI floating point metrics.
type DoubleStatistic struct {
	Name   string `xml:"name,attr"`
	Double string `xml:"double,attr"`
	Unit   string `xml:"unit,attr"`
}

// AverageStatistic stores PMI average-based metrics.
type AverageStatistic struct {
	Name         string `xml:"name,attr"`
	Count        string `xml:"count,attr"`
	Total        string `xml:"total,attr"`
	Mean         string `xml:"mean,attr"`
	Min          string `xml:"min,attr"`
	Max          string `xml:"max,attr"`
	SumOfSquares string `xml:"sumOfSquares,attr"`
	Unit         string `xml:"unit,attr"`
}
