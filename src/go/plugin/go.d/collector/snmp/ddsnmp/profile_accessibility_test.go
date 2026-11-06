// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/golangsnmp/gomib/mib"

	"github.com/netdata/netdata/go/plugins/pkg/multipath"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

const accessManifestPath = "testdata/profile-mib-access.json"
const accessSourceRevision = "b787b2e9d38a87f39d95a6a077c9382261262336"

// Facts describe exact objects, never nearest ancestors. ObjectOID differs from OID only for an explicit instance.
// Row is resolved in Module, rather than through the global OID tree (FREENAS/TRUENAS share numeric OIDs).
type profileAccessFact struct {
	Declared  string `json:"declared"`
	OID       string `json:"oid"`
	Module    string `json:"module"`
	ObjectOID string `json:"object_oid"`
	Object    string `json:"object"`
	Kind      string `json:"kind"`
	Access    string `json:"access"`
	Source    string `json:"source"`
	Start     int    `json:"start"`
	End       int    `json:"end"`
	Row       string `json:"row,omitempty"`
}

type profileAccessManifest struct {
	Revision string              `json:"revision"`
	Sources  map[string]string   `json:"sources"`
	Facts    []profileAccessFact `json:"facts"`
	// Exact defining profile, source role, declared module and OID: this baseline cannot authorize a new reference.
	Unresolved        []string          `json:"unresolved"`
	ExceptionSelector any               `json:"exception_selector"`
	ExceptionCapture  map[string]string `json:"exception_capture"`
}

func accessFactKey(module, oid string) string {
	return strings.ToLower(module) + " " + strings.TrimPrefix(oid, ".")
}
func accessRefKey(file string, ref profileOIDRef) string {
	return file + " " + ref.path + " " + accessFactKey(ref.module, ref.oid)
}

func readAccessManifest(t *testing.T) profileAccessManifest {
	t.Helper()
	data, err := os.ReadFile(accessManifestPath)
	require.NoError(t, err)
	var manifest profileAccessManifest
	require.NoError(t, json.Unmarshal(data, &manifest))
	require.Equal(t, accessSourceRevision, manifest.Revision)
	require.NotEmpty(t, manifest.Sources)
	return manifest
}

func stockAccessRefs(t *testing.T) map[string][]profileOIDRef {
	t.Helper()
	files, err := filepath.Glob("../../../config/go.d/snmp.profiles/default/*.yaml")
	require.NoError(t, err)
	require.NotEmpty(t, files)
	result := map[string][]profileOIDRef{}
	for _, file := range files {
		content, err := os.ReadFile(file)
		require.NoError(t, err)
		var doc any
		require.NoError(t, yaml.Unmarshal(content, &doc), file)
		var refs []profileOIDRef
		collectProfileOIDRefs(doc, "", "", false, &refs)
		result[filepath.Base(file)] = refs
	}
	return result
}

