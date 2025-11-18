package jobengine

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	zpre "github.com/netdata/netdata/go/plugins/pkg/zabbixpreproc"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/ids"
	pkgzabbix "github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/zabbix"
)

const pathLengthScript = `
var rows = JSON.parse(value);
var fsKey = "{#" + "FSNAME}";
var pathKey = "{#" + "FSPATH}";
var target = "{#FSNAME}";
var matched = "";
for (var i = 0; i < rows.length; i++) {
  if (rows[i][fsKey] === target) {
    matched = rows[i][pathKey] || "";
    break;
  }
}
value = (matched || "").length.toString();
return value;
`

const snmpValueScript = `
var rows = JSON.parse(value);
var indexKey = "{#" + "SNMPINDEX}";
var valueKey = "MACRO2";
var target = "{#SNMPINDEX}";
var matched = "";
for (var i = 0; i < rows.length; i++) {
  if (rows[i][indexKey] === target) {
    matched = rows[i][valueKey] || "";
    break;
  }
}
value = matched || "";
return value;
`

const promDiscoveryScript = `
var entries = JSON.parse(value);
var out = [];
for (var i = 0; i < entries.length; i++) {
  var entry = entries[i];
  if (entry.name !== "node_filesystem_info") {
    continue;
  }
  var labels = entry.labels || {};
  var mount = labels.mountpoint || "";
  if (!mount) {
    continue;
  }
  out.push({"{#MOUNT}": mount});
}
value = JSON.stringify(out);
return value;
`

const promValueScript = `
var entries = JSON.parse(value);
var target = "{#MOUNT}";
var result = "";
for (var i = 0; i < entries.length; i++) {
  var entry = entries[i];
  if (entry.name !== "node_filesystem_free_bytes") {
    continue;
  }
  var labels = entry.labels || {};
  if (labels.mountpoint === target) {
    result = entry.value || entry.valueStr || "";
    break;
  }
}
value = result ? result.toString() : "";
return value;
`

const csvMultiValueScript = `
var row = JSON.parse(value);
var raw = row["value"] || row["VALUE"] || "0";
value = raw.toString();
return value;
`

func TestJobProcessMatrix(t *testing.T) {
	cases := buildMatrixCases()
	if len(cases) < 100 {
		t.Fatalf("expected at least 100 scenarios, have %d", len(cases))
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			proc := zpre.NewPreprocessor("jobengine-" + tc.name)
			job, err := NewJob(tc.cfg, proc, Options{})
			if err != nil {
				t.Fatalf("new job failed: %v", err)
			}
			res := job.Process(tc.input)
			job.Destroy()
			tc.expect.assert(t, res)
		})
	}
}

// ----------------------------------------------------------------------------

type scenario struct {
	name   string
	cfg    pkgzabbix.JobConfig
	input  Input
	expect expectation
}

type expectation struct {
	metrics      int
	values       []float64
	active       int
	removed      int
	removedIDs   []string
	job          FailureFlags
	instances    map[string]FailureFlags
	macroKeys    []string
	errorMetrics int
}

func (e expectation) assert(t *testing.T, res Result) {
	if res.State.Job != e.job {
		t.Fatalf("job flags mismatch: want %+v got %+v", e.job, res.State.Job)
	}
	if e.instances != nil && len(res.State.Instances) != len(e.instances) {
		t.Fatalf("instance flag map mismatch: want %d got %d", len(e.instances), len(res.State.Instances))
	}
	for id, flags := range e.instances {
		if got, ok := res.State.Instances[id]; !ok || got != flags {
			t.Fatalf("instance %s flags mismatch: want %+v got %+v", id, flags, got)
		}
	}
	if len(res.Metrics) != e.metrics {
		t.Fatalf("metrics count mismatch: want %d got %d", e.metrics, len(res.Metrics))
	}
	if len(res.Active) != e.active {
		t.Fatalf("active instances mismatch: want %d got %d", e.active, len(res.Active))
	}
	if len(res.Removed) != e.removed {
		t.Fatalf("removed instances mismatch: want %d got %d", e.removed, len(res.Removed))
	}
	if len(e.removedIDs) > 0 {
		gotIDs := make([]string, 0, len(res.Removed))
		for _, inst := range res.Removed {
			gotIDs = append(gotIDs, inst.ID)
		}
		sort.Strings(gotIDs)
		expected := append([]string(nil), e.removedIDs...)
		sort.Strings(expected)
		if len(gotIDs) != len(expected) {
			t.Fatalf("removed IDs mismatch: want %v got %v", expected, gotIDs)
		}
		for i := range gotIDs {
			if gotIDs[i] != expected[i] {
				t.Fatalf("removed IDs mismatch: want %v got %v", expected, gotIDs)
			}
		}
	}
	if len(e.values) > 0 {
		gotVals := make([]float64, 0, len(res.Metrics))
		for _, m := range res.Metrics {
			gotVals = append(gotVals, m.Value)
		}
		sort.Float64s(gotVals)
		expected := append([]float64(nil), e.values...)
		sort.Float64s(expected)
		if len(gotVals) != len(expected) {
			t.Fatalf("value set mismatch: want %v got %v", expected, gotVals)
		}
		for i := range gotVals {
			if diff := gotVals[i] - expected[i]; diff > 1e-9 || diff < -1e-9 {
				t.Fatalf("value mismatch: want %v got %v", expected, gotVals)
			}
		}
	}
	if len(e.macroKeys) > 0 {
		for _, m := range res.Metrics {
			for _, key := range e.macroKeys {
				if _, ok := m.Instance.Macros[key]; !ok {
					t.Fatalf("expected macro %s in metric instance %+v", key, m.Instance)
				}
			}
		}
	}
	if e.errorMetrics > 0 {
		count := 0
		for _, m := range res.Metrics {
			if m.Error != nil {
				count++
			}
		}
		if count != e.errorMetrics {
			t.Fatalf("expected %d errored metrics, got %d", e.errorMetrics, count)
		}
	}
}

// ----------------------------------------------------------------------------

type variant int

