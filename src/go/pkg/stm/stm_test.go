// SPDX-License-Identifier: GPL-3.0-or-later

package stm_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/netdata/netdata/go/plugins/pkg/stm"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/metrix"
)

func TestToMap_empty(t *testing.T) {
	s := struct{}{}

	expected := map[string]int64{}

	assert.EqualValuesf(t, expected, stm.ToMap(s), "value test")
	assert.EqualValuesf(t, expected, stm.ToMap(&s), "ptr test")
}

func TestToMap_metrics(t *testing.T) {
	s := struct {
		C metrix.Counter   `stm:"c"`
		G metrix.Gauge     `stm:"g,100"`
		H metrix.Histogram `stm:"h,100"`
		S metrix.Summary   `stm:"s,200,2"`
	}{}
	s.C.Inc()
	s.G.Set(3.14)
	s.H = metrix.NewHistogram([]float64{1, 5, 10})

	s.H.Observe(3.14)
	s.H.Observe(6.28)
	s.H.Observe(20)

	s.S = metrix.NewSummary()
	s.S.Observe(3.14)
	s.S.Observe(6.28)

	expected := map[string]int64{
		"c": 1,
		"g": 314,

		"h_count":    3,
		"h_sum":      2942,
		"h_bucket_1": 0,
		"h_bucket_2": 1,
		"h_bucket_3": 2,

		"s_count": 2,
		"s_sum":   942,
		"s_min":   314,
		"s_max":   628,
		"s_avg":   471,
	}

	assert.Equal(t, expected, stm.ToMap(s), "value test")
	assert.Equal(t, expected, stm.ToMap(&s), "ptr test")
}

func TestToMap_int(t *testing.T) {
	s := struct {
		I   int   `stm:"int"`
		I8  int8  `stm:"int8"`
		I16 int16 `stm:"int16"`
		I32 int32 `stm:"int32"`
		I64 int64 `stm:"int64"`
	}{
		I: 1, I8: 2, I16: 3, I32: 4, I64: 5,
	}

	expected := map[string]int64{
		"int": 1, "int8": 2, "int16": 3, "int32": 4, "int64": 5,
	}

	assert.EqualValuesf(t, expected, stm.ToMap(s), "value test")
	assert.EqualValuesf(t, expected, stm.ToMap(&s), "ptr test")
}

func TestToMap_float(t *testing.T) {
	s := struct {
		F32 float32 `stm:"f32,100"`
		F64 float64 `stm:"f64"`
	}{
		3.14, 628,
	}

	expected := map[string]int64{
		"f32": 314, "f64": 628,
	}

	assert.EqualValuesf(t, expected, stm.ToMap(s), "value test")
	assert.EqualValuesf(t, expected, stm.ToMap(&s), "ptr test")
}

func TestToMap_struct(t *testing.T) {
	type pair struct {
		Left  int `stm:"left"`
		Right int `stm:"right"`
	}
	s := struct {
		I      int  `stm:"int"`
		Pempty pair `stm:""`
		Ps     pair `stm:"s"`
		Notag  int
	}{
		I:      1,
		Pempty: pair{2, 3},
		Ps:     pair{4, 5},
		Notag:  6,
	}

	expected := map[string]int64{
		"int":  1,
		"left": 2, "right": 3,
		"s_left": 4, "s_right": 5,
	}

	assert.EqualValuesf(t, expected, stm.ToMap(s), "value test")
	assert.EqualValuesf(t, expected, stm.ToMap(&s), "ptr test")
}

func TestToMap_tree(t *testing.T) {
	type node struct {
		Value int   `stm:"v"`
		Left  *node `stm:"left"`
		Right *node `stm:"right"`
	}
	s := node{1,
		&node{2, nil, nil},
		&node{3,
			&node{4, nil, nil},
			nil,
		},
	}
	expected := map[string]int64{
		"v":            1,
		"left_v":       2,
		"right_v":      3,
		"right_left_v": 4,
	}

	assert.EqualValuesf(t, expected, stm.ToMap(s), "value test")
	assert.EqualValuesf(t, expected, stm.ToMap(&s), "ptr test")
}

func TestToMap_map(t *testing.T) {
	s := struct {
		I int              `stm:"int"`
		M map[string]int64 `stm:""`
	}{
		I: 1,
		M: map[string]int64{
			"a": 2,
			"b": 3,
		},
	}

	expected := map[string]int64{
		"int": 1,
		"a":   2,
		"b":   3,
	}

	assert.EqualValuesf(t, expected, stm.ToMap(s), "value test")
	assert.EqualValuesf(t, expected, stm.ToMap(&s), "ptr test")
}

