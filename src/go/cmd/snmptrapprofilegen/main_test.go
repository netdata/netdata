// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
)

func TestParsePENsAndVendorForOID(t *testing.T) {
	pens := parsePENs([]byte("9\n  Cisco Systems, Inc.\n    Contact\n15\n  Xylogics, Inc.\n"))
	if got := pens["9"]; got != "cisco-systems-inc" {
		t.Fatalf("PEN 9 slug = %q", got)
	}
	tests := map[string]string{
		"1.3.6.1.2.1.1.5":      "standard",
		"1.3.6.1.6.3.1.1.5.1":  "standard",
		"1.0.8802.1.1.2.0.0.1": "ieee-lldp",
		"1.3.111.2.802.1":      "ieee-802",
		"1.3.6.1.4.1.9.1":      "cisco-systems-inc",
		"1.3.6.1.4.1.99999.1":  "enterprise-99999",
	}
	for oid, want := range tests {
		if got := vendorForOID(oid, pens); got != want {
			t.Fatalf("vendorForOID(%s) = %q, want %q", oid, got, want)
		}
	}
}

func TestExtractFixtureCoversSMIv1AndSMIv2Traps(t *testing.T) {
	dir := t.TempDir()
	writeTestMIB(t, dir, "TEST-SMIV1-MIB.mib", testSMIv1MIB)
	writeTestMIB(t, dir, "TEST-SMIV2-MIB.mib", testSMIv2MIB)

	records, report, sourceConflicts, err := extract(generatorOptions{
		SourceDirs: []string{dir},
		AllModules: true,
		BatchSize:  8,
	})
	if err != nil {
		t.Fatalf("extract fixture failed: %v", err)
	}
	if got := len(records); got != 2 {
		t.Fatalf("extracted records = %d, want 2: %#v", got, records)
	}
	if report.TrapsByForm["TRAP-TYPE"] != 1 || report.TrapsByForm["NOTIFICATION-TYPE"] != 1 {
		t.Fatalf("traps by form = %#v, want one SMIv1 and one SMIv2 trap", report.TrapsByForm)
	}
	if len(sourceConflicts) != 0 {
		t.Fatalf("source conflicts = %#v, want none", sourceConflicts)
	}

	v1 := trapByQualifiedName(t, records, "TEST-SMIV1-MIB::testSmiv1Trap")
	if v1.OID != "1.3.6.1.4.1.99998.0.7" || v1.Form != "TRAP-TYPE" || v1.Enterprise != "testSmiv1" || v1.TrapNumber != 7 {
		t.Fatalf("SMIv1 trap identity = oid=%q form=%q enterprise=%q trap=%d", v1.OID, v1.Form, v1.Enterprise, v1.TrapNumber)
	}
	assertVarbinds(t, v1, []string{"testSmiv1Status"}, []string{"1.3.6.1.4.1.99998.1.1"})

	v2 := trapByQualifiedName(t, records, "TEST-SMIV2-MIB::testSmiv2Notification")
	if v2.OID != "1.3.6.1.4.1.99999.0.1" || v2.Form != "NOTIFICATION-TYPE" {
		t.Fatalf("SMIv2 trap identity = oid=%q form=%q", v2.OID, v2.Form)
	}
	assertVarbinds(t, v2, []string{"testSmiv2Status", "testSmiv2Text"}, []string{"1.3.6.1.4.1.99999.1.1", "1.3.6.1.4.1.99999.1.2"})
	assertVarbindConstraints(t, v2, map[string]string{
		"testSmiv2Status": "(0..42)",
		"testSmiv2Text":   "SIZE(1..32)",
	})
}

