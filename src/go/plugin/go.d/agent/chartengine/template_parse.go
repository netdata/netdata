// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/chartengine/internal/program"
)

func parseTemplate(raw string) (program.Template, error) {
	tpl := program.Template{
		Raw: strings.TrimSpace(raw),
	}
	if tpl.Raw == "" {
		return tpl, fmt.Errorf("template cannot be empty")
	}

	keySeen := make(map[string]struct{})
	cursor := 0
	for i := 0; i < len(tpl.Raw); i++ {
		switch tpl.Raw[i] {
		case '{':
			if i > cursor {
				tpl.Parts = append(tpl.Parts, program.TemplatePart{
					Literal: tpl.Raw[cursor:i],
				})
			}
			end := strings.IndexByte(tpl.Raw[i+1:], '}')
			if end < 0 {
				return program.Template{}, fmt.Errorf("unclosed placeholder in %q", tpl.Raw)
			}
			content := strings.TrimSpace(tpl.Raw[i+1 : i+1+end])
			if content == "" {
				return program.Template{}, fmt.Errorf("empty placeholder in %q", tpl.Raw)
			}
			part, err := parsePlaceholder(content)
			if err != nil {
				return program.Template{}, err
			}
			tpl.Parts = append(tpl.Parts, part)

			if _, ok := keySeen[part.PlaceholderKey]; !ok {
				keySeen[part.PlaceholderKey] = struct{}{}
				tpl.Keys = append(tpl.Keys, part.PlaceholderKey)
			}
			i += end + 1
			cursor = i + 1
		case '}':
			return program.Template{}, fmt.Errorf("unexpected '}' in %q", tpl.Raw)
		}
	}

	if cursor < len(tpl.Raw) {
		tpl.Parts = append(tpl.Parts, program.TemplatePart{
			Literal: tpl.Raw[cursor:],
		})
	}
	if len(tpl.Parts) == 0 {
		tpl.Parts = append(tpl.Parts, program.TemplatePart{
			Literal: tpl.Raw,
		})
	}

	return tpl, nil
}

func parsePlaceholder(content string) (program.TemplatePart, error) {
	segments := splitPipeSegments(content)
	if len(segments) == 0 {
		return program.TemplatePart{}, fmt.Errorf("invalid placeholder %q", content)
	}

	key := strings.TrimSpace(segments[0])
	if key == "" {
		return program.TemplatePart{}, fmt.Errorf("placeholder key is required in %q", content)
	}

	part := program.TemplatePart{
		PlaceholderKey: key,
	}

	for _, seg := range segments[1:] {
		transform, err := parseTransform(seg)
		if err != nil {
			return program.TemplatePart{}, err
		}
		part.Transforms = append(part.Transforms, transform)
	}
	return part, nil
}

func splitPipeSegments(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}

	var (
		segments      []string
		buf           strings.Builder
		inQuotes      bool
		escapePending bool
		depth         int
	)
	for _, r := range value {
		switch {
		case escapePending:
			escapePending = false
			buf.WriteRune(r)
		case r == '\\':
			escapePending = true
			buf.WriteRune(r)
		case r == '"':
			inQuotes = !inQuotes
			buf.WriteRune(r)
		case r == '(' && !inQuotes:
			depth++
			buf.WriteRune(r)
		case r == ')' && !inQuotes:
			if depth > 0 {
				depth--
			}
			buf.WriteRune(r)
		case r == '|' && !inQuotes && depth == 0:
			segments = append(segments, strings.TrimSpace(buf.String()))
			buf.Reset()
		default:
			buf.WriteRune(r)
		}
	}

	segments = append(segments, strings.TrimSpace(buf.String()))
	out := make([]string, 0, len(segments))
	for _, seg := range segments {
		if seg != "" {
			out = append(out, seg)
		}
	}
	return out
}

func parseTransform(segment string) (program.TemplateTransform, error) {
	segment = strings.TrimSpace(segment)
	if segment == "" {
		return program.TemplateTransform{}, fmt.Errorf("empty template transform")
	}
	if !strings.Contains(segment, "(") {
		return program.TemplateTransform{Name: segment}, nil
	}

	open := strings.IndexByte(segment, '(')
	close := strings.LastIndexByte(segment, ')')
	if open <= 0 || close < open || close != len(segment)-1 {
		return program.TemplateTransform{}, fmt.Errorf("invalid template transform %q", segment)
	}

	name := strings.TrimSpace(segment[:open])
	if name == "" {
		return program.TemplateTransform{}, fmt.Errorf("transform name is required in %q", segment)
	}
	args := splitTransformArgs(segment[open+1 : close])
	return program.TemplateTransform{
		Name: name,
		Args: args,
	}, nil
}

func splitTransformArgs(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}

	var (
		args          []string
		buf           strings.Builder
		inQuotes      bool
		escapePending bool
	)
	for _, r := range value {
		switch {
		case escapePending:
			escapePending = false
			buf.WriteRune(r)
		case r == '\\':
			escapePending = true
			buf.WriteRune(r)
		case r == '"':
			inQuotes = !inQuotes
			buf.WriteRune(r)
		case r == ',' && !inQuotes:
			args = append(args, normalizeArg(buf.String()))
			buf.Reset()
		default:
			buf.WriteRune(r)
		}
	}
	args = append(args, normalizeArg(buf.String()))

	out := make([]string, 0, len(args))
	for _, arg := range args {
		if arg != "" {
			out = append(out, arg)
		}
	}
	return out
}

func normalizeArg(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "\"")
	value = strings.TrimSuffix(value, "\"")
	return strings.TrimSpace(value)
}
