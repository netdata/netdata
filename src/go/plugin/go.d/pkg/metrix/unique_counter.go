// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"github.com/axiomhq/hyperloglog"

	"github.com/netdata/netdata/go/plugins/pkg/stm"
)

type (
	UniqueCounter interface {
		stm.Value
		Insert(s string)
		Value() int
		Reset()
	}

	mapUniqueCounter struct {
		m map[string]bool
	}

	hyperLogLogUniqueCounter struct {
		sketch *hyperloglog.Sketch
	}

	UniqueCounterVec struct {
		useHyperLogLog bool
		Items          map[string]UniqueCounter
	}
)

var (
	_ stm.Value = mapUniqueCounter{}
	_ stm.Value = hyperLogLogUniqueCounter{}
	_ stm.Value = UniqueCounterVec{}
)

func NewUniqueCounter(useHyperLogLog bool) UniqueCounter {
	if useHyperLogLog {
		return &hyperLogLogUniqueCounter{hyperloglog.New()}
	}
	return mapUniqueCounter{map[string]bool{}}
}

func (c mapUniqueCounter) WriteTo(rv map[string]int64, key string, mul, div int) {
	rv[key] = int64(float64(c.Value()*mul) / float64(div))
}

func (c mapUniqueCounter) Insert(s string) {
	c.m[s] = true
}

func (c mapUniqueCounter) Value() int {
	return len(c.m)
}

func (c mapUniqueCounter) Reset() {
	for key := range c.m {
		delete(c.m, key)
	}
}

// WriteTo writes its value into given map.
func (c hyperLogLogUniqueCounter) WriteTo(rv map[string]int64, key string, mul, div int) {
	rv[key] = int64(float64(c.Value()*mul) / float64(div))
}

func (c *hyperLogLogUniqueCounter) Insert(s string) {
	c.sketch.Insert([]byte(s))
}

func (c *hyperLogLogUniqueCounter) Value() int {
	return int(c.sketch.Estimate())
}

func (c *hyperLogLogUniqueCounter) Reset() {
	c.sketch = hyperloglog.New()
}

func NewUniqueCounterVec(useHyperLogLog bool) UniqueCounterVec {
	return UniqueCounterVec{
		Items:          map[string]UniqueCounter{},
		useHyperLogLog: useHyperLogLog,
	}
}

// WriteTo writes its value into given map.
func (c UniqueCounterVec) WriteTo(rv map[string]int64, key string, mul, div int) {
	for name, value := range c.Items {
		value.WriteTo(rv, key+"_"+name, mul, div)
	}
}

// Get gets UniqueCounter instance by name
func (c UniqueCounterVec) Get(name string) UniqueCounter {
	item, ok := c.Items[name]
	if ok {
		return item
	}
	item = NewUniqueCounter(c.useHyperLogLog)
	c.Items[name] = item
	return item
}

func (c UniqueCounterVec) Reset() {
	for _, value := range c.Items {
		value.Reset()
	}
}
