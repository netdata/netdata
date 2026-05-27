// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	catosdk "github.com/catonetworks/cato-go-sdk"
	catomodels "github.com/catonetworks/cato-go-sdk/models"
)

type apiClient interface {
	LookupSites(ctx context.Context, accountID string, limit, from int64) (*catosdk.EntityLookup, error)
	AccountSnapshot(ctx context.Context, accountID string, siteIDs []string) (*catosdk.AccountSnapshot, error)
	AccountMetrics(ctx context.Context, accountID string, siteIDs []string, timeFrame string, buckets int64, groupInterfaces *bool) (*catosdk.AccountMetrics, error)
	EventsFeed(ctx context.Context, accountID string, marker *string) (*catosdk.EventsFeed, error)
	SiteBgpStatus(ctx context.Context, accountID, siteID string) ([]*catosdk.SiteBgpStatusResult, error)
}

type sdkAPIClient struct {
	client  *catosdk.Client
	raw     rawGraphQLClient
	retry   RetryConfig
	sleep   func(context.Context, time.Duration) error
	statsMu sync.Mutex
	stats   apiStats
}

type rawGraphQLClient struct {
	url        string
	apiKey     string
	headers    map[string]string
	httpClient *http.Client
}

type apiStatsProvider interface {
	APIStats() apiStats
}

type apiStats struct {
	Retries map[string]apiRetryStats
}

type apiRetryStats struct {
	RateLimit int64
	Transient int64
}

func newSDKAPIClient(cfg Config, httpClient *http.Client) (apiClient, error) {
	headers := catoRequestHeaders(cfg.Headers)

	client, err := catosdk.New(cfg.URL, cfg.APIKey, cfg.AccountID, httpClient, headers)
	if err != nil {
		return nil, err
	}

	return &sdkAPIClient{
		client: client,
		raw: rawGraphQLClient{
			url:        cfg.URL,
			apiKey:     cfg.APIKey,
			headers:    headers,
			httpClient: httpClient,
		},
		retry: cfg.Retry,
		sleep: sleepContext,
		stats: apiStats{Retries: make(map[string]apiRetryStats)},
	}, nil
}

func catoRequestHeaders(src map[string]string) map[string]string {
	headers := make(map[string]string, len(src)+1)
	for key, value := range src {
		if isCatoReservedHeader(key) {
			continue
		}
		headers[key] = value
	}
	if !hasCatoHeader(headers, "User-Agent") {
		headers["User-Agent"] = "Netdata go.d.plugin cato_networks"
	}
	return headers
}

func hasCatoHeader(headers map[string]string, key string) bool {
	want := http.CanonicalHeaderKey(strings.TrimSpace(key))
	for existing := range headers {
		if http.CanonicalHeaderKey(strings.TrimSpace(existing)) == want {
			return true
		}
	}
	return false
}

func (c *sdkAPIClient) LookupSites(ctx context.Context, accountID string, limit, from int64) (*catosdk.EntityLookup, error) {
	var res *catosdk.EntityLookup
	err := c.withRetry(ctx, "entityLookup", func() error {
		v, err := c.client.EntityLookup(ctx, accountID, catomodels.EntityTypeSite, &limit, &from, nil, nil, nil, nil, nil, nil)
		res = v
		return err
	})
	return res, err
}

func (c *sdkAPIClient) AccountSnapshot(ctx context.Context, accountID string, siteIDs []string) (*catosdk.AccountSnapshot, error) {
	var res *catosdk.AccountSnapshot
	err := c.withRetry(ctx, "accountSnapshot", func() error {
		v, err := c.client.AccountSnapshot(ctx, siteIDs, nil, &accountID)
		if err != nil && isAccountSnapshotEnumDecodeError(err) {
			v, err = c.raw.AccountSnapshot(ctx, accountID, siteIDs)
		}
		res = v
		return err
	})
	return res, err
}

