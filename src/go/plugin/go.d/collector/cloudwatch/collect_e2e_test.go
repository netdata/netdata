// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/awsauth"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/cwprofiles"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/cwquery"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
)

// TestCollect_E2E drives the whole collect pipeline for the REAL stock profiles:
// mock ListMetrics + GetMetricData -> discovery -> dimension filter -> query plan ->
// GetMetricData -> observe -> metrix store, then asserts BOTH the exact produced
// series (name, labels, value) AND chart coverage (every produced series flows into
// a chart, every resolvable chart dimension materializes). Each scenario is scoped
// to one profile via exact mode, so it exercises the shipped profile it names.
//
// Fixtures are built in-code as aws-sdk-go-v2 types. GetMetricData answers are keyed
// on the request's MetricStat (namespace, metric, statistic, dimensions) and echoed
// back under the query's synthetic Id, so fixtures are authored by metric/statistic
// rather than fragile q<N> ids. A key absent from the values map, or a NaN value, is
// dropped by the collector as a gap.
func TestCollect_E2E(t *testing.T) {
	const account = "000000000000"

	scenarios := map[string]e2eScenario{
		"billing total uses its static Currency grain": {
			profiles: []string{"billing_total"},
			gmd: map[string]float64{
				e2eKey("AWS/Billing", "EstimatedCharges", "Maximum", "Currency", "USD"): 123.45,
			},
			wantSeries: map[string]metrix.SampleValue{
				`billing_total.estimated_charges_maximum{account_id="000000000000",region="us-east-1"}`: 123.45,
			},
		},
		"billing total no-data is a successful gauge gap": {
			profiles:   []string{"billing_total"},
			wantSeries: map[string]metrix.SampleValue{},
		},
		"ec2 single dimension, rate sums normalized by effective period": {
			profiles: []string{"ec2"},
			listMetrics: map[string][]cwtypes.Metric{
				"AWS/EC2": {
					mkMetric("CPUUtilization", "InstanceId", "i-1"),
					mkMetric("NetworkIn", "InstanceId", "i-1"),
					mkMetric("NetworkOut", "InstanceId", "i-1"),
					mkMetric("DiskReadOps", "InstanceId", "i-1"),
					mkMetric("DiskWriteOps", "InstanceId", "i-1"),
					mkMetric("StatusCheckFailed", "InstanceId", "i-1"),
				},
			},
			gmd: map[string]float64{
				e2eKey("AWS/EC2", "CPUUtilization", "Average", "InstanceId", "i-1"):    3.2,
				e2eKey("AWS/EC2", "NetworkIn", "Sum", "InstanceId", "i-1"):             1500,
				e2eKey("AWS/EC2", "NetworkOut", "Sum", "InstanceId", "i-1"):            900,
				e2eKey("AWS/EC2", "DiskReadOps", "Sum", "InstanceId", "i-1"):           10,
				e2eKey("AWS/EC2", "DiskWriteOps", "Sum", "InstanceId", "i-1"):          20,
				e2eKey("AWS/EC2", "StatusCheckFailed", "Maximum", "InstanceId", "i-1"): 0,
			},
			wantSeries: map[string]metrix.SampleValue{
				`ec2.cpu_utilization_average{account_id="000000000000",instance_id="i-1",region="us-east-1"}`:     3.2,
				`ec2.network_in_sum{account_id="000000000000",instance_id="i-1",region="us-east-1"}`:              1500.0 / 300,
				`ec2.network_out_sum{account_id="000000000000",instance_id="i-1",region="us-east-1"}`:             900.0 / 300,
				`ec2.disk_read_ops_sum{account_id="000000000000",instance_id="i-1",region="us-east-1"}`:           10.0 / 300,
				`ec2.disk_write_ops_sum{account_id="000000000000",instance_id="i-1",region="us-east-1"}`:          20.0 / 300,
				`ec2.status_check_failed_maximum{account_id="000000000000",instance_id="i-1",region="us-east-1"}`: 0,
			},
		},
		"PrivateLink endpoint keeps Average gauges and normalizes Sum totals": {
			profiles: []string{"privatelink_endpoint"},
			listMetrics: map[string][]cwtypes.Metric{
				"AWS/PrivateLinkEndpoints": {
					mkMetric("ActiveConnections", "Endpoint Type", "Interface", "Service Name", "service-1", "VPC Endpoint Id", "vpce-1", "VPC Id", "vpc-1"),
					mkMetric("BytesProcessed", "Endpoint Type", "Interface", "Service Name", "service-1", "VPC Endpoint Id", "vpce-1", "VPC Id", "vpc-1"),
					mkMetric("NewConnections", "Endpoint Type", "Interface", "Service Name", "service-1", "VPC Endpoint Id", "vpce-1", "VPC Id", "vpc-1"),
					mkMetric("PacketsDropped", "Endpoint Type", "Interface", "Service Name", "service-1", "VPC Endpoint Id", "vpce-1", "VPC Id", "vpc-1"),
					mkMetric("RstPacketsReceived", "Endpoint Type", "Interface", "Service Name", "service-1", "VPC Endpoint Id", "vpce-1", "VPC Id", "vpc-1"),
				},
			},
			gmd: map[string]float64{
				e2eKey("AWS/PrivateLinkEndpoints", "ActiveConnections", "Average", "Endpoint Type", "Interface", "Service Name", "service-1", "VPC Endpoint Id", "vpce-1", "VPC Id", "vpc-1"): 17,
				e2eKey("AWS/PrivateLinkEndpoints", "BytesProcessed", "Average", "Endpoint Type", "Interface", "Service Name", "service-1", "VPC Endpoint Id", "vpce-1", "VPC Id", "vpc-1"):    100,
				e2eKey("AWS/PrivateLinkEndpoints", "BytesProcessed", "Sum", "Endpoint Type", "Interface", "Service Name", "service-1", "VPC Endpoint Id", "vpce-1", "VPC Id", "vpc-1"):        1500,
				e2eKey("AWS/PrivateLinkEndpoints", "NewConnections", "Average", "Endpoint Type", "Interface", "Service Name", "service-1", "VPC Endpoint Id", "vpce-1", "VPC Id", "vpc-1"):    2,
				e2eKey("AWS/PrivateLinkEndpoints", "NewConnections", "Sum", "Endpoint Type", "Interface", "Service Name", "service-1", "VPC Endpoint Id", "vpce-1", "VPC Id", "vpc-1"):        600,
				e2eKey("AWS/PrivateLinkEndpoints", "PacketsDropped", "Sum", "Endpoint Type", "Interface", "Service Name", "service-1", "VPC Endpoint Id", "vpce-1", "VPC Id", "vpc-1"):        30,
				e2eKey("AWS/PrivateLinkEndpoints", "RstPacketsReceived", "Sum", "Endpoint Type", "Interface", "Service Name", "service-1", "VPC Endpoint Id", "vpce-1", "VPC Id", "vpc-1"):    60,
			},
			wantSeries: map[string]metrix.SampleValue{
				`privatelink_endpoint.active_connections_average{account_id="000000000000",endpoint_type="Interface",region="us-east-1",service_name="service-1",vpc_endpoint_id="vpce-1",vpc_id="vpc-1"}`: 17,
				`privatelink_endpoint.bytes_processed_average{account_id="000000000000",endpoint_type="Interface",region="us-east-1",service_name="service-1",vpc_endpoint_id="vpce-1",vpc_id="vpc-1"}`:    100,
				`privatelink_endpoint.bytes_processed_sum{account_id="000000000000",endpoint_type="Interface",region="us-east-1",service_name="service-1",vpc_endpoint_id="vpce-1",vpc_id="vpc-1"}`:        1500.0 / 300,
				`privatelink_endpoint.new_connections_average{account_id="000000000000",endpoint_type="Interface",region="us-east-1",service_name="service-1",vpc_endpoint_id="vpce-1",vpc_id="vpc-1"}`:    2,
				`privatelink_endpoint.new_connections_sum{account_id="000000000000",endpoint_type="Interface",region="us-east-1",service_name="service-1",vpc_endpoint_id="vpce-1",vpc_id="vpc-1"}`:        600.0 / 300,
				`privatelink_endpoint.packets_dropped_sum{account_id="000000000000",endpoint_type="Interface",region="us-east-1",service_name="service-1",vpc_endpoint_id="vpce-1",vpc_id="vpc-1"}`:        30.0 / 300,
				`privatelink_endpoint.rst_packets_received_sum{account_id="000000000000",endpoint_type="Interface",region="us-east-1",service_name="service-1",vpc_endpoint_id="vpce-1",vpc_id="vpc-1"}`:   60.0 / 300,
			},
		},
		"PrivateLink service exposes connections, endpoints, and normalized traffic": {
			profiles: []string{"privatelink_service"},
			listMetrics: map[string][]cwtypes.Metric{
				"AWS/PrivateLinkServices": {
					mkMetric("ActiveConnections", "Service Id", "vpce-svc-1"),
					mkMetric("BytesProcessed", "Service Id", "vpce-svc-1"),
					mkMetric("EndpointsCount", "Service Id", "vpce-svc-1"),
					mkMetric("NewConnections", "Service Id", "vpce-svc-1"),
					mkMetric("RstPacketsSent", "Service Id", "vpce-svc-1"),
				},
			},
			gmd: map[string]float64{
				e2eKey("AWS/PrivateLinkServices", "ActiveConnections", "Average", "Service Id", "vpce-svc-1"): 17,
				e2eKey("AWS/PrivateLinkServices", "BytesProcessed", "Average", "Service Id", "vpce-svc-1"):    100,
				e2eKey("AWS/PrivateLinkServices", "BytesProcessed", "Sum", "Service Id", "vpce-svc-1"):        1500,
				e2eKey("AWS/PrivateLinkServices", "EndpointsCount", "Average", "Service Id", "vpce-svc-1"):    3,
				e2eKey("AWS/PrivateLinkServices", "NewConnections", "Average", "Service Id", "vpce-svc-1"):    2,
				e2eKey("AWS/PrivateLinkServices", "NewConnections", "Sum", "Service Id", "vpce-svc-1"):        600,
				e2eKey("AWS/PrivateLinkServices", "RstPacketsSent", "Average", "Service Id", "vpce-svc-1"):    1,
				e2eKey("AWS/PrivateLinkServices", "RstPacketsSent", "Sum", "Service Id", "vpce-svc-1"):        60,
			},
			wantSeries: map[string]metrix.SampleValue{
				`privatelink_service.active_connections_average{account_id="000000000000",region="us-east-1",service_id="vpce-svc-1"}`: 17,
				`privatelink_service.bytes_processed_average{account_id="000000000000",region="us-east-1",service_id="vpce-svc-1"}`:    100,
				`privatelink_service.bytes_processed_sum{account_id="000000000000",region="us-east-1",service_id="vpce-svc-1"}`:        1500.0 / 300,
				`privatelink_service.endpoints_count_average{account_id="000000000000",region="us-east-1",service_id="vpce-svc-1"}`:    3,
				`privatelink_service.new_connections_average{account_id="000000000000",region="us-east-1",service_id="vpce-svc-1"}`:    2,
				`privatelink_service.new_connections_sum{account_id="000000000000",region="us-east-1",service_id="vpce-svc-1"}`:        600.0 / 300,
				`privatelink_service.rst_packets_sent_average{account_id="000000000000",region="us-east-1",service_id="vpce-svc-1"}`:   1,
				`privatelink_service.rst_packets_sent_sum{account_id="000000000000",region="us-east-1",service_id="vpce-svc-1"}`:       60.0 / 300,
			},
		},
		"PrivateLink service no-data keeps inactivity counters at zero": {
			profiles: []string{"privatelink_service"},
			listMetrics: map[string][]cwtypes.Metric{
				"AWS/PrivateLinkServices": {
					mkMetric("EndpointsCount", "Service Id", "vpce-svc-1"),
				},
			},
			wantSeries: map[string]metrix.SampleValue{
				`privatelink_service.bytes_processed_sum{account_id="000000000000",region="us-east-1",service_id="vpce-svc-1"}`:     0,
				`privatelink_service.endpoints_count_average{account_id="000000000000",region="us-east-1",service_id="vpce-svc-1"}`: 0,
				`privatelink_service.new_connections_sum{account_id="000000000000",region="us-east-1",service_id="vpce-svc-1"}`:     0,
				`privatelink_service.rst_packets_sent_sum{account_id="000000000000",region="us-east-1",service_id="vpce-svc-1"}`:    0,
			},
		},
		"s3 multi-dimension identity, daily period": {
			profiles: []string{"s3"},
			listMetrics: map[string][]cwtypes.Metric{
				"AWS/S3": {
					mkMetric("BucketSizeBytes", "BucketName", "b1", "StorageType", "StandardStorage"),
					mkMetric("NumberOfObjects", "BucketName", "b1", "StorageType", "AllStorageTypes"),
				},
			},
			// Each storage type is a distinct instance; the cross-product queries gap.
			gmd: map[string]float64{
				e2eKey("AWS/S3", "BucketSizeBytes", "Average", "BucketName", "b1", "StorageType", "StandardStorage"): 1048576,
				e2eKey("AWS/S3", "NumberOfObjects", "Average", "BucketName", "b1", "StorageType", "AllStorageTypes"): 42,
			},
			wantSeries: map[string]metrix.SampleValue{
				`s3.bucket_size_bytes_average{account_id="000000000000",bucket_name="b1",region="us-east-1",storage_type="StandardStorage"}`: 1048576,
				`s3.number_of_objects_average{account_id="000000000000",bucket_name="b1",region="us-east-1",storage_type="AllStorageTypes"}`: 42,
			},
		},
		"lambda multi-statistic fan-out": {
			profiles: []string{"lambda"},
			listMetrics: map[string][]cwtypes.Metric{
				"AWS/Lambda": {
					mkMetric("Invocations", "FunctionName", "fn-1"),
					mkMetric("Errors", "FunctionName", "fn-1"),
					mkMetric("Throttles", "FunctionName", "fn-1"),
					mkMetric("Duration", "FunctionName", "fn-1"),
				},
			},
			gmd: map[string]float64{
				e2eKey("AWS/Lambda", "Invocations", "Sum", "FunctionName", "fn-1"):  100,
				e2eKey("AWS/Lambda", "Errors", "Sum", "FunctionName", "fn-1"):       2,
				e2eKey("AWS/Lambda", "Throttles", "Sum", "FunctionName", "fn-1"):    1,
				e2eKey("AWS/Lambda", "Duration", "Average", "FunctionName", "fn-1"): 120.5,
				e2eKey("AWS/Lambda", "Duration", "Maximum", "FunctionName", "fn-1"): 350,
				e2eKey("AWS/Lambda", "Duration", "p90", "FunctionName", "fn-1"):     200,
			},
			wantSeries: map[string]metrix.SampleValue{
				`lambda.invocations_sum{account_id="000000000000",function_name="fn-1",region="us-east-1"}`:  100.0 / 300,
				`lambda.errors_sum{account_id="000000000000",function_name="fn-1",region="us-east-1"}`:       2.0 / 300,
				`lambda.throttles_sum{account_id="000000000000",function_name="fn-1",region="us-east-1"}`:    1.0 / 300,
				`lambda.duration_average{account_id="000000000000",function_name="fn-1",region="us-east-1"}`: 120.5,
				`lambda.duration_maximum{account_id="000000000000",function_name="fn-1",region="us-east-1"}`: 350,
				`lambda.duration_p90{account_id="000000000000",function_name="fn-1",region="us-east-1"}`:     200,
			},
		},
		"msk broker partition health metrics": {
			profiles: []string{"msk"},
			listMetrics: map[string][]cwtypes.Metric{
				"AWS/Kafka": {
					mkMetric("UnderReplicatedPartitions", "Cluster Name", "cluster-a", "Broker ID", "1"),
					mkMetric("UnderMinIsrPartitionCount", "Cluster Name", "cluster-a", "Broker ID", "1"),
				},
			},
			gmd: map[string]float64{
				e2eKey("AWS/Kafka", "UnderReplicatedPartitions", "Maximum", "Cluster Name", "cluster-a", "Broker ID", "1"): 2,
				e2eKey("AWS/Kafka", "UnderMinIsrPartitionCount", "Maximum", "Cluster Name", "cluster-a", "Broker ID", "1"): 1,
			},
			wantSeries: map[string]metrix.SampleValue{
				`msk.under_replicated_partitions_maximum{account_id="000000000000",broker_id="1",cluster_name="cluster-a",region="us-east-1"}`:   2,
				`msk.under_min_isr_partition_count_maximum{account_id="000000000000",broker_id="1",cluster_name="cluster-a",region="us-east-1"}`: 1,
			},
		},
		"alb multi-granularity dimension filter keeps only {LoadBalancer}": {
			profiles: []string{"alb"},
			listMetrics: map[string][]cwtypes.Metric{
				"AWS/ApplicationELB": {
					mkMetric("RequestCount", "LoadBalancer", "app/lb1/aaa"),                                   // keep
					mkMetric("RequestCount", "LoadBalancer", "app/lb1/aaa", "TargetGroup", "tg/x/1"),          // reject (extra dim)
					mkMetric("RequestCount", "LoadBalancer", "app/lb1/aaa", "AvailabilityZone", "us-east-1a"), // reject
					mkMetric("RequestCount", "LoadBalancer", "app/lb2/bbb"),                                   // keep (2nd LB)
				},
			},
			// Only RequestCount is served; the profile's other rate/sum metrics
			// record 0 (no activity) and its gauges gap. The dimension filter is
			// proven by the keys: only {LoadBalancer} instances survive, no
			// target_group / availability_zone fan-out.
			gmd: map[string]float64{
				e2eKey("AWS/ApplicationELB", "RequestCount", "Sum", "LoadBalancer", "app/lb1/aaa"): 50,
				e2eKey("AWS/ApplicationELB", "RequestCount", "Sum", "LoadBalancer", "app/lb2/bbb"): 70,
			},
			wantSeries: map[string]metrix.SampleValue{
				`alb.request_count_sum{account_id="000000000000",load_balancer="app/lb1/aaa",region="us-east-1"}`:             50.0 / 300,
				`alb.active_connection_count_sum{account_id="000000000000",load_balancer="app/lb1/aaa",region="us-east-1"}`:   0,
				`alb.http_code_target_2xx_sum{account_id="000000000000",load_balancer="app/lb1/aaa",region="us-east-1"}`:      0,
				`alb.http_code_target_3xx_sum{account_id="000000000000",load_balancer="app/lb1/aaa",region="us-east-1"}`:      0,
				`alb.http_code_target_4xx_sum{account_id="000000000000",load_balancer="app/lb1/aaa",region="us-east-1"}`:      0,
				`alb.http_code_target_5xx_sum{account_id="000000000000",load_balancer="app/lb1/aaa",region="us-east-1"}`:      0,
				`alb.http_code_elb_3xx_sum{account_id="000000000000",load_balancer="app/lb1/aaa",region="us-east-1"}`:         0,
				`alb.http_code_elb_4xx_sum{account_id="000000000000",load_balancer="app/lb1/aaa",region="us-east-1"}`:         0,
				`alb.http_code_elb_5xx_sum{account_id="000000000000",load_balancer="app/lb1/aaa",region="us-east-1"}`:         0,
				`alb.new_connection_count_sum{account_id="000000000000",load_balancer="app/lb1/aaa",region="us-east-1"}`:      0,
				`alb.processed_bytes_sum{account_id="000000000000",load_balancer="app/lb1/aaa",region="us-east-1"}`:           0,
				`alb.rejected_connection_count_sum{account_id="000000000000",load_balancer="app/lb1/aaa",region="us-east-1"}`: 0,
				`alb.request_count_sum{account_id="000000000000",load_balancer="app/lb2/bbb",region="us-east-1"}`:             70.0 / 300,
				`alb.active_connection_count_sum{account_id="000000000000",load_balancer="app/lb2/bbb",region="us-east-1"}`:   0,
				`alb.http_code_target_2xx_sum{account_id="000000000000",load_balancer="app/lb2/bbb",region="us-east-1"}`:      0,
				`alb.http_code_target_3xx_sum{account_id="000000000000",load_balancer="app/lb2/bbb",region="us-east-1"}`:      0,
				`alb.http_code_target_4xx_sum{account_id="000000000000",load_balancer="app/lb2/bbb",region="us-east-1"}`:      0,
				`alb.http_code_target_5xx_sum{account_id="000000000000",load_balancer="app/lb2/bbb",region="us-east-1"}`:      0,
				`alb.http_code_elb_3xx_sum{account_id="000000000000",load_balancer="app/lb2/bbb",region="us-east-1"}`:         0,
				`alb.http_code_elb_4xx_sum{account_id="000000000000",load_balancer="app/lb2/bbb",region="us-east-1"}`:         0,
				`alb.http_code_elb_5xx_sum{account_id="000000000000",load_balancer="app/lb2/bbb",region="us-east-1"}`:         0,
				`alb.new_connection_count_sum{account_id="000000000000",load_balancer="app/lb2/bbb",region="us-east-1"}`:      0,
				`alb.processed_bytes_sum{account_id="000000000000",load_balancer="app/lb2/bbb",region="us-east-1"}`:           0,
				`alb.rejected_connection_count_sum{account_id="000000000000",load_balancer="app/lb2/bbb",region="us-east-1"}`: 0,
			},
		},
		"no-data: gauges gap, rate/sum metrics become zero": {
			profiles: []string{"ec2"},
			listMetrics: map[string][]cwtypes.Metric{
				"AWS/EC2": {mkMetric("CPUUtilization", "InstanceId", "i-1")},
			},
			gmd: map[string]float64{
				e2eKey("AWS/EC2", "CPUUtilization", "Average", "InstanceId", "i-1"):    math.NaN(), // gauge, NaN -> gap
				e2eKey("AWS/EC2", "NetworkOut", "Sum", "InstanceId", "i-1"):            900,
				e2eKey("AWS/EC2", "DiskReadOps", "Sum", "InstanceId", "i-1"):           10,
				e2eKey("AWS/EC2", "DiskWriteOps", "Sum", "InstanceId", "i-1"):          20,
				e2eKey("AWS/EC2", "StatusCheckFailed", "Maximum", "InstanceId", "i-1"): 0,
				// NetworkIn is absent -> no datapoint -> rate/sum metric -> recorded as 0.
			},
			wantSeries: map[string]metrix.SampleValue{
				`ec2.network_out_sum{account_id="000000000000",instance_id="i-1",region="us-east-1"}`:             900.0 / 300,
				`ec2.disk_read_ops_sum{account_id="000000000000",instance_id="i-1",region="us-east-1"}`:           10.0 / 300,
				`ec2.disk_write_ops_sum{account_id="000000000000",instance_id="i-1",region="us-east-1"}`:          20.0 / 300,
				`ec2.status_check_failed_maximum{account_id="000000000000",instance_id="i-1",region="us-east-1"}`: 0,
				`ec2.network_in_sum{account_id="000000000000",instance_id="i-1",region="us-east-1"}`:              0,
				// cpu_utilization_average (gauge, NaN) gaps; network_in_sum (rate, no data) records 0.
			},
		},
	}

	for name, tc := range scenarios {
		t.Run(name, func(t *testing.T) {
			c := New()
			configureExactRule(c, []string{"us-east-1"}, tc.profiles)
			c.newSTSClient = func(aws.Config) stsClient { return &fakeSTS{account: account} }
			useFakeClient(c, &e2eCloudWatch{list: tc.listMetrics, values: tc.gmd})
			c.now = func() time.Time { return time.Unix(1_700_000_000, 0) }

			series, err := collecttest.CollectScalarSeries(c)
			require.NoError(t, err)

			assert.Equal(t, tc.wantSeries, workloadSeries(series))
			collecttest.AssertChartCoverage(t, c, collecttest.ChartCoverageExpectation{})
			collecttest.AssertChartTemplateSchema(t, c.ChartTemplateYAML())
		})
	}
}