const (
	variantNormal variant = iota
	variantCollectErr
	variantExtractionErr
	variantDimensionErr
	variantLLDErr
)

func buildMatrixCases() []scenario {
	var cases []scenario

	simpleSamples := []float64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	simpleVariants := []variant{variantNormal, variantCollectErr, variantExtractionErr, variantDimensionErr}
	for idx, sample := range simpleSamples {
		for _, v := range simpleVariants {
			cases = append(cases, buildSimpleScenario(idx, sample, v))
		}
	}

	multiSamples := []struct{ used, free float64 }{
		{10, 90}, {20, 80}, {30, 70}, {40, 60}, {50, 50}, {60, 40}, {70, 30}, {80, 20},
	}
	for idx, sample := range multiSamples {
		for _, v := range simpleVariants {
			cases = append(cases, buildMultiScenario(idx, sample.used, sample.free, v))
		}
	}

	lldSamples := []struct{ root, home float64 }{
		{10, 20}, {15, 25}, {5, 35}, {40, 10}, {12, 13}, {7, 9},
	}
	lldVariants := []variant{variantNormal, variantCollectErr, variantExtractionErr, variantDimensionErr, variantLLDErr}
	for idx, sample := range lldSamples {
		for _, v := range lldVariants {
			cases = append(cases, buildLLDScenario(idx, sample.root, sample.home, v))
		}
	}

	for idx := 0; idx < 10; idx++ {
		cases = append(cases, buildCSVLLDScenario(idx))
		cases = append(cases, buildXMLLLDScenario(idx))
		cases = append(cases, buildSNMPLLDScenario(idx))
	}

	promSamples := []struct{ root, home float64 }{
		{60, 80}, {45, 70}, {90, 20}, {30, 55},
	}
	for idx, sample := range promSamples {
		for _, v := range lldVariants {
			cases = append(cases, buildPrometheusLLDScenario(idx, sample.root, sample.home, v))
		}
	}

	for idx := 0; idx < 5; idx++ {
		for _, v := range simpleVariants {
			cases = append(cases, buildXPathScenario(idx, float64(40+idx*3), float64(50+idx*4), v))
			cases = append(cases, buildCSVMultiScenario(idx, float64(10+idx), v))
		}
	}

	return cases
}

// ----------------------------------------------------------------------------

type stateVariant int

const (
	stateVariantStable stateVariant = iota
	stateVariantAddRemove
	stateVariantLLDFailure
	stateVariantExtractionFailure
	stateVariantDimensionFailure
)

type statefulScenario struct {
	name string
	cfg  pkgzabbix.JobConfig
	runs []stateRun
}

type stateRun struct {
	label  string
	input  Input
	expect expectation
}

func TestJobProcessStatefulMatrix(t *testing.T) {
	cases := buildStatefulCases()
	if len(cases) < 50 {
		t.Fatalf("expected >=50 stateful scenarios, have %d", len(cases))
	}
	for _, sc := range cases {
		t.Run(sc.name, func(t *testing.T) {
			proc := zpre.NewPreprocessor("state-" + sc.name)
			job, err := NewJob(sc.cfg, proc, Options{})
			if err != nil {
				t.Fatalf("new job failed: %v", err)
			}
			for _, run := range sc.runs {
				res := job.Process(run.input)
				run.expect.assert(t, res)
			}
			job.Destroy()
		})
	}
}

func TestJobProcessDuplicateDiscovery(t *testing.T) {
	cfg := lldJobConfig("duplicate")
	input := Input{Payload: []byte(`{"discovery":[{"{#FSNAME}":"/"},{"{#FSNAME}":"/"},{"{#FSNAME}":"/home"}],"data":[{"fs":"/","used":10,"free":90},{"fs":"/home","used":20,"free":80}]}`), Timestamp: time.Now()}
	proc := zpre.NewPreprocessor("dup")
	job, err := NewJob(cfg, proc, Options{})
	if err != nil {
		t.Fatalf("new job failed: %v", err)
	}
	res := job.Process(input)
	if !res.State.Job.LLD {
		t.Fatalf("expected LLD failure due to duplicate ids")
	}
	if len(res.Metrics) != 0 {
		t.Fatalf("expected no metrics when discovery fails, got %d", len(res.Metrics))
	}
}

func TestJobShouldDiscover(t *testing.T) {
	job := &Job{}
	if !job.shouldDiscover(time.Now()) {
		t.Fatalf("interval <=0 should force discovery")
	}
	job.cfg.LLD.Interval = confopt.Duration(10 * time.Second)
	job.lastDiscovery = time.Now()
	if job.shouldDiscover(time.Now()) {
		t.Fatalf("should not rediscover before interval")
	}
	job.lastDiscovery = time.Now().Add(-11 * time.Second)
	if !job.shouldDiscover(time.Now()) {
		t.Fatalf("should rediscover after interval elapsed")
	}
}

func TestLLDInstanceRemovalAfterMaxMissing(t *testing.T) {
	cfg := lldJobConfig("miss")
	cfg.LLD.MaxMissing = 2
	proc := zpre.NewPreprocessor("miss-test")
	job, err := NewJob(cfg, proc, Options{})
	if err != nil {
		t.Fatalf("new job failed: %v", err)
	}
	createPayload := Input{Payload: []byte(`{"discovery":[{"{#FSNAME}":"/"}],"data":[{"fs":"/","used":10,"free":90}]}`), Timestamp: time.Now()}
	res := job.Process(createPayload)
	if len(res.Active) != 1 {
		t.Fatalf("expected 1 active instance, got %d", len(res.Active))
	}
	missingPayload := Input{Payload: []byte(`{"discovery":[],"data":[]}`), Timestamp: time.Now().Add(time.Second)}
	res = job.Process(missingPayload)
	if len(res.Removed) != 0 {
		t.Fatalf("expected no removal after first miss, got %d", len(res.Removed))
	}
	missingPayload.Timestamp = missingPayload.Timestamp.Add(time.Second)
	res = job.Process(missingPayload)
	if len(res.Removed) != 1 {
		t.Fatalf("expected removal after reaching max_missing, got %d", len(res.Removed))
	}
}

