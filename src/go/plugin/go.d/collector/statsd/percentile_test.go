// SPDX-License-Identifier: GPL-3.0-or-later

package statsd

import (
	"fmt"
	"math"
	"math/big"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
)

type weightedSample struct{ value, rate float64 }

// Independent reference: exact rational reciprocal of each parsed binary64
// rate, exact cumulative weights, exact rational percentile fractions.
func referenceQuantile(input []weightedSample, num, den int64) float64 {
	input = slices.Clone(input)
	slices.SortFunc(input, func(a, b weightedSample) int {
		if a.value < b.value {
			return -1
		}
		if a.value > b.value {
			return 1
		}
		return 0
	})
	total := new(big.Rat)
	for _, s := range input {
		total.Add(total, new(big.Rat).Inv(new(big.Rat).SetFloat64(s.rate)))
	}
	target := new(big.Rat).Mul(total, big.NewRat(num, den))
	prefix := new(big.Rat)
	for _, s := range input {
		prefix.Add(prefix, new(big.Rat).Inv(new(big.Rat).SetFloat64(s.rate)))
		if prefix.Cmp(target) >= 0 {
			return s.value
		}
	}
	panic("no reference")
}

func checkPercentiles(t *testing.T, input []weightedSample, limit int) (available bool, reason string) {
	t.Helper()
	a := newPercentiles(limit)
	for _, s := range input {
		a.add(s.value, s.rate)
	}
	got, _ := a.query(make([]percentileBin, 0, 2*limit+1))
	if math.IsNaN(got[0]) || math.IsNaN(got[1]) {
		if !math.IsNaN(got[0]) || !math.IsNaN(got[1]) {
			t.Fatal("partial reliability")
		}
		if len(input) > 0 && a.reason == "" {
			t.Fatal("missing diagnostic")
		}
		return false, a.reason
	}
	for q, nd := range [][2]int64{{1, 2}, {19, 20}} {
		ref := referenceQuantile(input, nd[0], nd[1])
		diff := new(big.Rat).Sub(new(big.Rat).SetFloat64(got[q]), new(big.Rat).SetFloat64(ref))
		diff.Abs(diff)
		bound := new(big.Rat).Mul(new(big.Rat).Abs(new(big.Rat).SetFloat64(ref)), big.NewRat(1, 100))
		if diff.Cmp(bound) > 0 {
			t.Fatalf("q=%v got=%g ref=%g exact=%v input=%v", nd, got[q], ref, a.exact, input)
		}
	}
	return true, ""
}

func TestPercentileContracts(t *testing.T) {
	cases := map[string]struct {
		input []weightedSample
		want  bool
	}{
		"equal-pair":             {[]weightedSample{{10, 1}, {100, 1}}, true},
		"fractional-homogeneous": {[]weightedSample{{10, .3}, {100, .3}}, true},
		"binary-related-rates":   {[]weightedSample{{10, .1}, {100, .2}, {100, .4}}, true},
		"signed-zero":            {[]weightedSample{{-100, 1}, {-1, 1}, {0, 1}, {1, 1}, {100, 1}}, true},
		"zero-only":              {[]weightedSample{{0, 1}, {0, .1}}, true},
		"large-weight-unit":      {[]weightedSample{{1, 0x1p-53}, {100, 0x1p-53}, {100, 1}}, false},
		"mapping-tiny":           {[]weightedSample{{math.SmallestNonzeroFloat64, 1}}, false},
		"rate-domain":            {[]weightedSample{{1, 0x1p-129}}, false},
		"span":                   {[]weightedSample{{1e-20, 1}, {1e20, 1}}, false},
	}
	// Float64(1/.1)==10 would pick the wrong half in this example.
	deceptive := []weightedSample{{1, .1}}
	for range 10 {
		deceptive = append(deceptive, weightedSample{100, 1})
	}
	cases["rounded-reciprocal"] = struct {
		input []weightedSample
		want  bool
	}{deceptive, false}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, why := checkPercentiles(t, tc.input, 1024)
			assert.Equal(t, tc.want, got, why)
		})
	}
}

func TestPercentileCoverage(t *testing.T) {
	r := rand.New(rand.NewPCG(4, 9))
	for _, mode := range []string{"unsampled", "fixed-.1", "fixed-.3", "mixed", "signed-mixed"} {
		available := 0
		reasons := map[string]int{}
		for trial := 0; trial < 400; trial++ {
			input := make([]weightedSample, 2+r.IntN(200))
			for i := range input {
				rate := 1.0
				switch mode {
				case "fixed-.1":
					rate = .1
				case "fixed-.3":
					rate = .3
				case "mixed", "signed-mixed":
					rate = []float64{1, .5, .1, .03, .3, .01}[r.IntN(6)]
				}
				value := math.Exp(r.NormFloat64()*1.5 + 3)
				if mode == "signed-mixed" && r.IntN(2) == 0 {
					value = -value
				}
				if r.IntN(30) == 0 {
					value = 0
				}
				input[i] = weightedSample{value, rate}
			}
			ok, why := checkPercentiles(t, input, 1024)
			if ok {
				available++
			} else {
				reasons[why]++
			}
		}
		t.Logf("mode=%s available=%d/400 gaps=%v", mode, available, reasons)
		minimum := 390
		if mode == "unsampled" || mode == "fixed-.1" || mode == "fixed-.3" {
			minimum = 400
		}
		if available < minimum {
			t.Fatalf("useful coverage regressed: %d < %d", available, minimum)
		}
	}
}

