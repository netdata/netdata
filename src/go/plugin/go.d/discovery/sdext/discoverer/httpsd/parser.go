// SPDX-License-Identifier: GPL-3.0-or-later

package httpsd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/sd/model"

	"gopkg.in/yaml.v2"
)

type responseParser struct {
	format string
}

func (p responseParser) parse(bs []byte, contentType string) ([]any, error) {
	switch p.format {
	case formatJSON:
		return parseItemsJSON(bs)
	case formatYAML:
		return parseItemsYAML(bs)
	case formatAuto:
		return p.parseAuto(bs, contentType)
	default:
		return nil, fmt.Errorf("unsupported format %q", p.format)
	}
}

func (p responseParser) parseAuto(bs []byte, contentType string) ([]any, error) {
	switch detectContentTypeFormat(contentType) {
	case formatJSON:
		return parseItemsJSON(bs)
	case formatYAML:
		return parseItemsYAML(bs)
	}

	items, jsonErr := parseItemsJSON(bs)
	if jsonErr == nil {
		return items, nil
	}

	items, yamlErr := parseItemsYAML(bs)
	if yamlErr == nil {
		return items, nil
	}

	return nil, fmt.Errorf("parse response as json: %v; parse response as yaml: %v", jsonErr, yamlErr)
}

func detectContentTypeFormat(contentType string) string {
	if i := strings.IndexByte(contentType, ';'); i >= 0 {
		contentType = contentType[:i]
	}
	contentType = strings.TrimSpace(strings.ToLower(contentType))

	switch {
	case contentType == "application/json",
		contentType == "text/json",
		strings.HasSuffix(contentType, "+json"):
		return formatJSON
	case contentType == "application/yaml",
		contentType == "application/x-yaml",
		contentType == "text/yaml",
		contentType == "text/x-yaml",
		strings.HasSuffix(contentType, "+yaml"),
		strings.HasSuffix(contentType, "+x-yaml"):
		return formatYAML
	default:
		return ""
	}
}

func parseItemsJSON(bs []byte) ([]any, error) {
	dec := json.NewDecoder(bytes.NewReader(bs))

	var data any
	if err := dec.Decode(&data); err != nil {
		return nil, err
	}
	var extra any
	if err := dec.Decode(&extra); err == nil {
		return nil, errors.New("multiple JSON values are not supported")
	} else if !errors.Is(err, io.EOF) {
		return nil, err
	}

	return extractItems(data)
}

func parseItemsYAML(bs []byte) ([]any, error) {
	dec := yaml.NewDecoder(bytes.NewReader(bs))

	var data any
	if err := dec.Decode(&data); err != nil {
		return nil, err
	}
	var extra any
	if err := dec.Decode(&extra); err == nil {
		return nil, errors.New("multiple YAML documents are not supported")
	} else if !errors.Is(err, io.EOF) {
		return nil, err
	}

	data, err := normalizeYAMLValue(data)
	if err != nil {
		return nil, err
	}

	return extractItems(data)
}

func extractItems(data any) ([]any, error) {
	switch v := data.(type) {
	case []any:
		return normalizeItems(v)
	case map[string]any:
		items, ok := v["items"]
		if !ok {
			return nil, errors.New("unsupported response envelope: missing items field")
		}
		arr, ok := items.([]any)
		if !ok {
			return nil, fmt.Errorf("unsupported response envelope: items must be an array, got %T", items)
		}
		return normalizeItems(arr)
	default:
		return nil, fmt.Errorf("unsupported response format: expected array or object with items array, got %T", data)
	}
}

func normalizeItems(items []any) ([]any, error) {
	out := make([]any, 0, len(items))
	for i, item := range items {
		norm, err := normalizeItem(item)
		if err != nil {
			return nil, fmt.Errorf("item[%d]: %w", i, err)
		}
		out = append(out, norm)
	}
	return out, nil
}

func normalizeItem(item any) (any, error) {
	switch v := item.(type) {
	case map[string]any:
		return normalizeMap(v)
	case string:
		return v, nil
	default:
		return nil, fmt.Errorf("unsupported item type %T", item)
	}
}

func normalizeYAMLValue(v any) (any, error) {
	switch vv := v.(type) {
	case map[any]any:
		m := make(map[string]any, len(vv))
		for k, iv := range vv {
			ks, ok := k.(string)
			if !ok {
				return nil, fmt.Errorf("yaml map key must be string, got %T", k)
			}
			norm, err := normalizeYAMLValue(iv)
			if err != nil {
				return nil, err
			}
			m[ks] = norm
		}
		return m, nil
	case map[string]any:
		return normalizeMap(vv)
	case []any:
		arr := make([]any, 0, len(vv))
		for _, iv := range vv {
			norm, err := normalizeYAMLValue(iv)
			if err != nil {
				return nil, err
			}
			arr = append(arr, norm)
		}
		return arr, nil
	default:
		return v, nil
	}
}

func normalizeMap(src map[string]any) (map[string]any, error) {
	m := make(map[string]any, len(src))
	for k, v := range src {
		norm, err := normalizeYAMLValue(v)
		if err != nil {
			return nil, err
		}
		m[k] = norm
	}
	return m, nil
}

func targetsFromItems(source string, items []any) ([]model.Target, error) {
	targets := make([]model.Target, 0, len(items))
	for i, item := range items {
		canonical, err := canonicalJSON(item)
		if err != nil {
			return nil, fmt.Errorf("item[%d]: canonical encoding: %w", i, err)
		}

		tgt := &target{
			label: itemLabel(item, canonical),
			Item:  item,
		}
		hash, err := model.CalcHash(struct {
			Source string
			Item   string
		}{
			Source: source,
			Item:   string(canonical),
		})
		if err != nil {
			return nil, fmt.Errorf("item[%d]: target hash: %w", i, err)
		}
		tgt.hash = hash
		targets = append(targets, tgt)
	}
	return targets, nil
}

func itemLabel(item any, canonical []byte) string {
	if m, ok := item.(map[string]any); ok {
		if v, ok := m["name"].(string); ok {
			if name := strings.TrimSpace(v); name != "" {
				return name
			}
		}
	}
	return fmt.Sprintf("item-%x", hashBytes(canonical))
}

func canonicalJSON(v any) ([]byte, error) {
	return json.Marshal(canonicalValue(v))
}

func canonicalValue(v any) any {
	switch vv := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(vv))
		for k := range vv {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		m := make(map[string]any, len(vv))
		for _, k := range keys {
			m[k] = canonicalValue(vv[k])
		}
		return m
	case []any:
		arr := make([]any, 0, len(vv))
		for _, iv := range vv {
			arr = append(arr, canonicalValue(iv))
		}
		return arr
	default:
		return v
	}
}

func hashBytes(bs []byte) uint64 {
	h := fnv.New64a()
	_, _ = h.Write(bs)
	return h.Sum64()
}
