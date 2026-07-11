// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/cwprofiles"
)

// TestTagJoin_RoundTrip is the load-bearing check: for a representative resource
// ARN and the matching discovered instance's dimension values, the arn-derived and
// instance-derived join keys must be equal (both non-empty). If they diverge, the
// resource's tags would silently not attach.
func TestTagJoin_RoundTrip(t *testing.T) {
	const acct = "111122223333"
	tests := map[string]struct {
		profile  string
		dimNames []string
		values   []string
		arnStr   string
	}{
		"ec2":         {"ec2", []string{"InstanceId"}, []string{"i-0abc123"}, "arn:aws:ec2:us-east-1:" + acct + ":instance/i-0abc123"},
		"ebs":         {"ebs", []string{"VolumeId"}, []string{"vol-0abc123"}, "arn:aws:ec2:us-east-1:" + acct + ":volume/vol-0abc123"},
		"efs":         {"efs", []string{"FileSystemId"}, []string{"fs-0abc123"}, "arn:aws:elasticfilesystem:us-east-1:" + acct + ":file-system/fs-0abc123"},
		"nat_gateway": {"nat_gateway", []string{"NatGatewayId"}, []string{"nat-0abc123"}, "arn:aws:ec2:us-east-1:" + acct + ":natgateway/nat-0abc123"},
		"vpn":         {"vpn", []string{"VpnId"}, []string{"vpn-0abc123"}, "arn:aws:ec2:us-east-1:" + acct + ":vpn-connection/vpn-0abc123"},
		"rds":         {"rds", []string{"DBInstanceIdentifier"}, []string{"mydb"}, "arn:aws:rds:us-east-1:" + acct + ":db:mydb"},
		"docdb":       {"docdb", []string{"DBInstanceIdentifier"}, []string{"mydocdb"}, "arn:aws:rds:us-east-1:" + acct + ":db:mydocdb"},
		"lambda":      {"lambda", []string{"FunctionName"}, []string{"myfn"}, "arn:aws:lambda:us-east-1:" + acct + ":function:myfn"},
		"sqs":         {"sqs", []string{"QueueName"}, []string{"myqueue"}, "arn:aws:sqs:us-east-1:" + acct + ":myqueue"},
		"sns":         {"sns", []string{"TopicName"}, []string{"mytopic"}, "arn:aws:sns:us-east-1:" + acct + ":mytopic"},
		"eks":         {"eks", []string{"ClusterName"}, []string{"mycluster"}, "arn:aws:eks:us-east-1:" + acct + ":cluster/mycluster"},
		"kinesis":     {"kinesis", []string{"StreamName"}, []string{"mystream"}, "arn:aws:kinesis:us-east-1:" + acct + ":stream/mystream"},
		"firehose":    {"firehose", []string{"DeliveryStreamName"}, []string{"mystream"}, "arn:aws:firehose:us-east-1:" + acct + ":deliverystream/mystream"},
		"redshift":    {"redshift", []string{"ClusterIdentifier"}, []string{"mycluster"}, "arn:aws:redshift:us-east-1:" + acct + ":cluster:mycluster"},
		"dynamodb":    {"dynamodb", []string{"TableName"}, []string{"mytable"}, "arn:aws:dynamodb:us-east-1:" + acct + ":table/mytable"},
		"eventbridge": {"eventbridge", []string{"RuleName"}, []string{"myrule"}, "arn:aws:events:us-east-1:" + acct + ":rule/myrule"},

		// Parent-resource: the child dimension (storage_type/filter_id/operation) is
		// not in the ARN; the join key is the parent, so the child inherits its tags.
		"s3 (parent bucket, ignores storage_type)": {"s3", []string{"BucketName", "StorageType"}, []string{"mybucket", "StandardStorage"}, "arn:aws:s3:::mybucket"},
		"s3_requests (parent bucket)":              {"s3_requests", []string{"BucketName", "FilterId"}, []string{"mybucket", "myfilter"}, "arn:aws:s3:::mybucket"},
		"dynamodb_operation (parent table)":        {"dynamodb_operation", []string{"TableName", "Operation"}, []string{"mytable", "GetItem"}, "arn:aws:dynamodb:us-east-1:" + acct + ":table/mytable"},

		// ELB family shares the loadbalancer resource type; each flavour extracts differently.
		"elb classic (name)":       {"elb", []string{"LoadBalancerName"}, []string{"my-elb"}, "arn:aws:elasticloadbalancing:us-east-1:" + acct + ":loadbalancer/my-elb"},
		"alb (app/name/hash)":      {"alb", []string{"LoadBalancer"}, []string{"app/my-alb/50dc6c495c0c9188"}, "arn:aws:elasticloadbalancing:us-east-1:" + acct + ":loadbalancer/app/my-alb/50dc6c495c0c9188"},
		"nlb (net/name/hash)":      {"nlb", []string{"LoadBalancer"}, []string{"net/my-nlb/50dc6c495c0c9188"}, "arn:aws:elasticloadbalancing:us-east-1:" + acct + ":loadbalancer/net/my-nlb/50dc6c495c0c9188"},
		"alb_target (full prefix)": {"alb_target", []string{"LoadBalancer", "TargetGroup"}, []string{"app/my-alb/50dc6c495c0c9188", "targetgroup/my-tg/73e2d6bc24d8a067"}, "arn:aws:elasticloadbalancing:us-east-1:" + acct + ":targetgroup/my-tg/73e2d6bc24d8a067"},

		// Multi-dimension joins fully derivable from the ARN.
		"ecs (cluster+service)":    {"ecs", []string{"ClusterName", "ServiceName"}, []string{"mycluster", "mysvc"}, "arn:aws:ecs:us-east-1:" + acct + ":service/mycluster/mysvc"},
		"opensearch (account+dom)": {"opensearch", []string{"ClientId", "DomainName"}, []string{acct, "mydomain"}, "arn:aws:es:us-east-1:" + acct + ":domain/mydomain"},

		// StateMachineArn dimension value IS the ARN.
		"step_functions (arn-as-dim)": {"step_functions", []string{"StateMachineArn"}, []string{"arn:aws:states:us-east-1:" + acct + ":stateMachine:mysm"}, "arn:aws:states:us-east-1:" + acct + ":stateMachine:mysm"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			tj, ok := tagJoins[tc.profile]
			require.Truef(t, ok, "no tagJoin for profile %q", tc.profile)

			a, err := arn.Parse(tc.arnStr)
			require.NoError(t, err)

			ak, aok := tj.arnJoinKey(a)
			require.Truef(t, aok, "arnJoinKey failed for %q", tc.arnStr)

			ik, iok := tj.instanceJoinKey(tc.dimNames, tc.values)
			require.True(t, iok, "instanceJoinKey failed")

			assert.Equal(t, ik, ak, "arn-derived and instance-derived join keys must match")
		})
	}
}

