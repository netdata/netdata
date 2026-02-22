// SPDX-License-Identifier: GPL-3.0-or-later

package program

// Template is a parsed render template used for chart IDs and dimension names.
//
// Parsing/validation is done by compiler packages. In phase-1 templates are
// literal-only strings without placeholder/transform syntax.
type Template struct {
	Raw string
}

func (t Template) clone() Template {
	return t
}
