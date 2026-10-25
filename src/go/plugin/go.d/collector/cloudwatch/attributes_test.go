// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	rdstypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
	rgtatypes "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"
	"github.com/aws/smithy-go"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/cwprofiles"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/awsauth"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeRDS is a DescribeDBInstances stub. pages[i] answers call i of one fetch with a
// marker until the last page; err fails every call.
type fakeRDS struct {
	mu        sync.Mutex
	instances []rdstypes.DBInstance
	pages     [][]rdstypes.DBInstance
	err       error
	marker    func(call int) *string // overrides the returned marker when set
	calls     int
	inputs    []*rds.DescribeDBInstancesInput
}

func (f *fakeRDS) DescribeDBInstances(_ context.Context, in *rds.DescribeDBInstancesInput, _ ...func(*rds.Options)) (*rds.DescribeDBInstancesOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	call := f.calls
	f.calls++
	f.inputs = append(f.inputs, in)
	if f.err != nil {
		return nil, f.err
	}
	if f.marker != nil {
		return &rds.DescribeDBInstancesOutput{Marker: f.marker(call)}, nil
	}
	if f.pages == nil {
		return &rds.DescribeDBInstancesOutput{DBInstances: f.instances}, nil
	}
	out := &rds.DescribeDBInstancesOutput{}
	if call < len(f.pages) {
		out.DBInstances = f.pages[call]
	}
	if call < len(f.pages)-1 {
		out.Marker = aws.String(fmt.Sprintf("page-%d", call+1))
	}
	return out, nil
}

func (f *fakeRDS) set(instances []rdstypes.DBInstance, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.instances, f.err = instances, err
}

func (f *fakeRDS) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func rdsInstance(id, engine, version, class string) rdstypes.DBInstance {
	return rdstypes.DBInstance{
		DBInstanceIdentifier: aws.String(id),
		Engine:               aws.String(engine),
		EngineVersion:        aws.String(version),
		DBInstanceClass:      aws.String(class),
	}
}

func rdsMetrics(ids ...string) []cwtypes.Metric {
	out := make([]cwtypes.Metric, 0, len(ids))
	for _, id := range ids {
		out = append(out, mkMetric("CPUUtilization", "DBInstanceIdentifier", id))
	}
	return out
}

func rdsProfileConfig(targets ...string) Config {
	defaults := false
	cfg := validBaseConfig()
	cfg.Targets = nil
	for _, target := range targets {
		cfg.Targets = append(cfg.Targets, TargetConfig{Name: target, Credentials: "sdk_default"})
	}
	cfg.Rules = []RuleConfig{{
		Name: "rds", Targets: targets, Regions: []string{"us-east-1"},
		Profiles: &ProfileSelectorConfig{Defaults: &defaults, Include: []string{"rds"}},
	}}
	return cfg
}

// rdsAttributeCollector wires the stock RDS profile end to end. Each target gets its
// own account and RDS client; the target travels to the client seams as AppID.
func rdsAttributeCollector(t *testing.T, cfg Config, cw cloudwatchClient, accounts map[string]string, rdsByTarget map[string]rdsClient) *Collector {
	t.Helper()
	c := New()
	c.Config = cfg
	c.newCatalog = cwprofiles.DefaultCatalog
	c.newAWSConfig = func(_ context.Context, identity awsauth.Identity, region string) (aws.Config, error) {
		return aws.Config{Region: region, AppID: identity.Ref}, nil
	}
	c.newSTSClient = func(cfg aws.Config) stsClient { return &fakeSTS{account: accounts[cfg.AppID]} }
	c.newCloudWatchClient = func(aws.Config) cloudwatchClient { return cw }
	c.newRDSClient = func(cfg aws.Config) rdsClient { return rdsByTarget[cfg.AppID] }
	c.now = func() time.Time { return time.Unix(1_000_000_000, 0) }
	require.NoError(t, c.Init(context.Background()))
	return c
}

// labelsByInstance returns the label set of every emitted series with the given name,
// keyed by "<account_id>/<db_instance_identifier>".
func labelsByInstance(c *Collector, series string) map[string]map[string]string {
	out := make(map[string]map[string]string)
	c.MetricStore().Read().ForEachSeries(func(name string, labels metrix.LabelView, _ metrix.SampleValue) {
		if name != series {
			return
		}
		m := make(map[string]string)
		labels.Range(func(k, v string) bool {
			m[k] = v
			return true
		})
		out[m["account_id"]+"/"+m["db_instance_identifier"]] = m
	})
	return out
}

