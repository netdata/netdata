<!--
title: "stm"
custom_edit_url: "https://github.com/netdata/netdata/blob/master/src/go/collectors/go.d.plugin/pkg/stm/README.md"
sidebar_label: "stm"
learn_status: "Published"
learn_rel_path: "Developers/External plugins/go.d.plugin/Helper Packages"
-->

# stm

This package helps you to convert a struct to `map[string]int64`.

## Tags

The encoding of each struct field can be customized by the format string stored under the `stm` key in the struct
field's tag. The format string gives the name of the field, possibly followed by a comma-separated list of options.

**Lack of tag means no conversion performed.**
If you don't want a field to be added to the `map[string]int64` just don't add a tag to it.

Tag syntax:

```
`stm:"name,multiplier,divisor"`
```

Both `multiplier` and `divisor` are optional, `name` is mandatory.

Examples of struct field tags and their meanings:

```
// Field appears in map as key "name".
Field int `stm:"name"`

// Field appears in map as key "name" and its value is multiplied by 10.
Field int `stm:"name,10"`

// Field appears in map as key "name" and its value is multiplied by 10 and divided by 5.
Field int `stm:"name,10,5"`
```

## Supported field value kinds

The list is:

- `int`
- `float`
- `bool`
- `map`
- `array`
- `slice`
- `pointer`
- `struct`
- `interface { WriteTo(rv map[string]int64, key string, mul, div int) }`

It is ok to have nested structures.

## Usage

Use `ToMap` function. Keep in mind:

- this function is variadic (can be called with any number of trailing arguments).
- it doesn't allow to have duplicate in result map.
- if there is a duplicate key it panics.

```
	ms := struct {
		MetricA   int64            `stm:"metric_a"`
		MetricB   float64          `stm:"metric_b,1000"`
		MetricSet map[string]int64 `stm:"metric_set"`
	}{
		MetricA: 10,
		MetricB: 5.5,
		MetricSet: map[string]int64{
			"a": 10,
			"b": 10,
		},
	}
	fmt.Println(stm.ToMap(ms)) // => map[metric_a:10 metric_b:5500 metric_set_a:10 metric_set_b:10]
```