func TestBuildSourceReportsDuplicateModules(t *testing.T) {
	first := t.TempDir()
	second := t.TempDir()
	writeTestMIB(t, first, "DUP-MIB.mib", duplicateModuleMIB("firstDescr"))
	writeTestMIB(t, second, "DUP-MIB.mib", duplicateModuleMIB("secondDescr"))

	src, stats, err := buildSource([]string{first, second})
	if err != nil {
		t.Fatalf("buildSource failed: %v", err)
	}
	if stats.Files != 2 || stats.Modules != 1 {
		t.Fatalf("source stats = files=%d modules=%d, want 2/1", stats.Files, stats.Modules)
	}
	if len(stats.Conflicts) != 1 {
		t.Fatalf("source conflicts = %#v, want one duplicate module", stats.Conflicts)
	}
	conflict := stats.Conflicts[0]
	wantChosen := filepath.Join(first, "DUP-MIB.mib")
	wantRejected := filepath.Join(second, "DUP-MIB.mib")
	if conflict.Module != "DUP-MIB" || conflict.Chosen != wantChosen || len(conflict.Rejected) != 1 || conflict.Rejected[0] != wantRejected {
		t.Fatalf("source conflict = %#v, want chosen %s rejected %s", conflict, wantChosen, wantRejected)
	}
	found, err := src.Find("DUP-MIB")
	if err != nil {
		t.Fatalf("Find(DUP-MIB) failed: %v", err)
	}
	if found.Path != wantChosen {
		t.Fatalf("Find(DUP-MIB) path = %q, want first source %q", found.Path, wantChosen)
	}
}

func TestSourceRankUsesPathBoundary(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "mibs-a")
	second := filepath.Join(root, "mibs-a2")
	if err := os.MkdirAll(second, 0o755); err != nil {
		t.Fatalf("mkdir second source: %v", err)
	}
	path := filepath.Join(second, "TEST-MIB.mib")
	if got := sourceRank(path, []string{first, second}); got != 1 {
		t.Fatalf("sourceRank(%q) = %d, want second source", path, got)
	}
}

func TestNormalizeTrapRecordsCollapsesDotZeroAliases(t *testing.T) {
	records := []TrapRecord{
		{
			OID:           "1.3.6.1.4.1.818.1.1.18.11.0.40",
			MIB:           "GE-PARALLELUPS",
			Name:          "upsAlarmOutputOverloadRestored-eighth",
			QualifiedName: "GE-PARALLELUPS::upsAlarmOutputOverloadRestored-eighth",
			Form:          "TRAP-TYPE",
		},
		{
			OID:           "1.3.6.1.4.1.818.1.1.18.11.40",
			MIB:           "GEPARALLELUPS-MIB",
			Name:          "upsTrapAlarmOutputOverloadRestoredeighth",
			QualifiedName: "GEPARALLELUPS-MIB::upsTrapAlarmOutputOverloadRestoredeighth",
			Form:          "NOTIFICATION-TYPE",
		},
	}

	winners, exactConflicts, dotZeroConflicts := normalizeTrapRecords(records, nil)
	if len(exactConflicts) != 0 {
		t.Fatalf("exact conflicts = %#v, want none", exactConflicts)
	}
	if len(dotZeroConflicts) != 1 {
		t.Fatalf("dot-zero conflicts = %#v, want one conflict", dotZeroConflicts)
	}
	if dotZeroConflicts[0].OID != "1.3.6.1.4.1.818.1.1.18.11.40" {
		t.Fatalf("logical conflict OID = %q", dotZeroConflicts[0].OID)
	}
	if len(winners) != 1 {
		t.Fatalf("winners = %#v, want one", winners)
	}
	if winners[0].Form != "NOTIFICATION-TYPE" {
		t.Fatalf("winner form = %q, want NOTIFICATION-TYPE", winners[0].Form)
	}
	if len(dotZeroConflicts[0].Rejected) != 1 || dotZeroConflicts[0].Rejected[0].OID != records[0].OID {
		t.Fatalf("rejected records = %#v, want SMIv1 trap", dotZeroConflicts[0].Rejected)
	}
}

func TestConflictWinnersUsesRejectedRecordOID(t *testing.T) {
	records := []TrapRecord{
		{OID: "1.3.6.1.4.1.1.0.1", QualifiedName: "V1::trap", Form: "TRAP-TYPE"},
		{OID: "1.3.6.1.4.1.1.1", QualifiedName: "V2::trap", Form: "NOTIFICATION-TYPE"},
	}
	conflicts := []Conflict{{
		OID:      "1.3.6.1.4.1.1.1",
		Chosen:   conflictRec(records[1]),
		Rejected: []ConflictRec{conflictRec(records[0])},
		Rule:     "trap-oid-dot0-equivalence",
	}}
	winners := conflictWinners(records, conflicts)
	if len(winners) != 1 || winners[0].QualifiedName != "V2::trap" {
		t.Fatalf("winners = %#v, want only V2::trap", winners)
	}
}

