// SPDX-License-Identifier: GPL-3.0-or-later

package corpus

import (
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/netdata/netdata/tests/query-corpus/fixture"
	"github.com/netdata/netdata/tests/query-corpus/stream"
)

func weightsLimitParams(method, options string) url.Values {
	p := weightsV1Params(method, wContext, options, true)
	p.Set("scope_contexts", wContext)
	return p
}

func weightsLimitRows(t *testing.T, doc map[string]any) map[string][]any {
	t.Helper()
	dicts, err := decodeWeightsMultiNodeDictionaries(doc)
	if err != nil {
		t.Fatal(err)
	}
	rows := map[string][]any{}
	for _, entry := range doc["result"].([]any) {
		r := entry.([]any)
		if r[0].(float64) != 0 {
			continue
		}
		id, ok := dicts.dimensions[int64(r[4].(float64))]
		if !ok {
			t.Fatalf("dimension index has no dictionary entry: %v", r)
		}
		if _, exists := rows[id]; exists {
			t.Fatalf("duplicate dimension %q", id)
		}
		rows[id] = r
	}
	return rows
}

func weightsAssertLimit(t *testing.T, doc map[string]any, limit, total, returned int, unit string) {
	t.Helper()
	want := map[string]any{"limit": float64(limit), "total": float64(total), "returned": float64(returned),
		"unit": unit, "truncated": total > returned, "summary_scope": "all"}
	if !reflect.DeepEqual(doc["result_limit"], want) {
		t.Errorf("result_limit=%v, want %v", doc["result_limit"], want)
	}
}

func TestWeightsLimitAliases(t *testing.T) {
	weightsSettle(t, "weights-h", guid(160), weightsFixture())
	for name, aliases := range map[string]url.Values{
		"limit": {"limit": {"2"}}, "cardinality": {"cardinality_limit": {"2"}},
		"precedence":           {"limit": {"1"}, "cardinality_limit": {"2"}},
		"repeated":             {"limit": {"1", "2"}},
		"repeated-cardinality": {"cardinality_limit": {"1", "2"}},
		"empty-cardinality":    {"limit": {"2"}, "cardinality_limit": {""}},
	} {
		t.Run(name, func(t *testing.T) {
			trackContract(t, "W/limit-aliases")
			p := weightsLimitParams("value", "raw")
			for k, v := range aliases {
				p[k] = v
			}
			doc, err := td.HostJSON("weights-h", "api/v3/weights", p)
			if err != nil {
				t.Fatal(err)
			}
			rows := weightsLimitRows(t, doc)
			if len(rows) != 2 || rows["split"] == nil || rows["flat"] == nil {
				t.Errorf("retained %v; want split and flat", rows)
			}
			weightsAssertLimit(t, doc, 2, 4, 2, "dimensions")
		})
	}
}

func TestWeightsLimitBoundaries(t *testing.T) {
	weightsSettle(t, "weights-h", guid(160), weightsFixture())
	for name, extra := range map[string]url.Values{
		"absent": {}, "zero": {"limit": {"0"}}, "empty": {"limit": {""}},
		"zero-precedence": {"limit": {"1"}, "cardinality_limit": {"0"}},
		"exact":           {"limit": {"4"}}, "above": {"limit": {"5"}},
	} {
		t.Run(name, func(t *testing.T) {
			trackContract(t, "W/limit-boundaries")
			p := weightsLimitParams("value", "raw")
			for k, v := range extra {
				p[k] = v
			}
			doc, err := td.HostJSON("weights-h", "api/v2/weights", p)
			if err != nil {
				t.Fatal(err)
			}
			if len(weightsLimitRows(t, doc)) != 4 {
				t.Error("unlimited/exact request lost dimensions")
			}
			if name == "exact" || name == "above" {
				n, _ := strconv.Atoi(p.Get("limit"))
				weightsAssertLimit(t, doc, n, 4, 4, "dimensions")
			} else if _, ok := doc["result_limit"]; ok {
				t.Error("unlimited request changed response shape")
			}
		})
	}
	t.Run("size-max", func(t *testing.T) {
		trackContract(t, "W/limit-boundaries")
		p := weightsLimitParams("value", "raw")
		p.Set("limit", strconv.FormatUint(uint64(^uint(0)), 10))
		doc, err := td.HostJSON("weights-h", "api/v3/weights", p)
		if err != nil {
			t.Fatal(err)
		}
		if len(weightsLimitRows(t, doc)) != 4 {
			t.Error("large cap lost dimensions")
		}
		meta, ok := doc["result_limit"].(map[string]any)
		if !ok {
			t.Fatal("positive cap omitted result_limit metadata")
		}
		if meta["truncated"] != false || meta["returned"] != float64(4) {
			t.Errorf("large cap metadata %v", meta)
		}
	})
	for _, group := range []string{"", "dimension"} {
		t.Run("empty-"+group, func(t *testing.T) {
			trackContract(t, "W/limit-boundaries")
			p := weightsLimitParams("value", "raw")
			p.Set("scope_contexts", "fixture.no-such-weights-context")
			p.Set("limit", "1")
			p.Set("group_by", group)
			doc, err := td.HostJSON("weights-h", "api/v3/weights", p)
			if err != nil {
				t.Fatal(err)
			}
			unit := "dimensions"
			if group != "" {
				unit = "groups"
			}
			weightsAssertLimit(t, doc, 1, 0, 0, unit)
		})
	}
}

