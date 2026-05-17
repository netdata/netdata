// SPDX-License-Identifier: GPL-3.0-or-later

package topologyv1

type StringDictionary struct {
	values []any
	index  map[string]int
}

func NewStringDictionary(values ...string) *StringDictionary {
	dict := &StringDictionary{
		index: make(map[string]int, len(values)),
	}
	for _, value := range values {
		dict.Ref(value)
	}
	return dict
}

func (dict *StringDictionary) Ref(value string) int {
	if dict == nil {
		panic("topologyv1.StringDictionary.Ref called on nil dictionary")
	}
	if dict.index == nil {
		dict.index = make(map[string]int)
	}
	if index, ok := dict.index[value]; ok {
		return index
	}
	index := len(dict.values)
	dict.values = append(dict.values, value)
	dict.index[value] = index
	return index
}

func (dict *StringDictionary) Values() []any {
	if dict == nil || len(dict.values) == 0 {
		return []any{}
	}
	return append([]any(nil), dict.values...)
}
