// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	rdstypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
	rgtatypes "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yamlv2 "gopkg.in/yaml.v2"
	"gopkg.in/yaml.v3"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/cwprofiles"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/profilecatalog"
)

// Automatic metadata must preserve the charts, dimensions and numeric routing of
// an already valid user template through arrival, changes, removal and recovery.
func TestCollect_RDSAttributesPreserveTemplateBehavior(t *testing.T) {
	fixed := func() *charttpl.Instances {
		return &charttpl.Instances{
			ByLabels: []string{"account_id", "region", "db_instance_identifier"},
		}
	}
	wildcard := func() *charttpl.Instances {
		return &charttpl.Instances{
			ByLabels: []string{"*"},
		}
	}
	all := []string{"engine", "engine_version", "db_instance_class"}
	tests := map[string]struct {
		configure func(*charttpl.Group)
		wantAuto  []string
		tag       bool
	}{
		"fixed identity":     {wantAuto: all},
		"inherited wildcard": {configure: func(g *charttpl.Group) { g.ChartDefaults.Instances = wildcard() }},
		"partial wildcard exclusions": {
			configure: func(g *charttpl.Group) {
				g.ChartDefaults.Instances = &charttpl.Instances{
					ByLabels: []string{"*", "!engine_version"},
				}
			},
			wantAuto: []string{"engine_version"},
		},
		"all automatic keys excluded from wildcard": {
			configure: func(g *charttpl.Group) {
				g.ChartDefaults.Instances = &charttpl.Instances{
					ByLabels: []string{"*", "!engine", "!engine_version", "!db_instance_class"},
				}
			},
			wantAuto: all,
		},
		"inherited optional identity": {
			configure: func(g *charttpl.Group) { g.ChartDefaults.Instances.OptionalByLabels = []string{"engine_version"} },
			wantAuto:  []string{"engine", "db_instance_class"},
		},
		"explicit identity on another chart": {
			configure: func(g *charttpl.Group) {
				other := g.Charts[0]
				other.ID = "attribute_explicit"
				other.Instances = fixed()
				other.Instances.ByLabels = append(other.Instances.ByLabels, "engine_version")
				g.Charts = append(g.Charts, other)
			},
			wantAuto: []string{"engine", "db_instance_class"},
		},
		"dimension name": {
			configure: func(g *charttpl.Group) {
				g.Charts[0].Dimensions = append(g.Charts[0].Dimensions, charttpl.Dimension{
					Selector:      rdsCPUSeries,
					NameFromLabel: "engine",
				})
			},
			wantAuto: []string{"engine_version", "db_instance_class"},
		},
		"dimension selector": {
			configure: func(g *charttpl.Group) { g.Charts[0].Dimensions[0].Selector += `{engine!="mysql"}` },
			wantAuto:  []string{"engine_version", "db_instance_class"},
		},
		"nested inherited wildcard": {
			configure: func(g *charttpl.Group) {
				g.Groups = []charttpl.Group{
					{Family: "Nested", ChartDefaults: &charttpl.ChartDefaults{
						Instances: wildcard(),
					}, Charts: g.Charts},
				}
				g.Charts = nil
			},
		},
		"chart overrides safe parent with wildcard": {
			configure: func(g *charttpl.Group) { g.Charts[0].Instances = wildcard() },
		},
		"chart overrides unsafe parent": {
			configure: func(g *charttpl.Group) {
				g.ChartDefaults.Instances = wildcard()
				g.Charts[0].Instances = fixed()
			},
			wantAuto: all,
		},
		"nested override clears inherited optional identity": {
			configure: func(g *charttpl.Group) {
				g.ChartDefaults.Instances.OptionalByLabels = []string{"engine_version"}
				g.Groups = []charttpl.Group{
					{Family: "Nested", ChartDefaults: &charttpl.ChartDefaults{
						Instances: fixed(),
					}, Charts: g.Charts},
				}
				g.Charts = nil
			},
			wantAuto: all,
		},
		"explicit tag retains identity ownership": {
			configure: func(g *charttpl.Group) { g.ChartDefaults.Instances.OptionalByLabels = []string{"engine_version"} },
			wantAuto:  []string{"engine", "db_instance_class"},
			tag:       true,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			group := charttpl.Group{
				Family:           "RDS",
				ContextNamespace: "rds",
				ChartDefaults: &charttpl.ChartDefaults{
					Instances: fixed(),
				},
				Charts: []charttpl.Chart{{
					ID: "attribute_cpu", Title: "CPU", Context: "cpu_utilization", Units: "percentage", Algorithm: "absolute",
					Dimensions: []charttpl.Dimension{{Selector: rdsCPUSeries, Name: "cpu"}},
				}},
			}
			if tc.configure != nil {
				tc.configure(&group)
			}
			fake := mysqlRDSInstance()
			c := customRDSAttributeCollector(t, group, fake, tc.tag)
			now := time.Unix(1_000_000_000, 0)
			c.now = func() time.Time { return now }
			source, err := collectorapi.NewChartTemplateSource(c)
			require.NoError(t, err)
			engine, err := chartengine.New()
			require.NoError(t, err)
			tracked := make(map[string]bool)
			cycle := func(wait bool) map[string]map[string]float64 {
				t.Helper()
				if wait {
					_, err = collectWithAttributes(t, c)
				} else {
					_, err = collecttest.CollectScalarSeries(c)
				}
				require.NoError(t, err)
				set, err := source.Capture()
				require.NoError(t, err)
				attempt, err := engine.PreparePlanWithOptions(
					c.MetricStore().Read(metrix.ReadRaw(), metrix.ReadFlatten()),
					chartengine.PlanOptions{
						TemplateSet: set,
					},
				)
				require.NoError(t, err)
				state := make(map[string]map[string]float64)
				for _, action := range attempt.Plan().Actions {
					switch a := action.(type) {
					case chartengine.CreateChartAction:
						if strings.HasPrefix(a.Meta.Context, "cloudwatch.rds.") {
							tracked[a.ChartID] = true
						}
					case chartengine.UpdateChartAction:
						if !tracked[a.ChartID] {
							continue
						}
						state[a.ChartID] = make(map[string]float64)
						for _, value := range a.Values {
							if !value.IsEmpty {
								state[a.ChartID][value.Name] = value.Float64
							}
						}
					}
				}
				require.NoError(t, attempt.Commit())
				return state
			}
			baseline := cycle(false)
			require.Len(t, baseline, 1)
			for _, dims := range baseline {
				require.Equal(t, map[string]float64{"cpu": 1}, dims)
			}
			for _, phase := range []string{"arrival", "changed", "removed", "recovered"} {
				if phase != "arrival" {
					now = now.Add(6 * time.Minute)
					var instances []rdstypes.DBInstance
					if phase != "removed" {
						instances = []rdstypes.DBInstance{rdsInstance("db-a", "postgres", "16", "db.m6g.large")}
					}
					fake.set(instances, nil)
				}
				assert.Equal(
					t,
					baseline,
					cycle(true),
					phase+": automatic metadata must preserve chart IDs, dimensions and values",
				)
				want := rdsLabels("000000000000", "db-a")
				if phase != "removed" {
					values := map[string]string{
						"engine":            "postgres",
						"engine_version":    "16",
						"db_instance_class": "db.m6g.large",
					}
					if phase == "arrival" {
						values = map[string]string{
							"engine":            "mysql",
							"engine_version":    "8.0.35",
							"db_instance_class": "db.t3.micro",
						}
					}
					for _, key := range tc.wantAuto {
						want[key] = values[key]
					}
				}
				if tc.tag {
					want["engine_version"] = "operator-value"
				}
				assert.Equal(t, want, labelsByInstance(c, rdsCPUSeries)["000000000000/db-a"], phase)
			}
			if len(tc.wantAuto) == 0 {
				assert.Zero(t, fake.callCount(), "no lookup when every automatic label is ineligible")
			}
		})
	}
}