// Ordinary CI needs no installed MIBs. Unknown polling sources must acquire source evidence; an existing unresolved
// source remains explicitly unverified at its exact defining role. YAML aliases are expanded by yaml.Unmarshal, so
// descriptors, typed fields, anchors and legacy tag/from forms receive the same check. Abstract table roots do not.
func TestStockProfiles_PollingAccessibility(t *testing.T) {
	manifest := readAccessManifest(t)
	facts := map[string]profileAccessFact{}
	for _, fact := range manifest.Facts {
		key := accessFactKey(fact.Declared, fact.OID)
		_, duplicate := facts[key]
		require.False(t, duplicate, "duplicate fact %s", key)
		require.NotEmpty(t, manifest.Sources[fact.Source], "missing provenance: %s", key)
		require.NotEmpty(t, fact.Module)
		require.NotEmpty(t, fact.Object)
		require.Greater(t, fact.End, fact.Start)
		require.Contains(t, []string{"column", "scalar"}, fact.Kind)
		require.Contains(t, []string{"read-only", "read-write", "read-create", "write-only", "not-accessible", "accessible-for-notify"}, fact.Access)
		facts[key] = fact
	}
	unresolved := map[string]bool{}
	for _, key := range manifest.Unresolved {
		require.False(t, unresolved[key], "duplicate unresolved source %s", key)
		unresolved[key] = true
	}
	exceptionCount := 0
	var verified, unknown int
	for file, refs := range stockAccessRefs(t) {
		for _, ref := range refs {
			problem, known := checkPollingAccess(file, ref, facts, unresolved)
			if problem != "" {
				t.Errorf("%s %s: %s", file, ref.path, problem)
				continue
			}
			if !known {
				unknown++
				continue
			}
			if accessException(file, ref) {
				exceptionCount++
			}
			verified++
		}
	}
	// Raw definitions pin the source roles; effective consumers own their selectors after inheritance.
	files, err := filepath.Glob("../../../config/go.d/snmp.profiles/default/*.yaml")
	require.NoError(t, err)
	paths := multipath.New("../../../config/go.d/snmp.profiles/default")
	for _, file := range files {
		if strings.HasPrefix(filepath.Base(file), "_") {
			continue // Abstract profiles cannot be selected; their concrete consumers are checked below.
		}
		profile, err := loadProfile(file, paths)
		require.NoError(t, err, file)
		require.NoError(t, prepareLoadedProfile(profile), file)
		problems, err := effectivePollingScopeProblems(profile, facts, manifest.ExceptionSelector)
		require.NoError(t, err, file)
		for _, problem := range problems {
			t.Errorf("%s: %s", filepath.Base(file), problem)
		}
	}
	require.Equal(t, 3, exceptionCount, "only the three capture-proven RB750Gr3 source roles are permitted")
	content, err := os.ReadFile("../../../config/go.d/snmp.profiles/default/topology-role-mikrotik-rb750gr3.yaml")
	require.NoError(t, err)
	var exceptionDoc map[string]any
	require.NoError(t, yaml.Unmarshal(content, &exceptionDoc))
	actualSelector, err := json.Marshal(accessJSONValue(exceptionDoc["selector"]))
	require.NoError(t, err)
	expectedSelector, err := json.Marshal(manifest.ExceptionSelector)
	require.NoError(t, err)
	require.JSONEq(t, string(expectedSelector), string(actualSelector), "exception selector changed: re-check selector and capture coverage")
	for _, reason := range []string{"RouterOS RB750Gr3", "INDEX objects (.1/.2) are readable", "index-only decoding"} {
		require.Contains(t, string(content), reason, "retain the evidence-backed deviation explanation")
	}
	rows := exceptionDoc["topology"].([]any)
	require.Len(t, rows, 1)
	row := rows[0].(map[any]any)
	require.Equal(t, "lldp_rem_man_addr", row["kind"])
	require.Equal(t, map[any]any{"OID": "1.0.8802.1.1.2.1.4.2", "name": "lldpRemManAddrTable"}, row["table"])
	t.Logf("%d exact polling sources checked; %d explicitly unresolved sources (not certified readable)", verified, unknown)
}

// Inheritance preserves the child's selector rather than intersecting it with its parents. Check the
// prepared acquisition definition so direct and transitive consumers cannot broaden a source exception.
func effectivePollingScopeProblems(
	profile *Profile,
	facts map[string]profileAccessFact,
	exceptionSelector any,
) ([]string, error) {
	data, err := yaml.Marshal(profile.Definition)
	if err != nil {
		return nil, err
	}
	var doc map[any]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	actual, err := json.Marshal(accessJSONValue(doc["selector"]))
	if err != nil {
		return nil, err
	}
	expected, err := json.Marshal(exceptionSelector)
	if err != nil {
		return nil, err
	}
	var refs []profileOIDRef
	collectProfileOIDRefs(doc, "", "", false, &refs)
	var problems []string
	for _, ref := range refs {
		fact, ok := facts[accessFactKey(ref.module, ref.oid)]
		if !ok ||
			fact.Access != "not-accessible" && fact.Access != "accessible-for-notify" && fact.Access != "write-only" {
			continue
		}
		if string(actual) != string(expected) {
			problems = append(
				problems,
				fmt.Sprintf(
					"%s polls %s::%s (%s) outside the capture-proven exception selector",
					ref.path,
					fact.Module,
					fact.Object,
					ref.oid,
				),
			)
		}
	}
	return problems, nil
}

func checkPollingAccess(file string, ref profileOIDRef, facts map[string]profileAccessFact, unresolved map[string]bool) (problem string, known bool) {
	fact, ok := facts[accessFactKey(ref.module, ref.oid)]
	if !ok {
		if unresolved[accessRefKey(file, ref)] {
			return "", false
		}
		return "new polling source needs exact MIB access evidence; refresh TestStockProfiles_AccessManifestAgainstMIBs with the pinned corpus", false
	}
	if ref.column && fact.ObjectOID != strings.TrimPrefix(ref.oid, ".") {
		return "table column must name an exact object, not an instance", true
	}
	if fact.Access == "not-accessible" || fact.Access == "accessible-for-notify" || fact.Access == "write-only" {
		if !accessException(file, ref) {
			return fmt.Sprintf("polls %s::%s (%s, %s)", fact.Module, fact.Object, ref.oid, fact.Access), true
		}
	}
	return "", true
}

