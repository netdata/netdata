// SPDX-License-Identifier: GPL-3.0-or-later

package collecttest

import (
	"maps"
	"slices"
	"strings"
)

// SchemaResolver reads one config_schema.json jsonSchema document the way the DynCfg form renderer
// composes it: local $ref and allOf folded into one object, and the properties every dependencies
// branch reveals treated as properties of their parent. The repo-wide schema rule tests and
// AssertConfigSchemaMatchesMetadata share it so both see the same form.
type SchemaResolver struct {
	root map[string]any
}

// NewSchemaResolver wraps a parsed jsonSchema member.
func NewSchemaResolver(jsonSchema map[string]any) SchemaResolver {
	return SchemaResolver{root: jsonSchema}
}

// Root returns the document root, resolved.
func (r SchemaResolver) Root() map[string]any { return r.Resolve(r.root) }

// Resolve follows a local $ref and folds allOf members into one object. `properties` and `required`
// accumulate across the composed parts; every other keyword is last-writer-wins with the node's own
// keywords winning over the composed ones. Nil in, nil out.
func (r SchemaResolver) Resolve(node map[string]any) map[string]any {
	if node == nil {
		return nil
	}
	out := map[string]any{}
	if ref, ok := node["$ref"].(string); ok {
		mergeSchemaInto(out, r.Resolve(r.deref(ref)))
	}
	if members, ok := node["allOf"].([]any); ok {
		for _, member := range members {
			mergeSchemaInto(out, r.Resolve(asObject(member)))
		}
	}
	own := make(map[string]any, len(node))
	for key, value := range node {
		if key != "$ref" && key != "allOf" {
			own[key] = value
		}
	}
	mergeSchemaInto(out, own)
	return out
}

// Properties returns a node's properties plus the properties its dependencies branches reveal (a
// property dependency, a list of names, reveals nothing), each resolved, keyed by name. Discriminators
// are visited in sorted order so the first-seen copy of a duplicated branch key is deterministic.
func (r SchemaResolver) Properties(node map[string]any) map[string]map[string]any {
	node = r.Resolve(node)
	out := map[string]map[string]any{}
	for name, child := range asObject(node["properties"]) {
		out[name] = r.Resolve(asObject(child))
	}
	dependencies := asObject(node["dependencies"])
	for _, discriminator := range slices.Sorted(maps.Keys(dependencies)) {
		for _, branch := range SchemaBranches(dependencies[discriminator]) {
			for name, child := range asObject(r.Resolve(branch)["properties"]) {
				if name == discriminator {
					continue
				}
				if _, seen := out[name]; !seen {
					out[name] = r.Resolve(asObject(child))
				}
			}
		}
	}
	return out
}

// Node resolves a documented option name ("rules[].query.period", "mode_filters.regions") to its
// schema object, descending through properties, revealed branch properties, and array items.
func (r SchemaResolver) Node(option string) (map[string]any, bool) {
	node := r.Root()
	for segment := range strings.SplitSeq(option, ".") {
		name := strings.TrimSuffix(segment, "[]")
		child, ok := r.Properties(node)[name]
		if !ok {
			return nil, false
		}
		node = child
		if strings.HasSuffix(segment, "[]") {
			items := asObject(node["items"])
			if items == nil {
				return nil, false
			}
			node = r.Resolve(items)
		}
	}
	return node, true
}

// SchemaBranches returns a dependency's oneOf/anyOf alternatives, or the dependency itself when it
// applies unconditionally. A property dependency (a list of names) has no branches.
func SchemaBranches(dependency any) []map[string]any {
	obj := asObject(dependency)
	if obj == nil {
		return nil
	}
	for _, keyword := range []string{"oneOf", "anyOf"} {
		raw, ok := obj[keyword].([]any)
		if !ok {
			continue
		}
		branches := make([]map[string]any, 0, len(raw))
		for _, branch := range raw {
			if b := asObject(branch); b != nil {
				branches = append(branches, b)
			}
		}
		return branches
	}
	return []map[string]any{obj}
}

func (r SchemaResolver) deref(ref string) map[string]any {
	if !strings.HasPrefix(ref, "#/") {
		return nil
	}
	var node any = r.root
	for part := range strings.SplitSeq(ref[2:], "/") {
		// RFC 6901: "~1" encodes "/" and "~0" encodes "~", in that decoding order.
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		node = asObject(node)[part]
	}
	return asObject(node)
}

func mergeSchemaInto(dst, src map[string]any) {
	for key, value := range src {
		switch key {
		case "properties":
			merged := map[string]any{}
			maps.Copy(merged, asObject(dst[key]))
			maps.Copy(merged, asObject(value))
			dst[key] = merged
		case "required":
			have, _ := dst[key].([]any)
			add, _ := value.([]any)
			dst[key] = append(append([]any{}, have...), add...)
		default:
			dst[key] = value
		}
	}
}

// asObject returns node as an object, or nil when it is absent or another type.
func asObject(node any) map[string]any {
	obj, _ := node.(map[string]any)
	return obj
}
