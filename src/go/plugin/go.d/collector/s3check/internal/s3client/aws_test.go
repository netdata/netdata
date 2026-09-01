// SPDX-License-Identifier: GPL-3.0-or-later

package s3client

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/stretchr/testify/assert"
)

func TestConfigureS3OptionsCanonicalizesEndpointAndPathStyle(t *testing.T) {
	var options s3.Options
	configureS3Options(&options, Config{
		Endpoint: " https://s3.example ", PathStyle: true,
	})

	assert.Equal(t, "https://s3.example", aws.ToString(options.BaseEndpoint))
	assert.True(t, options.UsePathStyle)
}

func TestConvertReplicationRule(t *testing.T) {
	tests := map[string]struct {
		rule types.ReplicationRule
		want ReplicationRule
	}{
		"enabled prefix rule with delete markers": {
			rule: types.ReplicationRule{
				Status: types.ReplicationRuleStatusEnabled,
				Destination: &types.Destination{
					Bucket: aws.String("arn:aws:s3:::destination"),
				},
				Filter: &types.ReplicationRuleFilter{Prefix: aws.String("netdata/")},
				DeleteMarkerReplication: &types.DeleteMarkerReplication{
					Status: types.DeleteMarkerReplicationStatusEnabled,
				},
			},
			want: ReplicationRule{
				Enabled:                 true,
				DestinationBucket:       "destination",
				Prefix:                  "netdata/",
				DeleteMarkerReplication: true,
			},
		},
		"tag filtered rule": {
			rule: types.ReplicationRule{
				Destination: &types.Destination{Bucket: aws.String("arn:aws:s3:::destination")},
				Filter:      &types.ReplicationRuleFilter{Tag: &types.Tag{Key: aws.String("key"), Value: aws.String("value")}},
			},
			want: ReplicationRule{DestinationBucket: "destination", TagFiltered: true},
		},
		"and filter retains prefix and identifies tags": {
			rule: types.ReplicationRule{
				Destination: &types.Destination{Bucket: aws.String("destination")},
				Filter: &types.ReplicationRuleFilter{And: &types.ReplicationRuleAndOperator{
					Prefix: aws.String("prefix/"),
					Tags:   []types.Tag{{Key: aws.String("key"), Value: aws.String("value")}},
				}},
			},
			want: ReplicationRule{DestinationBucket: "destination", Prefix: "prefix/", TagFiltered: true},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, convertReplicationRule(tc.rule))
		})
	}
}

func TestBucketFromARN(t *testing.T) {
	assert.Equal(t, "bucket", bucketFromARN("arn:aws:s3:::bucket"))
	assert.Equal(t, "bucket", bucketFromARN("bucket"))
}