type e2eScenario struct {
	profiles    []string
	listMetrics map[string][]cwtypes.Metric
	gmd         map[string]float64 // e2eKey -> value; NaN or absent => gap
	wantSeries  map[string]metrix.SampleValue
}

// e2eCloudWatch is a fixture-driven fake serving both CloudWatch APIs. ListMetrics
// answers per namespace; GetMetricData answers per (namespace, metric, statistic,
// dimensions), echoing back each query's synthetic Id.
type e2eCloudWatch struct {
	mu       sync.Mutex
	list     map[string][]cwtypes.Metric
	values   map[string]float64
	listErr  error // when set, ListMetrics fails (discovery error path)
	getCalls int
}

func (f *e2eCloudWatch) ListMetrics(_ context.Context, in *cloudwatch.ListMetricsInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.ListMetricsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listErr != nil {
		return nil, f.listErr
	}
	return &cloudwatch.ListMetricsOutput{Metrics: f.list[aws.ToString(in.Namespace)]}, nil
}

func (f *e2eCloudWatch) GetMetricData(_ context.Context, in *cloudwatch.GetMetricDataInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricDataOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.getCalls++
	results := make([]cwtypes.MetricDataResult, 0, len(in.MetricDataQueries))
	for _, q := range in.MetricDataQueries {
		r := cwtypes.MetricDataResult{Id: q.Id, StatusCode: cwtypes.StatusCodeComplete}
		if q.MetricStat != nil {
			if v, ok := f.values[e2eKeyFromMetricStat(q.MetricStat)]; ok {
				r.Values = []float64{v}
				r.Timestamps = []time.Time{aws.ToTime(in.EndTime).Add(-time.Duration(aws.ToInt32(q.MetricStat.Period)) * time.Second)}
			}
		}
		results = append(results, r)
	}
	return &cloudwatch.GetMetricDataOutput{MetricDataResults: results}, nil
}