func buildStatefulCases() []statefulScenario {
	samples := []struct{ root, home float64 }{
		{10, 20}, {15, 25}, {5, 35}, {40, 10}, {12, 13}, {7, 9}, {22, 11}, {18, 16}, {9, 12}, {31, 17},
	}
	variants := []stateVariant{stateVariantStable, stateVariantAddRemove, stateVariantLLDFailure, stateVariantExtractionFailure, stateVariantDimensionFailure}
	var cases []statefulScenario
	for idx, sample := range samples {
		for _, v := range variants {
			cases = append(cases, buildStatefulScenario(idx, sample.root, sample.home, v))
		}
	}
	promStates := []struct{ root, home float64 }{{60, 45}, {55, 35}}
	for idx, sample := range promStates {
		cases = append(cases, buildPromStatefulScenario(idx, sample.root, sample.home))
	}
	cases = append(cases, buildXPathStatefulScenario(0, 42, 57))
	return cases
}

func buildStatefulScenario(idx int, root, home float64, v stateVariant) statefulScenario {
	name := fmt.Sprintf("state_%d_%s", idx, stateVariantName(v))
	cfg := lldJobConfig(fmt.Sprintf("state_%d", idx))
	rootID := ids.Sanitize("/")
	homeID := ids.Sanitize("/home")
	instBoth := []instData{{"/", root, 100 - root}, {"/home", home, 100 - home}}
	instRoot := []instData{{"/", root, 100 - root}}
	instHome := []instData{{"/home", home, 100 - home}}
	var runs []stateRun
	switch v {
	case stateVariantStable:
		payload := []byte(lldPayload(instBoth))
		expect := expectation{metrics: 4, values: valuesFor(instBoth), active: 2, job: FailureFlags{}, instances: zeroFlags(rootID, homeID), macroKeys: []string{"{#FSNAME}"}}
		runs = []stateRun{
			{label: "run1", input: Input{Payload: payload, Timestamp: time.Now()}, expect: expect},
			{label: "run2", input: Input{Payload: payload, Timestamp: time.Now()}, expect: expect},
			{label: "run3", input: Input{Payload: payload, Timestamp: time.Now()}, expect: expect},
		}
	case stateVariantAddRemove:
		payload1 := []byte(lldPayload(instRoot))
		payload2 := []byte(lldPayload(instBoth))
		payload3 := []byte(lldPayload(instHome))
		runs = []stateRun{
			{label: "run1", input: Input{Payload: payload1, Timestamp: time.Now()}, expect: expectation{metrics: 2, values: valuesFor(instRoot), active: 1, job: FailureFlags{}, instances: zeroFlags(rootID), macroKeys: []string{"{#FSNAME}"}}},
			{label: "run2", input: Input{Payload: payload2, Timestamp: time.Now()}, expect: expectation{metrics: 4, values: valuesFor(instBoth), active: 2, job: FailureFlags{}, instances: zeroFlags(rootID, homeID), macroKeys: []string{"{#FSNAME}"}}},
			{label: "run3", input: Input{Payload: payload3, Timestamp: time.Now()}, expect: expectation{metrics: 2, values: valuesFor(instHome), active: 1, removed: 1, removedIDs: []string{rootID}, job: FailureFlags{}, instances: zeroFlags(homeID), macroKeys: []string{"{#FSNAME}"}}},
		}
	case stateVariantLLDFailure:
		payload := []byte(lldPayload(instBoth))
		runs = []stateRun{
			{label: "run1", input: Input{Payload: payload, Timestamp: time.Now()}, expect: expectation{metrics: 4, values: valuesFor(instBoth), active: 2, job: FailureFlags{}, instances: zeroFlags(rootID, homeID), macroKeys: []string{"{#FSNAME}"}}},
			{label: "run2", input: Input{Payload: []byte("not-json"), Timestamp: time.Now()}, expect: expectation{metrics: 0, values: nil, active: 2, job: FailureFlags{LLD: true}, instances: map[string]FailureFlags{rootID: {LLD: true}, homeID: {LLD: true}}}},
			{label: "run3", input: Input{Payload: payload, Timestamp: time.Now()}, expect: expectation{metrics: 4, values: valuesFor(instBoth), active: 2, job: FailureFlags{}, instances: zeroFlags(rootID, homeID), macroKeys: []string{"{#FSNAME}"}}},
		}
	case stateVariantExtractionFailure:
		payload := []byte(lldPayload(instBoth))
		badPayload := []byte(`{"discovery":[{"{#FSNAME}":"/"},{"{#FSNAME}":"/home"}],"data":[{"fs":"/","used":"oops","free":"oops"},{"fs":"/home","used":"bad","free":"bad"}]}`)
		runs = []stateRun{
			{label: "run1", input: Input{Payload: payload, Timestamp: time.Now()}, expect: expectation{metrics: 4, values: valuesFor(instBoth), active: 2, job: FailureFlags{}, instances: zeroFlags(rootID, homeID), macroKeys: []string{"{#FSNAME}"}}},
			{label: "run2", input: Input{Payload: badPayload, Timestamp: time.Now()}, expect: expectation{metrics: 4, values: nil, active: 2, job: FailureFlags{Extraction: true}, instances: map[string]FailureFlags{rootID: {Extraction: true}, homeID: {Extraction: true}}, errorMetrics: 4}},
			{label: "run3", input: Input{Payload: payload, Timestamp: time.Now()}, expect: expectation{metrics: 4, values: valuesFor(instBoth), active: 2, job: FailureFlags{}, instances: zeroFlags(rootID, homeID), macroKeys: []string{"{#FSNAME}"}}},
		}
	case stateVariantDimensionFailure:
		payload := []byte(lldPayload(instBoth))
		badPayload := []byte(`{"discovery":[{"{#FSNAME}":"/"},{"{#FSNAME}":"/home"}],"data":[{"fs":"/","used":150,"free":-10},{"fs":"/home","used":180,"free":-20}]}`)
		runs = []stateRun{
			{label: "run1", input: Input{Payload: payload, Timestamp: time.Now()}, expect: expectation{metrics: 4, values: valuesFor(instBoth), active: 2, job: FailureFlags{}, instances: zeroFlags(rootID, homeID), macroKeys: []string{"{#FSNAME}"}}},
			{label: "run2", input: Input{Payload: badPayload, Timestamp: time.Now()}, expect: expectation{metrics: 4, values: nil, active: 2, job: FailureFlags{Dimension: true}, instances: map[string]FailureFlags{rootID: {Dimension: true}, homeID: {Dimension: true}}, errorMetrics: 4}},
			{label: "run3", input: Input{Payload: payload, Timestamp: time.Now()}, expect: expectation{metrics: 4, values: valuesFor(instBoth), active: 2, job: FailureFlags{}, instances: zeroFlags(rootID, homeID), macroKeys: []string{"{#FSNAME}"}}},
		}
	}
	return statefulScenario{name: name, cfg: cfg, runs: runs}
}