func rdsLabels(account, id string, extra ...string) map[string]string {
	m := map[string]string{"account_id": account, "region": "us-east-1", "db_instance_identifier": id}
	for i := 0; i+1 < len(extra); i += 2 {
		m[extra[i]] = extra[i+1]
	}
	return m
}

const rdsCPUSeries = "rds.cpu_utilization_average"

func TestCollect_RDSAttributeLabels(t *testing.T) {
	accessDenied := &smithy.GenericAPIError{Code: "AccessDenied", Message: "not authorized to perform rds:DescribeDBInstances"}
	tests := map[string]struct {
		rds  *fakeRDS
		want map[string]map[string]string
	}{
		"attaches engine, engine version and instance class": {
			rds: &fakeRDS{instances: []rdstypes.DBInstance{
				rdsInstance("db-a", "mysql", "8.0.35", "db.t3.micro"),
				rdsInstance("db-b", "aurora-postgresql", "16.3", "db.r6g.large"),
			}},
			want: map[string]map[string]string{
				"000000000000/db-a": rdsLabels("000000000000", "db-a", "engine", "mysql", "engine_version", "8.0.35", "db_instance_class", "db.t3.micro"),
				"000000000000/db-b": rdsLabels("000000000000", "db-b", "engine", "aurora-postgresql", "engine_version", "16.3", "db_instance_class", "db.r6g.large"),
			},
		},
		"missing permission keeps series without attribute labels": {
			rds: &fakeRDS{err: accessDenied},
			want: map[string]map[string]string{
				"000000000000/db-a": rdsLabels("000000000000", "db-a"),
				"000000000000/db-b": rdsLabels("000000000000", "db-b"),
			},
		},
		"instance absent from the describe result gets no attribute labels": {
			rds: &fakeRDS{instances: []rdstypes.DBInstance{rdsInstance("db-a", "postgres", "16.3", "db.m6g.large")}},
			want: map[string]map[string]string{
				"000000000000/db-a": rdsLabels("000000000000", "db-a", "engine", "postgres", "engine_version", "16.3", "db_instance_class", "db.m6g.large"),
				"000000000000/db-b": rdsLabels("000000000000", "db-b"),
			},
		},
		"empty attribute values are omitted": {
			rds: &fakeRDS{instances: []rdstypes.DBInstance{
				{DBInstanceIdentifier: aws.String("db-a"), Engine: aws.String("mariadb"), EngineVersion: aws.String("")},
				{Engine: aws.String("mysql")}, // no identifier: ignored
			}},
			want: map[string]map[string]string{
				"000000000000/db-a": rdsLabels("000000000000", "db-a", "engine", "mariadb"),
				"000000000000/db-b": rdsLabels("000000000000", "db-b"),
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cw := &fullCloudWatch{list: map[string][]cwtypes.Metric{"AWS/RDS": rdsMetrics("db-a", "db-b")}, gmdValue: 1}
			c := rdsAttributeCollector(t, rdsProfileConfig("base"), cw,
				map[string]string{"base": "000000000000"}, map[string]rdsClient{"base": tc.rds})

			_, err := collecttest.CollectScalarSeries(c)
			require.NoError(t, err, "attribute lookup never fails collection")
			assert.Equal(t, tc.want, labelsByInstance(c, rdsCPUSeries))
			assert.Equal(t, 1, tc.rds.callCount())
			require.Len(t, tc.rds.inputs, 1)
			assert.Equal(t, int32(rdsDescribePageSize), aws.ToInt32(tc.rds.inputs[0].MaxRecords))
		})
	}
}