func accessException(file string, ref profileOIDRef) bool {
	if file != "topology-role-mikrotik-rb750gr3.yaml" || ref.module != "LLDP-MIB" {
		return false
	}
	switch ref.path {
	case ".topology[0].symbols[0]":
		return ref.oid == "1.0.8802.1.1.2.1.4.2.1.1" && ref.name == "lldp_rem_man_addr"
	case ".topology[0].metric_tags[2].symbol":
		return ref.oid == "1.0.8802.1.1.2.1.4.2.1.1" && ref.name == "lldpRemManAddrSubtype"
	case ".topology[0].metric_tags[4].symbol":
		return ref.oid == "1.0.8802.1.1.2.1.4.2.1.2" && ref.name == "lldpRemManAddr"
	}
	return false
}

// Set NETDATA_SNMP_MIB_DIRS to the pinned LibreNMS mibs directory for verification. Add
// NETDATA_SNMP_MIB_REFRESH=1 to refresh exact facts and source hashes. Refresh never grows the unresolved baseline;
// missing objects must be investigated or explicitly reviewed as a baseline change, not silently grandfathered.
func TestStockProfiles_AccessManifestAgainstMIBs(t *testing.T) {
	dirs := filepath.SplitList(os.Getenv("NETDATA_SNMP_MIB_DIRS"))
	if len(dirs) == 0 {
		t.Skip("NETDATA_SNMP_MIB_DIRS is not set; the always-on accessibility guard still runs")
	}
	manifest := readAccessManifest(t)
	corpus := loadMIBCorpus(t, dirs)
	// These missing columns have known non-readable row ancestors, which cannot certify column access.
	for _, ref := range []profileOIDRef{{module: "CHECKPOINT-MIB", oid: "1.3.6.1.4.1.2620.500.9003.1.11"}, {module: "MIB-Dell-10892", oid: "1.3.6.1.4.1.674.10892.1.600.12.1.16"}} {
		_, resolved := corpus.accessFact(ref.module, ref.oid)
		require.False(t, resolved, "ancestor is not exact source evidence: %s", ref.oid)
	}
	// Global OID nodes may belong to TRUENAS-MIB; the profile and its parent row must remain FREENAS-MIB.
	freenas, resolved := corpus.accessFact("FREENAS-MIB", "1.3.6.1.4.1.50536.1.1.1.1.1")
	require.True(t, resolved)
	require.Equal(t, "FREENAS-MIB", freenas.Module)
	require.Equal(t, "zpoolEntry", freenas.Row)
	refreshing := os.Getenv("NETDATA_SNMP_MIB_REFRESH") == "1"
	sources := map[string]string{}
	facts := map[string]profileAccessFact{}
	// Retain evidence for removed non-pollable sources, so reintroducing one is rejected without a MIB installation.
	for _, old := range manifest.Facts {
		facts[accessFactKey(old.Declared, old.OID)] = old
	}
	// Retain exact source evidence for this historical readable field; it is not a proven join source.
	const huaweiDescriptionOID = "1.3.6.1.4.1.2011.5.25.177.1.1.8.1.11"
	huawei, ok := corpus.accessFact("HUAWEI-BGP-VPN-MIB", huaweiDescriptionOID)
	require.True(t, ok)
	require.Equal(t, "read-only", huawei.Access)
	require.Equal(t, "hwBgpPeerSessionExtDescription", huawei.Object)
	if refreshing {
		facts[accessFactKey(huawei.Declared, huawei.OID)] = huawei
	}
	baseline := map[string]bool{}
	for _, key := range manifest.Unresolved {
		baseline[key] = true
	}
	unresolved := 0
	for file, refs := range stockAccessRefs(t) {
		for _, ref := range refs {
			key := accessFactKey(ref.module, ref.oid)
			fact, ok := corpus.accessFact(ref.module, ref.oid)
			if !ok {
				require.True(t, baseline[accessRefKey(file, ref)], "new unresolved source %s", accessRefKey(file, ref))
				unresolved++
				continue
			}
			if !refreshing {
				require.Equal(t, facts[key], fact, "source fact drift: %s", key)
			}
			facts[key] = fact
		}
	}
	keys := make([]string, 0, len(facts))
	for key := range facts {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	manifest.Facts = nil
	for _, key := range keys {
		old := facts[key]
		fact, ok := corpus.accessFact(old.Declared, old.OID)
		require.True(t, ok, "recorded exact source vanished: %s", key)
		if !refreshing {
			require.Equal(t, old, fact, "source fact drift: %s", key)
		}
		mod := corpus.modules[strings.ToLower(fact.Module)]
		require.NotNil(t, mod)
		digest := sources[fact.Source]
		if digest == "" {
			data, err := os.ReadFile(mod.SourcePath())
			require.NoError(t, err)
			digest = fmt.Sprintf("%x", sha256.Sum256(data))
		}
		if expected, ok := manifest.Sources[fact.Source]; ok {
			require.Equal(t, expected, digest, "pinned source changed: %s; refresh requires the recorded source revision", fact.Source)
		}
		sources[fact.Source] = digest
		manifest.Facts = append(manifest.Facts, fact)
	}
	if refreshing {
		manifest.Sources = sources
		writeAccessManifest(t, manifest)
	}
	t.Logf("%d exact source facts verified; %d active references remain explicitly unresolved", len(manifest.Facts), unresolved)
}

func (c *mibCorpus) accessFact(declared, rawOID string) (profileAccessFact, bool) {
	oid, err := mib.ParseOID(strings.TrimPrefix(rawOID, "."))
	if err != nil {
		return profileAccessFact{}, false
	}
	obj := c.object(declared, oid)
	if obj == nil || !isLeafObject(obj) {
		// Scalars have exactly one .0 instance. Fixed table cells may also be fetched by GET; only an exact column
		// ancestor establishes their access. A row/table/root ancestor is never evidence for a missing object.
		for i := len(oid) - 1; i > 0; i-- {
			candidate := c.object(declared, oid[:i])
			if candidate == nil {
				continue
			}
			if candidate.Kind() == mib.KindScalar && len(oid) == i+1 && oid[i] == 0 || candidate.Kind() == mib.KindColumn {
				obj = candidate
			}
			break
		}
	}
	if obj == nil || !isLeafObject(obj) {
		return profileAccessFact{}, false
	}
	source := filepath.ToSlash(obj.Module().SourcePath())
	if i := strings.LastIndex(source, "/mibs/"); i >= 0 {
		source = source[i+1:]
	} else {
		source = "mibs/" + filepath.Base(source)
	}
	fact := profileAccessFact{Declared: declared, OID: strings.TrimPrefix(rawOID, "."), Module: obj.Module().Name(), ObjectOID: obj.OID().String(), Object: obj.Name(), Kind: obj.Kind().String(), Access: obj.Access().String(), Source: source, Start: int(obj.Span().Start), End: int(obj.Span().End)}
	if obj.Kind() == mib.KindColumn {
		rowOID := obj.OID()[:len(obj.OID())-1].String()
		if row := c.objects[strings.ToLower(obj.Module().Name())][rowOID]; row != nil && row.Kind() == mib.KindRow {
			fact.Row = row.Name()
		}
	}
	return fact, true
}

func writeAccessManifest(t *testing.T, manifest profileAccessManifest) {
	t.Helper()
	// One fact per line keeps the compact evidence diff reviewable without serializing the full MIB corpus.
	data, err := json.MarshalIndent(manifest, "", "  ")
	require.NoError(t, err)
	var generic map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &generic))
	facts := make([]string, 0, len(manifest.Facts))
	for _, fact := range manifest.Facts {
		line, err := json.Marshal(fact)
		require.NoError(t, err)
		facts = append(facts, "    "+string(line))
	}
	generic["facts"] = json.RawMessage("[\n" + strings.Join(facts, ",\n") + "\n  ]")
	// Marshal the small outer fields separately to preserve the one-line fact records.
	var out strings.Builder
	out.WriteString("{\n")
	keys := make([]string, 0, len(generic))
	for key := range generic {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for i, key := range keys {
		if i > 0 {
			out.WriteString(",\n")
		}
		fmt.Fprintf(&out, "  %q: %s", key, generic[key])
	}
	out.WriteString("\n}\n")
	require.NoError(t, os.WriteFile(accessManifestPath, []byte(out.String()), 0644))
}

