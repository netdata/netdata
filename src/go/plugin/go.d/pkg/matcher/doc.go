// SPDX-License-Identifier: GPL-3.0-or-later

/*
Package matcher implements vary formats of string matcher.

Supported Format

	string
	glob
	regexp
	simple patterns

The string matcher reports whether the given value equals to the string ( use == ).

The glob matcher reports whether the given value matches the wildcard pattern.
The pattern syntax is:

	pattern:
	    { term }
	term:
	    '*'         matches any sequence of characters
	    '?'         matches any single character
	    '[' [ '^' ] { character-range } ']'
	    character class (must be non-empty)
	    c           matches character c (c != '*', '?', '\\', '[')
	    '\\' c      matches character c

	character-range:
	    c           matches character c (c != '\\', '-', ']')
	    '\\' c      matches character c
	    lo '-' hi   matches character c for lo <= c <= hi

The regexp matcher reports whether the given value matches the RegExp pattern ( use regexp.Match ).
The RegExp syntax is described at https://golang.org/pkg/regexp/syntax/.

The simple patterns matcher reports whether the given value matches the simple patterns.
The simple patterns is a custom format used in netdata,
it's syntax is described at https://docs.netdata.cloud/libnetdata/simple_pattern/.
*/
package matcher