func (c *sdkAPIClient) AccountMetrics(ctx context.Context, accountID string, siteIDs []string, timeFrame string, buckets int64, groupInterfaces *bool) (*catosdk.AccountMetrics, error) {
	labels := []catomodels.TimeseriesMetricType{
		catomodels.TimeseriesMetricTypeBytesUpstreamMax,
		catomodels.TimeseriesMetricTypeBytesDownstreamMax,
		catomodels.TimeseriesMetricTypeLostUpstreamPcnt,
		catomodels.TimeseriesMetricTypeLostDownstreamPcnt,
		catomodels.TimeseriesMetricTypeJitterUpstream,
		catomodels.TimeseriesMetricTypeJitterDownstream,
		catomodels.TimeseriesMetricTypePacketsDiscardedUpstream,
		catomodels.TimeseriesMetricTypePacketsDiscardedDownstream,
		catomodels.TimeseriesMetricTypeRtt,
		catomodels.TimeseriesMetricTypeLastMileLatency,
		catomodels.TimeseriesMetricTypeLastMilePacketLoss,
	}

	var res *catosdk.AccountMetrics
	err := c.withRetry(ctx, "accountMetrics", func() error {
		v, err := c.client.AccountMetrics(ctx,
			nil, nil, nil, &buckets,
			labels,
			nil, nil, nil, nil, nil, nil, nil, nil,
			siteIDs,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, &accountID, nil, timeFrame, groupInterfaces, nil,
		)
		res = v
		return err
	})
	return res, err
}

func (c *sdkAPIClient) EventsFeed(ctx context.Context, accountID string, marker *string) (*catosdk.EventsFeed, error) {
	fields := []catomodels.EventFieldName{
		catomodels.EventFieldNameEventID,
		catomodels.EventFieldNameEventType,
		catomodels.EventFieldNameEventSubType,
		catomodels.EventFieldNameSeverity,
		catomodels.EventFieldNameStatus,
		catomodels.EventFieldNamePopName,
		catomodels.EventFieldNameSrcSiteID,
		catomodels.EventFieldNameSrcSiteName,
		catomodels.EventFieldNameDestSiteID,
		catomodels.EventFieldNameDestSiteName,
	}

	var res *catosdk.EventsFeed
	err := c.withRetry(ctx, "eventsFeed", func() error {
		v, err := c.client.EventsFeed(ctx, fields, []string{accountID}, nil, marker)
		res = v
		return err
	})
	return res, err
}

func (c *sdkAPIClient) SiteBgpStatus(ctx context.Context, accountID, siteID string) ([]*catosdk.SiteBgpStatusResult, error) {
	input := catomodels.SiteBgpStatusInput{
		Site: &catomodels.SiteRefInput{
			By:    catomodels.ObjectRefByID,
			Input: siteID,
		},
	}

	var res []*catosdk.SiteBgpStatusResult
	err := c.withRetry(ctx, "siteBgpStatus", func() error {
		_, v, err := c.client.SiteBgpStatus(ctx, input, accountID)
		res = v
		return err
	})
	return res, err
}

func (c *sdkAPIClient) withRetry(ctx context.Context, name string, fn func() error) error {
	attempts := c.retry.Attempts
	if attempts <= 0 {
		attempts = 1
	}

	var err error
	for attempt := 1; attempt <= attempts; attempt++ {
		if err = fn(); err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if attempt == attempts || !isRetryableCatoError(ctx, err) {
			return fmt.Errorf("%s: %w", name, err)
		}

		c.recordRetry(name, err)
		wait := retryWait(c.retry.WaitMin.Duration(), c.retry.WaitMax.Duration(), attempt)
		if sleepErr := c.sleep(ctx, wait); sleepErr != nil {
			return sleepErr
		}
	}

	return err
}

func (c *sdkAPIClient) recordRetry(name string, err error) {
	c.statsMu.Lock()
	defer c.statsMu.Unlock()

	if c.stats.Retries == nil {
		c.stats.Retries = make(map[string]apiRetryStats)
	}

	stats := c.stats.Retries[name]
	if isRateLimitCatoError(err) {
		stats.RateLimit++
	} else {
		stats.Transient++
	}
	c.stats.Retries[name] = stats
}

