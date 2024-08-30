// SPDX-License-Identifier: GPL-3.0-or-later

package matcher

import (
	"errors"
	"fmt"
	"regexp"
)

type (
	// Matcher is an interface that wraps MatchString method.
	Matcher interface {
		// Match performs match against given []byte
		Match(b []byte) bool
		// MatchString performs match against given string
		MatchString(string) bool
	}

	// Format matcher format
	Format string
)

const (
	// FmtString is a string match format.
	FmtString Format = "string"
	// FmtGlob is a glob match format.
	FmtGlob Format = "glob"
	// FmtRegExp is a regex match format.
	FmtRegExp Format = "regexp"
	// FmtSimplePattern is a simple pattern match format
	// https://docs.netdata.cloud/libnetdata/simple_pattern/
	FmtSimplePattern Format = "simple_patterns"

	// Separator is a separator between match format and expression.
	Separator = ":"
)

const (
	symString = "="
	symGlob   = "*"
	symRegExp = "~"
)

var (
	reShortSyntax = regexp.MustCompile(`(?s)^(!)?(.)\s*(.*)$`)
	reLongSyntax  = regexp.MustCompile(`(?s)^(!)?([^:]+):(.*)$`)

	errNotShortSyntax = errors.New("not short syntax")
)

// Must is a helper that wraps a call to a function returning (Matcher, error) and panics if the error is non-nil.
// It is intended for use in variable initializations such as
//
//	var m = matcher.Must(matcher.New(matcher.FmtString, "hello world"))
func Must(m Matcher, err error) Matcher {
	if err != nil {
		panic(err)
	}
	return m
}

// New create a matcher
func New(format Format, expr string) (Matcher, error) {
	switch format {
	case FmtString:
		return NewStringMatcher(expr, true, true)
	case FmtGlob:
		return NewGlobMatcher(expr)
	case FmtRegExp:
		return NewRegExpMatcher(expr)
	case FmtSimplePattern:
		return NewSimplePatternsMatcher(expr)
	default:
		return nil, fmt.Errorf("unsupported matcher format: '%s'", format)
	}
}

// Parse parses line and returns appropriate matcher based on matched format.
//
// Short Syntax
//
//	<line>      ::= [ <not> ] <format> <space> <expr>
//	<not>       ::= '!'
//	                  negative expression
//	<format>    ::= [ '=', '~', '*' ]
//	                  '=' means string match
//	                  '~' means regexp match
//	                  '*' means glob match
//	<space>     ::= { ' ' | '\t' | '\n' | '\n' | '\r' }
//	<expr>      ::= any string
//
// Long Syntax
//
//	<line>      ::= [ <not> ] <format> <separator> <expr>
//	<format>    ::= [ 'string' | 'glob' | 'regexp' | 'simple_patterns' ]
//	<not>       ::= '!'
//	                  negative expression
//	<separator> ::= ':'
//	<expr>      ::= any string
func Parse(line string) (Matcher, error) {
	matcher, err := parseShortFormat(line)
	if err == nil {
		return matcher, nil
	}
	return parseLongSyntax(line)
}

func parseShortFormat(line string) (Matcher, error) {
	m := reShortSyntax.FindStringSubmatch(line)
	if m == nil {
		return nil, errNotShortSyntax
	}
	var format Format
	switch m[2] {
	case symString:
		format = FmtString
	case symGlob:
		format = FmtGlob
	case symRegExp:
		format = FmtRegExp
	default:
		return nil, fmt.Errorf("invalid short syntax: unknown symbol '%s'", m[2])
	}
	expr := m[3]
	matcher, err := New(format, expr)
	if err != nil {
		return nil, err
	}
	if m[1] != "" {
		matcher = Not(matcher)
	}
	return matcher, nil
}

func parseLongSyntax(line string) (Matcher, error) {
	m := reLongSyntax.FindStringSubmatch(line)
	if m == nil {
		return nil, fmt.Errorf("invalid syntax")
	}
	matcher, err := New(Format(m[2]), m[3])
	if err != nil {
		return nil, err
	}
	if m[1] != "" {
		matcher = Not(matcher)
	}
	return matcher, nil
}