func TestCollect_RDSAttributesRefreshCadenceAndStability(t *testing.T) {
	fake := &fakeRDS{instances: []rdstypes.DBInstance{rdsInstance("db-a", "mysql", "8.0.35", "db.t3.micro")}}
	cw := &fullCloudWatch{list: map[string][]cwtypes.Metric{"AWS/RDS": rdsMetrics("db-a")}, gmdValue: 1}
	c := rdsAttributeCollector(t, rdsProfileConfig("base"), cw,
		map[string]string{"base": "000000000000"}, map[string]rdsClient{"base": fake})
	now := time.Unix(1_000_000_000, 0)
	c.now = func() time.Time { return now }
	collect := func() map[string]string {
		t.Helper()
		_, err := collecttest.CollectScalarSeries(c)
		require.NoError(t, err)
		labels := labelsByInstance(c, rdsCPUSeries)
		require.Len(t, labels, 1, "one chart instance throughout")
		return labels["000000000000/db-a"]
	}
	queryKeys := func() []structuralID {
		keys := make([]structuralID, 0, len(c.queryPlan))
		for _, q := range c.queryPlan {
			keys = append(keys, q.key)
		}
		return keys
	}

	assert.Equal(t, "8.0.35", collect()["engine_version"])
	initialKeys := queryKeys()
	require.NotEmpty(t, initialKeys)

	now = now.Add(time.Minute)
	collect()
	assert.Equal(t, 1, fake.callCount(), "no describe call before the discovery TTL expires")

	now = now.Add(5 * time.Minute)
	fake.set(nil, errors.New("throttled"))
	assert.Equal(t, rdsLabels("000000000000", "db-a", "engine", "mysql", "engine_version", "8.0.35", "db_instance_class", "db.t3.micro"),
		collect(), "a failed refresh keeps the last-known labels")
	assert.Equal(t, 2, fake.callCount())

	now = now.Add(time.Minute)
	collect()
	assert.Equal(t, 2, fake.callCount(), "a failed refresh retries at the next TTL, not every collect")

	now = now.Add(5 * time.Minute)
	fake.set([]rdstypes.DBInstance{rdsInstance("db-a", "mysql", "8.4.3", "db.t3.small")}, nil)
	got := collect()
	assert.Equal(t, rdsLabels("000000000000", "db-a", "engine", "mysql", "engine_version", "8.4.3", "db_instance_class", "db.t3.small"), got)
	assert.Equal(t, 3, fake.callCount())
	assert.Equal(t, initialKeys, queryKeys(), "an attribute change never changes stable query identity")
}

func TestCollect_RDSAttributesSkipUnusedScopes(t *testing.T) {
	tests := map[string]struct {
		profiles []string
		list     map[string][]cwtypes.Metric
	}{
		"RDS selected but no RDS instances discovered": {
			profiles: []string{"rds", "ec2"},
			list:     map[string][]cwtypes.Metric{"AWS/EC2": {mkMetric("CPUUtilization", "InstanceId", "i-1")}},
		},
		"RDS not selected": {
			profiles: []string{"ec2"},
			list: map[string][]cwtypes.Metric{
				"AWS/EC2": {mkMetric("CPUUtilization", "InstanceId", "i-1")},
				"AWS/RDS": rdsMetrics("db-a"),
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := rdsProfileConfig("base")
			cfg.Rules[0].Profiles.Include = tc.profiles
			fake := &fakeRDS{instances: []rdstypes.DBInstance{rdsInstance("db-a", "mysql", "8.0.35", "db.t3.micro")}}
			c := rdsAttributeCollector(t, cfg, &fullCloudWatch{list: tc.list, gmdValue: 1},
				map[string]string{"base": "000000000000"}, map[string]rdsClient{"base": fake})

			_, err := collecttest.CollectScalarSeries(c)
			require.NoError(t, err)
			assert.Zero(t, fake.callCount())
		})
	}
}

func TestCollect_RDSAttributesArePerTarget(t *testing.T) {
	// Two targets in different accounts expose an instance with the same identifier;
	// each chart must carry its own account's attributes.
	cw := &fullCloudWatch{list: map[string][]cwtypes.Metric{"AWS/RDS": rdsMetrics("db-a")}, gmdValue: 1}
	first := &fakeRDS{instances: []rdstypes.DBInstance{rdsInstance("db-a", "mysql", "8.0.35", "db.t3.micro")}}
	second := &fakeRDS{err: errors.New("access denied")}
	c := rdsAttributeCollector(t, rdsProfileConfig("first", "second"), cw,
		map[string]string{"first": "111111111111", "second": "222222222222"},
		map[string]rdsClient{"first": first, "second": second})

	_, err := collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	assert.Equal(t, map[string]map[string]string{
		"111111111111/db-a": rdsLabels("111111111111", "db-a", "engine", "mysql", "engine_version", "8.0.35", "db_instance_class", "db.t3.micro"),
		"222222222222/db-a": rdsLabels("222222222222", "db-a"),
	}, labelsByInstance(c, rdsCPUSeries))
	assert.Equal(t, 1, first.callCount())
	assert.Equal(t, 1, second.callCount())
}