func TestCompareBaselineProfilesReportsExactRuntimeAndLogicalOverlap(t *testing.T) {
	dir := t.TempDir()
	writeTestProfile(t, dir, "baseline.yaml", []string{
		"1.2.3.0.1",
		"1.2.3.0.2",
		"1.2.3.0.3",
	})
	records := []TrapRecord{
		{OID: "1.2.3.0.1", QualifiedName: "TEST::exact"},
		{OID: "1.2.3.2", QualifiedName: "TEST::dotZeroEquivalent"},
		{OID: "1.2.3.0.4", QualifiedName: "TEST::new"},
	}

	report, err := compareBaselineProfiles(dir, records)
	if err != nil {
		t.Fatalf("compareBaselineProfiles failed: %v", err)
	}
	summary := report.Summary
	if summary.ExactOverlap != 1 || summary.ExactCandidateNew != 2 || summary.ExactBaselineMissing != 2 {
		t.Fatalf("exact summary = %#v", summary)
	}
	if summary.BaselineRuntimeMatched != 2 || summary.BaselineRuntimeMissing != 1 {
		t.Fatalf("baseline runtime summary = %#v", summary)
	}
	if summary.CandidateRuntimeMatched != 2 || summary.CandidateRuntimeNew != 1 {
		t.Fatalf("candidate runtime summary = %#v", summary)
	}
	if summary.LogicalOverlap != 2 || summary.LogicalCandidateNew != 1 || summary.LogicalBaselineMissing != 1 {
		t.Fatalf("logical summary = %#v", summary)
	}
}

func TestClassifyCommandNormalizesBeforeCallingLLM(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]string{
					"content": `{"category":"state_change","severity":"notice","description":"Logical trap changed on {{hostname}}."}`,
				},
			}},
		})
	}))
	defer server.Close()

	dir := t.TempDir()
	inPath := filepath.Join(dir, "traps.jsonl")
	outPath := filepath.Join(dir, "enriched.jsonl")
	cachePath := filepath.Join(dir, "classification-cache.jsonl")
	records := []TrapRecord{
		{OID: "1.3.6.1.4.1.1.0.1", QualifiedName: "V1::trap", Name: "trap", MIB: "V1", Form: "TRAP-TYPE"},
		{OID: "1.3.6.1.4.1.1.1", QualifiedName: "V2::trap", Name: "trap", MIB: "V2", Form: "NOTIFICATION-TYPE"},
	}
	if err := writeJSONL(inPath, records); err != nil {
		t.Fatalf("write input JSONL: %v", err)
	}

	err := classifyCommand([]string{
		"--in", inPath,
		"--out", outPath,
		"--cache", cachePath,
		"--base-url", server.URL,
		"--model", "test-model",
		"--concurrency", "1",
		"--require-llm",
	})
	if err != nil {
		t.Fatalf("classifyCommand failed: %v", err)
	}
	if calls != 1 {
		t.Fatalf("LLM calls = %d, want 1 normalized winner", calls)
	}
	got, err := readTrapJSONL(outPath)
	if err != nil {
		t.Fatalf("read output JSONL: %v", err)
	}
	if len(got) != 1 || got[0].Form != "NOTIFICATION-TYPE" {
		t.Fatalf("classified output = %#v, want one NOTIFICATION-TYPE winner", got)
	}
}

func TestParseOptionsRequiresSourceDir(t *testing.T) {
	if _, err := parseOptions([]string{"--all"}, true); err == nil || !strings.Contains(err.Error(), "provide at least one --source-dir") {
		t.Fatalf("parseOptions without source dir error = %v, want --source-dir requirement", err)
	}
}

