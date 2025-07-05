// SPDX-License-Identifier: GPL-3.0-or-later

package iprange

import (
	"fmt"
	"math/big"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPool(t *testing.T) {
	t.Parallel()

	r1 := mustParseRange(t, "192.0.2.0", "192.0.2.10")
	r2 := mustParseRange(t, "192.0.2.20", "192.0.2.30")

	tests := []struct {
		name      string
		ranges    []Range
		wantCount int
	}{
		{
			name:      "empty pool",
			ranges:    nil,
			wantCount: 0,
		},
		{
			name:      "single range",
			ranges:    []Range{r1},
			wantCount: 1,
		},
		{
			name:      "multiple ranges",
			ranges:    []Range{r1, r2},
			wantCount: 2,
		},
		{
			name:      "with nil ranges",
			ranges:    []Range{r1, nil, r2, nil},
			wantCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			pool := NewPool(tt.ranges...)
			assert.Equal(t, tt.wantCount, pool.Len())
		})
	}
}

func TestParsePool(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantLen int
		wantErr bool
	}{
		{
			name:    "single range",
			input:   "192.0.2.0-192.0.2.10",
			wantLen: 1,
		},
		{
			name:    "multiple ranges",
			input:   "192.0.2.0-192.0.2.10 2001:db8::-2001:db8::10",
			wantLen: 2,
		},
		{
			name:    "invalid range",
			input:   "192.0.2.0-192.0.2.10 invalid",
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   "",
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			pool, err := ParsePool(tt.input)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, pool)
			} else {
				assert.NoError(t, err)
				require.NotNil(t, pool)
				assert.Equal(t, tt.wantLen, pool.Len())
			}
		})
	}
}

func TestPool_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		input      string
		wantString string
	}{
		{
			name:       "single range",
			input:      "192.0.2.0-192.0.2.10",
			wantString: "192.0.2.0-192.0.2.10",
		},
		{
			name:       "multiple ranges",
			input:      "192.0.2.0-192.0.2.10 2001:db8::-2001:db8::10",
			wantString: "192.0.2.0-192.0.2.10 2001:db8::-2001:db8::10",
		},
		{
			name:       "empty pool",
			input:      "",
			wantString: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			pool, err := ParsePool(tt.input)
			require.NoError(t, err)

			assert.Equal(t, tt.wantString, pool.String())
		})
	}
}

func TestPool_NilSafety(t *testing.T) {
	t.Parallel()

	var pool *Pool

	// All methods should handle nil pool gracefully
	assert.Equal(t, 0, pool.Len())
	assert.True(t, pool.IsEmpty())
	assert.Equal(t, "", pool.String())
	assert.Equal(t, big.NewInt(0), pool.Size())
	assert.False(t, pool.Contains(netip.MustParseAddr("192.0.2.1")))
	assert.Nil(t, pool.Ranges())
	assert.Nil(t, pool.Clone())

	// Iterators should not panic
	for _ = range pool.Iterate() {
		t.Fatal("nil pool should not yield any addresses")
	}
	for _ = range pool.IterateRanges() {
		t.Fatal("nil pool should not yield any ranges")
	}
}

func TestPool_Size(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		wantSize int64
	}{
		{
			name:     "single range",
			input:    "192.0.2.0-192.0.2.10",
			wantSize: 11,
		},
		{
			name:     "multiple ranges",
			input:    "192.0.2.0-192.0.2.10 2001:db8::-2001:db8::10",
			wantSize: 11 + 17,
		},
		{
			name:     "empty pool",
			input:    "",
			wantSize: 0,
		},
		{
			name:     "overlapping ranges (counted separately)",
			input:    "192.0.2.0-192.0.2.10 192.0.2.5-192.0.2.15",
			wantSize: 11 + 11, // Overlaps are counted twice
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			pool, err := ParsePool(tt.input)
			require.NoError(t, err)

			assert.Equal(t, big.NewInt(tt.wantSize), pool.Size())
		})
	}
}