func (f *e2eCloudWatch) replaceValues(values map[string]float64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.values = values
}

func TestCollect_BillingTotalRetainsPriorMonthUntilLookbackExpires(t *testing.T) {
	const seriesKey = `billing_total.estimated_charges_maximum{account_id="000000000000",region="us-east-1"}`
	valueKey := e2eKey("AWS/Billing", "EstimatedCharges", "Maximum", "Currency", "USD")
	fake := &e2eCloudWatch{values: map[string]float64{valueKey: 123.45}}
	c := New()
	configureExactRule(c, []string{"us-east-1"}, []string{"billing_total"})
	c.newSTSClient = func(aws.Config) stsClient { return &fakeSTS{account: "000000000000"} }
	useFakeClient(c, fake)

	now := time.Date(2026, time.August, 1, 0, 5, 0, 0, time.UTC)
	c.now = func() time.Time { return now }
	series, err := collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	assert.Equal(t, metrix.SampleValue(123.45), series[seriesKey], "the first query observes the July 31 bucket")

	fake.replaceValues(nil)
	now = time.Date(2026, time.August, 1, 0, 15, 0, 0, time.UTC)
	series, err = collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	assert.Equal(t, metrix.SampleValue(123.45), series[seriesKey], "the August no-data result retains the July observation")

	now = time.Date(2026, time.August, 2, 0, 15, 0, 0, time.UTC)
	series, err = collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	assert.NotContains(t, series, seriesKey, "the July observation expires after leaving the 24h lookback")
	assert.Equal(t, 3, fake.getCalls)
}