func writeTestMIB(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func writeTestProfile(t *testing.T, dir, name string, oids []string) {
	t.Helper()
	var b strings.Builder
	b.WriteString("traps:\n")
	for i, oid := range oids {
		fmt.Fprintf(&b, "  - oid: %s\n    name: TEST-MIB::trap%d\n    category: unknown\n    severity: notice\n", oid, i)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(b.String()), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func trapByQualifiedName(t *testing.T, records []TrapRecord, name string) TrapRecord {
	t.Helper()
	for _, rec := range records {
		if rec.QualifiedName == name {
			return rec
		}
	}
	t.Fatalf("trap %s not found in %#v", name, records)
	return TrapRecord{}
}

func assertVarbinds(t *testing.T, rec TrapRecord, names, oids []string) {
	t.Helper()
	if len(rec.Varbinds) != len(names) {
		t.Fatalf("%s varbind count = %d, want %d", rec.QualifiedName, len(rec.Varbinds), len(names))
	}
	for i := range names {
		if rec.Varbinds[i].Name != names[i] || rec.Varbinds[i].OID != oids[i] {
			t.Fatalf("%s varbind[%d] = %s/%s, want %s/%s", rec.QualifiedName, i, rec.Varbinds[i].Name, rec.Varbinds[i].OID, names[i], oids[i])
		}
	}
}

func assertVarbindConstraints(t *testing.T, rec TrapRecord, constraints map[string]string) {
	t.Helper()
	for _, vb := range rec.Varbinds {
		if want, ok := constraints[vb.Name]; ok && vb.Constraints != want {
			t.Fatalf("%s varbind %s constraints = %q, want %q", rec.QualifiedName, vb.Name, vb.Constraints, want)
		}
	}
	pf := buildProfile("test", []TrapRecord{rec})
	for name, want := range constraints {
		vb, ok := pf.Varbinds[name]
		if !ok {
			t.Fatalf("profile varbind %s missing", name)
		}
		if vb.Constraints != want {
			t.Fatalf("profile varbind %s constraints = %q, want %q", name, vb.Constraints, want)
		}
	}
}

func TestDefaultPENFilePathUsesStockConfigDir(t *testing.T) {
	oldStockConfigDir := buildinfo.StockConfigDir
	t.Cleanup(func() {
		buildinfo.StockConfigDir = oldStockConfigDir
	})
	buildinfo.StockConfigDir = "/usr/lib/netdata/conf.d"

	want := filepath.Join("/usr/lib/netdata/conf.d", "go.d", "snmp.trap-profiles", "iana-enterprise-numbers.txt")
	if got := defaultPENFilePath(); got != want {
		t.Fatalf("defaultPENFilePath() = %q, want %q", got, want)
	}
}

func TestHashTrapStableAndOrderSensitive(t *testing.T) {
	rec := TrapRecord{
		OID:             "1.3.6.1.2.1.1.0.1",
		Name:            "testTrap",
		MIB:             "TEST-MIB",
		Form:            "NOTIFICATION-TYPE",
		TrapDescription: "A test trap.",
		Varbinds: []VarbindRecord{
			{Name: "first", OID: "1.3.6.1.2.1.1.1", Type: "Integer32"},
			{Name: "second", OID: "1.3.6.1.2.1.1.2", Type: "OctetString"},
		},
	}
	h1 := hashTrap(rec)
	h2 := hashTrap(rec)
	if h1 != h2 {
		t.Fatalf("hash changed between identical records")
	}
	rec.Varbinds[0], rec.Varbinds[1] = rec.Varbinds[1], rec.Varbinds[0]
	if hashTrap(rec) == h1 {
		t.Fatalf("hash did not change when varbind order changed")
	}
}

func TestBuildProfileDropsUnresolvedVarbinds(t *testing.T) {
	pf := buildProfile("test", []TrapRecord{{
		OID:           "1.3.6.1.4.1.9.0.1",
		QualifiedName: "TEST-MIB::testTrap",
		Category:      "unknown",
		Severity:      "notice",
		Description:   "TEST-MIB::testTrap on {{hostname}}.",
		TrapStatus:    "current",
		MIB:           "TEST-MIB",
		Varbinds: []VarbindRecord{
			{Name: "ok", OID: "1.3.6.1.4.1.9.1", Type: "Integer32"},
			{Name: "missingType", OID: "1.3.6.1.4.1.9.2"},
			{Name: "missingOID", Type: "Integer32"},
		},
	}})
	if len(pf.Varbinds) != 1 {
		t.Fatalf("varbind table length = %d, want 1", len(pf.Varbinds))
	}
	if len(pf.Traps) != 1 || len(pf.Traps[0].Varbinds) != 1 {
		t.Fatalf("trap varbind refs = %#v, want one resolved ref", pf.Traps)
	}
	if got := pf.Traps[0].Varbinds[0]; got != "ok" {
		t.Fatalf("trap varbind ref = %#v", got)
	}
}

func TestLLMResponseValidation(t *testing.T) {
	rec := TrapRecord{
		QualifiedName: "TEST-MIB::testTrap",
		Varbinds:      []VarbindRecord{{Name: "ifIndex", OID: "1.2.3", Type: "Integer32"}},
	}
	cat, sev, desc, err := parseLLMResponse(`{"category":"state_change","severity":"warning","description":"Trap for interface {{value \"ifIndex\"}} on {{hostname}}."}`, rec)
	if err != nil {
		t.Fatalf("parseLLMResponse failed: %v", err)
	}
	if cat != "state_change" || sev != "warning" {
		t.Fatalf("classification = %s/%s", cat, sev)
	}
	if !strings.Contains(desc, "{{hostname}}") {
		t.Fatalf("description did not normalize hostname placeholder: %q", desc)
	}
	_, _, desc, err = parseLLMResponse(`{"category":"state_change","severity":"notice","description":"Trap{{with value \"ifIndex\"}} for interface {{.}}{{end}} on {{hostname}}."}`, rec)
	if err != nil {
		t.Fatalf("parseLLMResponse failed for with block without else: %v", err)
	}
	if !strings.Contains(desc, `{{with value "ifIndex"}}`) {
		t.Fatalf("description did not preserve with block: %q", desc)
	}
	_, _, desc, err = parseLLMResponse(`{"category":"state_change","severity":"warning","description":"Trap from TRAP_SOURCE_IP through TRAP_DEVICE_VENDOR on SNMP_DEVICE_HOSTNAME."}`, rec)
	if err != nil {
		t.Fatalf("parseLLMResponse failed for bare built-in placeholders: %v", err)
	}
	if !strings.Contains(desc, "{{source_ip}}") || !strings.Contains(desc, "{{vendor}}") {
		t.Fatalf("description did not normalize bare built-in placeholders: %q", desc)
	}
	if _, _, _, err := parseLLMResponse(`{"category":"bad","severity":"warning","description":"x"}`, rec); err == nil {
		t.Fatalf("invalid category accepted")
	}
	if _, _, _, err := parseLLMResponse(`{"category":"state_change","severity":"warning","description":"Bad {{value \"invented\"}} on {{hostname}}."}`, rec); err == nil {
		t.Fatalf("invented placeholder accepted")
	}
	if _, _, _, err := parseLLMResponse(`{"category":"state_change","severity":"warning","description":"Bad legacy {ifIndex} on {{hostname}}."}`, rec); err == nil {
		t.Fatalf("legacy placeholder accepted")
	}
	if _, _, _, err := parseLLMResponse(`{"category":"state_change","severity":"warning","description":"x","extra":"bad"}`, rec); err == nil {
		t.Fatalf("schema-invalid response with extra property accepted")
	}
}

func TestValidateClassificationForRecordRejectsMismatchedPlaceholders(t *testing.T) {
	rec := TrapRecord{
		QualifiedName: "TEST-MIB::testTrap",
		Varbinds:      []VarbindRecord{{Name: "ifIndex", OID: "1.2.3", Type: "Integer32"}},
	}
	valid := Classification{
		Hash:        "hash",
		Schema:      defaultSchemaVer,
		Prompt:      defaultPromptVer,
		Category:    "state_change",
		Severity:    "warning",
		Description: "Trap for interface {{value \"ifIndex\"}} on {{hostname}}.",
	}
	c, err := validateClassificationForRecord(valid, rec)
	if err != nil {
		t.Fatalf("valid classification rejected: %v", err)
	}
	if !strings.Contains(c.Description, "{{hostname}}") {
		t.Fatalf("description did not normalize hostname placeholder: %q", c.Description)
	}

	invalidPlaceholder := valid
	invalidPlaceholder.Description = "Trap for interface {{value \"ifName\"}} on {{hostname}}."
	if _, err := validateClassificationForRecord(invalidPlaceholder, rec); err == nil {
		t.Fatalf("classification with unavailable placeholder accepted")
	}

	stalePrompt := valid
	stalePrompt.Prompt = "old-prompt"
	if _, err := validateClassificationForRecord(stalePrompt, rec); err == nil {
		t.Fatalf("classification with stale prompt accepted")
	}
}

func TestParseLLMResponseRepairsUniquePlaceholderSuffix(t *testing.T) {
	rec := TrapRecord{
		QualifiedName: "TEST-MIB::testTrap",
		Varbinds: []VarbindRecord{{
			Name: "iBMPSGNetworkAdapterOnlineEventTargetObjectPath",
			OID:  "1.2.3",
			Type: "DisplayString",
		}},
	}
	_, _, desc, err := parseLLMResponse(`{"category":"state_change","severity":"notice","description":"Network adapter target {{value \"iBMPSGNetworkAdapterTargetObjectPath\"}} came online on {{hostname}}."}`, rec)
	if err != nil {
		t.Fatalf("parseLLMResponse failed: %v", err)
	}
	if !strings.Contains(desc, `{{value "iBMPSGNetworkAdapterOnlineEventTargetObjectPath"}}`) {
		t.Fatalf("description was not repaired to exact placeholder: %q", desc)
	}
}

func TestParseLLMResponseRejectsAmbiguousPlaceholderSuffix(t *testing.T) {
	rec := TrapRecord{
		QualifiedName: "TEST-MIB::testTrap",
		Varbinds: []VarbindRecord{
			{Name: "prefixOneTargetObjectPath", OID: "1.2.3", Type: "DisplayString"},
			{Name: "prefixTwoTargetObjectPath", OID: "1.2.4", Type: "DisplayString"},
		},
	}
	if _, _, _, err := parseLLMResponse(`{"category":"state_change","severity":"notice","description":"Target {{value \"TargetObjectPath\"}} changed on {{hostname}}."}`, rec); err == nil {
		t.Fatalf("ambiguous placeholder suffix accepted")
	}
}

func TestParseLLMResponseRepairsDuplicatedFinalCamelWord(t *testing.T) {
	rec := TrapRecord{
		QualifiedName: "TEST-MIB::testTrap",
		Varbinds: []VarbindRecord{{
			Name: "hwPhysicalPortCrcPerCurrentValueString",
			OID:  "1.2.3",
			Type: "DisplayString",
		}},
	}
	_, _, desc, err := parseLLMResponse(`{"category":"diagnostic","severity":"warning","description":"CRC value {{value \"hwPhysicalPortCrcPerCurrentValueStringString\"}} exceeded threshold on {{hostname}}."}`, rec)
	if err != nil {
		t.Fatalf("parseLLMResponse failed: %v", err)
	}
	if !strings.Contains(desc, `{{value "hwPhysicalPortCrcPerCurrentValueString"}}`) {
		t.Fatalf("description was not repaired to exact placeholder: %q", desc)
	}
}

func TestClassifierUserPromptSeparatesUnavailableVarbinds(t *testing.T) {
	rec := TrapRecord{
		Name:          "testTrap",
		MIB:           "TEST-MIB",
		QualifiedName: "TEST-MIB::testTrap",
		OID:           "1.2.3",
		Varbinds: []VarbindRecord{
			{Name: "usable", OID: "1.2.3.1", Type: "Integer32", Description: "usable value"},
			{Name: "missingType", OID: "1.2.3.2", Description: "unavailable value"},
		},
	}
	prompt := classifierUserPrompt(rec, "")
	if !strings.Contains(prompt, `{{value "usable"}}`) || !strings.Contains(prompt, `{{raw "usable"}}`) {
		t.Fatalf("prompt missing usable placeholders:\n%s", prompt)
	}
	if strings.Contains(prompt, `{{value "missingType"}}`) || strings.Contains(prompt, `{{raw "missingType"}}`) {
		t.Fatalf("prompt allowed unavailable placeholder:\n%s", prompt)
	}
	if !strings.Contains(prompt, "unavailable_varbind_names_do_not_use_as_placeholders: missingType") {
		t.Fatalf("prompt missing unavailable varbind warning:\n%s", prompt)
	}
}

func TestSanitizePromptTextEscapesAndRespectsByteLimit(t *testing.T) {
	if got := sanitizePromptText("x <tag> {name}\n", 100); got != "x &lt;tag&gt; &#123;name&#125;" {
		t.Fatalf("sanitizePromptText escaped output = %q", got)
	}
	if got := sanitizePromptText("aa€", 3); got != "aa" {
		t.Fatalf("sanitizePromptText byte-limited output = %q, want %q", got, "aa")
	}
}

func TestClassifyOneFeedsValidationErrorBackToLLM(t *testing.T) {
	var prompts []string
	var sawThinkingDisabledDirect bool
	var sawThinkingDisabledExtraBody bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		var req struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
			ChatTemplateKwargs map[string]bool `json:"chat_template_kwargs"`
			ExtraBody          struct {
				ChatTemplateKwargs map[string]bool `json:"chat_template_kwargs"`
			} `json:"extra_body"`
		}
		if err := json.Unmarshal(data, &req); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if enabled, ok := req.ChatTemplateKwargs["enable_thinking"]; ok && !enabled {
			sawThinkingDisabledDirect = true
		}
		if enabled, ok := req.ExtraBody.ChatTemplateKwargs["enable_thinking"]; ok && !enabled {
			sawThinkingDisabledExtraBody = true
		}
		for _, msg := range req.Messages {
			if msg.Role == "user" {
				prompts = append(prompts, msg.Content)
			}
		}

		content := `{"category":"state_change","severity":"warning","description":"state changed to {{value \"notAvailableValue\"}} on {{hostname}}."}`
		if len(prompts) == 2 {
			content = `{"category":"state_change","severity":"warning","description":"state changed to {{value \"rtmOperStatus\"}} on {{hostname}}."}`
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"content": content}}},
		})
	}))
	defer server.Close()

	rec := TrapRecord{
		Hash:          "hash",
		QualifiedName: "DC-RTM-MIB::rtmOperStateChange",
		Name:          "rtmOperStateChange",
		MIB:           "DC-RTM-MIB",
		OID:           "1.3.6.1.4.1.1.1",
		Form:          "NOTIFICATION-TYPE",
		Varbinds:      []VarbindRecord{{Name: "rtmOperStatus", OID: "1.3.6.1.4.1.1.2", Type: "Integer32"}},
	}
	c, err := classifyOne(server.Client(), generatorOptions{
		BaseURL:     server.URL,
		Model:       "test-model",
		MaxTokens:   100,
		HTTPTimeout: time.Second,
		RequireLLM:  true,
	}, rec)
	if err != nil {
		t.Fatalf("classifyOne failed: %v", err)
	}
	if c.Source != "llm:test-model:attempt-2" {
		t.Fatalf("classification source = %q, want second LLM attempt", c.Source)
	}
	if !sawThinkingDisabledDirect {
		t.Fatalf("LLM request did not disable thinking through top-level chat_template_kwargs")
	}
	if !sawThinkingDisabledExtraBody {
		t.Fatalf("LLM request did not disable thinking through extra_body.chat_template_kwargs")
	}
	if len(prompts) != 2 {
		t.Fatalf("LLM prompts = %d, want 2", len(prompts))
	}
	if !strings.Contains(prompts[1], `previous_validation_error: function "value" references unknown varbind "notAvailableValue"`) {
		t.Fatalf("second prompt did not include validation feedback:\n%s", prompts[1])
	}
	if !strings.Contains(prompts[0], `{{value "rtmOperStatus"}}`) {
		t.Fatalf("prompt did not list exact allowed varbind placeholder:\n%s", prompts[0])
	}
}

