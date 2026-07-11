// SPDX-License-Identifier: GPL-3.0-or-later

package cwprofiles

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/profilecatalog"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const minimalProfileYAML = `version: v1
display_name: Test
namespace: AWS/Test
period: 60
instance:
  dimensions:
    - name: InstanceId
      label: instance_id
metrics:
  - id: m1
    metric_name: M1
    statistics: [average]
template:
  family: Test
  context_namespace: test
  chart_defaults:
    instances:
      by_labels: [account_id, region, instance_id]
  charts:
    - id: c1
      context: c1
      title: C1
      family: C1
      units: count
      algorithm: absolute
      dimensions:
        - selector: m1_average
          name: avg
`

func TestLoadFromDefaultDirs_LoadsStockProfiles(t *testing.T) {
	dir := cwProfilesDirFromThisFile()
	require.NotEmpty(t, dir)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)

	var want int
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext == ".yaml" || ext == ".yml" {
			want++
		}
	}

	catalog, err := LoadFromDefaultDirs()
	require.NoError(t, err)

	assert.Equal(t, want, catalog.Len())

	// Lock the curated stock set and its namespaces so a renamed/dropped
	// profile is caught.
	wantNamespaces := map[string]string{
		"ec2":               "AWS/EC2",
		"rds":               "AWS/RDS",
		"elb":               "AWS/ELB",
		"alb":               "AWS/ApplicationELB",
		"alb_target_health": "AWS/ApplicationELB",
		"s3":                "AWS/S3",
		"lambda":            "AWS/Lambda",
		"sqs":               "AWS/SQS",
		"dynamodb":          "AWS/DynamoDB",
		"nlb":               "AWS/NetworkELB",
		"nlb_target_health": "AWS/NetworkELB",
		"nat_gateway":       "AWS/NATGateway",
		"step_functions":    "AWS/States",
		"api_gateway":       "AWS/ApiGateway",
		"kinesis":           "AWS/Kinesis",
		"firehose":          "AWS/Firehose",
		"sns":               "AWS/SNS",
		"ebs":               "AWS/EBS",
		"efs":               "AWS/EFS",
		"ecs":               "AWS/ECS",
		"elasticache":       "AWS/ElastiCache",
		"opensearch":        "AWS/ES",
		"docdb":             "AWS/DocDB",
		"redshift":          "AWS/Redshift",
		"msk":               "AWS/Kafka",
		"msk_cluster":       "AWS/Kafka",
		"cloudfront":        "AWS/CloudFront",
		"auto_scaling":      "AWS/AutoScaling",
		"bedrock":           "AWS/Bedrock",
		"eventbridge":       "AWS/Events",
		"vpn":               "AWS/VPN",
		"eks":               "AWS/EKS",
		// opt-in profiles (disabled by default; rules may include them explicitly)
		"alb_target":         "AWS/ApplicationELB",
		"dynamodb_operation": "AWS/DynamoDB",
		"ebs_stalled_io":     "AWS/EBS",
		"s3_requests":        "AWS/S3",
	}
	for baseName, namespace := range wantNamespaces {
		prof, ok := catalog.Get(baseName)
		if assert.Truef(t, ok, "missing stock profile %q", baseName) {
			assert.Equalf(t, namespace, prof.Namespace, "profile %q namespace", baseName)
			if baseName == "cloudfront" {
				assert.Equal(t, []string{"us-east-1"}, prof.SupportedRegions)
			} else {
				assert.Emptyf(t, prof.SupportedRegions, "profile %q should remain region-unrestricted", baseName)
			}
		}
	}
}

func TestLoadFromDefaultDirs_StockProfilesUseSelectorShorthand(t *testing.T) {
	dir := cwProfilesDirFromThisFile()
	require.NotEmpty(t, dir)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		require.NoError(t, err)

		for line := range strings.SplitSeq(string(data), "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "- selector:") {
				continue
			}
			selector := strings.TrimSpace(strings.TrimPrefix(line, "- selector:"))
			if selector == "" || strings.HasPrefix(selector, "{") {
				continue
			}
			assert.NotContainsf(t, selector, ".", "stock profile %q should use selector shorthand, found %q", entry.Name(), selector)
		}
	}
}

func TestDecodeProfileBytes(t *testing.T) {
	tests := map[string]struct {
		data    string
		wantErr bool
	}{
		"valid": {
			data: minimalProfileYAML,
		},
		// The decoder is non-strict: unknown keys are ignored (forward-compat — an
		// older collector tolerates profiles carrying newer optional fields).
		"unknown top-level key ignored": {
			data: minimalProfileYAML + "bogus_key: 1\n",
		},
		"unknown nested chart key ignored": {
			data: strings.Replace(minimalProfileYAML, "      algorithm: absolute\n", "      algorithm: absolute\n      bogus_chart_key: x\n", 1),
		},
		"unknown nested metric key ignored": {
			data: strings.Replace(minimalProfileYAML, "    metric_name: M1\n", "    metric_name: M1\n    bogus_metric_key: x\n", 1),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := decodeProfileBytes([]byte(tc.data), "test")
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestLoadFromDirs_UserOverridesStock(t *testing.T) {
	stockDir := t.TempDir()
	userDir := t.TempDir()

	writeFile(t, filepath.Join(stockDir, "ec2.yaml"), minimalProfileYAML)
	writeFile(t, filepath.Join(userDir, "ec2.yaml"), strings.Replace(minimalProfileYAML, "display_name: Test", "display_name: UserOverride", 1))

	catalog, err := loadFromDirs([]profilecatalog.DirSpec{
		{Path: userDir, IsStock: false},
		{Path: stockDir, IsStock: true},
	})
	require.NoError(t, err)

	prof, ok := catalog.Get("ec2")
	require.True(t, ok)
	assert.Equal(t, "UserOverride", prof.DisplayName)
	assert.True(t, catalog.HasStock("ec2"))
}

func TestLoadFromDirs_InvalidUserProfileSkipped(t *testing.T) {
	stockDir := t.TempDir()
	userDir := t.TempDir()

	writeFile(t, filepath.Join(stockDir, "ec2.yaml"), minimalProfileYAML)
	writeFile(t, filepath.Join(userDir, "broken.yaml"), "version: v1\nnot_a_real_key: true\n")

	catalog, err := loadFromDirs([]profilecatalog.DirSpec{
		{Path: userDir, IsStock: false},
		{Path: stockDir, IsStock: true},
	})
	require.NoError(t, err)

	assert.True(t, catalog.Has("ec2"))
	assert.False(t, catalog.Has("broken"))
}

func TestLoadFromDirs_InvalidStockProfileFatal(t *testing.T) {
	stockDir := t.TempDir()
	writeFile(t, filepath.Join(stockDir, "broken.yaml"), "version: v1\nnot_a_real_key: true\n")

	_, err := loadFromDirs([]profilecatalog.DirSpec{{Path: stockDir, IsStock: true}})
	assert.Error(t, err)
}

func TestLoadFromDirs_EmptyCatalogIsError(t *testing.T) {
	emptyDir := t.TempDir()
	_, err := loadFromDirs([]profilecatalog.DirSpec{{Path: emptyDir, IsStock: true}})
	assert.Error(t, err, "an empty catalog must be rejected")
}

func TestCatalog_AllProfiles(t *testing.T) {
	catalog, err := LoadFromDefaultDirs()
	require.NoError(t, err)

	names := baseNames(catalog)
	assert.Contains(t, names, "ec2")
	assert.Contains(t, names, "s3")
	assert.IsIncreasing(t, names)
}

func baseNames(c Catalog) []string {
	profiles := c.AllProfiles()
	names := make([]string, len(profiles))
	for i, p := range profiles {
		names[i] = p.Name
	}
	return names
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

func TestValidateUniqueChartIDs(t *testing.T) {
	mkNamed := func(name, chartID string) profilecatalog.Named[Profile] {
		return profilecatalog.Named[Profile]{
			Name:    name,
			Profile: Profile{Template: charttpl.Group{Charts: []charttpl.Chart{{ID: chartID}}}},
		}
	}
	tests := map[string]struct {
		profiles []profilecatalog.Named[Profile]
		stock    map[string]bool
		wantErr  bool
	}{
		"unique ids": {
			profiles: []profilecatalog.Named[Profile]{mkNamed("a", "aws_cloudwatch_a"), mkNamed("b", "aws_cloudwatch_b")},
			stock:    map[string]bool{"a": true, "b": true},
		},
		"stock-vs-stock duplicate is fatal": {
			profiles: []profilecatalog.Named[Profile]{mkNamed("a", "aws_cloudwatch_dup"), mkNamed("b", "aws_cloudwatch_dup")},
			stock:    map[string]bool{"a": true, "b": true},
			wantErr:  true,
		},
		"user-profile duplicate is tolerated (warned, not fatal)": {
			profiles: []profilecatalog.Named[Profile]{mkNamed("a", "aws_cloudwatch_dup"), mkNamed("b", "aws_cloudwatch_dup")},
			stock:    map[string]bool{"a": true, "b": false}, // "b" is a user profile
		},
		"user override of a stock profile is not misclassified as stock": {
			// "a" is a user override (effective origin user) that collides with stock
			// "b"; a user-involved collision warns, it must not be a fatal stock dup.
			profiles: []profilecatalog.Named[Profile]{mkNamed("a", "aws_cloudwatch_dup"), mkNamed("b", "aws_cloudwatch_dup")},
			stock:    map[string]bool{"a": false, "b": true},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			isStock := func(n string) bool { return tc.stock[n] }
			err := validateUniqueChartIDs(tc.profiles, isStock)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