type policyQueryRequest struct {
	period      int32
	windowStart time.Time
	windowEnd   time.Time
}

type sparsePolicyCloudWatch struct {
	mu       sync.Mutex
	requests map[string][]policyQueryRequest
}

func (f *sparsePolicyCloudWatch) ListMetrics(_ context.Context, _ *cloudwatch.ListMetricsInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.ListMetricsOutput, error) {
	return &cloudwatch.ListMetricsOutput{Metrics: []cwtypes.Metric{
		mkMetric("Invocations", "FunctionName", "fn-1"),
		mkMetric("Errors", "FunctionName", "fn-1"),
	}}, nil
}

func (f *sparsePolicyCloudWatch) GetMetricData(_ context.Context, in *cloudwatch.GetMetricDataInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricDataOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	results := make([]cwtypes.MetricDataResult, 0, len(in.MetricDataQueries))
	for _, query := range in.MetricDataQueries {
		metric := aws.ToString(query.MetricStat.Metric.MetricName)
		period := aws.ToInt32(query.MetricStat.Period)
		f.requests[metric] = append(f.requests[metric], policyQueryRequest{
			period: period, windowStart: aws.ToTime(in.StartTime), windowEnd: aws.ToTime(in.EndTime),
		})
		result := cwtypes.MetricDataResult{Id: query.Id, StatusCode: cwtypes.StatusCodeComplete}
		switch metric {
		case "Invocations":
			result.Values = []float64{float64(period)}
			result.Timestamps = []time.Time{aws.ToTime(in.EndTime).Add(-time.Duration(period) * time.Second)}
		case "Errors":
			if !aws.ToTime(in.EndTime).Before(time.Unix(1200, 0)) {
				result.Values = []float64{600}
				result.Timestamps = []time.Time{time.Unix(300, 0)}
			}
		}
		results = append(results, result)
	}
	return &cloudwatch.GetMetricDataOutput{MetricDataResults: results}, nil
}

