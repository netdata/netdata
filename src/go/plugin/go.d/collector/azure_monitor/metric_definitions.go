// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	azcloud "github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
)

type definitionKey struct{ resourceID, namespace string }

// Keep the spelling returned by discovery for the wire request, but compare ARM identities case-insensitively.
func (k definitionKey) canonical() definitionKey {
	return definitionKey{strings.ToLower(k.resourceID), strings.ToLower(k.namespace)}
}

type metricAvailability struct{ TimeGrain string }

type metricDefinition struct {
	Name                      struct{ Value string }
	SupportedAggregationTypes []string
	MetricAvailabilities      []metricAvailability
}
type metricDefinitions map[string]metricDefinition

type metricDefinitionsClient struct{ *arm.Client }

func newMetricDefinitionsClient(cred azcore.TokenCredential, cloud azcloud.Configuration) (*metricDefinitionsClient, error) {
	opts := armClientOptions{Cloud: cloud}.toARM()
	opts.ClientOptions = withAzureRequestTransport(opts.ClientOptions)
	client, err := arm.NewClient("azure_monitor", "v1.0.0", cred, opts)
	if err != nil {
		return nil, err
	}
	return &metricDefinitionsClient{client}, nil
}

func (c *metricDefinitionsClient) list(ctx context.Context, key definitionKey) (metricDefinitions, error) {
	endpoint, err := url.Parse(c.Endpoint())
	if err != nil {
		return nil, err
	}
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/" + strings.Trim(key.resourceID, "/") + "/providers/Microsoft.Insights/metricDefinitions"
	endpoint.RawQuery = url.Values{"api-version": {"2023-10-01"}, "metricnamespace": {key.namespace}}.Encode()
	// The collector schedules retries across polls. Return throttling responses
	// immediately so server deadlines survive the current recovery budget.
	ctx = policy.WithRetryOptions(ctx, policy.RetryOptions{MaxRetries: -1})
	ctx, op := queryOperationContext(ctx)
	req, err := runtime.NewRequest(ctx, http.MethodGet, endpoint.String())
	if err != nil {
		return nil, err
	}
	resp, err := c.Pipeline().Do(req)
	if err != nil {
		return nil, preserveCapturedThrottle(err, op.captured)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, preserveCapturedThrottle(runtime.NewResponseError(resp), op.captured)
	}
	var result struct{ Value []metricDefinition }
	if err := runtime.UnmarshalAsJSON(resp, &result); err != nil {
		return nil, err
	}
	if result.Value == nil {
		return nil, errors.New("missing value in Azure metric definitions response")
	}
	defs := make(metricDefinitions, len(result.Value))
	for _, d := range result.Value {
		if d.Name.Value == "" {
			return nil, errors.New("unnamed Azure metric definition")
		}
		defs[stringsLowerTrim(d.Name.Value)] = d
	}
	return defs, nil
}

func (defs metricDefinitions) supports(name, aggregation, grain string) bool {
	d, ok := defs[stringsLowerTrim(name)]
	if !ok {
		return false
	}
	if len(d.SupportedAggregationTypes) > 0 && !slices.ContainsFunc(d.SupportedAggregationTypes, func(value string) bool {
		return strings.EqualFold(value, aggregation)
	}) {
		return false
	}
	return len(d.MetricAvailabilities) == 0 || slices.ContainsFunc(d.MetricAvailabilities, func(a metricAvailability) bool {
		return strings.EqualFold(a.TimeGrain, grain)
	})
}