func TestClassifyOneRetriesSchemaFailuresFiveTimes(t *testing.T) {
	var attempts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		content := `{"category":"state_change","severity":"warning","description":"state changed","extra":"not allowed"}`
		if attempts == maxLLMAttempts {
			content = `{"category":"state_change","severity":"warning","description":"state changed on {{hostname}}."}`
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"content": content}}},
		})
	}))
	defer server.Close()

	rec := TrapRecord{
		Hash:          "hash",
		QualifiedName: "TEST-MIB::stateChanged",
		Name:          "stateChanged",
		MIB:           "TEST-MIB",
		OID:           "1.3.6.1.4.1.1.1",
		Form:          "NOTIFICATION-TYPE",
	}
	c, err := classifyOne(server.Client(), generatorOptions{
		BaseURL:     server.URL,
		Model:       "test-model",
		MaxTokens:   100,
		HTTPTimeout: time.Second,
		RequireLLM:  true,
	}, rec)
	if err != nil {
		t.Fatalf("classifyOne failed: %v", err)
	}
	if attempts != maxLLMAttempts {
		t.Fatalf("attempts = %d, want %d", attempts, maxLLMAttempts)
	}
	if c.Source != "llm:test-model:attempt-5" {
		t.Fatalf("classification source = %q, want fifth LLM attempt", c.Source)
	}
}