func (f *sparsePolicyCloudWatch) requestsFor(metric string) []policyQueryRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.requests[metric])
}

func TestCollect_TwoRulesApplyIndependentQueryPolicies(t *testing.T) {
	defaults := false
	selector := &ProfileSelectorConfig{Defaults: &defaults, Include: []string{"lambda"}}
	cfg := validBaseConfig()
	cfg.Rules = []RuleConfig{
		{
			Name: "lambda-fast", Targets: []string{"base"}, Profiles: selector, Regions: []string{"us-east-1"},
			Metrics: []ProfileMetricSelectorConfig{{Profile: "lambda", Statistics: []string{"Sum"}, Include: []MetricSelectionConfig{{Name: "Invocations"}}}},
			Query:   &cwquery.Config{Period: longDuration(time.Minute), Lookback: longDuration(5 * time.Minute), PublicationDelay: longDuration(0)},
		},
		{
			Name: "lambda-sparse", Targets: []string{"base"}, Profiles: selector, Regions: []string{"us-east-1"},
			Metrics: []ProfileMetricSelectorConfig{{Profile: "lambda", Statistics: []string{"Sum"}, Include: []MetricSelectionConfig{{Name: "Errors"}}}},
			Query:   &cwquery.Config{Period: longDuration(5 * time.Minute), Lookback: longDuration(15 * time.Minute), PublicationDelay: longDuration(0)},
		},
	}

	fake := &sparsePolicyCloudWatch{requests: make(map[string][]policyQueryRequest)}
	c := New()
	c.Config = cfg
	useFakeClient(c, fake)
	c.newSTSClient = func(aws.Config) stsClient { return &fakeSTS{account: "000000000000"} }
	base := time.Unix(900, 0)

	c.now = func() time.Time { return base }
	first, err := collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	assert.Equal(t, metrix.SampleValue(1), seriesValue(t, first, `lambda.invocations_sum{`))
	assert.Equal(t, metrix.SampleValue(0), seriesValue(t, first, `lambda.errors_sum{`))

	c.now = func() time.Time { return base.Add(5 * time.Minute) }
	second, err := collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	assert.Equal(t, metrix.SampleValue(1), seriesValue(t, second, `lambda.invocations_sum{`))
	assert.Equal(t, metrix.SampleValue(2), seriesValue(t, second, `lambda.errors_sum{`),
		"the real late bucket replaces zero and uses the sparse rule's five-minute divisor")

	invocations := fake.requestsFor("Invocations")
	require.Len(t, invocations, 2)
	assert.Equal(t, int32(60), invocations[0].period)
	assert.Equal(t, int64(600), invocations[0].windowStart.Unix())
	assert.Equal(t, int64(900), invocations[0].windowEnd.Unix())
	assert.Equal(t, int32(60), invocations[1].period)
	assert.Equal(t, int64(900), invocations[1].windowStart.Unix())
	assert.Equal(t, int64(1200), invocations[1].windowEnd.Unix())

	errors := fake.requestsFor("Errors")
	require.Len(t, errors, 2)
	assert.Equal(t, int32(300), errors[0].period)
	assert.Equal(t, int64(0), errors[0].windowStart.Unix())
	assert.Equal(t, int64(900), errors[0].windowEnd.Unix())
	assert.Equal(t, int32(300), errors[1].period)
	assert.Equal(t, int64(300), errors[1].windowStart.Unix())
	assert.Equal(t, int64(1200), errors[1].windowEnd.Unix())
}

