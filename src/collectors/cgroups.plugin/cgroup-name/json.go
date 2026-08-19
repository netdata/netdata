// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
)

type keyValue struct {
	key   string
	value string
}

func orderedStringEntries(raw json.RawMessage) ([]keyValue, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil, nil
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return nil, fmt.Errorf("not an object")
	}

	var out []keyValue
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := tok.(string)
		if !ok {
			return nil, fmt.Errorf("non-string key")
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return nil, err
		}
		out = append(out, keyValue{key: key, value: rawJSONScalar(value)})
	}
	_, err = dec.Token()
	return out, err
}

func rawJSONScalar(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return "null"
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var value any
	if err := json.Unmarshal(raw, &value); err == nil {
		return fmt.Sprint(value)
	}
	return string(raw)
}
