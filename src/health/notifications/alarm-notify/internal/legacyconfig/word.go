// SPDX-License-Identifier: GPL-3.0-or-later

package legacyconfig

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

type expression []wordPart

type wordPart struct {
	literal  string
	variable string
}

func compileWord(word *syntax.Word, literalOnly bool) (expression, error) {
	if word == nil {
		return nil, nil
	}
	return compileParts(word.Parts, false, literalOnly)
}

func compileParts(parts []syntax.WordPart, quoted, literalOnly bool) (expression, error) {
	var result expression
	for i, part := range parts {
		pos := at(part.Pos())
		switch part := part.(type) {
		case *syntax.Lit:
			literal, err := unescape(part.Value, quoted, i == 0, pos)
			if err != nil {
				return nil, err
			}
			result = append(result, wordPart{literal: literal})
		case *syntax.SglQuoted:
			if part.Dollar {
				return nil, pos.error("ANSI-C quoting is unsupported")
			}
			result = append(result, wordPart{literal: part.Value})
		case *syntax.DblQuoted:
			if part.Dollar {
				return nil, pos.error("localized quoting is unsupported")
			}
			inner, err := compileParts(part.Parts, true, literalOnly)
			if err != nil {
				return nil, err
			}
			result = append(result, inner...)
		case *syntax.ParamExp:
			if literalOnly || part.Param == nil || !syntax.ValidName(part.Param.Value) || recipientMap(part.Param.Value) || part.Index != nil ||
				part.Excl || part.Length || part.Width || part.Slice != nil || part.Repl != nil || part.Exp != nil || part.Names != 0 {
				return nil, pos.error("only simple scalar variable references are supported in values")
			}
			result = append(result, wordPart{variable: part.Param.Value})
		default:
			return nil, pos.error("shell substitutions and computed expressions are unsupported")
		}
	}
	return result, nil
}

func unescape(raw string, quoted, first bool, pos position) (string, error) {
	var text strings.Builder
	tildeStart := first
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if c == '\\' && i+1 < len(raw) {
			next := raw[i+1]
			if !quoted || strings.ContainsRune("$`\"\\\n", rune(next)) {
				i++
				if next != '\n' {
					text.WriteByte(next)
					tildeStart = false
				}
				continue
			}
		}
		if !quoted && c == '~' && tildeStart {
			return "", pos.error("tilde expansion is unsupported; quote literal tildes")
		}
		text.WriteByte(c)
		tildeStart = c == ':'
	}
	return text.String(), nil
}
