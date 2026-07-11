// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws/arn"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/cwprofiles"
)

// tagJoin describes, for one profile, how to project BOTH a discovered instance and
// an RGTA-returned resource ARN onto a common "join key", so a resource's tags
// attach to the right series.
//
//   - resourceTypes are the RGTA GetResources ResourceTypeFilters that narrow the
//     scan to this profile's resources, and (reversed) map an ARN's service[:type]
//     back to the profile.
//   - joinDims names the profile dimensions that form the key, in order. For a
//     single-resource profile it is the one identifying dimension; for a
//     parent-resource profile (e.g. S3 across storage_type) it is only the parent
//     dimension, so every child instance inherits the parent's tags.
//   - arnValues extracts the joinDims values from an ARN (nil => defaultARNValues,
//     a single-segment resource id after the first '/' or ':'). It returns ok=false when the ARN
//     is not this profile's flavour (e.g. a classic-ELB extractor rejects an ALB
//     ARN), so a shared resource type can fan out to the right profile.
//
// Safety requirement: every registered extractor must reject ambiguous ARN shapes
// and project exactly the dimensions named by joinDims. A wrong projection could
// collide with another discovered instance, so associations are internal, reviewed,
// and covered by ARN-shape tests rather than inferred from arbitrary profiles.
type tagJoin struct {
	resourceTypes []string
	joinDims      []string
	arnValues     func(a arn.ARN) ([]string, bool)
}

// tagJoins is the per-profile (by basename) join registry. Only profiles listed
// here can use generic resource-tag filtering and labels. Labels-only use skips
// an unregistered profile; an effective filter applies the compiler's explicit
// skip/error policy instead of collecting that profile unfiltered.
var tagJoins = map[string]tagJoin{
	// Sound single-dimension joins: default extractor (resource-id = last segment).
	"ec2":         {resourceTypes: []string{"ec2:instance"}, joinDims: []string{"InstanceId"}},
	"ebs":         {resourceTypes: []string{"ec2:volume"}, joinDims: []string{"VolumeId"}},
	"efs":         {resourceTypes: []string{"elasticfilesystem:file-system"}, joinDims: []string{"FileSystemId"}},
	"nat_gateway": {resourceTypes: []string{"ec2:natgateway"}, joinDims: []string{"NatGatewayId"}},
	"vpn":         {resourceTypes: []string{"ec2:vpn-connection"}, joinDims: []string{"VpnId"}},
	// rds and docdb share the rds:db ARN type. The join is unambiguous because a
	// DBInstanceIdentifier is unique per account+region across the rds:db family
	// (RDS/DocDB/Neptune), so an ARN matches at most one discovered instance and
	// cross-family entries are inert. (Confirmed in live smoke.)
	"rds":      {resourceTypes: []string{"rds:db"}, joinDims: []string{"DBInstanceIdentifier"}},
	"docdb":    {resourceTypes: []string{"rds:db"}, joinDims: []string{"DBInstanceIdentifier"}},
	"lambda":   {resourceTypes: []string{"lambda:function"}, joinDims: []string{"FunctionName"}},
	"sqs":      {resourceTypes: []string{"sqs"}, joinDims: []string{"QueueName"}},
	"sns":      {resourceTypes: []string{"sns"}, joinDims: []string{"TopicName"}},
	"eks":      {resourceTypes: []string{"eks:cluster"}, joinDims: []string{"ClusterName"}},
	"kinesis":  {resourceTypes: []string{"kinesis:stream"}, joinDims: []string{"StreamName"}},
	"firehose": {resourceTypes: []string{"firehose:deliverystream"}, joinDims: []string{"DeliveryStreamName"}},
	"redshift": {resourceTypes: []string{"redshift:cluster"}, joinDims: []string{"ClusterIdentifier"}},
	"dynamodb": {resourceTypes: []string{"dynamodb:table"}, joinDims: []string{"TableName"}},

	// Parent-resource joins: joinDims is the parent dimension only, so children
	// (across storage_type / filter_id / operation) inherit the parent's tags.
	"s3":                 {resourceTypes: []string{"s3"}, joinDims: []string{"BucketName"}},
	"s3_requests":        {resourceTypes: []string{"s3"}, joinDims: []string{"BucketName"}},
	"dynamodb_operation": {resourceTypes: []string{"dynamodb:table"}, joinDims: []string{"TableName"}},

	// Overrides: shared/quirky ARN shapes.
	"elb":            {resourceTypes: []string{"elasticloadbalancing:loadbalancer"}, joinDims: []string{"LoadBalancerName"}, arnValues: elbClassicARNValues},
	"alb":            {resourceTypes: []string{"elasticloadbalancing:loadbalancer"}, joinDims: []string{"LoadBalancer"}, arnValues: albARNValues},
	"nlb":            {resourceTypes: []string{"elasticloadbalancing:loadbalancer"}, joinDims: []string{"LoadBalancer"}, arnValues: nlbARNValues},
	"alb_target":     {resourceTypes: []string{"elasticloadbalancing:targetgroup"}, joinDims: []string{"TargetGroup"}, arnValues: albTargetARNValues},
	"ecs":            {resourceTypes: []string{"ecs:service"}, joinDims: []string{"ClusterName", "ServiceName"}, arnValues: ecsARNValues},
	"opensearch":     {resourceTypes: []string{"es:domain"}, joinDims: []string{"ClientId", "DomainName"}, arnValues: opensearchARNValues},
	"step_functions": {resourceTypes: []string{"states:stateMachine"}, joinDims: []string{"StateMachineArn"}, arnValues: stepFunctionsARNValues},
	// eventbridge is default-bus-only ({RuleName}); its override rejects custom-bus
	// rule ARNs so a custom-bus rule cannot mis-tag a same-named default-bus rule.
	"eventbridge": {resourceTypes: []string{"events:rule"}, joinDims: []string{"RuleName"}, arnValues: eventbridgeARNValues},
}

