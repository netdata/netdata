// SPDX-License-Identifier: GPL-3.0-or-later

package cwprofiles

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"

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

	assert.Len(t, catalog.byBaseName, want)

	// Lock the curated stock set and its namespaces so a renamed/dropped
	// profile is caught.
	wantNamespaces := map[string]string{
		"ec2":            "AWS/EC2",
		"rds":            "AWS/RDS",
		"elb":            "AWS/ELB",
		"alb":            "AWS/ApplicationELB",
		"s3":             "AWS/S3",
		"lambda":         "AWS/Lambda",
		"sqs":            "AWS/SQS",
		"dynamodb":       "AWS/DynamoDB",
		"nlb":            "AWS/NetworkELB",
		"nat_gateway":    "AWS/NATGateway",
		"step_functions": "AWS/States",
		"api_gateway":    "AWS/ApiGateway",
		"kinesis":        "AWS/Kinesis",
		"firehose":       "AWS/Firehose",
		"sns":            "AWS/SNS",
		"ebs":            "AWS/EBS",
		"efs":            "AWS/EFS",
		"ecs":            "AWS/ECS",
		"elasticache":    "AWS/ElastiCache",
		"opensearch":     "AWS/ES",
		"docdb":          "AWS/DocDB",
		"redshift":       "AWS/Redshift",
		"msk":            "AWS/Kafka",
		// deep-grain profiles (disabled by default; namespaces.mode combined)
		"alb_target":         "AWS/ApplicationELB",
		"dynamodb_operation": "AWS/DynamoDB",
		"s3_requests":        "AWS/S3",
	}
	for baseName, namespace := range wantNamespaces {
		prof, ok := catalog.byBaseName[baseName]
		if assert.Truef(t, ok, "missing stock profile %q", baseName) {
			assert.Equalf(t, namespace, prof.Namespace, "profile %q namespace", baseName)
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

	catalog, err := LoadFromDirs([]DirSpec{
		{Path: userDir, IsStock: false},
		{Path: stockDir, IsStock: true},
	})
	require.NoError(t, err)

	assert.Equal(t, "UserOverride", catalog.byBaseName["ec2"].DisplayName)
	assert.Contains(t, catalog.stockProfileBaseNames, "ec2")
}

func TestLoadFromDirs_InvalidUserProfileSkipped(t *testing.T) {
	stockDir := t.TempDir()
	userDir := t.TempDir()

	writeFile(t, filepath.Join(stockDir, "ec2.yaml"), minimalProfileYAML)
	writeFile(t, filepath.Join(userDir, "broken.yaml"), "version: v1\nnot_a_real_key: true\n")

	catalog, err := LoadFromDirs([]DirSpec{
		{Path: userDir, IsStock: false},
		{Path: stockDir, IsStock: true},
	})
	require.NoError(t, err)

	assert.Contains(t, catalog.byBaseName, "ec2")
	assert.NotContains(t, catalog.byBaseName, "broken")
}

func TestLoadFromDirs_InvalidStockProfileFatal(t *testing.T) {
	stockDir := t.TempDir()
	writeFile(t, filepath.Join(stockDir, "broken.yaml"), "version: v1\nnot_a_real_key: true\n")

	_, err := LoadFromDirs([]DirSpec{{Path: stockDir, IsStock: true}})
	assert.Error(t, err)
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

func TestDefaultCatalog_CachesSuccessfulLoads(t *testing.T) {
	calls := 0
	restore := stubDefaultCatalog(t, func() bool { return true }, func() (Catalog, error) {
		calls++
		return testCatalog("ec2"), nil
	})
	defer restore()

	first, err := DefaultCatalog()
	require.NoError(t, err)
	second, err := DefaultCatalog()
	require.NoError(t, err)

	assert.Equal(t, 1, calls)
	assert.Equal(t, []string{"ec2"}, baseNames(first))
	assert.Equal(t, []string{"ec2"}, baseNames(second))
}

func TestDefaultCatalog_RetriesAfterFailure(t *testing.T) {
	calls := 0
	restore := stubDefaultCatalog(t, func() bool { return true }, func() (Catalog, error) {
		calls++
		if calls == 1 {
			return Catalog{}, errors.New("boom")
		}
		return testCatalog("s3"), nil
	})
	defer restore()

	_, err := DefaultCatalog()
	require.Error(t, err)
	catalog, err := DefaultCatalog()
	require.NoError(t, err)

	assert.Equal(t, 2, calls)
	assert.Equal(t, []string{"s3"}, baseNames(catalog))
}

func TestDefaultCatalog_DoesNotCacheWhenDisabled(t *testing.T) {
	calls := 0
	restore := stubDefaultCatalog(t, func() bool { return false }, func() (Catalog, error) {
		calls++
		return testCatalog("ec2"), nil
	})
	defer restore()

	_, err := DefaultCatalog()
	require.NoError(t, err)
	_, err = DefaultCatalog()
	require.NoError(t, err)

	assert.Equal(t, 2, calls)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

func stubDefaultCatalog(t *testing.T, cacheEnabled func() bool, loader func() (Catalog, error)) func() {
	t.Helper()

	defaultCatalogMu.Lock()
	prevCatalog := defaultCatalog
	prevLoaded := defaultCatalogLoaded
	prevLoader := defaultCatalogLoader
	prevEnabled := defaultCatalogCacheEnabled

	defaultCatalog = Catalog{}
	defaultCatalogLoaded = false
	defaultCatalogLoader = loader
	defaultCatalogCacheEnabled = cacheEnabled
	defaultCatalogMu.Unlock()

	return func() {
		defaultCatalogMu.Lock()
		defaultCatalog = prevCatalog
		defaultCatalogLoaded = prevLoaded
		defaultCatalogLoader = prevLoader
		defaultCatalogCacheEnabled = prevEnabled
		defaultCatalogMu.Unlock()
	}
}

func testCatalog(id string) Catalog {
	return Catalog{
		byBaseName:            map[string]Profile{id: {DisplayName: id}},
		stockProfileBaseNames: map[string]struct{}{id: {}},
	}
}

func TestValidateUniqueChartIDs(t *testing.T) {
	mkProfile := func(chartID string) Profile {
		return Profile{Template: charttpl.Group{Charts: []charttpl.Chart{{ID: chartID}}}}
	}
	mkCatalog := func(profiles map[string]Profile, stock map[string]bool) Catalog {
		return Catalog{byBaseName: profiles, entryIsStock: stock}
	}
	tests := map[string]struct {
		catalog Catalog
		wantErr bool
	}{
		"unique ids": {
			catalog: mkCatalog(
				map[string]Profile{"a": mkProfile("cw_a"), "b": mkProfile("cw_b")},
				map[string]bool{"a": true, "b": true}),
		},
		"stock-vs-stock duplicate is fatal": {
			catalog: mkCatalog(
				map[string]Profile{"a": mkProfile("cw_dup"), "b": mkProfile("cw_dup")},
				map[string]bool{"a": true, "b": true}),
			wantErr: true,
		},
		"user-profile duplicate is tolerated (warned, not fatal)": {
			catalog: mkCatalog(
				map[string]Profile{"a": mkProfile("cw_dup"), "b": mkProfile("cw_dup")},
				map[string]bool{"a": true, "b": false}), // "b" is a user profile
		},
		"user override of a stock profile is not misclassified as stock": {
			// "a" is a user override (effective origin user) that collides with stock
			// "b"; a user-involved collision warns, it must not be a fatal stock dup.
			catalog: mkCatalog(
				map[string]Profile{"a": mkProfile("cw_dup"), "b": mkProfile("cw_dup")},
				map[string]bool{"a": false, "b": true}),
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := tc.catalog.validateUniqueChartIDs()
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