func TestTagJoin_ArnJoinKeyCases(t *testing.T) {
	// arnJoinKey acceptance/rejection. An unexpected shape (foreign load-balancer
	// flavour on the shared type, a custom-bus rule, or a sub-segment id) must MISS
	// (never mis-tag). wantKey == "" means "expect a miss (ok == false)".
	tests := map[string]struct {
		profile string
		arn     string
		wantKey string
	}{
		"classic elb accepts loadbalancer/<name>":    {"elb", "arn:aws:elasticloadbalancing:us-east-1:111122223333:loadbalancer/my-elb", "my-elb"},
		"classic elb rejects an ALB arn":             {"elb", "arn:aws:elasticloadbalancing:us-east-1:111122223333:loadbalancer/app/my-alb/hash", ""},
		"classic elb rejects an NLB arn":             {"elb", "arn:aws:elasticloadbalancing:us-east-1:111122223333:loadbalancer/net/my-nlb/hash", ""},
		"alb rejects a classic elb arn":              {"alb", "arn:aws:elasticloadbalancing:us-east-1:111122223333:loadbalancer/my-elb", ""},
		"alb rejects an NLB arn":                     {"alb", "arn:aws:elasticloadbalancing:us-east-1:111122223333:loadbalancer/net/my-nlb/hash", ""},
		"nlb rejects an ALB arn":                     {"nlb", "arn:aws:elasticloadbalancing:us-east-1:111122223333:loadbalancer/app/my-alb/hash", ""},
		"eventbridge accepts a default-bus rule":     {"eventbridge", "arn:aws:events:us-east-1:111122223333:rule/myrule", "myrule"},
		"eventbridge rejects a custom-bus rule":      {"eventbridge", "arn:aws:events:us-east-1:111122223333:rule/mybus/myrule", ""},
		"default extractor accepts instance/<id>":    {"ec2", "arn:aws:ec2:us-east-1:111122223333:instance/i-1", "i-1"},
		"default extractor rejects a sub-segment id": {"ec2", "arn:aws:ec2:us-east-1:111122223333:instance/foo/i-1", ""},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, ok := tagJoins[tc.profile].arnJoinKey(mustParseARN(t, tc.arn))
			if tc.wantKey == "" {
				assert.False(t, ok, "expected a miss")
				return
			}
			require.True(t, ok)
			assert.Equal(t, tc.wantKey, got)
		})
	}
}