func validateTagJoinProfile(profile cwprofiles.ResolvedProfile) error {
	join, ok := tagJoins[profile.Name]
	if !ok {
		return fmt.Errorf("profile is not registered")
	}
	dimensions := profile.Config.DimensionNames()
	for _, name := range join.joinDims {
		if indexOfString(dimensions, name) < 0 {
			return fmt.Errorf("association requires dimension %q", name)
		}
	}
	return nil
}

// defaultARNValues extracts a single-segment resource id: the part after the FIRST
// '/' or ':' (e.g. "instance/i-abc" -> "i-abc", "db:name" -> "name"), or the whole
// resource when it has no separator (e.g. an SQS queue name or an S3 bucket). It
// REJECTS a resource whose id part carries a further '/' or ':' (a sub-resource or
// qualifier, e.g. "instance/foo/i-1" or "function:name:version") so an unexpected
// ARN misses rather than mis-tagging via a coincidental trailing segment.
func defaultARNValues(a arn.ARN) ([]string, bool) {
	res := a.Resource
	i := strings.IndexAny(res, "/:")
	if i < 0 {
		if res == "" {
			return nil, false
		}
		return []string{res}, true
	}
	id := res[i+1:]
	if id == "" || strings.ContainsAny(id, "/:") {
		return nil, false
	}
	return []string{id}, true
}

// elbClassicARNValues matches a classic ELB (loadbalancer/<name>); it rejects the
// ALB/NLB shape (loadbalancer/app|net/...), which shares the resource type.
func elbClassicARNValues(a arn.ARN) ([]string, bool) {
	const p = "loadbalancer/"
	if !strings.HasPrefix(a.Resource, p) {
		return nil, false
	}
	name := strings.TrimPrefix(a.Resource, p)
	if name == "" || strings.Contains(name, "/") {
		return nil, false // "app/..."/"net/..." => v2 load balancer, not classic
	}
	return []string{name}, true
}

func albARNValues(a arn.ARN) ([]string, bool) { return elbV2Values(a, "app/") }
func nlbARNValues(a arn.ARN) ([]string, bool) { return elbV2Values(a, "net/") }

// elbV2Values matches an ALB/NLB whose CloudWatch LoadBalancer dimension is the ARN
// suffix after "loadbalancer/" (e.g. "app/name/hash").
func elbV2Values(a arn.ARN, kind string) ([]string, bool) {
	const p = "loadbalancer/"
	if !strings.HasPrefix(a.Resource, p) {
		return nil, false
	}
	lb := strings.TrimPrefix(a.Resource, p)
	if !strings.HasPrefix(lb, kind) {
		return nil, false
	}
	return []string{lb}, true
}