func TestPool_Contains(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		poolStr   string
		ip        string
		wantFound bool
	}{
		{
			name:      "IP in first range",
			poolStr:   "192.0.2.0-192.0.2.10 192.0.2.20-192.0.2.30 2001:db8::-2001:db8::10",
			ip:        "192.0.2.5",
			wantFound: true,
		},
		{
			name:      "IP in last range",
			poolStr:   "192.0.2.0-192.0.2.10 192.0.2.20-192.0.2.30 2001:db8::-2001:db8::10",
			ip:        "2001:db8::5",
			wantFound: true,
		},
		{
			name:      "IP not in any range",
			poolStr:   "192.0.2.0-192.0.2.10 192.0.2.20-192.0.2.30 2001:db8::-2001:db8::10",
			ip:        "192.0.2.100",
			wantFound: false,
		},
		{
			name:      "IP in gap between ranges",
			poolStr:   "192.0.2.0-192.0.2.10 192.0.2.20-192.0.2.30",
			ip:        "192.0.2.15",
			wantFound: false,
		},
		{
			name:      "empty pool",
			poolStr:   "",
			ip:        "192.0.2.1",
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			pool, err := ParsePool(tt.poolStr)
			require.NoError(t, err)

			ip, err := netip.ParseAddr(tt.ip)
			require.NoError(t, err)

			assert.Equal(t, tt.wantFound, pool.Contains(ip))
		})
	}
}

func TestPool_ContainsRange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		poolStr      string
		rangeStr     string
		wantContains bool
	}{
		{
			name:         "range fully within single pool range",
			poolStr:      "192.0.2.0-192.0.2.100",
			rangeStr:     "192.0.2.10-192.0.2.20",
			wantContains: true,
		},
		{
			name:         "range equals pool range",
			poolStr:      "192.0.2.0-192.0.2.100",
			rangeStr:     "192.0.2.0-192.0.2.100",
			wantContains: true,
		},
		{
			name:         "range extends beyond pool",
			poolStr:      "192.0.2.0-192.0.2.100",
			rangeStr:     "192.0.2.50-192.0.2.150",
			wantContains: false,
		},
		{
			name:         "range not in pool",
			poolStr:      "192.0.2.0-192.0.2.100",
			rangeStr:     "192.0.3.0-192.0.3.100",
			wantContains: false,
		},
		{
			name:         "empty pool",
			poolStr:      "",
			rangeStr:     "192.0.2.0-192.0.2.10",
			wantContains: false,
		},
		{
			name:         "large range with gap in pool",
			poolStr:      "10.0.0.0-10.0.0.255 10.0.2.0-10.0.3.255", // Gap: 10.0.1.0-10.0.1.255
			rangeStr:     "10.0.0.128-10.0.2.128",                   // 513 addresses, spans the gap
			wantContains: false,
		},
		{
			name:         "large range fully covered by multiple pool ranges",
			poolStr:      "10.0.0.0-10.0.1.255 10.0.2.0-10.0.3.255", // Combined: 1024 addresses
			rangeStr:     "10.0.0.0-10.0.1.255",                     // 512 addresses
			wantContains: true,
		},
		{
			name:         "large range with adjacent pool ranges",
			poolStr:      "172.16.0.0-172.16.0.255 172.16.1.0-172.16.1.255 172.16.2.0-172.16.2.255",
			rangeStr:     "172.16.0.100-172.16.2.100", // >500 addresses, continuous coverage
			wantContains: true,
		},
		{
			name:         "large IPv6 range with gap",
			poolStr:      "2001:db8::-2001:db8::fff 2001:db8::2000-2001:db8::2fff", // Gap from ::1000 to ::1fff
			rangeStr:     "2001:db8::500-2001:db8::2500",                           // Spans the gap
			wantContains: false,
		},
		{
			name:         "large range at pool boundaries",
			poolStr:      "192.168.0.0-192.168.1.255 192.168.2.0-192.168.3.255",
			rangeStr:     "192.168.1.0-192.168.2.255", // 512 addresses, needs both ranges
			wantContains: true,
		},
		{
			name:         "very large range /16 with small gap",
			poolStr:      "10.0.0.0/17 10.0.128.1-10.0.255.255", // Missing exactly 10.0.128.0
			rangeStr:     "10.0.0.0/16",                         // 65536 addresses
			wantContains: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			pool, err := ParsePool(tt.poolStr)
			require.NoError(t, err)

			r, err := ParseRange(tt.rangeStr)
			require.NoError(t, err)

			assert.Equal(t, tt.wantContains, pool.ContainsRange(r))
		})
	}
}