func buildPromStatefulScenario(idx int, root, home float64) statefulScenario {
	name := fmt.Sprintf("state_prom_%d", idx)
	cfg := prometheusJobConfig(fmt.Sprintf("state_prom_%d", idx))
	instRoot := ids.Sanitize("fs_/")
	instHome := ids.Sanitize("fs_/home")
	runs := []stateRun{
		{
			label:  "run1",
			input:  Input{Payload: []byte(prometheusPayload(root, home)), Timestamp: time.Now()},
			expect: expectation{metrics: 2, values: []float64{root, home}, active: 2, job: FailureFlags{}, instances: zeroFlags(instRoot, instHome)},
		},
		{
			label:  "run2",
			input:  Input{Payload: []byte("not-prom"), Timestamp: time.Now()},
			expect: expectation{metrics: 0, values: nil, active: 2, job: FailureFlags{LLD: true}, instances: map[string]FailureFlags{instRoot: {LLD: true}, instHome: {LLD: true}}},
		},
		{
			label:  "run3",
			input:  Input{Payload: []byte(prometheusPayload(root+5, home+3)), Timestamp: time.Now()},
			expect: expectation{metrics: 2, values: []float64{root + 5, home + 3}, active: 2, job: FailureFlags{}, instances: zeroFlags(instRoot, instHome)},
		},
	}
	return statefulScenario{name: name, cfg: cfg, runs: runs}
}

func buildXPathStatefulScenario(idx int, cpu, gpu float64) statefulScenario {
	name := fmt.Sprintf("state_xpath_%d", idx)
	cfg := xpathJobConfig(fmt.Sprintf("state_xpath_%d", idx))
	defaultID := "default"
	runs := []stateRun{
		{
			label:  "run1",
			input:  Input{Payload: []byte(fmt.Sprintf(`<sensors><sensor id="cpu">%f</sensor><sensor id="gpu">%f</sensor></sensors>`, cpu, gpu)), Timestamp: time.Now()},
			expect: expectation{metrics: 2, values: []float64{cpu, gpu}, active: 1, job: FailureFlags{}, instances: zeroFlags(defaultID)},
		},
		{
			label:  "run2",
			input:  Input{Payload: []byte(`<sensors><sensor id="cpu">oops</sensor><sensor id="gpu">bad</sensor></sensors>`), Timestamp: time.Now()},
			expect: expectation{metrics: 2, values: nil, active: 1, job: FailureFlags{Extraction: true}, instances: map[string]FailureFlags{defaultID: {Extraction: true}}, errorMetrics: 2},
		},
		{
			label:  "run3",
			input:  Input{Payload: []byte(fmt.Sprintf(`<sensors><sensor id="cpu">%f</sensor><sensor id="gpu">%f</sensor></sensors>`, cpu+2, gpu+2)), Timestamp: time.Now()},
			expect: expectation{metrics: 2, values: []float64{cpu + 2, gpu + 2}, active: 1, job: FailureFlags{}, instances: zeroFlags(defaultID)},
		},
	}
	return statefulScenario{name: name, cfg: cfg, runs: runs}
}

func stateVariantName(v stateVariant) string {
	switch v {
	case stateVariantStable:
		return "stable"
	case stateVariantAddRemove:
		return "add_remove"
	case stateVariantLLDFailure:
		return "lld_fail"
	case stateVariantExtractionFailure:
		return "extract_fail"
	case stateVariantDimensionFailure:
		return "dimension_fail"
	default:
		return "unknown"
	}
}

type instData struct {
	name string
	used float64
	free float64
}

func lldPayload(insts []instData) string {
	var discParts []string
	var dataParts []string
	for _, inst := range insts {
		discParts = append(discParts, fmt.Sprintf(`{"{#FSNAME}":"%s"}`, inst.name))
		dataParts = append(dataParts, fmt.Sprintf(`{"fs":"%s","used":%g,"free":%g}`, inst.name, inst.used, inst.free))
	}
	return fmt.Sprintf(`{"discovery":[%s],"data":[%s]}`, strings.Join(discParts, ","), strings.Join(dataParts, ","))
}

func valuesFor(insts []instData) []float64 {
	var vals []float64
	for _, inst := range insts {
		vals = append(vals, inst.used, inst.free)
	}
	return vals
}

func prometheusPayload(root, home float64) string {
	return prometheusPayloadStrings(fmt.Sprintf("%f", root), fmt.Sprintf("%f", home))
}

func prometheusPayloadStrings(rootVal, homeVal string) string {
	return fmt.Sprintf(`# HELP node_filesystem_free_bytes Free bytes
# TYPE node_filesystem_free_bytes gauge
# HELP node_filesystem_info Filesystem info
# TYPE node_filesystem_info gauge
node_filesystem_info{mountpoint="/",fstype="ext4"} 1
node_filesystem_info{mountpoint="/home",fstype="xfs"} 1
node_filesystem_free_bytes{mountpoint="/",fstype="ext4"} %s
node_filesystem_free_bytes{mountpoint="/home",fstype="xfs"} %s
`, rootVal, homeVal)
}

