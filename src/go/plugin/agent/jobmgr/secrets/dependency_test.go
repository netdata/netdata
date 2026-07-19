// SPDX-License-Identifier: GPL-3.0-or-later

package secrets

import (
	"fmt"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"gopkg.in/yaml.v2"
)

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
		"accepted job": {
			id: "module_two", status: dyncfg.StatusAccepted,
			references: []string{"vault:main"},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			config := map[string]any{
				"module": "module",
				"name":   test.id[len("module_"):],
			}
			for keyIndex, key := range test.references {
				config[fmt.Sprintf("secret_%d", keyIndex)] =
					"${store:" + key + ":value}"
			}
			payload, err := yaml.Marshal(config)
			if err != nil {
				t.Fatal(err)
			}
			commit, err := index.PrepareJobChange(
				test.id,
				&dyncfg.GraphConfig{
					ID: test.id, Module: "module",
					Name:   config["name"].(string),
					Status: test.status.String(), Payload: payload,
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			commit()
		})
	}

	if refs := index.Affected("vault:main", false); len(refs) != 2 {
		t.Fatalf("all affected=%+v", refs)
	}
	if refs := index.Affected("vault:main", true); len(refs) != 1 ||
		refs[0].ID != "module_one" {
		t.Fatalf("running affected=%+v", refs)
	}
	commit, err := index.PrepareJobChange("module_one", nil)
	if err != nil {
		t.Fatal(err)
	}
	commit()
	if refs := index.Affected("aws-sm:prod", false); len(refs) != 0 {
		t.Fatalf("removed job retained dependencies=%+v", refs)
	}
	if census := index.Census(); census.Jobs != 1 ||
		census.StoreKeys != 1 ||
		census.References != 1 ||
		census.Commits != 3 {
		t.Fatalf("dependency census=%+v", census)
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
			"secret": "${store:" + key + ":value}",
		})
		if err != nil {
			b.Fatal(err)
		}
		commit, err := index.PrepareJobChange(
			id,
			&dyncfg.GraphConfig{
				ID: id, Module: "module", Name: fmt.Sprintf("%d", job),
				Status: dyncfg.StatusRunning.String(), Payload: payload,
			},
		)
		if err != nil {
			b.Fatal(err)
		}
		commit()
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if refs := index.Affected("vault:target", true); len(refs) != 3 {
			b.Fatal(len(refs))
		}
	}
}