func TestWeightsLimitInvalid(t *testing.T) {
	weightsSettle(t, "weights-h", guid(160), weightsFixture())
	for _, endpoint := range []string{"api/v1/weights", "api/v1/metric_correlations", "api/v2/weights", "api/v3/weights"} {
		for _, alias := range []string{"limit", "cardinality_limit"} {
			for _, value := range []string{"-1", "1.5", "1junk", "junk", "18446744073709551616", " 2", "+2"} {
				t.Run(endpoint+"/"+alias+"/"+value, func(t *testing.T) {
					trackContract(t, "W/limit-invalid")
					p := weightsLimitParams("value", "raw")
					p.Set(alias, value)
					_, err := td.HostJSON("weights-h", endpoint, p)
					if err == nil || !strings.Contains(err.Error(), "HTTP 400") {
						t.Errorf("want HTTP 400, got %v", err)
					}
				})
			}
		}
	}
	for _, aliases := range []url.Values{
		{"limit": {"bad", "1"}}, {"cardinality_limit": {"bad", "1"}},
		{"limit": {"bad"}, "cardinality_limit": {"1"}},
		{"limit": {"1"}, "cardinality_limit": {"bad"}},
	} {
		t.Run(aliases.Encode(), func(t *testing.T) {
			trackContract(t, "W/limit-invalid")
			p := weightsLimitParams("value", "raw")
			for k, v := range aliases {
				p[k] = v
			}
			_, err := td.HostJSON("weights-h", "api/v3/weights", p)
			if err == nil || !strings.Contains(err.Error(), "HTTP 400") {
				t.Errorf("invalid supplied alias accepted: %v", err)
			}
		})
	}
}

func TestWeightsLimitScoresAndSummaries(t *testing.T) {
	weightsSettle(t, "weights-h", guid(160), weightsFixture())
	for _, method := range []string{"value", "volume", "ks2", "anomaly-rate"} {
		for _, options := range []string{"raw", "null2zero", "raw|anomaly-bit", "null2zero|anomaly-bit"} {
			t.Run(method+"/"+options, func(t *testing.T) {
				component := method + "/" + options
				registerContractComponent(t, "W/limit-ranking", component)
				registerContractComponent(t, "W/limit-summaries", component)
				p := weightsLimitParams(method, options)
				full, err := td.HostJSON("weights-h", "api/v3/weights", p)
				if err != nil {
					t.Fatal(err)
				}
				p.Set("limit", "1")
				limited, err := td.HostJSON("weights-h", "api/v3/weights", p)
				if err != nil {
					t.Fatal(err)
				}
				all, kept := weightsLimitRows(t, full), weightsLimitRows(t, limited)
				t.Run("ranking", func(t *testing.T) {
					trackContractComponent(t, "W/limit-ranking", component)
					ids := make([]string, 0, len(all))
					for id := range all {
						ids = append(ids, id)
					}
					desc := strings.Contains(options, "raw") || method == "value"
					sort.Slice(ids, func(i, j int) bool {
						a, b := all[ids[i]][5].(float64), all[ids[j]][5].(float64)
						if a == b {
							return ids[i] < ids[j]
						}
						if desc {
							return a > b
						}
						return a < b
					})
					want := 0
					if len(ids) > 0 {
						want = 1
					}
					if len(kept) != want {
						t.Fatalf("kept %d, want %d", len(kept), want)
					}
					if want > 0 {
						// Class C: the JSON formatter rounds scores, so identical printed
						// values do not imply an internal tie. Exact ties have their own fixture.
						for id, r := range kept {
							if r[5] != all[ids[0]][5] || !reflect.DeepEqual(r[5:], all[id][5:]) {
								t.Errorf("strongest score/vector not preserved: %v", kept)
							}
						}
					}
					weightsAssertLimit(t, limited, 1, len(all), want, "dimensions")
				})
				t.Run("summaries", func(t *testing.T) {
					trackContractComponent(t, "W/limit-summaries", component)
					// This fixture has one chart; TestWeightsLimitHierarchy checks
					// sibling parents by semantic identity across dictionary compaction.
					parents := map[float64][]any{}
					for _, entry := range full["result"].([]any) {
						r := entry.([]any)
						if r[0].(float64) != 0 {
							parents[r[0].(float64)] = r[5:]
						}
					}
					for _, entry := range limited["result"].([]any) {
						r := entry.([]any)
						if r[0].(float64) != 0 && !reflect.DeepEqual(r[5:], parents[r[0].(float64)]) {
							t.Errorf("parent value changed: %v", r)
						}
					}
					for _, key := range []string{"correlated_dimensions", "total_dimensions_count"} {
						if !reflect.DeepEqual(limited[key], full[key]) {
							t.Errorf("%s changed", key)
						}
					}
				})
			})
		}
	}
}