func zeroFlags(ids ...string) map[string]FailureFlags {
	if len(ids) == 0 {
		return map[string]FailureFlags{}
	}
	m := make(map[string]FailureFlags, len(ids))
	for _, id := range ids {
		m[id] = FailureFlags{}
	}
	return m
}

func buildSimpleScenario(idx int, value float64, v variant) scenario {
	name := fmt.Sprintf("simple_%d_%s", idx, variantName(v))
	cfg := simpleJobConfig(fmt.Sprintf("simple_%d", idx))
	input := Input{Payload: []byte(fmt.Sprintf(`{"value": %f}`, value)), Timestamp: time.Now()}
	expect := expectation{metrics: 1, values: []float64{value}, active: 1, job: FailureFlags{}, instances: map[string]FailureFlags{"default": {}}}
	switch v {
	case variantCollectErr:
		input.CollectError = errors.New("collect")
		expect.metrics = 0
		expect.values = nil
		expect.active = 1
		expect.job = FailureFlags{Collect: true}
		expect.instances = map[string]FailureFlags{"default": {Collect: true}}
	case variantExtractionErr:
		input.Payload = []byte(`{"value":"oops"}`)
		expect.metrics = 1
		expect.values = nil
		expect.job = FailureFlags{Extraction: true}
		expect.instances = map[string]FailureFlags{"default": {Extraction: true}}
	case variantDimensionErr:
		input.Payload = []byte(`{"value": 200}`)
		expect.metrics = 1
		expect.values = nil
		expect.job = FailureFlags{Dimension: true}
		expect.instances = map[string]FailureFlags{"default": {Dimension: true}}
	}
	return scenario{name: name, cfg: cfg, input: input, expect: expect}
}

func buildMultiScenario(idx int, used, free float64, v variant) scenario {
	name := fmt.Sprintf("multi_%d_%s", idx, variantName(v))
	cfg := multiPipelineJobConfig(fmt.Sprintf("multi_%d", idx))
	input := Input{Payload: []byte(fmt.Sprintf(`{"used": %f, "free": %f}`, used, free)), Timestamp: time.Now()}
	// Validation happens BEFORE multiplier, so all normal inputs pass validation
	// Multiplier=2 is applied after validation
	usedAfterMultiplier := used * 2
	expect := expectation{metrics: 2, values: []float64{usedAfterMultiplier, free}, active: 1, job: FailureFlags{}, instances: map[string]FailureFlags{"default": {}}}
	switch v {
	case variantCollectErr:
		input.CollectError = errors.New("collect")
		expect.metrics = 0
		expect.values = nil
		expect.active = 1
		expect.job = FailureFlags{Collect: true}
		expect.instances = map[string]FailureFlags{"default": {Collect: true}}
	case variantExtractionErr:
		input.Payload = []byte(`{"used":"oops","free":"oops"}`)
		expect.metrics = 2
		expect.values = nil
		expect.job = FailureFlags{Extraction: true}
		expect.instances = map[string]FailureFlags{"default": {Extraction: true}}
	case variantDimensionErr:
		input.Payload = []byte(`{"used": 200, "free": 300}`)
		expect.metrics = 2
		expect.values = nil
		expect.job = FailureFlags{Dimension: true}
		expect.instances = map[string]FailureFlags{"default": {Dimension: true}}
	}
	return scenario{name: name, cfg: cfg, input: input, expect: expect}
}

func buildLLDScenario(idx int, root, home float64, v variant) scenario {
	name := fmt.Sprintf("lld_%d_%s", idx, variantName(v))
	cfg := lldJobConfig(fmt.Sprintf("lld_%d", idx))
	payload := fmt.Sprintf(`{"discovery":[{"{#FSNAME}":"/"},{"{#FSNAME}":"/home"}],"data":[{"fs":"/","used":%f,"free":%f},{"fs":"/home","used":%f,"free":%f}]}`, root, 100-root, home, 100-home)
	input := Input{Payload: []byte(payload), Timestamp: time.Now()}
	values := []float64{root, home, 100 - root, 100 - home}
	instA := ids.Sanitize("/")
	instB := ids.Sanitize("/home")
	macroKeys := []string{}
	if v == variantNormal {
		macroKeys = []string{"{#FSNAME}"}
	}
	expect := expectation{
		metrics:   4,
		values:    values,
		active:    2,
		job:       FailureFlags{},
		instances: map[string]FailureFlags{instA: {}, instB: {}},
		macroKeys: macroKeys,
	}
	switch v {
	case variantCollectErr:
		input.CollectError = errors.New("collect")
		expect.metrics = 0
		expect.values = nil
		expect.active = 1
		expect.job = FailureFlags{Collect: true}
		expect.instances = map[string]FailureFlags{"default": {Collect: true}}
	case variantExtractionErr:
		input.Payload = []byte(`{"discovery":[{"{#FSNAME}":"/"},{"{#FSNAME}":"/home"}],"data":[{"fs":"/","used":"abc","free":"def"},{"fs":"/home","used":"ghi","free":"jkl"}]}`)
		expect.metrics = 4
		expect.values = nil
		expect.job = FailureFlags{Extraction: true}
		expect.instances = map[string]FailureFlags{instA: {Extraction: true}, instB: {Extraction: true}}
	case variantDimensionErr:
		input.Payload = []byte(`{"discovery":[{"{#FSNAME}":"/"},{"{#FSNAME}":"/home"}],"data":[{"fs":"/","used":150,"free":-10},{"fs":"/home","used":180,"free":-20}]}`)
		expect.metrics = 4
		expect.values = nil
		expect.job = FailureFlags{Dimension: true}
		expect.instances = map[string]FailureFlags{instA: {Dimension: true}, instB: {Dimension: true}}
	case variantLLDErr:
		input.Payload = []byte(`not-json`)
		expect.metrics = 0
		expect.values = nil
		expect.active = 0
		expect.job = FailureFlags{LLD: true}
		expect.instances = map[string]FailureFlags{}
		return scenario{name: name, cfg: cfg, input: input, expect: expect}
	}
	return scenario{name: name, cfg: cfg, input: input, expect: expect}
}

