// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/stm"
)

type (
	// Counter is a Metric that represents a single numerical bits that only ever
	// goes up. That implies that it cannot be used to count items whose number can
	// also go down, e.g. the number of currently running goroutines. Those
	// "counters" are represented by Gauges.
	//
	// A Counter is typically used to count requests served, tasks completed, errors
	// occurred, etc.
	Counter struct {
		valInt   int64
		valFloat float64
	}

	// CounterVec is a Collector that bundles a set of Counters which have different values for their names.
	// This is used if you want to count the same thing partitioned by various dimensions
	// (e.g. number of HTTP requests, partitioned by response code and method).
	//
	// Create instances with NewCounterVec.
	CounterVec map[string]*Counter
)

var (
	_ stm.Value = Counter{}
	_ stm.Value = CounterVec{}
)

// WriteTo writes its value into given map.
func (c Counter) WriteTo(rv map[string]int64, key string, mul, div int) {
	rv[key] = int64(c.Value() * float64(mul) / float64(div))
}

// Value gets current counter.
func (c Counter) Value() float64 {
	return float64(c.valInt) + c.valFloat
}

// Inc increments the counter by 1. Use Add to increment it by arbitrary
// non-negative values.
func (c *Counter) Inc() {
	c.valInt++
}

// Add adds the given bits to the counter. It panics if the value is < 0.
func (c *Counter) Add(v float64) {
	if v < 0 {
		panic(errors.New("counter cannot decrease in value"))
	}
	val := int64(v)
	if float64(val) == v {
		c.valInt += val
		return
	}
	c.valFloat += v
}

// NewCounterVec creates a new CounterVec
func NewCounterVec() CounterVec {
	return CounterVec{}
}

// WriteTo writes its value into given map.
func (c CounterVec) WriteTo(rv map[string]int64, key string, mul, div int) {
	for name, value := range c {
		rv[key+"_"+name] = int64(value.Value() * float64(mul) / float64(div))
	}
}

// Get gets counter instance by name
func (c CounterVec) Get(name string) *Counter {
	item, _ := c.GetP(name)
	return item
}

// GetP gets counter instance by name
func (c CounterVec) GetP(name string) (counter *Counter, ok bool) {
	counter, ok = c[name]
	if ok {
		return
	}
	counter = &Counter{}
	c[name] = counter
	return
}
