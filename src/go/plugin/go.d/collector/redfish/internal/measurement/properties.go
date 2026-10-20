// SPDX-License-Identifier: GPL-3.0-or-later

package measurement

import "strings"

// Properties provides the same JSON property semantics to acquisition and
// measurement adapters. Lookup uses dotted paths; Text reads a literal key.
type Properties map[string]any

func (data Properties) Lookup(path string) (any, bool) {
	var current any = map[string]any(data)
	for segment := range strings.SplitSeq(path, ".") {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = object[segment]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func stringValue(value any) (string, bool) {
	result, ok := value.(string)
	result = strings.TrimSpace(result)
	return result, ok && result != ""
}

func (data Properties) Text(key string) (string, bool) { return stringValue(data[key]) }
func (data Properties) String(path string) (string, bool) {
	value, ok := data.Lookup(path)
	if !ok {
		return "", false
	}
	return stringValue(value)
}
