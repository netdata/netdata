// SPDX-License-Identifier: GPL-3.0-or-later

package s3client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/awsauth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const assumeRoleResponse = `<AssumeRoleResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <AssumeRoleResult>
    <Credentials>
      <AccessKeyId>ASIATEMPKEY</AccessKeyId>
      <SecretAccessKey>temp-secret</SecretAccessKey>
      <SessionToken>temp-session-token</SessionToken>
      <Expiration>2999-12-31T23:59:59Z</Expiration>
    </Credentials>
    <AssumedRoleUser>
      <Arn>arn:aws:sts::000000000000:assumed-role/example/netdata</Arn>
      <AssumedRoleId>AROAEXAMPLE:netdata</AssumedRoleId>
    </AssumedRoleUser>
  </AssumeRoleResult>
  <ResponseMetadata><RequestId>test</RequestId></ResponseMetadata>
</AssumeRoleResponse>`

func TestNewRoutesAssumeRoleAndS3ThroughConfiguredProxy(t *testing.T) {
	var stsCalls, s3Calls atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		switch {
		case r.PostForm.Get("Action") == "AssumeRole":
			stsCalls.Add(1)
			w.Header().Set("Content-Type", "text/xml")
			_, _ = w.Write([]byte(assumeRoleResponse))
		case r.URL.Query().Has("versioning"):
			s3Calls.Add(1)
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write(
				[]byte(
					`<VersioningConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Status>Enabled</Status></VersioningConfiguration>`,
				),
			)
		default:
			http.Error(w, "unexpected request", http.StatusBadRequest)
		}
	}))
	defer proxy.Close()

	const unreachableEndpoint = "http://127.0.0.1:1"
	t.Setenv("AWS_ENDPOINT_URL_STS", unreachableEndpoint)
	identity := awsauth.NewIdentity("source", awsauth.CredentialConfig{
		Type: awsauth.CredentialTypeStatic,
		TypeStatic: &awsauth.StaticCredentialConfig{
			AccessKeyID:     "AKIABASEIDENTITY",
			SecretAccessKey: "base-secret",
		},
	}, &awsauth.AssumeRoleConfig{
		RoleARN: "arn:aws:iam::000000000000:role/example",
	})
	client, err := New(context.Background(), Config{
		Identity:  identity,
		Region:    "us-east-1",
		Endpoint:  unreachableEndpoint,
		PathStyle: true,
		Timeout:   time.Second,
		ProxyURL:  proxy.URL,
	})
	require.NoError(t, err)
	defer client.CloseIdleConnections()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	versioning, err := client.BucketVersioning(ctx, "bucket")
	require.NoError(t, err)
	assert.Equal(t, VersioningStatus("Enabled"), versioning.Status)
	assert.EqualValues(t, 1, stsCalls.Load())
	assert.EqualValues(t, 1, s3Calls.Load())
}

func TestConfigureS3OptionsCanonicalizesEndpointAndPathStyle(t *testing.T) {
	var options s3.Options
	configureS3Options(&options, Config{
		Endpoint:  "https://s3.example",
		PathStyle: true,
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
				Status:   types.ReplicationRuleStatusEnabled,
				Priority: aws.Int32(17),
				Destination: &types.Destination{
					Bucket: aws.String("arn:aws:s3:::destination"),
				},
				Filter: &types.ReplicationRuleFilter{
					Prefix: aws.String("netdata/"),
				},
				DeleteMarkerReplication: &types.DeleteMarkerReplication{
					Status: types.DeleteMarkerReplicationStatusEnabled,
				},
			},
			want: ReplicationRule{
				Enabled:                 true,
				DestinationBucket:       "destination",
				Prefix:                  "netdata/",
				DeleteMarkerReplication: true,
				Priority:                17,
			},
		},
		"tag filtered rule": {
			rule: types.ReplicationRule{
				Destination: &types.Destination{
					Bucket: aws.String("arn:aws:s3:::destination"),
				},
				Filter: &types.ReplicationRuleFilter{
					Tag: &types.Tag{
						Key:   aws.String("key"),
						Value: aws.String("value"),
					},
				},
			},
			want: ReplicationRule{
				DestinationBucket: "destination",
				TagFiltered:       true,
			},
		},
		"and filter retains prefix and identifies tags": {
			rule: types.ReplicationRule{
				Destination: &types.Destination{
					Bucket: aws.String("destination"),
				},
				Filter: &types.ReplicationRuleFilter{
					And: &types.ReplicationRuleAndOperator{
						Prefix: aws.String("prefix/"),
						Tags:   []types.Tag{{Key: aws.String("key"), Value: aws.String("value")}},
					},
				},
			},
			want: ReplicationRule{
				DestinationBucket: "destination",
				Prefix:            "prefix/",
				TagFiltered:       true,
			},
		},
		"V1 rule implicitly replicates user delete markers": {
			rule: types.ReplicationRule{
				Status: types.ReplicationRuleStatusEnabled,
				Destination: &types.Destination{
					Bucket: aws.String("destination"),
				},
				Prefix: aws.String("netdata/"),
			},
			want: ReplicationRule{
				Enabled:                 true,
				DestinationBucket:       "destination",
				Prefix:                  "netdata/",
				DeleteMarkerReplication: true,
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, convertReplicationRule(tc.rule))
		})
	}
}

func TestObjectAbsenceRejectsGenericNotFound(t *testing.T) {
	assert.True(t, isObjectNotFound(&types.NoSuchKey{}))
	assert.True(t, isObjectNotFound(&smithy.GenericAPIError{
		Code: "NoSuchKey",
	}))
	assert.False(t, isObjectNotFound(&smithy.GenericAPIError{
		Code: "NotFound",
	}))
}
