// SPDX-License-Identifier: GPL-3.0-or-later

package corpus

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/netdata/netdata/tests/query-corpus/canon"
	"github.com/netdata/netdata/tests/query-corpus/daemon"
	"github.com/netdata/netdata/tests/query-corpus/fixture"
	"github.com/netdata/netdata/tests/query-corpus/stream"
)

// All values are positive powers of two, isolating interpolation from signed
// extrema semantics and storage quantization. Guard records allow rollup writes.
func c041Chart(context string, base int64, gran int, gapped bool) fixture.Chart {
	return fixture.Series(context, context, base, 8*gran, 1, func(i int) string {
		record, offset := (i-1)/gran, (i-1)%gran
		value := 64
		if offset == gran/2 {
			switch record {
			case 2, 4:
				value = 4
			case 3:
				value = 1024
			}
		}
		return strconv.Itoa(value)
	}, func(i int) string {
		if gapped && (i-1)/gran == 5 {
			return stream.FlagEmpty
		}
		return stream.FlagNotAnomalous
	})
}

// Selecting an extremum is fixture arithmetic; it needs no executor model.
func c041Extremum(ch fixture.Chart, after, before int64, group string) (float64, bool) {
	var result float64
	found := false
	for _, point := range ch.Dimensions[0].Points {
		if point.T <= after || point.T > before {
			continue
		}
		value, numeric := point.CollectedValue("load")
		if !numeric {
			continue
		}
		if !found || group == "min" && value < result || group == "max" && value > result {
			result = value
		}
		found = true
	}
	return result, found
}

func c041Read(t *testing.T, host, context, group string, tier int, after, before, points int64) []canon.Pt {
	t.Helper()
	p := daemon.DataParamsTier(context, tier, after, before, points, group)
	p.Set("options", "jsonwrap|unaligned|virtual-points")
	doc, err := td.DataV3(host, p)
	if err != nil {
		t.Fatal(err)
	}
	cols, err := canon.Columns(doc)
	if err != nil {
		t.Fatal(err)
	}
	if !assertOnlyColumn(t, cols, "load") || !assertSelectedTier(t, doc, tier) ||
		!queryTimestampGridExact(t, doc, queryExpectedVirtualGrid(t, after, before, points, false)) {
		t.Fatal("extrema query changed its dimension, forced source tier, or requested grid")
	}
	return cols["load"]
}

func TestCase041TieredExtrema(t *testing.T) {
	for tier, gran := range []int{0, tier1Gran, tier2Gran} {
		if tier == 0 {
			continue
		}
		for variant, flavor := range []string{"complete", "gapped"} {
			prefix := fmt.Sprintf("tier%d/%s", tier, flavor)
			t.Run(prefix, func(t *testing.T) {
				for _, group := range []string{"min", "max"} {
					for _, source := range []int{0, tier} {
						for _, shape := range []string{"aligned", "coarser"} {
							registerContractComponent(t, "CASE-041/"+group+"-source-values", fmt.Sprintf("%s/tier%d/%s", prefix, source, shape))
						}
					}
					for _, shape := range []string{"fine", "shifted"} {
						for _, invariant := range []string{"no-interpolation", "preserves-extremum"} {
							registerContractComponent(t, "CASE-041/"+group+"-"+invariant, prefix+"/"+shape)
						}
					}
				}

				base := fixture.T0 - fixture.T0%int64(gran)
				host := fmt.Sprintf("c041-tier%d-%s", tier, flavor)
				context := fmt.Sprintf("fixture.c041_tier%d_%s", tier, flavor)
				ch := c041Chart(context, base, gran, variant == 1)
				pushLiveBurst(t, host, guid(9950+2*(tier-1)+variant), ch)
				if _, err := td.WaitRetention(host, context, ch.FirstT(), ch.LastT(), 30*time.Second); err != nil {
					t.Fatal(err)
				}
				// Exact controls end at 6G; later guard retention is not part of
				// the contract. Both sharp witnesses precede the final gap record.
				waitDedicatedTierLastEntry(t, td, host, context, tier, base+6*int64(gran))

				for _, group := range []string{"min", "max"} {
					for _, source := range []int{0, tier} {
						for _, shape := range []string{"aligned", "coarser"} {
							component := fmt.Sprintf("%s/tier%d/%s", prefix, source, shape)
							t.Run(group+fmt.Sprintf("/tier%d/", source)+shape, func(t *testing.T) {
								trackContractComponent(t, "CASE-041/"+group+"-source-values", component)
								step := int64(gran)
								if shape == "coarser" {
									step *= 2
								}
								before := base + 6*int64(gran)
								rows := c041Read(t, host, context, group, source, base, before, (before-base)/step)
								var want []expectedColumnPoint
								for end := base + step; end <= before; end += step {
									value, numeric := c041Extremum(ch, end-step, end, group)
									if numeric {
										want = append(want, wantNumberAt(end, value))
									} else {
										want = append(want, wantEmptyAt(end))
									}
								}
								if !assertExactColumnValues(t, map[string][]canon.Pt{"load": rows}, "load", want, 0) {
									t.Error("aligned extrema differ from the literal source samples")
								}
							})
						}
					}

					allowed := map[float64]bool{}
					for end := base + int64(gran); end <= ch.LastT(); end += int64(gran) {
						if value, numeric := c041Extremum(ch, end-int64(gran), end, group); numeric {
							allowed[value] = true
						}
					}
					if len(allowed) != 2 {
						t.Fatal("fixture must contain two distinct retained extrema")
					}
					for _, shape := range []string{"fine", "shifted"} {
						t.Run(group+"/"+shape, func(t *testing.T) {
							after, before := base+2*int64(gran), base+6*int64(gran)
							if shape == "shifted" {
								after += int64(gran) / 12
								before += int64(gran) / 12
							}
							rows := c041Read(t, host, context, group, tier, after, before, 12)
							component := prefix + "/" + shape
							t.Run("selection", func(t *testing.T) {
								trackContractComponent(t, "CASE-041/"+group+"-no-interpolation", component)
								numeric := 0
								for _, row := range rows {
									if row.Value != nil {
										numeric++
										if !allowed[*row.Value] {
											t.Errorf("%s at %d invented %v; retained extrema are %v", group, row.T, *row.Value, allowed)
										}
									}
								}
								if numeric == 0 {
									t.Error("populated extrema window returned no numeric values")
								}
							})
							t.Run("preservation", func(t *testing.T) {
								trackContractComponent(t, "CASE-041/"+group+"-preserves-extremum", component)
								want, numeric := c041Extremum(ch, after, before, group)
								if !numeric {
									t.Fatal("fixture window must contain a complete sharp record")
								}
								for _, row := range rows {
									if row.Value != nil && *row.Value == want {
										return
									}
								}
								t.Errorf("%s lost the fixture's overall retained extremum %v", group, want)
							})
						})
					}
				}
			})
		}
	}
}
