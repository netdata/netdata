// SPDX-License-Identifier: GPL-3.0-or-later

package secrets

import (
	"fmt"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestSecretJobSummariesAreBounded(t *testing.T) {
	tests := map[string]struct {
		names []string
	}{
		"many jobs": {
			names: func() []string {
				names := make([]string, 1_000)
				for index := range names {
					names[index] = fmt.Sprintf("module:job-%04d", index)
				}
				return names
			}(),
		},
		"one oversized job": {names: []string{strings.Repeat("x", maximumSecretJobSummaryBytes*2)}},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			summary := formatSecretJobNames(test.names)
			require.False(t, len(summary) > maximumSecretJobSummaryBytes)
			require.Contains(t, summary, "more")
		})
	}
}

func TestSecretImpactMessageHasOneCombinedBound(t *testing.T) {
	names := make([]string, 1_000)
	for index := range names {
		names[index] = fmt.Sprintf("module:job-%04d", index)
	}
	summary := formatSecretJobNames(names)
	tests := map[string]struct {
		validationOnly bool
	}{
		"validation": {validationOnly: true},
		"update":     {},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			message := secretImpactMessage(summary, summary, test.validationOnly)
			require.False(t, len(message) > maximumSecretJobSummaryBytes)
		})
	}
}

func TestSecretDependencyIndexTracksAcknowledgedPostimages(t *testing.T) {
	index := NewSecretDependencyIndex()
	tests := map[string]struct {
		id         string
		status     dyncfg.Status
		references []string
		remove     bool
	}{
		"running job": {
			id: "module_one", status: dyncfg.StatusRunning,
			references: []string{"vault:main", "aws-sm:prod"},
		},
		"accepted job": {id: "module_two", status: dyncfg.StatusAccepted, references: []string{"vault:main"}},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			config := map[string]any{
				"module":          "module",
				"name":            test.id[len("module_"):],
				"__source_type__": confgroup.TypeDyncfg,
			}
			for keyIndex, key := range test.references {
				config[fmt.Sprintf("secret_%d", keyIndex)] = "${store:" + key + ":value}"
			}
			payload, err := yaml.Marshal(config)
			require.NoError(t, err)
			commit, err := index.PrepareJobChange(
				test.id,
				&dyncfg.GraphConfig{
					ID:      test.id,
					Module:  "module",
					Name:    config["name"].(string),
					Status:  test.status.String(),
					Payload: payload,
				},
			)
			require.NoError(t, err)
			commit()
		})
	}

	require.EqualValues(t, 2, len(index.Affected("vault:main", false)))

	refs := index.Affected("vault:main", true)
	require.False(t, len(refs) != 1 || refs[0].ID != "module_one")
	require.True(t, index.Affects("vault:main", "module_one", true))
	require.False(t, index.Affects("vault:main", "module_two", true))
	require.True(t, index.Affects("vault:main", "module_two", false))

	creators, err := secretstore.NewCreatorCatalog([]secretstore.Creator{{
		Kind: secretstore.KindVault,
		Create: func() secretstore.Store {
			return &transactionTestStore{}
		},
	}})
	require.NoError(t, err)
	controller := &Controller{
		prefix:       "go.d:secretstore:",
		creators:     creators,
		dependencies: index,
	}
	require.True(t, controller.CompositeChildLaneConflict(
		CommandInput{Args: []string{"go.d:secretstore:vault:main", "update"}},
		"module_one",
	))
	require.True(t, controller.CompositeChildLaneConflict(
		CommandInput{Args: []string{"go.d:secretstore:vault", "add", "main"}},
		"module_one",
	))
	require.False(t, controller.CompositeChildLaneConflict(
		CommandInput{Args: []string{"go.d:secretstore:vault:main", "update"}},
		"module_two",
	))
	require.False(t, controller.CompositeChildLaneConflict(
		CommandInput{Args: []string{"go.d:secretstore:vault:main", "remove"}},
		"module_one",
	))

	commit, err := index.PrepareJobChange("module_one", nil)
	require.NoError(t, err)
	commit()

	require.EqualValues(t, 0, len(index.Affected("aws-sm:prod", false)))

	refs = index.Affected("vault:main", false)
	require.False(t, len(refs) != 1 || refs[0].ID != "module_two")
}