func TestCollect_RDSAttributeLabelWinsOverResourceTag(t *testing.T) {
	cfg := rdsProfileConfig("base")
	cfg.Labels.ResourceTags = []ResourceTagLabelConfig{{Key: "engine"}, {Key: "owner"}}
	cw := &fullCloudWatch{list: map[string][]cwtypes.Metric{"AWS/RDS": rdsMetrics("db-a")}, gmdValue: 1}
	fake := &fakeRDS{instances: []rdstypes.DBInstance{rdsInstance("db-a", "postgres", "16.3", "db.m6g.large")}}
	c := rdsAttributeCollector(t, cfg, cw, map[string]string{"base": "000000000000"}, map[string]rdsClient{"base": fake})
	c.newRGTAClient = func(aws.Config) rgtaClient {
		return &fakeRGTA{resources: []rgtatypes.ResourceTagMapping{
			rgtaResource("arn:aws:rds:us-east-1:000000000000:db:db-a", "engine", "tagged-value", "owner", "dba"),
		}}
	}

	_, err := collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	assert.Equal(t, map[string]map[string]string{
		"000000000000/db-a": rdsLabels("000000000000", "db-a",
			"engine", "postgres", "engine_version", "16.3", "db_instance_class", "db.m6g.large", "owner", "dba"),
	}, labelsByInstance(c, rdsCPUSeries))
}

func TestDescribeRDSInstanceAttributes(t *testing.T) {
	tests := map[string]struct {
		rds       *fakeRDS
		want      map[string][]metrix.Label
		wantErr   bool
		wantCalls int
	}{
		"follows markers across pages": {
			rds: &fakeRDS{pages: [][]rdstypes.DBInstance{
				{rdsInstance("db-a", "mysql", "8.0.35", "db.t3.micro")},
				{rdsInstance("db-b", "sqlserver-se", "16.00", "db.m5.xlarge")},
			}},
			want: map[string][]metrix.Label{
				"db-a": {{Key: "engine", Value: "mysql"}, {Key: "engine_version", Value: "8.0.35"}, {Key: "db_instance_class", Value: "db.t3.micro"}},
				"db-b": {{Key: "engine", Value: "sqlserver-se"}, {Key: "engine_version", Value: "16.00"}, {Key: "db_instance_class", Value: "db.m5.xlarge"}},
			},
			wantCalls: 2,
		},
		"duplicate marker fails instead of looping": {
			rds:       &fakeRDS{marker: func(int) *string { return aws.String("same") }},
			wantErr:   true,
			wantCalls: 2,
		},
		"page cap fails instead of truncating": {
			rds:       &fakeRDS{marker: func(call int) *string { return aws.String(fmt.Sprint(call)) }},
			wantErr:   true,
			wantCalls: rdsDescribeMaxPages,
		},
		"API error propagates": {
			rds:       &fakeRDS{err: errors.New("boom")},
			wantErr:   true,
			wantCalls: 1,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := describeRDSInstanceAttributes(context.Background(), tc.rds, time.Second)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.want, got)
			}
			assert.Equal(t, tc.wantCalls, tc.rds.callCount())
			markers := make([]string, 0, len(tc.rds.inputs))
			for _, in := range tc.rds.inputs {
				markers = append(markers, aws.ToString(in.Marker))
			}
			if tc.wantCalls > 1 && !tc.wantErr {
				assert.Equal(t, []string{"", "page-1"}, markers)
			}
			assert.False(t, slices.ContainsFunc(tc.rds.inputs, func(in *rds.DescribeDBInstancesInput) bool {
				return aws.ToInt32(in.MaxRecords) != rdsDescribePageSize
			}), "every page requests the maximum page size")
		})
	}
}