func TestPercentileMappingBoundaries(t *testing.T) {
	a := newPercentiles(1024)
	available := 0
	for index := -39000; index <= 39000; index += 17 {
		v := a.mapping.LowerBound(index)
		for _, x := range []float64{math.Nextafter(v, 0), v, math.Nextafter(v, math.Inf(1)), -v} {
			if !finite(x) || x == 0 {
				continue
			}
			ok, _ := checkPercentiles(t, []weightedSample{{x, .3}}, 1024)
			if ok {
				available++
			}
		}
	}
	t.Logf("certified mapping-boundary cases=%d", available)
}

func TestSpanBeforeMutationAndReset(t *testing.T) {
	a := newPercentiles(128)
	a.add(1, 1)
	before := a.positive.TotalCount()
	a.add(1e20, 1)
	if a.reason != "span" || a.positive.TotalCount() != before {
		t.Fatal("collapsed/mutated failed span")
	}
	a.add(1, 1)
	if a.positive.TotalCount() != before {
		t.Fatal("mutated unreliable window")
	}
	a.reset()
	a.add(20, .3)
	a.add(10, .3)
	got, _ := a.query(nil)
	if a.reason != "" || math.Abs(got[0]-10) > .1 || math.Abs(got[1]-20) > .2 {
		t.Fatal(got, a.reason)
	}
}

func BenchmarkInsert(b *testing.B) {
	for _, mixed := range []bool{false, true} {
		b.Run(fmt.Sprint(mixed), func(b *testing.B) {
			a := newPercentiles(1024)
			a.add(1, 1)
			a.add(60000, 1)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if a.n > 1<<20 {
					a.reset()
				}
				rate := 1.0
				if mixed {
					rate = []float64{.3, .1, .01, 1}[i%4]
				}
				a.add(float64(i%10000+1), rate)
			}
		})
	}
}

func BenchmarkQuery(b *testing.B) {
	for _, mixed := range []bool{false, true} {
		b.Run(fmt.Sprint(mixed), func(b *testing.B) {
			a := newPercentiles(1024)
			for i := 0; i < 10000; i++ {
				rate := 1.0
				if mixed {
					rate = []float64{.3, .1, .01, 1}[i%4]
				}
				a.add(float64(i+1), rate)
			}
			scratch := make([]percentileBin, 0, 2049)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, scratch = a.query(scratch)
			}
		})
	}
}

func TestExactLaneTransitions(t *testing.T) {
	for _, order := range [][]float64{{.1, .2, .4}, {.4, .1, .2}, {.2, .4, .1}} {
		input := []weightedSample{}
		for j := 0; j < 20; j++ {
			input = append(input, weightedSample{float64(j + 1), order[j%3]})
		}
		if ok, why := checkPercentiles(t, input, 1024); !ok {
			t.Fatal(why)
		}
	}
	a := newPercentiles(1024)
	a.add(1, 1)
	a.add(100, 0x1p-52)
	if !a.exact {
		t.Fatal("expected exact <=2^53 units")
	}
	a.add(100, 0x1p-52)
	if a.exact {
		t.Fatal("expected transition beyond exact unit budget")
	}
	got, _ := a.query(nil)
	if math.IsNaN(got[0]) {
		t.Fatal("dominant mass remains certifiable", a.reason)
	}
	// Unit refinement must account for already accumulated mass.
	if ok, why := checkPercentiles(t, []weightedSample{{10, .1}, {100, .2}, {100, .4}, {100, .8}}, 1024); !ok {
		t.Fatal(why)
	}
}

func TestWideCoverage(t *testing.T) {
	for _, tc := range []struct {
		name   string
		lo, hi float64
	}{{"1us-60s-ms", .001, 60000}, {"1ns-1h-ms", .000001, 3600000}, {"signed-1us-60s", .001, 60000}} {
		for _, limit := range []int{512, 1024, 1536} {
			input := make([]weightedSample, 201)
			for i := range input {
				value := tc.lo * math.Pow(tc.hi/tc.lo, float64(i)/200)
				if tc.name[0] == 's' && i%2 == 0 {
					value = -value
				}
				input[i] = weightedSample{value, .1}
			}
			ok, why := checkPercentiles(t, input, limit)
			t.Logf("%s cap=%d available=%v reason=%s", tc.name, limit, ok, why)
		}
	}
}

func TestAdversarialWeights(t *testing.T) {
	r := rand.New(rand.NewPCG(91, 17))
	published := 0
	gaps := 0
	for trial := 0; trial < 1200; trial++ {
		input := make([]weightedSample, 2+r.IntN(18))
		for i := range input {
			exponent := r.IntN(129)
			rate := math.Ldexp(1, -exponent)
			if r.IntN(2) == 0 {
				rate *= 1 + r.Float64()
			}
			if rate > 1 {
				rate = 1
			}
			value := []float64{-100, -1, 0, 1, 100}[r.IntN(5)]
			input[i] = weightedSample{value, rate}
		}
		if ok, _ := checkPercentiles(t, input, 1024); ok {
			published++
		} else {
			gaps++
		}
		slices.Reverse(input)
		checkPercentiles(t, input, 1024)
	}
	t.Logf("adversarial forward windows published=%d gaps=%d; reverse permutations also checked", published, gaps)
}

// Allocation measurements run as benchmarks: race instrumentation changes escape
// analysis and is not a useful allocation envelope for the production build.
func BenchmarkPercentileReuse(b *testing.B) {
	a := newPercentiles(1024)
	for i := 0; i < 1024; i++ {
		a.add(a.mapping.Value(i), 1)
		a.add(-a.mapping.Value(i), 1)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		a.reset()
		for j := 0; j < 1024; j += 7 {
			a.add(a.mapping.Value(j+5000), .3)
			a.add(-a.mapping.Value(j+5000), .3)
		}
		if a.reason != "" {
			b.Fatal(a.reason)
		}
	}
}