func accessJSONValue(v any) any {
	switch x := v.(type) {
	case map[any]any:
		r := map[string]any{}
		for k, value := range x {
			r[fmt.Sprint(k)] = accessJSONValue(value)
		}
		return r
	case []any:
		r := make([]any, len(x))
		for i, value := range x {
			r[i] = accessJSONValue(value)
		}
		return r
	default:
		return v
	}
}

// This minimized upstream projection retains the incomplete suffix and both returned INDEX objects. Management
// addresses are replaced with documentation addresses; no original address is necessary to prove the deviation.
func TestProfileAccessibility_RB750CaptureEvidence(t *testing.T) {
	manifest := readAccessManifest(t)
	capture, err := os.ReadFile("testdata/profile-mib-access-rb750gr3.snmprec")
	require.NoError(t, err)
	require.Equal(t, manifest.ExceptionCapture["projection_sha256"], fmt.Sprintf("%x", sha256.Sum256(capture)))
	require.Equal(t, "tests/snmpsim/routeros_rb750gr3.snmprec", manifest.ExceptionCapture["source"])
	require.Len(t, manifest.ExceptionCapture["source_sha256"], 64)
	values := map[string]map[string]string{}
	for _, line := range strings.Split(string(capture), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "|")
		require.Len(t, fields, 3)
		const prefix = "1.0.8802.1.1.2.1.4.2.1."
		require.True(t, strings.HasPrefix(fields[0], prefix))
		parts := strings.SplitN(strings.TrimPrefix(fields[0], prefix), ".", 2)
		require.Len(t, parts, 2)
		require.Len(t, strings.Split(parts[1], "."), 3, "suffix intentionally omits subtype, length and address")
		if values[parts[1]] == nil {
			values[parts[1]] = map[string]string{}
		}
		values[parts[1]][parts[0]] = fields[2]
	}
	require.Len(t, values, 2)
	for _, row := range values {
		require.Len(t, row, 2, "only inaccessible columns .1/.2 are returned; standard readable anchor .3 is absent")
		address, err := hex.DecodeString(row["2"])
		require.NoError(t, err)
		switch row["1"] {
		case "1":
			require.Len(t, address, 4)
		case "2":
			require.Len(t, address, 16)
		default:
			t.Fatalf("unexpected address subtype %s", row["1"])
		}
	}
}