func customRDSAttributeCollector(t *testing.T, group charttpl.Group, rds rdsClient, tag bool) *Collector {
	t.Helper()
	stock, err := cwprofiles.DefaultCatalog()
	require.NoError(t, err)
	profile, ok := stock.Get("rds")
	require.True(t, ok)
	profile.Template = group
	data, err := yamlv2.Marshal(profile)
	require.NoError(t, err)
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "rds.yaml"), data, 0600))
	core, err := profilecatalog.Load([]profilecatalog.DirSpec{{Path: dir}}, profilecatalog.Options[cwprofiles.Profile]{
		Decode: func(fc profilecatalog.FileContext, data []byte) (cwprofiles.Profile, error) {
			var p cwprofiles.Profile
			if err := yaml.Unmarshal(data, &p); err != nil {
				return p, err
			}
			if err := p.Normalize(fc.BaseName); err != nil {
				return p, err
			}
			return p, p.Validate("profile", fc.BaseName)
		},
	})
	require.NoError(t, err)
	require.True(t, core.Has("rds"), "user profile must pass catalog decoding and validation")
	c := New()
	c.Config = rdsProfileConfig("base")
	c.newCatalog = func() (cwprofiles.Catalog, error) {
		return cwprofiles.Catalog{
			Catalog: core,
		}, nil
	}
	useFakeClient(c, &fullCloudWatch{
		list:     map[string][]cwtypes.Metric{"AWS/RDS": rdsMetrics("db-a")},
		gmdValue: 1,
	})
	c.newSTSClient = func(aws.Config) stsClient {
		return &fakeSTS{
			account: "000000000000",
		}
	}
	c.newRDSClient = func(aws.Config) rdsClient { return rds }
	if tag {
		c.Labels.ResourceTags = []ResourceTagLabelConfig{{Key: "version", Label: "engine_version"}}
		c.newRGTAClient = func(aws.Config) rgtaClient {
			return &fakeRGTA{
				resources: []rgtatypes.ResourceTagMapping{
					rgtaResource("arn:aws:rds:us-east-1:000000000000:db:db-a", "version", "operator-value"),
				},
			}
		}
	}
	require.NoError(t, c.Init(t.Context()))
	require.NoError(t, c.Check(t.Context()))
	startAttributeRun(t, c)
	return c
}
