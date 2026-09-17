// SPDX-License-Identifier: GPL-3.0-or-later

// Package legacyconfig reads a declarative subset of the old Bash configuration.
// Parsing and evaluation never execute shell code, read the environment or resolve secrets.
package legacyconfig

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

const maxSourceSize = 1 << 20

// Program holds checked assignments and inert function definitions in source order.
// It does not expose the shell parser's syntax tree to its callers.
type Program struct {
	instructions []instruction
}

type instruction struct {
	name     string
	key      string
	value    expression
	function string
	reset    bool
	position position
}

type position struct {
	line   uint
	column uint
}

func at(pos syntax.Pos) position { return position{pos.Line(), pos.Col()} }

func (p position) error(message string) error {
	return fmt.Errorf("line %d, column %d: %s", p.line, p.column, message)
}

// Parse checks shell syntax and the supported top-level subset. Function bodies
// are parsed as Bash but retained without evaluating or validating their behavior.
func Parse(r io.Reader) (*Program, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxSourceSize+1))
	if err != nil {
		return nil, errors.New("could not read legacy configuration")
	}
	if len(data) > maxSourceSize {
		return nil, errors.New("legacy configuration exceeds the 1 MiB limit")
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return nil, errors.New("legacy configuration contains NUL")
	}
	file, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(bytes.NewReader(data), "")
	if err != nil {
		var parseErr syntax.ParseError
		if errors.As(err, &parseErr) {
			return nil, at(parseErr.Pos).error("invalid shell syntax")
		}
		return nil, errors.New("invalid shell syntax")
	}
	program := &Program{}
	for _, stmt := range file.Stmts {
		if err := program.statement(stmt, data); err != nil {
			return nil, err
		}
	}
	return program, nil
}

func plainStatement(stmt *syntax.Stmt) bool {
	return !stmt.Negated && !stmt.Background && !stmt.Coprocess && !stmt.Disown && len(stmt.Redirs) == 0
}

func (p *Program) statement(stmt *syntax.Stmt, source []byte) error {
	if !plainStatement(stmt) {
		return at(stmt.Pos()).error("top-level redirects and command modifiers are unsupported")
	}
	switch cmd := stmt.Cmd.(type) {
	case *syntax.CallExpr:
		if len(cmd.Args) != 0 {
			return at(stmt.Pos()).error("top-level commands are unsupported")
		}
		for _, assignment := range cmd.Assigns {
			if err := p.assignment(assignment, false, source); err != nil {
				return err
			}
		}
	case *syntax.DeclClause:
		// Recipient maps are associative in alarm-notify.sh, even without declarations.
		if cmd.Variant.Value != "declare" || len(cmd.Args) < 2 || !cmd.Args[0].Naked ||
			cmd.Args[0].Name != nil || cmd.Args[0].Value == nil || cmd.Args[0].Value.Lit() != "-A" {
			return at(stmt.Pos()).error("only declare -A recipient-map declarations are supported")
		}
		for _, assignment := range cmd.Args[1:] {
			if err := p.assignment(assignment, true, source); err != nil {
				return err
			}
		}
	case *syntax.FuncDecl:
		if cmd.Name == nil || !syntax.ValidName(cmd.Name.Value) || !plainStatement(cmd.Body) {
			return at(stmt.Pos()).error("only plain named function declarations are supported")
		}
		if _, ok := cmd.Body.Cmd.(*syntax.Block); !ok {
			return at(stmt.Pos()).error("function declarations require a brace body")
		}
		p.instructions = append(p.instructions, instruction{
			name: cmd.Name.Value, function: string(source[cmd.Pos().Offset():cmd.End().Offset()]), position: at(cmd.Pos()),
		})
	default:
		return at(stmt.Pos()).error("top-level shell control flow is unsupported")
	}
	return nil
}

func recipientMap(name string) bool {
	return strings.HasPrefix(name, "role_recipients_") && len(name) > len("role_recipients_")
}

func (p *Program) assignment(a *syntax.Assign, declaration bool, source []byte) error {
	pos := at(a.Pos())
	if a.Name == nil || !syntax.ValidName(a.Name.Value) || a.Append {
		return pos.error("only named assignments with = are supported")
	}
	name := a.Name.Value
	if declaration && (!recipientMap(name) || a.Index != nil || (!a.Naked && a.Array == nil)) {
		return pos.error("declare -A requires recipient maps with optional keyed initializers")
	}
	if a.Naked {
		if !declaration {
			return pos.error("assignment requires =")
		}
		// A declaration without an initializer preserves existing entries.
		p.instructions = append(p.instructions, instruction{name: name, position: pos})
		return nil
	}
	if a.Array != nil {
		if !recipientMap(name) {
			return pos.error("only recipient maps support array assignments")
		}
		p.instructions = append(p.instructions, instruction{name: name, reset: true, position: pos})
		for _, element := range a.Array.Elems {
			if err := p.entry(name, element.Index, element.Value, at(element.Pos()), source); err != nil {
				return err
			}
		}
		return nil
	}
	if a.Index != nil {
		if !recipientMap(name) {
			return pos.error("only recipient maps support indexed assignments")
		}
		return p.entry(name, a.Index, a.Value, pos, source)
	}
	if recipientMap(name) {
		return pos.error("recipient maps require an index or a keyed array initializer")
	}
	value, err := compileWord(a.Value, false)
	if err != nil {
		return err
	}
	p.instructions = append(p.instructions, instruction{name: name, value: value, position: pos})
	return nil
}

func (p *Program) entry(name string, index syntax.ArithmExpr, value *syntax.Word, pos position, source []byte) error {
	if index == nil {
		return pos.error("recipient map keys must be nonempty literal strings")
	}
	start, end := index.Pos().Offset(), index.End().Offset()
	// The parser omits continuations as well as whitespace at key boundaries.
	// Continuations add no key bytes; actual padding must be quoted to preserve it.
	for start >= 2 && source[start-1] == '\n' {
		length := uint(2)
		if source[start-2] == '\r' {
			length++
		}
		if start < length || source[start-length] != '\\' {
			break
		}
		start -= length
	}
	for end < uint(len(source)) && source[end] == '\\' {
		next := end + 1
		if next < uint(len(source)) && source[next] == '\r' {
			next++
		}
		if next >= uint(len(source)) || source[next] != '\n' {
			break
		}
		end = next + 1
	}
	if start == 0 || end >= uint(len(source)) || source[start-1] != '[' || source[end] != ']' {
		return pos.error("recipient key padding must be quoted")
	}
	word, ok := index.(*syntax.Word)
	if !ok {
		// The parser treats subscripts as arithmetic without knowing the array's type.
		// Here db-admin and 1+2 are literal associative keys, not arithmetic operations.
		raw := source[start:end]
		count := 0
		err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Words(bytes.NewReader(raw), func(parsed *syntax.Word) bool {
			word = parsed
			count++
			return count < 2
		})
		if err != nil || count != 1 {
			return pos.error("recipient map keys must be nonempty literal strings")
		}
	}
	keyParts, err := compileWord(word, true)
	if err != nil {
		return pos.error("recipient map keys must be nonempty literal strings")
	}
	var key strings.Builder
	for _, part := range keyParts {
		key.WriteString(part.literal)
	}
	if key.Len() == 0 {
		return pos.error("recipient map keys must be nonempty literal strings")
	}
	expr, err := compileWord(value, false)
	if err != nil {
		return err
	}
	p.instructions = append(p.instructions, instruction{name: name, key: key.String(), value: expr, position: pos})
	return nil
}