// albTargetARNValues: the TargetGroup dimension value is the full "targetgroup/..."
// resource, so the join key keeps the prefix.
func albTargetARNValues(a arn.ARN) ([]string, bool) {
	if !strings.HasPrefix(a.Resource, "targetgroup/") {
		return nil, false
	}
	return []string{a.Resource}, true
}

// ecsARNValues: an ECS service ARN is "service/<cluster>/<service>".
func ecsARNValues(a arn.ARN) ([]string, bool) {
	if !strings.HasPrefix(a.Resource, "service/") {
		return nil, false
	}
	parts := strings.Split(a.Resource, "/")
	if len(parts) != 3 || parts[1] == "" || parts[2] == "" {
		return nil, false
	}
	return []string{parts[1], parts[2]}, true
}

// opensearchARNValues: OpenSearch's ClientId dimension is the AWS account id and
// DomainName is the domain ("domain/<name>" resource).
func opensearchARNValues(a arn.ARN) ([]string, bool) {
	const p = "domain/"
	if a.AccountID == "" || !strings.HasPrefix(a.Resource, p) {
		return nil, false
	}
	name := strings.TrimPrefix(a.Resource, p)
	if name == "" {
		return nil, false
	}
	return []string{a.AccountID, name}, true
}

// stepFunctionsARNValues: the StateMachineArn dimension value IS the full ARN.
func stepFunctionsARNValues(a arn.ARN) ([]string, bool) {
	if a.Resource == "" {
		return nil, false
	}
	return []string{a.String()}, true
}

// eventbridgeARNValues matches only a DEFAULT-bus rule ("rule/<RuleName>"). It
// rejects a custom-bus rule ("rule/<EventBusName>/<RuleName>"): the {RuleName}-only
// profile represents default-bus rules, so a custom-bus rule of the same name must
// not join (miss, never mis-tag).
func eventbridgeARNValues(a arn.ARN) ([]string, bool) {
	const p = "rule/"
	if !strings.HasPrefix(a.Resource, p) {
		return nil, false
	}
	name := strings.TrimPrefix(a.Resource, p)
	if name == "" || strings.Contains(name, "/") {
		return nil, false
	}
	return []string{name}, true
}

// arnJoinKey computes this profile's join key from a resource ARN, or ok=false when
// the ARN is not this profile's flavour.
func (tj tagJoin) arnJoinKey(a arn.ARN) (string, bool) {
	fn := tj.arnValues
	if fn == nil {
		fn = defaultARNValues
	}
	vals, ok := fn(a)
	if !ok || len(vals) != len(tj.joinDims) {
		return "", false
	}
	return strings.Join(vals, instanceKeySep), true
}

// instanceJoinKey computes this profile's join key from a discovered instance's
// dimension values (ordered by dimNames = Profile.DimensionNames()).
func (tj tagJoin) instanceJoinKey(dimNames, values []string) (string, bool) {
	if len(tj.joinDims) == 1 {
		idx := indexOfString(dimNames, tj.joinDims[0])
		if idx < 0 || idx >= len(values) {
			return "", false
		}
		return values[idx], true
	}
	parts := make([]string, len(tj.joinDims))
	for i, jd := range tj.joinDims {
		idx := indexOfString(dimNames, jd)
		if idx < 0 || idx >= len(values) {
			return "", false
		}
		parts[i] = values[idx]
	}
	return strings.Join(parts, instanceKeySep), true
}

func indexOfString(ss []string, s string) int {
	for i, v := range ss {
		if v == s {
			return i
		}
	}
	return -1
}

// deriveResourceType maps an ARN to the "service[:type]" key used to look a profile
// up in the resource-type index (and matching the ResourceTypeFilters strings).
func deriveResourceType(a arn.ARN) string {
	if i := strings.IndexAny(a.Resource, "/:"); i >= 0 {
		return a.Service + ":" + a.Resource[:i]
	}
	return a.Service
}
