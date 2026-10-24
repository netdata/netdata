// SPDX-License-Identifier: GPL-3.0-or-later

package promvalidation

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix/selector"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/promprofiles"
)

func TestResolveProfileAutogenRuleOwnersUsesRulePolicyOrder(t *testing.T) {
	dir := t.TempDir()
	writeProfile := func(name, match, deny string) {
		t.Helper()
		content := "match: " + match + "\n" +
			"autogen:\n  selector:\n    deny: [" + deny + "]\n" +
			"template:\n  family: Test\n  context_namespace: test\n  charts: []\n"
		if err := os.WriteFile(filepath.Join(dir, name+".yaml"), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeProfile("alpha", "alpha_*", "alpha_private")
	writeProfile("zeta", "zeta_*", "zeta_private")

	catalog, err := promprofiles.LoadFromDirs([]promprofiles.DirSpec{{Path: dir, IsStock: true}})
	if err != nil {
		t.Fatal(err)
	}
	profiles := catalog.OrderedProfiles()
	spec := &charttpl.Spec{Engine: &charttpl.Engine{Autogen: &charttpl.EngineAutogen{Rules: []charttpl.EngineAutogenRule{
		{Scope: "zeta_*", Selector: selector.Expr{Deny: []string{"zeta_private"}}},
		{Scope: "alpha_*", Selector: selector.Expr{Deny: []string{"alpha_private"}}},
	}}}}

	owners, ok := resolveProfileAutogenRuleOwners(profiles, spec)
	if !ok {
		t.Fatal("resolveProfileAutogenRuleOwners() rejected the same policies in generated rule order")
	}
	if len(owners) != 2 || owners[0].profile != "zeta" || owners[1].profile != "alpha" {
		t.Fatalf("resolved owners = %#v, want zeta then alpha", owners)
	}
}