func TestCollect_OrderedRulesFirstTargetOwnsSameAccountSeries(t *testing.T) {
	const account = "000000000000"
	defaults := false
	cfg := validBaseConfig()
	cfg.Targets = []TargetConfig{
		{Name: "first", Credentials: "sdk_default"},
		{Name: "second", Credentials: "sdk_default"},
	}
	selector := &ProfileSelectorConfig{Defaults: &defaults, Include: []string{"ec2"}}
	cfg.Rules = []RuleConfig{
		{Name: "first-rule", Targets: []string{"first"}, Profiles: selector, Regions: []string{"us-east-1"}},
		{Name: "second-rule", Targets: []string{"second"}, Profiles: selector, Regions: []string{"us-east-1"}},
	}

	first := &e2eCloudWatch{
		list:   map[string][]cwtypes.Metric{"AWS/EC2": {mkMetric("CPUUtilization", "InstanceId", "i-1")}},
		values: map[string]float64{e2eKey("AWS/EC2", "CPUUtilization", "Average", "InstanceId", "i-1"): 5},
	}
	second := &e2eCloudWatch{
		list:   map[string][]cwtypes.Metric{"AWS/EC2": {mkMetric("CPUUtilization", "InstanceId", "i-1")}},
		values: map[string]float64{e2eKey("AWS/EC2", "CPUUtilization", "Average", "InstanceId", "i-1"): 9},
	}

	c := New()
	c.Config = cfg
	c.newAWSConfig = func(_ context.Context, identity awsauth.Identity, _ string) (aws.Config, error) {
		return aws.Config{Region: identity.Ref}, nil
	}
	c.newSTSClient = func(aws.Config) stsClient { return &fakeSTS{account: account} }
	c.newCloudWatchClient = func(cfg aws.Config) cloudwatchClient {
		if cfg.Region == "first" {
			return first
		}
		return second
	}
	c.now = func() time.Time { return time.Unix(1_700_000_000, 0) }

	series, err := collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	assert.Equal(t, metrix.SampleValue(5), series[`ec2.cpu_utilization_average{account_id="000000000000",instance_id="i-1",region="us-east-1"}`])
	assert.Equal(t, metrix.SampleValue(2), series[activityAPICallsMetric+`{account_id="000000000000",operation="list_metrics",region="us-east-1"}`],
		"same-account targets aggregate into one account/region activity series")
	assert.Equal(t, metrix.SampleValue(1), series[activityAPICallsMetric+`{account_id="000000000000",operation="get_metric_data",region="us-east-1"}`])
	assert.Equal(t, 1, first.getCalls)
	assert.Zero(t, second.getCalls, "the later rule's duplicate final series must not be queried")

	series, err = collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	assert.Equal(t, metrix.SampleValue(0), series[activityAPICallsMetric+`{account_id="000000000000",operation="list_metrics",region="us-east-1"}`])
	assert.Equal(t, metrix.SampleValue(0), series[activityAPICallsMetric+`{account_id="000000000000",operation="get_metric_data",region="us-east-1"}`])
	assert.Equal(t, metrix.SampleValue(0), series[activityMetricRequestsMetric+`{account_id="000000000000",region="us-east-1"}`])
	assert.Equal(t, metrix.SampleValue(0), series[activityQueriesMetric+`{account_id="000000000000",profile="ec2",region="us-east-1"}`])
	assert.Equal(t, 1, first.getCalls, "a cached interval must publish zero activity without making another query")
}