func TestWeightsLimitLegacy(t *testing.T) {
	weightsSettle(t, "weights-h", guid(160), weightsFixture())
	for _, endpoint := range []string{"api/v1/weights", "api/v1/metric_correlations"} {
		for _, alias := range []string{"limit", "cardinality_limit"} {
			t.Run(endpoint+"/"+alias, func(t *testing.T) {
				trackContract(t, "W/limit-legacy")
				p := weightsLimitParams("value", "raw")
				p.Set(alias, "1")
				doc, err := td.HostJSON("weights-h", endpoint, p)
				if err != nil {
					t.Fatal(err)
				}
				var got map[string]float64
				if endpoint == "api/v1/weights" {
					got = v1ContextsWeights(t, doc, wContext)
				} else {
					got = map[string]float64{}
					for _, entry := range doc["correlated_charts"].(map[string]any) {
						for k, v := range entry.(map[string]any)["dimensions"].(map[string]any) {
							got[k] = v.(float64)
						}
					}
				}
				// buffer.h:print_netdata_double rounds to seven fractional digits.
				if len(got) != 1 || math.Abs(got["split"]-weightsHighlightAverages()["split"]) > 5e-8 {
					t.Errorf("got %v, want strongest split", got)
				}
				weightsAssertLimit(t, doc, 1, 4, 1, "dimensions")
				if doc["correlated_dimensions"] != float64(4) {
					t.Error("pre-limit correlated population lost")
				}
			})
		}
	}
}

func TestWeightsLimitGrouped(t *testing.T) {
	weightsSettle(t, "weights-h", guid(160), weightsFixture())
	for _, aggregation := range []string{"min", "average", "max", "sum", "percentage", "extremes"} {
		t.Run(aggregation, func(t *testing.T) {
			trackContract(t, "W/limit-grouped")
			p := weightsLimitParams("value", "raw")
			p.Set("group_by", "dimension")
			p.Set("aggregation", aggregation)
			full, err := td.HostJSON("weights-h", "api/v3/weights", p)
			if err != nil {
				t.Fatal(err)
			}
			p.Set("limit", "1")
			doc, err := td.HostJSON("weights-h", "api/v3/weights", p)
			if err != nil {
				t.Fatal(err)
			}
			rows := doc["result"].([]any)
			if len(rows) != 1 {
				t.Fatalf("got %d groups, want 1", len(rows))
			}
			r := rows[0].(map[string]any)
			if r["id"] != "split" {
				t.Errorf("wrong group: %v", r)
			}
			for _, entry := range full["result"].([]any) {
				old := entry.(map[string]any)
				if old["id"] == r["id"] && !reflect.DeepEqual(old, r) {
					t.Errorf("group vector changed: %v", r)
				}
			}
			weightsAssertLimit(t, doc, 1, 4, 1, "groups")
		})
	}
}