func TestPool_Iterate(t *testing.T) {
	t.Parallel()

	pool, err := ParsePool("192.0.2.0-192.0.2.2 192.0.2.10-192.0.2.12")
	require.NoError(t, err)

	var addresses []string
	for addr := range pool.Iterate() {
		addresses = append(addresses, addr.String())
	}

	expected := []string{
		"192.0.2.0", "192.0.2.1", "192.0.2.2",
		"192.0.2.10", "192.0.2.11", "192.0.2.12",
	}
	assert.Equal(t, expected, addresses)
}

func TestPool_IterateRanges(t *testing.T) {
	t.Parallel()

	pool, err := ParsePool("192.0.2.0-192.0.2.10 2001:db8::-2001:db8::10")
	require.NoError(t, err)

	var ranges []string
	for r := range pool.IterateRanges() {
		ranges = append(ranges, r.String())
	}

	expected := []string{
		"192.0.2.0-192.0.2.10",
		"2001:db8::-2001:db8::10",
	}
	assert.Equal(t, expected, ranges)
}

func TestPool_Add(t *testing.T) {
	t.Parallel()

	pool := NewPool()
	assert.Equal(t, 0, pool.Len())

	r1 := mustParseRange(t, "192.0.2.0", "192.0.2.10")
	pool.Add(r1)
	assert.Equal(t, 1, pool.Len())

	r2 := mustParseRange(t, "192.0.2.20", "192.0.2.30")
	r3 := mustParseRange(t, "192.0.2.40", "192.0.2.50")
	pool.Add(r2, nil, r3) // nil should be ignored
	assert.Equal(t, 3, pool.Len())
}

func TestPool_AddString(t *testing.T) {
	t.Parallel()

	pool := NewPool()

	err := pool.AddString("192.0.2.0-192.0.2.10 192.0.2.20/24")
	assert.NoError(t, err)
	assert.Equal(t, 2, pool.Len())

	err = pool.AddString("invalid")
	assert.Error(t, err)
	assert.Equal(t, 2, pool.Len()) // Should not change
}

func TestPool_Clone(t *testing.T) {
	t.Parallel()

	original, err := ParsePool("192.0.2.0-192.0.2.10 192.0.2.20-192.0.2.30")
	require.NoError(t, err)

	clone := original.Clone()

	// Should be equal
	assert.Equal(t, original.String(), clone.String())
	assert.Equal(t, original.Len(), clone.Len())

	// But independent
	r := mustParseRange(t, "192.0.2.40", "192.0.2.50")
	clone.Add(r)

	assert.NotEqual(t, original.Len(), clone.Len())
}

func TestPool_Ranges(t *testing.T) {
	t.Parallel()

	r1 := mustParseRange(t, "192.0.2.0", "192.0.2.10")
	r2 := mustParseRange(t, "192.0.2.20", "192.0.2.30")

	pool := NewPool(r1, r2)
	ranges := pool.Ranges()

	assert.Len(t, ranges, 2)
	assert.Equal(t, []Range{r1, r2}, ranges)

	// Modifying returned slice should not affect pool
	ranges[0] = nil
	assert.Equal(t, 2, pool.Len())
}

// Benchmark tests
func BenchmarkPool_Contains(b *testing.B) {
	// Create a pool with multiple ranges
	pool := NewPool()
	for i := 0; i < 10; i++ {
		start := fmt.Sprintf("192.0.%d.0", i)
		end := fmt.Sprintf("192.0.%d.255", i)
		r, _ := ParseRange(fmt.Sprintf("%s-%s", start, end))
		pool.Add(r)
	}

	ip := netip.MustParseAddr("192.0.5.100")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = pool.Contains(ip)
	}
}

func BenchmarkPool_Iterate(b *testing.B) {
	pool, _ := ParsePool("192.0.2.0/24 192.0.3.0/24")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		count := 0
		for _ = range pool.Iterate() {
			count++
		}
	}
}