func duplicateModuleMIB(description string) string {
	return `DUP-MIB DEFINITIONS ::= BEGIN

IMPORTS
    MODULE-IDENTITY, enterprises
        FROM SNMPv2-SMI;

dup MODULE-IDENTITY
    LAST-UPDATED "202601010000Z"
    ORGANIZATION "Netdata"
    CONTACT-INFO ""
    DESCRIPTION "` + description + `"
    ::= { enterprises 99997 }

END
`
}

const testSMIv1MIB = `TEST-SMIV1-MIB DEFINITIONS ::= BEGIN

IMPORTS
    enterprises FROM RFC1155-SMI
    OBJECT-TYPE FROM RFC-1212
    TRAP-TYPE FROM RFC-1215;

testSmiv1 OBJECT IDENTIFIER ::= { enterprises 99998 }
testSmiv1Objects OBJECT IDENTIFIER ::= { testSmiv1 1 }

testSmiv1Status OBJECT-TYPE
    SYNTAX INTEGER
    ACCESS read-only
    STATUS mandatory
    DESCRIPTION "Status."
    ::= { testSmiv1Objects 1 }

testSmiv1Trap TRAP-TYPE
    ENTERPRISE testSmiv1
    VARIABLES { testSmiv1Status }
    DESCRIPTION "V1 trap."
    ::= 7

END
`