func weightsLimitChart(id, context string, values []int, changing bool) fixture.Chart {
	ch := fixture.Chart{ID: id, Context: context, Title: "weights limit", Units: "units", Family: "fixture", UpdateEvery: 1}
	for d, value := range values {
		dim := fixture.Dimension{ID: fmt.Sprintf("d%04d", d)}
		for i := 1; i <= wRows; i++ {
			v := value
			if changing {
				v = 10
				if i > wSplit {
					v = 10 * (value + 1)
				}
			}
			dim.Points = append(dim.Points, fixture.Point{T: fixture.T0 + int64(i), Collected: strconv.Itoa(v), Flags: stream.FlagNotAnomalous})
		}
		ch.Dimensions = append(ch.Dimensions, dim)
	}
	return ch
}

func weightsLimitFixtureParams(method, options string) url.Values {
	p := weightsLimitParams(method, options)
	p.Set("context", "fixture.limit*")
	p.Set("scope_contexts", "fixture.limit*")
	return p
}

func weightsLimitExport(t *testing.T, name, endpoint string, params url.Values, doc map[string]any) {
	t.Helper()
	dir := os.Getenv("QUERY_CORPUS_WEIGHTS_EXPORT")
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	data, err := json.MarshalIndent(map[string]any{"endpoint": endpoint, "query": params.Encode(), "response": doc}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".json"), data, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestWeightsLimitCompleteGroups(t *testing.T) {
	charts := []fixture.Chart{
		weightsLimitChart("fixture.limita", "fixture.limit", []int{1, 100}, true),
		weightsLimitChart("fixture.limitb", "fixture.limit", []int{60, 60}, true),
		weightsLimitChart("fixture.limitc", "fixture.limit-other", []int{40, 40, 40, 40}, true),
	}
	c020PushCharts(t, "weights-limit-groups", guid(450), charts...)
	for _, ch := range charts {
		weightsSettle(t, "weights-limit-groups", guid(450), ch)
	}
	for _, options := range []string{"raw", "null2zero"} {
		for aggregation, rawWinner := range map[string]string{"min": "b", "average": "b", "max": "a", "sum": "c", "percentage": "c", "extremes": "a"} {
			t.Run(options+"/"+aggregation, func(t *testing.T) {
				trackContract(t, "W/limit-complete-groups")
				p := weightsLimitFixtureParams("volume", options)
				p.Set("group_by", "instance")
				p.Set("aggregation", aggregation)
				full, err := td.HostJSON("weights-limit-groups", "api/v3/weights", p)
				if err != nil {
					t.Fatal(err)
				}
				weightsLimitExport(t, "group-"+options+"-"+aggregation+"-full", "api/v3/weights", p, full)
				p.Set("limit", "1")
				limited, err := td.HostJSON("weights-limit-groups", "api/v3/weights", p)
				if err != nil {
					t.Fatal(err)
				}
				weightsLimitExport(t, "group-"+options+"-"+aggregation+"-limited", "api/v3/weights", p, limited)
				rows := limited["result"].([]any)
				if len(rows) != 1 {
					t.Fatalf("got %d groups", len(rows))
				}
				winner := rawWinner
				if options != "raw" {
					winner = "b"
					if aggregation == "min" {
						winner = "a"
					}
				}
				row := rows[0].(map[string]any)
				if !strings.HasPrefix(row["id"].(string), "fixture.limit"+winner+"@") {
					t.Errorf("winner=%v, want %s", row["id"], winner)
				}
				for _, entry := range full["result"].([]any) {
					old := entry.(map[string]any)
					if old["id"] == row["id"] && !reflect.DeepEqual(old, row) {
						t.Error("complete group vector changed")
					}
				}
				weightsAssertLimit(t, limited, 1, 3, 1, "groups")
			})
		}
	}
	if os.Getenv("QUERY_CORPUS_WEIGHTS_EXPORT") == "" {
		return
	}
	for _, method := range []string{"volume", "value"} {
		for _, options := range []string{"raw", "null2zero"} {
			for _, endpoint := range []string{"api/v3/weights", "api/v1/weights"} {
				p := weightsLimitFixtureParams(method, options)
				for _, suffix := range []string{"full", "limited"} {
					if suffix == "limited" {
						p.Set("limit", "1")
					}
					doc, err := td.HostJSON("weights-limit-groups", endpoint, p)
					if err != nil {
						t.Fatal(err)
					}
					weightsLimitExport(t, strings.ReplaceAll(endpoint, "/", "-")+"-"+method+"-"+options+"-"+suffix, endpoint, p, doc)
				}
			}
		}
	}
}

func TestWeightsLimitThousand(t *testing.T) {
	trackContract(t, "W/limit-thousand")
	values := make([]int, 1005)
	for i := range values {
		values[i] = i + 1
	}
	ch := weightsLimitChart("fixture.limit-thousand", "fixture.limit-thousand", values, false)
	weightsSettle(t, "weights-limit-thousand", guid(451), ch)
	p := weightsLimitFixtureParams("value", "raw")
	p.Set("scope_contexts", ch.Context)
	p.Set("limit", "1000")
	doc, err := td.HostJSON("weights-limit-thousand", "api/v3/weights", p)
	if err != nil {
		t.Fatal(err)
	}
	rows := weightsLimitRows(t, doc)
	if len(rows) != 1000 {
		t.Fatalf("got %d dimensions", len(rows))
	}
	for i := 0; i < 1005; i++ {
		_, ok := rows[fmt.Sprintf("d%04d", i)]
		if ok != (i >= 5) {
			t.Errorf("incorrect membership for dimension %d", i)
		}
	}
	weightsAssertLimit(t, doc, 1000, 1005, 1000, "dimensions")
	for _, entry := range doc["result"].([]any) {
		row := entry.([]any)
		if row[0].(float64) != 0 && row[5].(float64) != 503 {
			t.Errorf("complete parent mean changed: %v", row[5])
		}
	}
	weightsLimitExport(t, "thousand-limited", "api/v3/weights", p, doc)
}

// Class C parity of complete-query ancestor vectors is paired with Class A
// fixture membership, dictionary reachability, and full population assertions.
func TestWeightsLimitHierarchy(t *testing.T) {
	trackContract(t, "W/limit-hierarchy")
	charts := []fixture.Chart{
		weightsLimitChart("fixture.limith-a", "fixture.limith", []int{1, 100}, false),
		weightsLimitChart("fixture.limith-b", "fixture.limith", []int{2, 90}, false),
		weightsLimitChart("fixture.limith-c", "fixture.limith-other", []int{3, 80}, false),
	}
	c020PushCharts(t, "weights-limit-hierarchy", guid(452), charts...)
	for _, ch := range charts {
		weightsSettle(t, "weights-limit-hierarchy", guid(452), ch)
	}
	p := weightsLimitFixtureParams("value", "raw")
	p.Set("scope_nodes", guid(452))
	full, err := td.HostJSON("weights-limit-hierarchy", "api/v3/weights", p)
	if err != nil {
		t.Fatal(err)
	}
	decode := func(doc map[string]any) map[string][]any {
		d, err := decodeWeightsMultiNodeDictionaries(doc)
		if err != nil {
			t.Fatal(err)
		}
		rows := map[string][]any{}
		for _, item := range doc["result"].([]any) {
			r := item.([]any)
			kind := int(r[0].(float64))
			key := fmt.Sprint(kind)
			if kind <= 2 {
				key += "/" + d.contexts[int64(r[2].(float64))]["id"].(string)
			}
			if kind <= 1 {
				key += "/" + d.instances[int64(r[3].(float64))]["id"].(string)
			}
			if kind == 0 {
				key += "/" + d.dimensions[int64(r[4].(float64))]
			}
			if rows[key] != nil {
				t.Fatalf("duplicate physical row %s", key)
			}
			rows[key] = r[5:]
		}
		return rows
	}
	all := decode(full)
	for _, limit := range []int{1, 2, 3} {
		p.Set("limit", strconv.Itoa(limit))
		doc, err := td.HostJSON("weights-limit-hierarchy", "api/v3/weights", p)
		if err != nil {
			t.Fatal(err)
		}
		rows := decode(doc)
		for key, vector := range rows {
			if !reflect.DeepEqual(vector, all[key]) {
				t.Errorf("complete-query vector changed for %s", key)
			}
		}
		dicts := doc["dictionaries"].(map[string]any)
		wantContexts := 1
		if limit == 3 {
			wantContexts = 2
		}
		if len(dicts["dimensions"].([]any)) != 1 || len(dicts["instances"].([]any)) != limit || len(dicts["contexts"].([]any)) != wantContexts {
			t.Errorf("unused metric dictionaries retained: %v", dicts)
		}
		if len(rows) != limit*2+wantContexts+1 {
			t.Errorf("orphan/missing parent rows: %v", rows)
		}
		weightsAssertLimit(t, doc, limit, 6, limit, "dimensions")
		if doc["correlated_dimensions"] != float64(6) {
			t.Error("full population changed")
		}
	}
}

func TestWeightsLimitNodeTies(t *testing.T) {
	trackContract(t, "W/limit-node-ties")
	ch := weightsLimitChart("fixture.limittie", "fixture.limittie", []int{20, 20}, false)
	// Reverse creation order and repeat the query: pointer/order ties must not win.
	for _, n := range []int{454, 453} {
		weightsSettle(t, fmt.Sprintf("weights-limit-tie-%d", n), guid(n), ch)
	}
	p := weightsLimitFixtureParams("value", "raw")
	p.Set("scope_nodes", guid(453)+"|"+guid(454))
	p.Set("scope_contexts", ch.Context)
	p.Set("limit", "1")
	for i := 0; i < 5; i++ {
		doc, err := td.HostJSON("weights-limit-tie-453", "api/v3/weights", p)
		if err != nil {
			t.Fatal(err)
		}
		d, err := decodeWeightsMultiNodeDictionaries(doc)
		if err != nil {
			t.Fatal(err)
		}
		if len(d.nodes) != 2 {
			t.Fatalf("node metadata lost: %v", d.nodes)
		}
		rows := weightsLimitRows(t, doc)
		if len(rows) != 1 || rows["d0000"] == nil {
			t.Fatalf("dimension tie selected %v", rows)
		}
		node := d.nodes[int64(rows["d0000"][1].(float64))]
		if node["mg"] != guid(453) {
			t.Errorf("node tie selected %v", node)
		}
		if len(doc["result"].([]any)) != 4 {
			t.Error("unrelated node result row retained")
		}
		weightsAssertLimit(t, doc, 1, 4, 1, "dimensions")
	}
}

func TestWeightsLimitMCP(t *testing.T) {
	values := make([]int, 55)
	for i := range values {
		values[i] = i + 1
	}
	ch := weightsLimitChart("fixture.limitmcp", "fixture.limitmcp", values, true)
	for d := range ch.Dimensions {
		for i := wSplit; i < wSplit+d+1; i++ {
			ch.Dimensions[d].Points[i].Flags = stream.FlagAnomalous
		}
	}
	weightsSettle(t, "weights-limit-mcp", guid(455), ch)
	init := c023MCPPost(t, 1, "initialize", map[string]any{
		"protocolVersion": "2025-03-26", "capabilities": map[string]any{},
		"clientInfo": map[string]any{"name": "query-corpus", "version": "1"},
	}, "")
	c023MCPNotify(t, "notifications/initialized", map[string]any{}, init.Session)
	for _, cap := range []int{-1, 1, 30, 55, 60} {
		t.Run(strconv.Itoa(cap), func(t *testing.T) {
			trackContract(t, "W/limit-mcp")
			args := map[string]any{
				"nodes": []string{guid(455)}, "metrics": []string{ch.Context},
				"after": fixture.T0 + wSplit, "before": fixture.T0 + wRows,
				"baseline_after": fixture.T0, "baseline_before": fixture.T0 + wSplit,
			}
			if cap >= 0 {
				args["cardinality_limit"] = cap
			}
			response := c023MCPPost(t, cap+100, "tools/call", map[string]any{
				"name": "find_anomalous_metrics", "arguments": args,
			}, init.Session)
			doc := c023MCPQueryDocument(t, response.Document)
			rows := doc["results"].([]any)
			want := cap
			if cap == -1 {
				want = 50
			} else if cap < 30 {
				want = 30
			} else if cap > 55 {
				want = 55
			}
			if len(rows) != want {
				t.Fatalf("MCP returned %d, want %d", len(rows), want)
			}
			for i, entry := range rows {
				row := entry.([]any)
				if row[9] != fmt.Sprintf("d%04d", 54-i) {
					t.Errorf("MCP order %v", row)
				}
				// First-principles anomaly count, including the normal left-edge sample.
				if math.Abs(row[0].(float64)-float64(55-i)*100/121) > 5e-8 {
					t.Errorf("MCP anomaly percentage was altered: %v", row[0])
				}
			}
			meta := doc["metadata"].(map[string]any)
			if meta["total_time_series_analyzed"] != float64(55) || meta["total_time_series_returned"] != float64(want) {
				t.Errorf("MCP counts %v", meta)
			}
			truncated, _ := meta["truncated"].(bool)
			if truncated != (want < 55) {
				t.Errorf("MCP truncation %v", meta)
			}
		})
	}
}
