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

	catosdk "github.com/catonetworks/cato-go-sdk"
	catomodels "github.com/catonetworks/cato-go-sdk/models"
)

type apiClient interface {
	Probe(ctx context.Context, accountID string) error
	LookupSites(ctx context.Context, accountID string, limit, from int64) (*catosdk.EntityLookup, error)
	AccountSnapshot(ctx context.Context, accountID string, siteIDs []string) (*catosdk.AccountSnapshot, error)
	AccountMetrics(ctx context.Context, accountID string, siteIDs []string, timeFrame string, buckets int64, groupInterfaces *bool) (*catosdk.AccountMetrics, error)
	SiteBgpStatus(ctx context.Context, accountID, siteID string) ([]*catosdk.SiteBgpStatusResult, error)
}

type sdkAPIClient struct {
	client *catosdk.Client
	raw    rawGraphQLClient
}

type rawGraphQLClient struct {
	url        string
	apiKey     string
	headers    map[string]string
	httpClient *http.Client
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

func (c *sdkAPIClient) Probe(ctx context.Context, accountID string) error {
	limit := int64(1)
	from := int64(0)
	_, err := c.client.EntityLookup(ctx, accountID, catomodels.EntityTypeSite, &limit, &from, nil, nil, nil, nil, nil, nil)
	if err != nil {
		return fmt.Errorf("entityLookup: %w", err)
	}
	return nil
}

func (c *sdkAPIClient) LookupSites(ctx context.Context, accountID string, limit, from int64) (*catosdk.EntityLookup, error) {
	res, err := c.client.EntityLookup(ctx, accountID, catomodels.EntityTypeSite, &limit, &from, nil, nil, nil, nil, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("entityLookup: %w", err)
	}
	return res, nil
}

func (c *sdkAPIClient) AccountSnapshot(ctx context.Context, accountID string, siteIDs []string) (*catosdk.AccountSnapshot, error) {
	res, err := c.client.AccountSnapshot(ctx, siteIDs, nil, &accountID)
	if err != nil && isAccountSnapshotEnumDecodeError(err) {
		res, err = c.raw.AccountSnapshot(ctx, accountID, siteIDs)
	}
	if err != nil {
		return nil, fmt.Errorf("accountSnapshot: %w", err)
	}
	return res, nil
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

	res, err := c.client.AccountMetrics(ctx,
		nil, nil, nil, &buckets,
		labels,
		nil, nil, nil, nil, nil, nil, nil, nil,
		siteIDs,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		nil, nil, &accountID, nil, timeFrame, groupInterfaces, nil,
	)
	if err != nil {
		return nil, fmt.Errorf("accountMetrics: %w", err)
	}
	return res, nil
}

func (c *sdkAPIClient) SiteBgpStatus(ctx context.Context, accountID, siteID string) ([]*catosdk.SiteBgpStatusResult, error) {
	input := catomodels.SiteBgpStatusInput{
		Site: &catomodels.SiteRefInput{
			By:    catomodels.ObjectRefByID,
			Input: siteID,
		},
	}

	_, res, err := c.client.SiteBgpStatus(ctx, input, accountID)
	if err != nil {
		return nil, fmt.Errorf("siteBgpStatus: %w", err)
	}
	return res, nil
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