const testSMIv2MIB = `TEST-SMIV2-MIB DEFINITIONS ::= BEGIN

IMPORTS
    MODULE-IDENTITY, OBJECT-TYPE, NOTIFICATION-TYPE, Integer32, enterprises
        FROM SNMPv2-SMI;

testSmiv2 MODULE-IDENTITY
    LAST-UPDATED "202601010000Z"
    ORGANIZATION "Netdata"
    CONTACT-INFO ""
    DESCRIPTION "Test SMIv2 module."
    ::= { enterprises 99999 }

testSmiv2Objects OBJECT IDENTIFIER ::= { testSmiv2 1 }
testSmiv2Notifications OBJECT IDENTIFIER ::= { testSmiv2 0 }

testSmiv2Status OBJECT-TYPE
    SYNTAX Integer32 (0..42)
    MAX-ACCESS read-only
    STATUS current
    DESCRIPTION "Status."
    ::= { testSmiv2Objects 1 }

testSmiv2Text OBJECT-TYPE
    SYNTAX OCTET STRING (SIZE(1..32))
    MAX-ACCESS read-only
    STATUS current
    DESCRIPTION "Text."
    ::= { testSmiv2Objects 2 }

testSmiv2Notification NOTIFICATION-TYPE
    OBJECTS { testSmiv2Status, testSmiv2Text }
    STATUS current
    DESCRIPTION "Notification."
    ::= { testSmiv2Notifications 1 }

END
`