func buildCSVLLDScenario(idx int) scenario {
	name := fmt.Sprintf("lld_csv_%d", idx)
	cfg := csvLLDJobConfig(fmt.Sprintf("csv_job_%d", idx))
	input := Input{Payload: []byte(`{#FSNAME},{#FSPATH}
root,/
home,/home`), Timestamp: time.Now()}
	instA := ids.Sanitize("root")
	instB := ids.Sanitize("home")
	expect := expectation{metrics: 2, values: []float64{1, 5}, active: 2, job: FailureFlags{}, instances: map[string]FailureFlags{instA: {}, instB: {}}, macroKeys: []string{"{#FSNAME}"}}
	return scenario{name: name, cfg: cfg, input: input, expect: expect}
}

func buildXMLLLDScenario(idx int) scenario {
	name := fmt.Sprintf("lld_xml_%d", idx)
	cfg := xmlLLDJobConfig(fmt.Sprintf("xml_job_%d", idx))
	input := Input{Payload: []byte(`<files><fs name="root" path="/"/><fs name="home" path="/home"/></files>`), Timestamp: time.Now()}
	instA := ids.Sanitize("root")
	instB := ids.Sanitize("home")
	expect := expectation{metrics: 2, values: []float64{1, 5}, active: 2, job: FailureFlags{}, instances: map[string]FailureFlags{instA: {}, instB: {}}, macroKeys: []string{"{#FSNAME}"}}
	return scenario{name: name, cfg: cfg, input: input, expect: expect}
}

func buildSNMPLLDScenario(idx int) scenario {
	name := fmt.Sprintf("lld_snmp_%d", idx)
	cfg := snmpLLDJobConfig(fmt.Sprintf("snmp_job_%d", idx))
	snmpData := `.1.3.6.1.1 = STRING: "ifA"
.1.3.6.1.2 = STRING: "ifB"
.1.3.6.2.1 = STRING: "100"
.1.3.6.2.2 = STRING: "200"`
	input := Input{Payload: []byte(snmpData), Timestamp: time.Now()}
	instA := ids.Sanitize("ifA")
	instB := ids.Sanitize("ifB")
	expect := expectation{metrics: 2, values: []float64{100, 200}, active: 2, job: FailureFlags{}, instances: map[string]FailureFlags{instA: {}, instB: {}}, macroKeys: []string{"MACRO1", "MACRO2", "{#SNMPINDEX}"}}
	return scenario{name: name, cfg: cfg, input: input, expect: expect}
}

func buildPrometheusLLDScenario(idx int, root, home float64, v variant) scenario {
	name := fmt.Sprintf("lld_prom_%d_%s", idx, variantName(v))
	cfg := prometheusJobConfig(fmt.Sprintf("prom_job_%d", idx))
	input := Input{Payload: []byte(prometheusPayload(root, home)), Timestamp: time.Now()}
	instRoot := ids.Sanitize("fs_/")
	instHome := ids.Sanitize("fs_/home")
	expect := expectation{
		metrics:   2,
		values:    []float64{root, home},
		active:    2,
		job:       FailureFlags{},
		instances: map[string]FailureFlags{instRoot: {}, instHome: {}},
		macroKeys: []string{"{#MOUNT}"},
	}
	switch v {
	case variantCollectErr:
		input.CollectError = errors.New("collect")
		expect.metrics = 0
		expect.values = nil
		expect.active = 1
		expect.job = FailureFlags{Collect: true}
		expect.instances = map[string]FailureFlags{"default": {Collect: true}}
	case variantExtractionErr:
		input.Payload = []byte(prometheusPayloadStrings("oops", "123"))
		expect.values = nil
		expect.job = FailureFlags{Extraction: true}
		expect.instances = map[string]FailureFlags{instRoot: {Extraction: true}, instHome: {}}
	case variantDimensionErr:
		input.Payload = []byte(prometheusPayload(1500, 2000))
		expect.values = nil
		expect.job = FailureFlags{Dimension: true}
		expect.instances = map[string]FailureFlags{instRoot: {Dimension: true}, instHome: {Dimension: true}}
	case variantLLDErr:
		input.Payload = []byte("not-prometheus")
		expect.metrics = 0
		expect.values = nil
		expect.active = 0
		expect.job = FailureFlags{LLD: true}
		expect.instances = map[string]FailureFlags{}
	}
	return scenario{name: name, cfg: cfg, input: input, expect: expect}
}

func buildXPathScenario(idx int, cpu, gpu float64, v variant) scenario {
	name := fmt.Sprintf("xpath_%d_%s", idx, variantName(v))
	cfg := xpathJobConfig(fmt.Sprintf("xpath_%d", idx))
	payload := fmt.Sprintf(`<sensors><sensor id="cpu">%f</sensor><sensor id="gpu">%f</sensor></sensors>`, cpu, gpu)
	input := Input{Payload: []byte(payload), Timestamp: time.Now()}
	expect := expectation{metrics: 2, values: []float64{cpu, gpu}, active: 1, job: FailureFlags{}, instances: map[string]FailureFlags{"default": {}}}
	switch v {
	case variantCollectErr:
		input.CollectError = errors.New("collect")
		expect.metrics = 0
		expect.values = nil
		expect.active = 1
		expect.job = FailureFlags{Collect: true}
		expect.instances = map[string]FailureFlags{"default": {Collect: true}}
	case variantExtractionErr:
		input.Payload = []byte(`<sensors><sensor id="cpu">oops</sensor><sensor id="gpu">bad</sensor></sensors>`)
		expect.values = nil
		expect.job = FailureFlags{Extraction: true}
		expect.instances = map[string]FailureFlags{"default": {Extraction: true}}
	case variantDimensionErr:
		input.Payload = []byte(`<sensors><sensor id="cpu">500</sensor><sensor id="gpu">600</sensor></sensors>`)
		expect.values = nil
		expect.job = FailureFlags{Dimension: true}
		expect.instances = map[string]FailureFlags{"default": {Dimension: true}}
	}
	return scenario{name: name, cfg: cfg, input: input, expect: expect}
}