func TestToMap_nestMap(t *testing.T) {
	s := struct {
		I int            `stm:"int"`
		M map[string]any `stm:""`
	}{
		I: 1,
		M: map[string]any{
			"a": 2,
			"b": 3,
			"m": map[string]any{
				"c": 4,
			},
		},
	}

	expected := map[string]int64{
		"int": 1,
		"a":   2,
		"b":   3,
		"m_c": 4,
	}

	assert.EqualValuesf(t, expected, stm.ToMap(s), "value test")
	assert.EqualValuesf(t, expected, stm.ToMap(&s), "ptr test")
}

func TestToMap_ptr(t *testing.T) {
	two := 2
	s := struct {
		I   int  `stm:"int"`
		Ptr *int `stm:"ptr"`
		Nil *int `stm:"nil"`
	}{
		I:   1,
		Ptr: &two,
		Nil: nil,
	}

	expected := map[string]int64{
		"int": 1,
		"ptr": 2,
	}

	assert.EqualValuesf(t, expected, stm.ToMap(s), "value test")
	assert.EqualValuesf(t, expected, stm.ToMap(&s), "ptr test")
}

func TestToMap_invalidType(t *testing.T) {
	s := struct {
		Str string `stm:"int"`
	}{
		Str: "abc",
	}

	assert.Panics(t, func() {
		stm.ToMap(s)
	}, "value test")
	assert.Panics(t, func() {
		stm.ToMap(&s)
	}, "ptr test")
}

func TestToMap_duplicateKey(t *testing.T) {
	{
		s := struct {
			Key int            `stm:"key"`
			M   map[string]int `stm:""`
		}{
			Key: 1,
			M: map[string]int{
				"key": 2,
			},
		}

		assert.Panics(t, func() {
			stm.ToMap(s)
		}, "value test")
		assert.Panics(t, func() {
			stm.ToMap(&s)
		}, "ptr test")
	}
	{
		s := struct {
			Key float64            `stm:"key"`
			M   map[string]float64 `stm:""`
		}{
			Key: 1,
			M: map[string]float64{
				"key": 2,
			},
		}

		assert.Panics(t, func() {
			stm.ToMap(s)
		}, "value test")
		assert.Panics(t, func() {
			stm.ToMap(&s)
		}, "ptr test")
	}
}

func TestToMap_Variadic(t *testing.T) {
	s1 := struct {
		Key1 int `stm:"key1"`
	}{
		Key1: 1,
	}
	s2 := struct {
		Key2 int `stm:"key2"`
	}{
		Key2: 2,
	}
	s3 := struct {
		Key3 int `stm:"key3"`
	}{
		Key3: 3,
	}

	assert.Equal(
		t,
		map[string]int64{
			"key1": 1,
			"key2": 2,
			"key3": 3,
		},
		stm.ToMap(s1, s2, s3),
	)
}

func TestToMap_badTag(t *testing.T) {
	assert.Panics(t, func() {
		s := struct {
			A int `stm:"a,not_int"`
		}{1}
		stm.ToMap(s)
	})
	assert.Panics(t, func() {
		s := struct {
			A int `stm:"a,1,not_int"`
		}{1}
		stm.ToMap(s)
	})
	assert.Panics(t, func() {
		s := struct {
			A int `stm:"a,not_int,1"`
		}{1}
		stm.ToMap(s)
	})
	assert.Panics(t, func() {
		s := struct {
			A int `stm:"a,1,2,3"`
		}{1}
		stm.ToMap(s)
	})
}

func TestToMap_nilValue(t *testing.T) {
	assert.Panics(t, func() {
		s := struct {
			a metrix.CounterVec `stm:"a"`
		}{nil}
		stm.ToMap(s)
	})
}
func TestToMap_bool(t *testing.T) {
	s := struct {
		A bool `stm:"a"`
		B bool `stm:"b"`
	}{
		A: true,
		B: false,
	}
	assert.Equal(
		t,
		map[string]int64{
			"a": 1,
			"b": 0,
		},
		stm.ToMap(s),
	)
}

func TestToMap_ArraySlice(t *testing.T) {
	s := [4]any{
		map[string]int{
			"B": 1,
			"C": 2,
		},
		struct {
			D int `stm:"D"`
			E int `stm:"E"`
		}{
			D: 3,
			E: 4,
		},
		struct {
			STMKey string
			F      int `stm:"F"`
			G      int `stm:"G"`
		}{
			F: 5,
			G: 6,
		},
		struct {
			STMKey string
			H      int `stm:"H"`
			I      int `stm:"I"`
		}{
			STMKey: "KEY",
			H:      7,
			I:      8,
		},
	}

	assert.Equal(
		t,
		map[string]int64{
			"B":     1,
			"C":     2,
			"D":     3,
			"E":     4,
			"F":     5,
			"G":     6,
			"KEY_H": 7,
			"KEY_I": 8,
		},
		stm.ToMap(s),
	)

	assert.Equal(
		t,
		map[string]int64{
			"B":     1,
			"C":     2,
			"D":     3,
			"E":     4,
			"F":     5,
			"G":     6,
			"KEY_H": 7,
			"KEY_I": 8,
		},
		stm.ToMap(s[:]),
	)
}