func TestProfileAccessibility_SourceTraversal(t *testing.T) {
	var doc any
	require.NoError(t, yaml.Unmarshal([]byte(`topology:
 - MIB: EXAMPLE-MIB
   table: {OID: 1.2.3, name: exampleTable}
   symbols: [ &anchor {OID: 1.2.3.1.2, name: exampleValue} ]
   metric_tags:
    - symbol: {OID: 1.2.3.1.3, name: exampleTag}
    - {OID: 1.2.3.1.4, symbol: exampleLegacy}
    - index: 1
   descriptors:
    description: {symbol: *anchor}
   identity:
    peer: {from: 1.2.3.1.5}
`), &doc))
	var refs []profileOIDRef
	collectProfileOIDRefs(doc, "", "", false, &refs)
	require.Len(t, refs, 5)
	for _, ref := range refs {
		require.Equal(t, "EXAMPLE-MIB", ref.module)
		require.True(t, ref.column)
		require.NotEqual(t, "1.2.3", ref.oid)
	}
	require.False(t, accessException("another-device.yaml", profileOIDRef{module: "LLDP-MIB", path: ".topology[0].symbols[0]", oid: "1.0.8802.1.1.2.1.4.2.1.1", name: "lldp_rem_man_addr"}))
}

func TestProfileAccessibility_RejectsUnprovenAndNonPollableSources(t *testing.T) {
	ref := profileOIDRef{path: ".bgp[0].descriptors.description.symbol", module: "EXAMPLE-MIB", oid: "1.2.3.1.2", column: true}
	for _, access := range []string{"not-accessible", "accessible-for-notify", "write-only", "read-only"} {
		t.Run(access, func(t *testing.T) {
			facts := map[string]profileAccessFact{accessFactKey(ref.module, ref.oid): {ObjectOID: ref.oid, Access: access}}
			problem, known := checkPollingAccess("device.yaml", ref, facts, nil)
			require.True(t, known)
			if access == "read-only" {
				require.Empty(t, problem)
			} else {
				require.NotEmpty(t, problem)
			}
		})
	}
	baseline := map[string]bool{accessRefKey("device.yaml", ref): true}
	problem, known := checkPollingAccess("device.yaml", ref, nil, baseline)
	require.Empty(t, problem)
	require.False(t, known)
	problem, _ = checkPollingAccess("another-device.yaml", ref, nil, baseline)
	require.NotEmpty(t, problem)
	ref.path = ".bgp[1].descriptors.description.symbol"
	problem, _ = checkPollingAccess("device.yaml", ref, nil, baseline)
	require.NotEmpty(t, problem)
}
