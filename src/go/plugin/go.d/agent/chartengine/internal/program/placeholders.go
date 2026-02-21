// SPDX-License-Identifier: GPL-3.0-or-later

package program

// Template is a parsed render template used for chart IDs and dimension names.
//
// Parsing/validation is done by compiler packages; this type stores normalized
// output only.
type Template struct {
	Raw string
	// Parts are rendered in-order.
	Parts []TemplatePart
	// Keys are placeholder keys in first-seen order.
	Keys []string
}

// TemplatePart is one segment of a parsed template.
//
// Exactly one of Literal or PlaceholderKey is expected to be set.
type TemplatePart struct {
	Literal string

	PlaceholderKey string
	Transforms     []TemplateTransform
}

// TemplateTransform is a normalized placeholder transform operation.
type TemplateTransform struct {
	Name string
	Args []string
}

// IsDynamic reports whether template rendering depends on sample labels.
func (t Template) IsDynamic() bool {
	return len(t.Keys) > 0
}

func (t Template) clone() Template {
	out := t
	out.Parts = make([]TemplatePart, 0, len(t.Parts))
	for _, part := range t.Parts {
		out.Parts = append(out.Parts, part.clone())
	}
	out.Keys = append([]string(nil), t.Keys...)
	return out
}

func (p TemplatePart) clone() TemplatePart {
	out := p
	out.Transforms = make([]TemplateTransform, 0, len(p.Transforms))
	for _, transform := range p.Transforms {
		out.Transforms = append(out.Transforms, transform.clone())
	}
	return out
}

func (t TemplateTransform) clone() TemplateTransform {
	out := t
	out.Args = append([]string(nil), t.Args...)
	return out
}