func (c *sdkAPIClient) APIStats() apiStats {
	c.statsMu.Lock()
	defer c.statsMu.Unlock()

	out := apiStats{Retries: make(map[string]apiRetryStats, len(c.stats.Retries))}
	for name, stats := range c.stats.Retries {
		out.Retries[name] = stats
	}
	return out
}

type rawGraphQLRequest struct {
	OperationName string         `json:"operationName,omitempty"`
	Query         string         `json:"query"`
	Variables     map[string]any `json:"variables,omitempty"`
}

type rawGraphQLError struct {
	Message string `json:"message"`
}

type rawAccountSnapshotResponse struct {
	Data          catosdk.AccountSnapshot `json:"data"`
	Errors        []rawGraphQLError       `json:"errors"`
	NetworkErrors []rawGraphQLError       `json:"networkErrors"`
	GraphQLErrors []rawGraphQLError       `json:"graphqlErrors"`
}

func (c rawGraphQLClient) AccountSnapshot(ctx context.Context, accountID string, siteIDs []string) (*catosdk.AccountSnapshot, error) {
	payload := rawGraphQLRequest{
		OperationName: "accountSnapshot",
		Query:         catosdk.AccountSnapshotDocument,
		Variables: map[string]any{
			"siteIDs":   siteIDs,
			"userIDs":   nil,
			"accountID": accountID,
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	for key, value := range c.headers {
		if isCatoReservedHeader(key) {
			continue
		}
		req.Header.Set(key, value)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("x-account-id", accountID)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("http status %d", resp.StatusCode)
	}

	var decoded rawAccountSnapshotResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, err
	}
	if msg := firstRawGraphQLError(decoded.Errors, decoded.NetworkErrors, decoded.GraphQLErrors); msg != "" {
		return nil, fmt.Errorf("graphql: %s", msg)
	}
	if decoded.Data.AccountSnapshot == nil {
		return nil, errors.New("graphql accountSnapshot returned no data")
	}

	return &decoded.Data, nil
}

func firstRawGraphQLError(groups ...[]rawGraphQLError) string {
	for _, group := range groups {
		for _, err := range group {
			if msg := strings.TrimSpace(err.Message); msg != "" {
				return msg
			}
		}
	}
	return ""
}

func isCatoReservedHeader(key string) bool {
	switch http.CanonicalHeaderKey(strings.TrimSpace(key)) {
	case "Content-Type", "X-Api-Key", "X-Account-Id":
		return true
	default:
		return false
	}
}

func isAccountSnapshotEnumDecodeError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "ConnectivityStatus") && strings.Contains(msg, "not a valid")
}

func retryWait(minWait, maxWait time.Duration, attempt int) time.Duration {
	if minWait <= 0 {
		minWait = time.Second
	}
	if maxWait <= 0 {
		maxWait = minWait
	}

	wait := minWait
	for range max(0, attempt-1) {
		wait *= 2
		if wait >= maxWait {
			return maxWait
		}
	}
	return wait
}

func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func isRetryableCatoError(ctx context.Context, err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return ctx == nil || ctx.Err() == nil
	}
	if ctx != nil && ctx.Err() != nil {
		return false
	}

	return isRateLimitCatoError(err) || isTransientCatoError(err)
}

func isRateLimitCatoError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return containsHTTPStatus(msg, "429") ||
		strings.Contains(msg, "too many requests") ||
		strings.Contains(msg, "rate limit") ||
		strings.Contains(msg, "rate-limit") ||
		strings.Contains(msg, "ratelimit")
}

func isTransientCatoError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return containsHTTPStatusRange(msg, 500, 599) ||
		strings.Contains(msg, "temporarily unavailable") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "i/o timeout") ||
		strings.Contains(msg, "server misbehaving") ||
		strings.Contains(msg, "eof")
}

func containsHTTPStatusRange(msg string, first, last int) bool {
	for code := first; code <= last; code++ {
		if containsHTTPStatus(msg, fmt.Sprint(code)) {
			return true
		}
	}
	return false
}