func TestResolveAttributeProvider(t *testing.T) {
	rdsDim := cwprofiles.InstanceDimension{Name: "DBInstanceIdentifier", Label: "db_instance_identifier"}
	rdsProfile := func(dims ...cwprofiles.InstanceDimension) cwprofiles.Profile {
		return cwprofiles.Profile{Namespace: "AWS/RDS", Instance: cwprofiles.InstanceSpec{Dimensions: dims}}
	}
	constant := "fixed"
	tests := map[string]struct {
		profile     cwprofiles.Profile
		want        profileAttributes
		wantFound   bool
		wantSkipped bool
	}{
		"RDS instance profile": {
			profile:   rdsProfile(rdsDim),
			want:      profileAttributes{provider: rdsInstanceAttributes},
			wantFound: true,
		},
		"join dimension after another dimension": {
			profile:   rdsProfile(cwprofiles.InstanceDimension{Name: "VolumeName", Label: "volume_name"}, rdsDim),
			want:      profileAttributes{provider: rdsInstanceAttributes, dimIndex: 1},
			wantFound: true,
		},
		"constant join dimension describes an aggregate": {
			profile: rdsProfile(cwprofiles.InstanceDimension{Name: "DBInstanceIdentifier", Constant: &constant}),
		},
		"RDS profile without the instance dimension": {
			profile: rdsProfile(cwprofiles.InstanceDimension{Name: "EngineName", Label: "engine_name"}),
		},
		"same dimension in another namespace": {
			profile: cwprofiles.Profile{Namespace: "AWS/DocDB", Instance: cwprofiles.InstanceSpec{Dimensions: []cwprofiles.InstanceDimension{rdsDim}}},
		},
		"identity label already uses an attribute label": {
			profile:     rdsProfile(rdsDim, cwprofiles.InstanceDimension{Name: "EngineName", Label: "engine"}),
			wantSkipped: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, ok, skipped := resolveAttributeProvider(tc.profile)
			assert.Equal(t, tc.wantFound, ok)
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.wantSkipped, skipped != "")
		})
	}
}

func TestRefreshAttributes_InvalidatesPlanOnlyOnLabelChange(t *testing.T) {
	fake := &fakeRDS{instances: []rdstypes.DBInstance{rdsInstance("db-a", "mysql", "8.0.35", "db.t3.micro")}}
	cw := &fullCloudWatch{list: map[string][]cwtypes.Metric{"AWS/RDS": rdsMetrics("db-a")}, gmdValue: 1}
	c := rdsAttributeCollector(t, rdsProfileConfig("base"), cw,
		map[string]string{"base": "000000000000"}, map[string]rdsClient{"base": fake})
	_, err := collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	require.False(t, c.planDirty)

	c.markAttributesStale()
	c.refreshAttributes(context.Background())
	assert.False(t, c.planDirty, "an unchanged result keeps the query plan")

	c.markAttributesStale()
	fake.set(nil, errors.New("throttled"))
	c.refreshAttributes(context.Background())
	assert.False(t, c.planDirty, "a failed refresh keeps the last-known labels and the plan")

	c.markAttributesStale()
	fake.set([]rdstypes.DBInstance{
		rdsInstance("db-a", "mysql", "8.0.35", "db.t3.micro"),
		rdsInstance("db-undiscovered", "postgres", "16.3", "db.t3.micro"),
	}, nil)
	c.refreshAttributes(context.Background())
	assert.False(t, c.planDirty, "an undiscovered instance is not cached and does not rebuild the plan")
	assert.Equal(t, []string{"db-a"}, slices.Collect(maps.Keys(c.attributes[attributeGroupKey{
		target: "base", region: "us-east-1", provider: rdsInstanceAttributes.name,
	}].labels)))

	c.markAttributesStale()
	fake.set([]rdstypes.DBInstance{rdsInstance("db-a", "mysql", "8.4.3", "db.t3.micro")}, nil)
	c.refreshAttributes(context.Background())
	assert.True(t, c.planDirty, "a changed label set rebuilds the plan once")
	assert.Equal(t, 5, fake.callCount())

	c.planDirty = false
	before := c.attributes
	c.refreshAttributes(context.Background())
	assert.Equal(t, 5, fake.callCount(), "nothing due: no call")
	assert.False(t, c.planDirty)
	assert.Equal(t, reflect.ValueOf(before).Pointer(), reflect.ValueOf(c.attributes).Pointer(),
		"nothing due: the cache map is kept, not rebuilt")
}