func TestSecretDependencyIndexAcceptsMixedReferenceProviders(t *testing.T) {
	index := NewSecretDependencyIndex()
	payload, err := yaml.Marshal(map[string]any{
		"module":          "module",
		"name":            "mixed",
		"token":           "${store:vault:main:value}",
		"host":            "${HOST:-localhost}",
		"path":            "${file:/run/config}",
		"custom":          "${custom:operand}",
		"__source_type__": confgroup.TypeDyncfg,
	})
	require.NoError(t, err)

	commit, err := index.PrepareJobChange(
		"module_mixed",
		&dyncfg.GraphConfig{
			ID:      "module_mixed",
			Module:  "module",
			Name:    "mixed",
			Status:  dyncfg.StatusRunning.String(),
			Payload: payload,
		},
	)
	require.NoError(t, err)
	commit()

	refs := index.Affected("vault:main", true)
	require.Len(t, refs, 1)
	require.Equal(t, "module_mixed", refs[0].ID)
}

func TestSecretDependencyIndexSourcePolicy(t *testing.T) {
	tests := map[string]struct {
		sourceType string
		allowed    bool
	}{
		"stock":      {sourceType: confgroup.TypeStock, allowed: true},
		"user":       {sourceType: confgroup.TypeUser, allowed: true},
		"dyncfg":     {sourceType: confgroup.TypeDyncfg, allowed: true},
		"discovered": {sourceType: confgroup.TypeDiscovered},
		"empty":      {},
		"unknown":    {sourceType: "future-source"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			index := NewSecretDependencyIndex()
			config := confgroup.Config{
				"module": "module", "name": "job",
				"secret":    "${store:vault:main:value}",
				"providers": "${env:VALUE}|${file:/synthetic/value}|${cmd:/synthetic/command}",
			}
			expected := []secretstore.JobRef{{ID: config.FullName(), Display: "module:job"}}
			postimage := func() *dyncfg.GraphConfig {
				payload, err := yaml.Marshal(config)
				require.NoError(t, err)
				return &dyncfg.GraphConfig{
					ID: config.FullName(), Module: config.Module(), Name: config.Name(),
					Status: dyncfg.StatusRunning.String(), Payload: payload,
				}
			}
			// Begin with a dependency so replacement must remove any old Store edge.
			config.SetSourceType(confgroup.TypeUser)
			commit, err := index.PrepareJobChange(config.FullName(), postimage())
			require.NoError(t, err)
			commit()
			require.Equal(t, expected, index.Affected("vault:main", true))

			config.SetSourceType(test.sourceType)
			if !test.allowed {
				config["malformed"] = "${store:invalid}|${env:}"
			}
			commit, err = index.PrepareJobChange(config.FullName(), postimage())
			require.NoError(t, err)
			require.Equal(t, expected, index.Affected("vault:main", true))
			commit()
			if test.allowed {
				require.Equal(t, expected, index.Affected("vault:main", true))
			} else {
				require.Empty(t, index.Affected("vault:main", false))
				require.False(t, index.Affects("vault:main", config.FullName(), false))
			}
		})
	}
}

func BenchmarkBSecretDependencyLookup(b *testing.B) {
	index := NewSecretDependencyIndex()
	const population = 1_000
	for job := range population {
		key := "vault:other"
		if job < 3 {
			key = "vault:target"
		}
		id := fmt.Sprintf("module_%d", job)
		payload, err := yaml.Marshal(map[string]any{
			"module": "module", "name": fmt.Sprintf("%d", job),
			"secret": "${store:" + key + ":value}", "__source_type__": confgroup.TypeDyncfg,
		})
		if err != nil {
			require.FailNow(b, "benchmark failed", err)
		}
		commit, err := index.PrepareJobChange(
			id,
			&dyncfg.GraphConfig{
				ID:      id,
				Module:  "module",
				Name:    fmt.Sprintf("%d", job),
				Status:  dyncfg.StatusRunning.String(),
				Payload: payload,
			},
		)
		if err != nil {
			require.FailNow(b, "benchmark failed", err)
		}
		commit()
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if refs := index.Affected("vault:target", true); len(refs) != 3 {
			require.FailNow(b, "benchmark failed", len(refs))
		}
	}
}
