// SPDX-License-Identifier: GPL-3.0-or-later

package promprofileproof

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validInventory = "authored_selector\temitted_metric\tsource_family\tsource_type\tsource_labels\toperator_owner\tentity_identity\tsignal_role\tobservation_population\tcross_family_relationship\tunit_algebra\tlabel_roles_and_optionality\tavailability_gate\tevidence_and_uncertainty\tdisposition\tdestination\tsource_path\n" +
	"app_value\tapp_value\tapp_value\tgauge\t\tApp\tglobal\tstate\tvalue\tindependent\tabsolute\tnone\talways\tsource-derived\tchart\tApp / Value\towner/repo @ commit; value.go:1\n" +
	"app_created\tapp_created\tapp_created\tgauge\t\t<excluded>\t<none>\tconfiguration\tcreation epoch\tcompanion\tnone\tnone\talways\tsource-derived\tjob-excluded\tlost creation time\towner/repo @ commit; value.go:2\n"

func TestLoadSourceInventory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "SOURCE-INVENTORY.tsv")
	if err := os.WriteFile(path, []byte(validInventory), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := LoadSourceInventory(path)
	if err != nil {
		t.Fatal(err)
	}
	want := SourceInventoryExpected{
		Rows: 2, SourceFamilies: 2, AuthoredSelectors: 1,
		Dispositions: InventoryDisposition{Chart: 1, JobExcluded: 1},
	}
	if err := got.VerifyExpected(want); err != nil {
		t.Fatal(err)
	}
	if _, ok := got.SourceFamilies["app_value"]; !ok {
		t.Fatalf("source-family set does not contain app_value: %v", got.SourceFamilies)
	}
	if _, ok := got.AuthoredSelectors["app_value"]; !ok {
		t.Fatalf("authored-selector set does not contain app_value: %v", got.AuthoredSelectors)
	}
}

func TestLoadSourceInventoryRejectsMalformedContracts(t *testing.T) {
	tests := map[string]struct {
		content string
		want    string
	}{
		"wrong header": {
			content: strings.Replace(validInventory, "authored_selector", "selector", 1),
			want:    "header",
		},
		"missing required value": {
			content: strings.Replace(validInventory, "\tApp\tglobal", "\t\tglobal", 1),
			want:    "operator_owner",
		},
		"unknown disposition": {
			content: strings.Replace(validInventory, "\tchart\t", "\tunknown\t", 1),
			want:    "disposition",
		},
		"duplicate row": {
			content: validInventory + strings.Split(validInventory, "\n")[1] + "\n",
			want:    "duplicates row",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "SOURCE-INVENTORY.tsv")
			if err := os.WriteFile(path, []byte(test.content), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := LoadSourceInventory(path)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("LoadSourceInventory: got %v, want error containing %q", err, test.want)
			}
		})
	}
}

func TestSourceInventoryVerifyExpectedRejectsDrift(t *testing.T) {
	path := filepath.Join(t.TempDir(), "SOURCE-INVENTORY.tsv")
	if err := os.WriteFile(path, []byte(validInventory), 0o600); err != nil {
		t.Fatal(err)
	}
	inventory, err := LoadSourceInventory(path)
	if err != nil {
		t.Fatal(err)
	}
	want := SourceInventoryExpected{
		Rows: 3, SourceFamilies: 2, AuthoredSelectors: 1,
		Dispositions: InventoryDisposition{Chart: 1, JobExcluded: 1, WriterIneligible: 1},
	}
	if err := inventory.VerifyExpected(want); err == nil || !strings.Contains(err.Error(), "rows") {
		t.Fatalf("VerifyExpected: got %v, want row-count drift", err)
	}
}