func TestRefreshAttributes_ParentCancellationCommitsNothing(t *testing.T) {
	fake := &fakeRDS{instances: []rdstypes.DBInstance{rdsInstance("db-a", "mysql", "8.0.35", "db.t3.micro")}}
	cw := &fullCloudWatch{list: map[string][]cwtypes.Metric{"AWS/RDS": rdsMetrics("db-a")}, gmdValue: 1}
	c := rdsAttributeCollector(t, rdsProfileConfig("base"), cw,
		map[string]string{"base": "000000000000"}, map[string]rdsClient{"base": fake})
	_, err := collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	before := c.attributes

	c.markAttributesStale()
	fake.set([]rdstypes.DBInstance{rdsInstance("db-a", "postgres", "16.3", "db.t3.micro")}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c.refreshAttributes(ctx)
	assert.Equal(t, before, c.attributes, "a canceled refresh installs nothing")
}

func TestCollect_RDSAttributeLabelsAreNonIdentityChartLabels(t *testing.T) {
	fake := &fakeRDS{instances: []rdstypes.DBInstance{rdsInstance("db-a", "mysql", "8.0.35", "db.t3.micro")}}
	cw := &fullCloudWatch{list: map[string][]cwtypes.Metric{"AWS/RDS": rdsMetrics("db-a")}, gmdValue: 1}
	c := rdsAttributeCollector(t, rdsProfileConfig("base"), cw,
		map[string]string{"base": "000000000000"}, map[string]rdsClient{"base": fake})
	now := time.Unix(1_000_000_000, 0)
	c.now = func() time.Time { return now }

	source, err := collectorapi.NewChartTemplateSource(c)
	require.NoError(t, err)
	engine, err := chartengine.New()
	require.NoError(t, err)
	plan := func() chartengine.Plan {
		t.Helper()
		_, err := collecttest.CollectScalarSeries(c)
		require.NoError(t, err)
		set, err := source.Capture()
		require.NoError(t, err)
		attempt, err := engine.PreparePlanWithOptions(c.MetricStore().Read(metrix.ReadRaw(), metrix.ReadFlatten()), chartengine.PlanOptions{TemplateSet: set})
		require.NoError(t, err)
		p := attempt.Plan()
		require.NoError(t, attempt.Commit())
		return p
	}
	created := func(p chartengine.Plan) map[string]map[string]string {
		out := make(map[string]map[string]string)
		for _, action := range p.Actions {
			if create, ok := action.(chartengine.CreateChartAction); ok && create.Meta.Context == "cloudwatch.rds.cpu_utilization" {
				out[create.ChartID] = create.Labels
			}
		}
		return out
	}
	relabeled := func(p chartengine.Plan) map[string]map[string]string {
		out := make(map[string]map[string]string)
		for _, action := range p.Actions {
			if update, ok := action.(chartengine.UpdateChartLabelsAction); ok {
				out[update.ChartID] = update.Labels
			}
		}
		return out
	}

	first := created(plan())
	require.Len(t, first, 1)
	var chartID string
	for id, labels := range first {
		chartID = id
		assert.Equal(t, "mysql", labels["engine"])
		assert.Equal(t, "8.0.35", labels["engine_version"])
		assert.Equal(t, "db.t3.micro", labels["db_instance_class"])
		assert.NotContains(t, id, "mysql", "attribute labels are not part of chart identity")
	}

	now = now.Add(6 * time.Minute)
	fake.set([]rdstypes.DBInstance{rdsInstance("db-a", "mysql", "8.4.3", "db.t3.micro")}, nil)
	second := plan()
	assert.Empty(t, created(second), "a changed attribute value never creates another chart")
	updated := relabeled(second)
	require.Contains(t, updated, chartID, "the existing chart gets a label-only update")
	assert.Equal(t, "8.4.3", updated[chartID]["engine_version"])
}

func TestCollect_RDSAttributeLabelsAppearOnceLookupSucceeds(t *testing.T) {
	// The permission is granted after the job started: the first lookup fails, a later
	// one succeeds, and the existing chart gains the labels without a new identity.
	fake := &fakeRDS{err: &smithy.GenericAPIError{Code: "AccessDenied"}}
	cw := &fullCloudWatch{list: map[string][]cwtypes.Metric{"AWS/RDS": rdsMetrics("db-a")}, gmdValue: 1}
	c := rdsAttributeCollector(t, rdsProfileConfig("base"), cw,
		map[string]string{"base": "000000000000"}, map[string]rdsClient{"base": fake})
	now := time.Unix(1_000_000_000, 0)
	c.now = func() time.Time { return now }

	_, err := collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	assert.Equal(t, map[string]map[string]string{"000000000000/db-a": rdsLabels("000000000000", "db-a")},
		labelsByInstance(c, rdsCPUSeries))

	now = now.Add(6 * time.Minute)
	fake.set([]rdstypes.DBInstance{rdsInstance("db-a", "mysql", "8.0.35", "db.t3.micro")}, nil)
	_, err = collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	assert.Equal(t, map[string]map[string]string{
		"000000000000/db-a": rdsLabels("000000000000", "db-a", "engine", "mysql", "engine_version", "8.0.35", "db_instance_class", "db.t3.micro"),
	}, labelsByInstance(c, rdsCPUSeries))
}

func TestCollect_RDSAttributesFollowDiscoveryInstall(t *testing.T) {
	// Discovery can install a new snapshot before the attribute TTL expires (real
	// clocks drift by the stage duration, and a late target resolution forces a
	// rediscovery). A newly discovered instance must still be labeled on its first plan.
	fake := &fakeRDS{instances: []rdstypes.DBInstance{rdsInstance("db-a", "mysql", "8.0.35", "db.t3.micro")}}
	cw := &fullCloudWatch{list: map[string][]cwtypes.Metric{"AWS/RDS": rdsMetrics("db-a")}, gmdValue: 1}
	c := rdsAttributeCollector(t, rdsProfileConfig("base"), cw,
		map[string]string{"base": "000000000000"}, map[string]rdsClient{"base": fake})
	now := time.Unix(1_000_000_000, 0)
	c.now = func() time.Time { return now }
	_, err := collecttest.CollectScalarSeries(c)
	require.NoError(t, err)

	now = now.Add(100 * time.Second)
	cw.mu.Lock()
	cw.list["AWS/RDS"] = rdsMetrics("db-a", "db-new")
	cw.mu.Unlock()
	fake.set([]rdstypes.DBInstance{
		rdsInstance("db-a", "mysql", "8.0.35", "db.t3.micro"),
		rdsInstance("db-new", "postgres", "16.3", "db.t3.micro"),
	}, nil)
	c.markDiscoveryStale()
	_, err = collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	assert.Equal(t, map[string]map[string]string{
		"000000000000/db-a":   rdsLabels("000000000000", "db-a", "engine", "mysql", "engine_version", "8.0.35", "db_instance_class", "db.t3.micro"),
		"000000000000/db-new": rdsLabels("000000000000", "db-new", "engine", "postgres", "engine_version", "16.3", "db_instance_class", "db.t3.micro"),
	}, labelsByInstance(c, rdsCPUSeries))
	assert.Equal(t, 2, fake.callCount())
}

func TestCollect_RDSAttributesLateResolvedTargetLabeledOnFirstPlan(t *testing.T) {
	cw := &fullCloudWatch{list: map[string][]cwtypes.Metric{"AWS/RDS": rdsMetrics("db-a")}, gmdValue: 1}
	first := &fakeRDS{instances: []rdstypes.DBInstance{rdsInstance("db-a", "mysql", "8.0.35", "db.t3.micro")}}
	second := &fakeRDS{instances: []rdstypes.DBInstance{rdsInstance("db-a", "postgres", "16.3", "db.m6g.large")}}
	c := rdsAttributeCollector(t, rdsProfileConfig("first", "second"), cw,
		map[string]string{"first": "111111111111"}, map[string]rdsClient{"first": first, "second": second})
	secondSTS := &seqSTS{accounts: []string{"", "222222222222"}, failAt: map[int]bool{0: true}}
	c.newSTSClient = func(cfg aws.Config) stsClient {
		if cfg.AppID == "second" {
			return secondSTS
		}
		return &fakeSTS{account: "111111111111"}
	}
	now := time.Unix(1_000_000_000, 0)
	c.now = func() time.Time { return now }

	_, err := collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	require.Len(t, labelsByInstance(c, rdsCPUSeries), 1, "only the resolved target collects")

	now = now.Add(time.Minute)
	_, err = collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	assert.Equal(t, map[string]map[string]string{
		"111111111111/db-a": rdsLabels("111111111111", "db-a", "engine", "mysql", "engine_version", "8.0.35", "db_instance_class", "db.t3.micro"),
		"222222222222/db-a": rdsLabels("222222222222", "db-a", "engine", "postgres", "engine_version", "16.3", "db_instance_class", "db.m6g.large"),
	}, labelsByInstance(c, rdsCPUSeries), "the late target's first charts carry its attributes")
	assert.Equal(t, 1, second.callCount())
}
