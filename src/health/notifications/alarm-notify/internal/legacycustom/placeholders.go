// SPDX-License-Identifier: GPL-3.0-or-later

package legacycustom

import (
	"bytes"
	"errors"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// These are the two shipped no-op bodies, without comments. Compare parsed bodies,
// not substrings: an actual sender may legitimately contain either log message.
const stockDefault = `custom_sender() {
 info "custom notification mechanism is not configured; not sending ${notification_description}"
}`
const stockExample = `custom_sender() {
 local msg="${host} ${status_message}: ${alarm} ${raised_for}"
 urlencode "${msg:0:160}" >/dev/null; msg="${REPLY}"
 to="${1}"
 info "not sending custom notification to ${to}, for ${status} of '${host}.${chart}.${name}' - custom_sender() is not configured."
}`

func functionBody(name, source string) (string, error) {
	invalid := errors.New("custom source must contain one plain named function definition")
	f, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(source), "")
	if err != nil || len(f.Stmts) != 1 || strings.ContainsRune(source, 0) {
		return "", invalid
	}
	stmt := f.Stmts[0]
	plain := func(s *syntax.Stmt) bool {
		return !s.Negated && !s.Background && !s.Coprocess && !s.Disown && len(s.Redirs) == 0
	}
	fn, ok := stmt.Cmd.(*syntax.FuncDecl)
	if !ok || !plain(stmt) || !plain(fn.Body) || fn.Name.Value != name || !syntax.ValidName(name) {
		return "", invalid
	}
	body, ok := fn.Body.Cmd.(*syntax.Block)
	if !ok {
		return "", invalid
	}
	var normalized bytes.Buffer
	if err := syntax.NewPrinter(syntax.Minify(true)).Print(&normalized, body); err != nil {
		return "", invalid
	}
	return normalized.String(), nil
}

func stockPlaceholder(body string) bool {
	for _, source := range []string{stockDefault, stockExample} {
		known, _ := functionBody("custom_sender", source)
		if body == known {
			return true
		}
	}
	return false
}
