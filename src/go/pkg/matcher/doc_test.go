// SPDX-License-Identifier: GPL-3.0-or-later

package matcher_test

import (
	"github.com/netdata/netdata/go/plugins/pkg/matcher"
)

func ExampleNew_string_format() {
	// create a string matcher, which perform full text match
	m, err := matcher.New(matcher.FmtString, "hello")
	if err != nil {
		panic(err)
	}
	m.MatchString("hello")       // => true
	m.MatchString("hello world") // => false
}

func ExampleNew_glob_format() {
	// create a glob matcher, which perform wildcard match
	m, err := matcher.New(matcher.FmtString, "hello*")
	if err != nil {
		panic(err)
	}
	m.MatchString("hello")       // => true
	m.MatchString("hello world") // => true
	m.MatchString("Hello world") // => false
}

func ExampleNew_simple_patterns_format() {
	// create a simple patterns matcher, which perform wildcard match
	m, err := matcher.New(matcher.FmtSimplePattern, "hello* !*world *")
	if err != nil {
		panic(err)
	}
	m.MatchString("hello")        // => true
	m.MatchString("hello world")  // => true
	m.MatchString("Hello world")  // => false
	m.MatchString("Hello world!") // => false
}

func ExampleNew_regexp_format() {
	// create a regexp matcher, which perform wildcard match
	m, err := matcher.New(matcher.FmtRegExp, "[0-9]+")
	if err != nil {
		panic(err)
	}
	m.MatchString("1")  // => true
	m.MatchString("1a") // => true
	m.MatchString("a")  // => false
}