// e2eKey builds the fixture key from (namespace, metric, statistic) plus dimension
// name/value pairs. Dimensions are sorted so authoring order does not matter.
func e2eKey(namespace, metric, stat string, dimNameValue ...string) string {
	pairs := make([]string, 0, len(dimNameValue)/2)
	for i := 0; i+1 < len(dimNameValue); i += 2 {
		pairs = append(pairs, dimNameValue[i]+"="+dimNameValue[i+1])
	}
	sort.Strings(pairs)
	return strings.Join([]string{namespace, metric, stat, strings.Join(pairs, ",")}, "|")
}

func e2eKeyFromMetricStat(ms *cwtypes.MetricStat) string {
	var dimNameValue []string
	if ms.Metric != nil {
		for _, d := range ms.Metric.Dimensions {
			dimNameValue = append(dimNameValue, aws.ToString(d.Name), aws.ToString(d.Value))
		}
		return e2eKey(aws.ToString(ms.Metric.Namespace), aws.ToString(ms.Metric.MetricName), aws.ToString(ms.Stat), dimNameValue...)
	}
	return e2eKey("", "", aws.ToString(ms.Stat))
}

func seriesName(key string) string {
	if before, _, ok := strings.Cut(key, "{"); ok {
		return before
	}
	return key
}

// TestAllStockProfiles_PipelineChartComplete is the full-catalog sweep: for EVERY
// stock profile (including profiles disabled by default), it feeds a
// synthetic instance with a datapoint for every (metric, statistic), runs the real
// collect, and asserts (a) every profile's every active series is produced and
// (b) every produced series flows into a chart (AssertChartCoverage). Profiles that
// share a namespace (alb/alb_target, s3/s3_requests, dynamodb/dynamodb_operation)
// get profile-unique dimension values so they never collide.
func TestAllStockProfiles_PipelineChartComplete(t *testing.T) {
	const account = "000000000000"
	const region = "us-east-1"

	cat, err := cwprofiles.LoadFromDefaultDirs()
	require.NoError(t, err)
	profiles := cat.AllProfiles()
	require.NotEmpty(t, profiles)
	profileNames := make([]string, 0, len(profiles))

	list := map[string][]cwtypes.Metric{}
	values := map[string]float64{}
	wantNames := map[string]struct{}{}

	for _, rp := range profiles {
		profileNames = append(profileNames, rp.Name)
		prof := rp.Config
		require.NotEmptyf(t, prof.Metrics, "%s has no metrics", rp.Name)
		require.NotEmptyf(t, prof.Instance.Dimensions, "%s has no instance dimensions", rp.Name)

		var dimPairs []string
		for i, d := range prof.Instance.Dimensions {
			// A constant (match-and-query-only) dimension only matches its pinned
			// value, so the synthesized metric must carry that value or discovery
			// would reject it (fail-closed).
			value := fmt.Sprintf("%s-%d", rp.Name, i)
			if d.IsConstant() {
				value = *d.Constant
			}
			dimPairs = append(dimPairs, d.Name, value)
		}

		// A metric under the instance's dimensions so discovery finds it.
		list[prof.Namespace] = append(list[prof.Namespace], mkMetric(prof.Metrics[0].MetricName, dimPairs...))

		for _, m := range prof.Metrics {
			for _, stat := range m.Statistics {
				token := cwprofiles.NormalizeStatistic(stat)
				require.NotEmptyf(t, token, "%s.%s has bad statistic %q", rp.Name, m.ID, stat)
				values[e2eKey(prof.Namespace, m.MetricName, cwprofiles.StatString(token), dimPairs...)] = 1
				wantNames[cwprofiles.ExportedSeriesName(rp.Name, m.ID, token)] = struct{}{}
			}
		}
	}

	c := New()
	configureExactRule(c, []string{region}, profileNames)
	c.newSTSClient = func(aws.Config) stsClient { return &fakeSTS{account: account} }
	useFakeClient(c, &e2eCloudWatch{list: list, values: values})
	c.now = func() time.Time { return time.Unix(1_700_000_000, 0) }

	series, err := collecttest.CollectScalarSeries(c)
	require.NoError(t, err)

	gotNames := make(map[string]struct{}, len(series))
	for k := range workloadSeries(series) {
		gotNames[seriesName(k)] = struct{}{}
	}
	assert.Equal(t, wantNames, gotNames, "every stock profile's every active (metric, statistic) must produce a series")
	collecttest.AssertChartCoverage(t, c, collecttest.ChartCoverageExpectation{})
	collecttest.AssertChartTemplateSchema(t, c.ChartTemplateYAML())
}