func buildCSVMultiScenario(idx int, first float64, v variant) scenario {
	name := fmt.Sprintf("csv_multi_%d_%s", idx, variantName(v))
	cfg := csvMultiJobConfig(fmt.Sprintf("csv_multi_%d", idx))
	payload := fmt.Sprintf("name,value\nrow1,%f\nrow2,20", first)
	input := Input{Payload: []byte(payload), Timestamp: time.Now()}
	// CSVToJSONMulti returns 2 metrics but pipeline expects 1
	// Job engine drops output and returns 1 metric with value=0 and Dimension error
	expect := expectation{metrics: 1, values: []float64{0}, active: 1, job: FailureFlags{Dimension: true}, instances: map[string]FailureFlags{"default": {Dimension: true}}}
	switch v {
	case variantCollectErr:
		input.CollectError = errors.New("collect")
		expect.metrics = 0
		expect.values = nil
		expect.active = 1
		expect.job = FailureFlags{Collect: true}
		expect.instances = map[string]FailureFlags{"default": {Collect: true}}
	case variantExtractionErr:
		input.Payload = []byte("name,value\nrow1,oops\nrow2,20")
		expect.metrics = 1
		expect.values = nil
		// Extraction error takes precedence; multi-metric warning doesn't add Dimension flag
		expect.job = FailureFlags{Extraction: true}
		expect.instances = map[string]FailureFlags{"default": {Extraction: true}}
	case variantDimensionErr:
		input.Payload = []byte("name,value\nrow1,5000\nrow2,20")
		expect.metrics = 1
		expect.values = nil
		expect.job = FailureFlags{Dimension: true}
		expect.instances = map[string]FailureFlags{"default": {Dimension: true}}
	}
	return scenario{name: name, cfg: cfg, input: input, expect: expect}
}

func variantName(v variant) string {
	switch v {
	case variantNormal:
		return "normal"
	case variantCollectErr:
		return "collect"
	case variantExtractionErr:
		return "extract"
	case variantDimensionErr:
		return "dimension"
	case variantLLDErr:
		return "lld"
	default:
		return "unknown"
	}
}

// job config builders -------------------------------------------------------

func simpleJobConfig(name string) pkgzabbix.JobConfig {
	return pkgzabbix.JobConfig{
		Name: name,
		Collection: pkgzabbix.CollectionConfig{
			Type:    pkgzabbix.CollectionCommand,
			Command: "/usr/bin/true",
		},
		Pipelines: []pkgzabbix.PipelineConfig{
			{
				Name:      "value",
				Context:   "zabbix.simple.value",
				Dimension: "value",
				Unit:      "value",
				Steps: []zpre.Step{
					{Type: zpre.StepTypeJSONPath, Params: "$.value"},
					{Type: zpre.StepTypeValidateRange, Params: "0\n100"},
				},
			},
		},
	}
}

func multiPipelineJobConfig(name string) pkgzabbix.JobConfig {
	return pkgzabbix.JobConfig{
		Name: name,
		Collection: pkgzabbix.CollectionConfig{
			Type:    pkgzabbix.CollectionCommand,
			Command: "/usr/bin/true",
		},
		Pipelines: []pkgzabbix.PipelineConfig{
			{
				Name:      "used",
				Context:   "zabbix.multi.used",
				Dimension: "used",
				Unit:      "bytes",
				Precision: 0,
				Steps: []zpre.Step{
					{Type: zpre.StepTypeJSONPath, Params: "$.used"},
					{Type: zpre.StepTypeValidateRange, Params: "0\n100"},
					{Type: zpre.StepTypeMultiplier, Params: "2"},
				},
			},
			{
				Name:      "free",
				Context:   "zabbix.multi.free",
				Dimension: "free",
				Unit:      "bytes",
				Steps: []zpre.Step{
					{Type: zpre.StepTypeJSONPath, Params: "$.free"},
					{Type: zpre.StepTypeValidateRange, Params: "0\n100"},
				},
			},
		},
	}
}

func lldJobConfig(name string) pkgzabbix.JobConfig {
	return pkgzabbix.JobConfig{
		Name:       name,
		Collection: pkgzabbix.CollectionConfig{Type: pkgzabbix.CollectionCommand, Command: "/usr/bin/true"},
		LLD: pkgzabbix.LLDConfig{
			Steps:            []zpre.Step{{Type: zpre.StepTypeJSONPath, Params: "$.discovery"}},
			InstanceTemplate: "{#FSNAME}",
			MaxMissing:       0,
		},
		Pipelines: []pkgzabbix.PipelineConfig{
			{
				Name:      "used",
				Context:   "zabbix.lld.used",
				Dimension: "used",
				Unit:      "bytes",
				Steps: []zpre.Step{
					{Type: zpre.StepTypeJSONPath, Params: "$.data[?(@.fs == '{#FSNAME}')].used"},
					{Type: zpre.StepTypeValidateRange, Params: "0\n100"},
				},
			},
			{
				Name:      "free",
				Context:   "zabbix.lld.free",
				Dimension: "free",
				Unit:      "bytes",
				Steps: []zpre.Step{
					{Type: zpre.StepTypeJSONPath, Params: "$.data[?(@.fs == '{#FSNAME}')].free"},
					{Type: zpre.StepTypeValidateRange, Params: "0\n100"},
				},
			},
		},
	}
}

func csvLLDJobConfig(name string) pkgzabbix.JobConfig {
	const csvParams = `,
"
1`
	return pkgzabbix.JobConfig{
		Name:       name,
		Collection: pkgzabbix.CollectionConfig{Type: pkgzabbix.CollectionCommand, Command: "/usr/bin/true"},
		LLD: pkgzabbix.LLDConfig{
			Steps: []zpre.Step{
				{Type: zpre.StepTypeCSVToJSON, Params: csvParams},
				{Type: zpre.StepTypeJSONPath, Params: "$"},
			},
			InstanceTemplate: "{#FSNAME}",
			MaxMissing:       0,
		},
		Pipelines: []pkgzabbix.PipelineConfig{
			{
				Name:      "path",
				Context:   "zabbix.csv.path",
				Dimension: "value",
				Unit:      "characters",
				Steps: []zpre.Step{
					{Type: zpre.StepTypeCSVToJSON, Params: csvParams},
					{Type: zpre.StepTypeJavaScript, Params: pathLengthScript},
				},
			},
		},
	}
}

