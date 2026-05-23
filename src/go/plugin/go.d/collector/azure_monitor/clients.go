// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	azcloud "github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/monitor/query/azmetrics"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resourcegraph/armresourcegraph"
)

type resourceGraphClient interface {
	Resources(ctx context.Context, query armresourcegraph.QueryRequest, options *armresourcegraph.ClientResourcesOptions) (armresourcegraph.ClientResourcesResponse, error)
}

type metricsQueryClient interface {
	QueryResources(ctx context.Context, subscriptionID string, metricNamespace string, metricNames []string, resourceIDs azmetrics.ResourceIDList, options *azmetrics.QueryResourcesOptions) (azmetrics.QueryResourcesResponse, error)
}

type armClientOptions struct {
	Cloud azcloud.Configuration
}

func (a armClientOptions) toARM() *arm.ClientOptions {
	return &arm.ClientOptions{ClientOptions: azcore.ClientOptions{Cloud: a.Cloud}}
}

func cloudConfigFromName(name string) (azcloud.Configuration, error) {
	switch stringsLowerTrim(name) {
	case cloudPublic, "":
		return azcloud.AzurePublic, nil
	case cloudGovernment:
		return azcloud.AzureGovernment, nil
	case cloudChina:
		return azcloud.AzureChina, nil
	default:
		return azcloud.Configuration{}, fmt.Errorf("unsupported cloud %q", name)
	}
}
