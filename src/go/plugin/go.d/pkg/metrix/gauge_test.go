// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGauge_Set(t *testing.T) {
	var g Gauge
	assert.Equal(t, 0.0, g.Value())
	g.Set(100)
	assert.Equal(t, 100.0, g.Value())
	g.Set(200)
	assert.Equal(t, 200.0, g.Value())
}

func TestGauge_Add(t *testing.T) {
	var g Gauge
	assert.Equal(t, 0.0, g.Value())
	g.Add(100)
	assert.Equal(t, 100.0, g.Value())
	g.Add(200)
	assert.Equal(t, 300.0, g.Value())
}
func TestGauge_Sub(t *testing.T) {
	var g Gauge
	assert.Equal(t, 0.0, g.Value())
	g.Sub(100)
	assert.Equal(t, -100.0, g.Value())
	g.Sub(200)
	assert.Equal(t, -300.0, g.Value())
}

func TestGauge_Inc(t *testing.T) {
	var g Gauge
	assert.Equal(t, 0.0, g.Value())
	g.Inc()
	assert.Equal(t, 1.0, g.Value())
}

func TestGauge_Dec(t *testing.T) {
	var g Gauge
	assert.Equal(t, 0.0, g.Value())
	g.Dec()
	assert.Equal(t, -1.0, g.Value())
}

func TestGauge_SetToCurrentTime(t *testing.T) {
	var g Gauge
	g.SetToCurrentTime()
	assert.InDelta(t, time.Now().Unix(), g.Value(), 1)
}

func TestGauge_WriteTo(t *testing.T) {
	g := Gauge(3.14)
	m := map[string]int64{}
	g.WriteTo(m, "pi", 100, 1)
	assert.Len(t, m, 1)
	assert.EqualValues(t, 314, m["pi"])
}

func TestGaugeVec_WriteTo(t *testing.T) {
	g := NewGaugeVec()
	g.Get("foo").Inc()
	g.Get("foo").Inc()
	g.Get("bar").Inc()
	g.Get("bar").Add(0.14)

	m := map[string]int64{}
	g.WriteTo(m, "pi", 100, 1)
	assert.Len(t, m, 2)
	assert.EqualValues(t, 200, m["pi_foo"])
	assert.EqualValues(t, 114, m["pi_bar"])
}

func BenchmarkGauge_Add(b *testing.B) {
	benchmarks := []struct {
		name  string
		value float64
	}{
		{"int", 1},
		{"float", 3.14},
	}
	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			var c Gauge
			for i := 0; i < b.N; i++ {
				c.Add(bm.value)
			}
		})
	}
}

func BenchmarkGauge_Inc(b *testing.B) {
	var c Gauge
	for i := 0; i < b.N; i++ {
		c.Inc()
	}
}

func BenchmarkGauge_Set(b *testing.B) {
	var c Gauge
	for i := 0; i < b.N; i++ {
		c.Set(3.14)
	}
}

func BenchmarkGauge_Value(b *testing.B) {
	var c Gauge
	c.Inc()
	c.Add(3.14)
	for i := 0; i < b.N; i++ {
		c.Value()
	}
}

func BenchmarkGauge_WriteTo(b *testing.B) {
	var c Gauge
	c.Inc()
	c.Add(3.14)
	m := map[string]int64{}
	for i := 0; i < b.N; i++ {
		c.WriteTo(m, "pi", 100, 1)
	}
}