func xmlLLDJobConfig(name string) pkgzabbix.JobConfig {
	script := `var data = JSON.parse(value);
var list = data.files.fs;
if (!Array.isArray(list)) list = [list];
var fsMacro = "{#" + "FSNAME}";
var pathMacro = "{#" + "FSPATH}";
var out = list.map(function(node){
  var entry = {};
  entry[fsMacro] = node["@name"];
  entry[pathMacro] = node["@path"];
  return entry;
});
value = JSON.stringify(out);
return value;`
	return pkgzabbix.JobConfig{
		Name:       name,
		Collection: pkgzabbix.CollectionConfig{Type: pkgzabbix.CollectionCommand, Command: "/usr/bin/true"},
		LLD: pkgzabbix.LLDConfig{
			Steps: []zpre.Step{
				{Type: zpre.StepTypeXMLToJSON},
				{Type: zpre.StepTypeJavaScript, Params: script},
			},
			InstanceTemplate: "{#FSNAME}",
			MaxMissing:       0,
		},
		Pipelines: []pkgzabbix.PipelineConfig{
			{
				Name:      "path",
				Context:   "zabbix.xml.path",
				Dimension: "value",
				Unit:      "characters",
				Steps: []zpre.Step{
					{Type: zpre.StepTypeXMLToJSON},
					{Type: zpre.StepTypeJavaScript, Params: script},
					{Type: zpre.StepTypeJavaScript, Params: pathLengthScript},
				},
			},
		},
	}
}

func snmpLLDJobConfig(name string) pkgzabbix.JobConfig {
	params := "MACRO1\n.1.3.6.1\n0\nMACRO2\n.1.3.6.2\n0"
	return pkgzabbix.JobConfig{
		Name:       name,
		Collection: pkgzabbix.CollectionConfig{Type: pkgzabbix.CollectionCommand, Command: "/usr/bin/true"},
		LLD: pkgzabbix.LLDConfig{
			Steps:            []zpre.Step{{Type: zpre.StepTypeSNMPWalkToJSON, Params: params}},
			InstanceTemplate: "MACRO1",
			MaxMissing:       0,
		},
		Pipelines: []pkgzabbix.PipelineConfig{
			{
				Name:      "snmp_value",
				Context:   "zabbix.snmp.value",
				Dimension: "value",
				Unit:      "items",
				Steps: []zpre.Step{
					{Type: zpre.StepTypeSNMPWalkToJSON, Params: params},
					{Type: zpre.StepTypeJavaScript, Params: snmpValueScript},
				},
			},
		},
	}
}

func prometheusJobConfig(name string) pkgzabbix.JobConfig {
	return pkgzabbix.JobConfig{
		Name:       name,
		Collection: pkgzabbix.CollectionConfig{Type: pkgzabbix.CollectionCommand, Command: "/usr/bin/true"},
		LLD: pkgzabbix.LLDConfig{
			Steps: []zpre.Step{
				{Type: zpre.StepTypePrometheusToJSON},
				{Type: zpre.StepTypeJavaScript, Params: promDiscoveryScript},
			},
			InstanceTemplate: "fs_{#MOUNT}",
			MaxMissing:       0,
		},
		Pipelines: []pkgzabbix.PipelineConfig{
			{
				Name:      "free",
				Context:   "zabbix.prom.free",
				Dimension: "free",
				Unit:      "bytes",
				Steps: []zpre.Step{
					{Type: zpre.StepTypePrometheusToJSON},
					{Type: zpre.StepTypeJavaScript, Params: promValueScript},
					{Type: zpre.StepTypeValidateRange, Params: "0\n1000"},
				},
			},
		},
	}
}

func xpathJobConfig(name string) pkgzabbix.JobConfig {
	return pkgzabbix.JobConfig{
		Name:       name,
		Collection: pkgzabbix.CollectionConfig{Type: pkgzabbix.CollectionCommand, Command: "/usr/bin/true"},
		Pipelines: []pkgzabbix.PipelineConfig{
			{
				Name:      "cpu",
				Context:   "zabbix.xpath.cpu",
				Dimension: "cpu",
				Unit:      "celsius",
				Steps: []zpre.Step{
					{Type: zpre.StepTypeXPath, Params: "string(/sensors/sensor[@id='cpu']/text())"},
					{Type: zpre.StepTypeValidateRange, Params: "0\n150"},
				},
			},
			{
				Name:      "gpu",
				Context:   "zabbix.xpath.gpu",
				Dimension: "gpu",
				Unit:      "celsius",
				Steps: []zpre.Step{
					{Type: zpre.StepTypeXPath, Params: "string(/sensors/sensor[@id='gpu']/text())"},
					{Type: zpre.StepTypeValidateRange, Params: "0\n150"},
				},
			},
		},
	}
}

func csvMultiJobConfig(name string) pkgzabbix.JobConfig {
	const csvParams = `,
"
1`
	return pkgzabbix.JobConfig{
		Name:       name,
		Collection: pkgzabbix.CollectionConfig{Type: pkgzabbix.CollectionCommand, Command: "/usr/bin/true"},
		Pipelines: []pkgzabbix.PipelineConfig{
			{
				Name:      "first_value",
				Context:   "zabbix.csv.multi",
				Dimension: "value",
				Unit:      "items",
				Steps: []zpre.Step{
					{Type: zpre.StepTypeCSVToJSONMulti, Params: csvParams},
					{Type: zpre.StepTypeJavaScript, Params: csvMultiValueScript},
					{Type: zpre.StepTypeValidateRange, Params: "0\n1000"},
				},
			},
		},
	}
}