// TestCollect_MultiRegion verifies that one instance discovered in several regions
// produces one series per region (region is part of the identity).
func TestCollect_MultiRegion(t *testing.T) {
	const account = "000000000000"

	c := New()
	configureExactRule(c, []string{"us-east-1", "eu-west-1"}, []string{"ec2"})
	c.newSTSClient = func(aws.Config) stsClient { return &fakeSTS{account: account} }
	useFakeClient(c, &e2eCloudWatch{
		list:   map[string][]cwtypes.Metric{"AWS/EC2": {mkMetric("CPUUtilization", "InstanceId", "i-1")}},
		values: map[string]float64{e2eKey("AWS/EC2", "CPUUtilization", "Average", "InstanceId", "i-1"): 5},
	})
	c.now = func() time.Time { return time.Unix(1_700_000_000, 0) }

	series, err := collecttest.CollectScalarSeries(c)
	require.NoError(t, err)

	// cpu is a gauge with data in both regions; the ec2 rate/sum metrics have no
	// data and record 0 per region; status_check_failed (gauge) gaps.
	assert.Equal(t, map[string]metrix.SampleValue{
		`ec2.cpu_utilization_average{account_id="000000000000",instance_id="i-1",region="us-east-1"}`: 5,
		`ec2.cpu_utilization_average{account_id="000000000000",instance_id="i-1",region="eu-west-1"}`: 5,
		`ec2.network_in_sum{account_id="000000000000",instance_id="i-1",region="us-east-1"}`:          0,
		`ec2.network_in_sum{account_id="000000000000",instance_id="i-1",region="eu-west-1"}`:          0,
		`ec2.network_out_sum{account_id="000000000000",instance_id="i-1",region="us-east-1"}`:         0,
		`ec2.network_out_sum{account_id="000000000000",instance_id="i-1",region="eu-west-1"}`:         0,
		`ec2.disk_read_ops_sum{account_id="000000000000",instance_id="i-1",region="us-east-1"}`:       0,
		`ec2.disk_read_ops_sum{account_id="000000000000",instance_id="i-1",region="eu-west-1"}`:       0,
		`ec2.disk_write_ops_sum{account_id="000000000000",instance_id="i-1",region="us-east-1"}`:      0,
		`ec2.disk_write_ops_sum{account_id="000000000000",instance_id="i-1",region="eu-west-1"}`:      0,
	}, workloadSeries(series))
	collecttest.AssertChartCoverage(t, c, collecttest.ChartCoverageExpectation{})
}

// TestCollect_DiscoveryFailSoft covers the fail-soft discovery contract end-to-end.
func TestCollect_DiscoveryFailSoft(t *testing.T) {
	const account = "000000000000"

	newBase := func(regions ...string) *Collector {
		c := New()
		configureExactRule(c, regions, []string{"ec2"})
		c.newSTSClient = func(aws.Config) stsClient { return &fakeSTS{account: account} }
		c.now = func() time.Time { return time.Unix(1_700_000_000, 0) }
		return c
	}

	t.Run("total discovery failure on the first pass errors the collect", func(t *testing.T) {
		c := newBase("us-east-1")
		fake := &e2eCloudWatch{listErr: errors.New("AccessDenied")}
		useFakeClient(c, fake)
		_, err := collecttest.CollectScalarSeries(c)
		require.Error(t, err)

		fake.mu.Lock()
		fake.listErr = nil
		fake.list = map[string][]cwtypes.Metric{"AWS/EC2": {mkMetric("CPUUtilization", "InstanceId", "i-1")}}
		fake.values = map[string]float64{e2eKey("AWS/EC2", "CPUUtilization", "Average", "InstanceId", "i-1"): 7}
		fake.mu.Unlock()
		c.now = func() time.Time {
			return time.Unix(1_700_000_000, 0).Add(time.Duration(c.Discovery.RefreshEvery) * time.Second)
		}

		series, err := collecttest.CollectScalarSeries(c)
		require.NoError(t, err)
		assert.Equal(t, metrix.SampleValue(2), series[activityAPICallsMetric+`{account_id="000000000000",operation="list_metrics",region="us-east-1"}`],
			"the failed cycle's ListMetrics call must appear after recovery")
		assert.Equal(t, metrix.SampleValue(1), series[activityAPICallsMetric+`{account_id="000000000000",operation="get_metric_data",region="us-east-1"}`])
	})

	t.Run("partial region failure is tolerated", func(t *testing.T) {
		c := newBase("us-east-1", "eu-west-1")
		good := &e2eCloudWatch{
			list:   map[string][]cwtypes.Metric{"AWS/EC2": {mkMetric("CPUUtilization", "InstanceId", "i-1")}},
			values: map[string]float64{e2eKey("AWS/EC2", "CPUUtilization", "Average", "InstanceId", "i-1"): 7},
		}
		bad := &e2eCloudWatch{listErr: errors.New("region unavailable")}
		useFakeClient(c, good) // sets the region-aware newAWSConfig
		c.newCloudWatchClient = func(cfg aws.Config) cloudwatchClient {
			if cfg.Region == "eu-west-1" {
				return bad
			}
			return good
		}

		series, err := collecttest.CollectScalarSeries(c)
		require.NoError(t, err)
		assert.Equal(t, map[string]metrix.SampleValue{
			`ec2.cpu_utilization_average{account_id="000000000000",instance_id="i-1",region="us-east-1"}`: 7,
			`ec2.network_in_sum{account_id="000000000000",instance_id="i-1",region="us-east-1"}`:          0,
			`ec2.network_out_sum{account_id="000000000000",instance_id="i-1",region="us-east-1"}`:         0,
			`ec2.disk_read_ops_sum{account_id="000000000000",instance_id="i-1",region="us-east-1"}`:       0,
			`ec2.disk_write_ops_sum{account_id="000000000000",instance_id="i-1",region="us-east-1"}`:      0,
		}, workloadSeries(series), "the healthy region still produces its series (rate metrics with no data record 0)")
	})
}
