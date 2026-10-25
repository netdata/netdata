// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rds"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

const (
	// rdsDescribePageSize is the DescribeDBInstances maximum (valid 20..100).
	rdsDescribePageSize = 100
	// rdsDescribeMaxPages bounds one (target, region) fetch to 10,000 instances.
	rdsDescribeMaxPages = 100
)

// CloudWatch publishes AWS/RDS metrics per DBInstanceIdentifier, and separately
// aggregated per EngineName or DatabaseClass, but never combines the instance with
// its engine. DescribeDBInstances supplies those facts per instance.
var rdsInstanceAttributes = &resourceAttributeProvider{
	name:      "rds:DescribeDBInstances",
	namespace: "AWS/RDS",
	joinDim:   "DBInstanceIdentifier",
	labels:    []string{"engine", "engine_version", "db_instance_class"},
	fetch:     fetchRDSInstanceAttributes,
}

func fetchRDSInstanceAttributes(ctx context.Context, c *Collector, target, region string) (map[string][]metrix.Label, error) {
	client, err := c.rdsClients.forTargetRegion(ctx, target, region)
	if err != nil {
		return nil, fmt.Errorf("build client: %w", err)
	}
	return describeRDSInstanceAttributes(ctx, client, c.Timeout.Duration())
}

// describeRDSInstanceAttributes pages DescribeDBInstances under one timeout. The
// result may include Neptune and DocumentDB instances; they never match AWS/RDS
// instances because a DBInstanceIdentifier is unique per account and region.
func describeRDSInstanceAttributes(ctx context.Context, client rdsClient, timeout time.Duration) (map[string][]metrix.Label, error) {
	cctx, cancel := withTimeout(ctx, timeout)
	defer cancel()

	out := make(map[string][]metrix.Label)
	seenMarkers := make(map[string]struct{})
	var marker *string
	for range rdsDescribeMaxPages {
		response, err := client.DescribeDBInstances(cctx, &rds.DescribeDBInstancesInput{
			Marker:     marker,
			MaxRecords: aws.Int32(rdsDescribePageSize),
		})
		if err != nil {
			return nil, err
		}
		for _, instance := range response.DBInstances {
			id := aws.ToString(instance.DBInstanceIdentifier)
			if id == "" {
				continue
			}
			var labels []metrix.Label
			labels = appendAttributeLabel(labels, "engine", aws.ToString(instance.Engine))
			labels = appendAttributeLabel(labels, "engine_version", aws.ToString(instance.EngineVersion))
			labels = appendAttributeLabel(labels, "db_instance_class", aws.ToString(instance.DBInstanceClass))
			if len(labels) > 0 {
				out[id] = labels
			}
		}
		if aws.ToString(response.Marker) == "" {
			return out, nil
		}
		if _, duplicate := seenMarkers[*response.Marker]; duplicate {
			return nil, fmt.Errorf("DescribeDBInstances returned a duplicate pagination marker")
		}
		seenMarkers[*response.Marker] = struct{}{}
		marker = response.Marker
	}
	return nil, fmt.Errorf("DescribeDBInstances exceeded %d pages", rdsDescribeMaxPages)
}