func TestDeriveResourceType(t *testing.T) {
	tests := map[string]string{
		"arn:aws:ec2:us-east-1:111122223333:instance/i-0abc":                  "ec2:instance",
		"arn:aws:rds:us-east-1:111122223333:db:mydb":                          "rds:db",
		"arn:aws:sqs:us-east-1:111122223333:myqueue":                          "sqs",
		"arn:aws:s3:::mybucket":                                               "s3",
		"arn:aws:elasticloadbalancing:us-east-1:111122223333:loadbalancer/x":  "elasticloadbalancing:loadbalancer",
		"arn:aws:elasticloadbalancing:us-east-1:111122223333:targetgroup/x/y": "elasticloadbalancing:targetgroup",
		"arn:aws:es:us-east-1:111122223333:domain/mydomain":                   "es:domain",
		"arn:aws:states:us-east-1:111122223333:stateMachine:mysm":             "states:stateMachine",
	}
	for arnStr, want := range tests {
		t.Run(want, func(t *testing.T) {
			assert.Equal(t, want, deriveResourceType(mustParseARN(t, arnStr)))
		})
	}
}

// TestTagJoins_DimsExistInProfiles is a drift guard: every registered join must
// reference dimensions that actually exist in its stock profile, and the profile
// must exist. Catches a profile rename/removal silently disabling a tag join.
func TestTagJoins_DimsExistInProfiles(t *testing.T) {
	cat, err := cwprofiles.DefaultCatalog()
	require.NoError(t, err)

	byName := make(map[string]cwprofiles.ResolvedProfile)
	for _, p := range cat.AllProfiles() {
		byName[p.Name] = p
	}

	for name, tj := range tagJoins {
		p, ok := byName[name]
		require.Truef(t, ok, "tagJoins references unknown profile %q", name)
		assert.Equalf(t, p.Config.Namespace, tj.namespace, "profile %q association namespace", name)
		for _, jd := range tj.joinDims {
			var dimension *cwprofiles.InstanceDimension
			for i := range p.Config.Instance.Dimensions {
				if p.Config.Instance.Dimensions[i].Name == jd {
					dimension = &p.Config.Instance.Dimensions[i]
					break
				}
			}
			if assert.NotNilf(t, dimension, "profile %q joinDim %q missing", name, jd) {
				assert.Falsef(t, dimension.IsConstant(), "profile %q joinDim %q must identify the resource", name, jd)
			}
		}
	}
}

func TestResolveTagJoinProfile_RejectsIncompatibleOverride(t *testing.T) {
	t.Run("namespace changed", func(t *testing.T) {
		profile := cwprofiles.ResolvedProfile{Name: "ec2", Config: ec2QueryProfile()}
		profile.Config.Namespace = "Custom/Unrelated"

		_, err := resolveTagJoinProfile(profile)
		require.Error(t, err)
		assert.ErrorContains(t, err, "namespace")
	})

	t.Run("join dimension made constant", func(t *testing.T) {
		profile := cwprofiles.ResolvedProfile{Name: "ec2", Config: ec2QueryProfile()}
		profile.Config.Instance.Dimensions[0].Label = ""
		profile.Config.Instance.Dimensions[0].Constant = aws.String("i-fixed")

		_, err := resolveTagJoinProfile(profile)
		require.Error(t, err)
		assert.ErrorContains(t, err, "identifying")
	})
}

func mustParseARN(t *testing.T, s string) arn.ARN {
	t.Helper()
	a, err := arn.Parse(s)
	require.NoError(t, err)
	return a
}
