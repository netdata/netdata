# matcher
## Supported Format

* string
* glob
* regexp
* simple patterns

Depending on the symbol at the start of the string, the `matcher` will use one of the supported formats.

| matcher         | short format | long format       |
|-----------------|--------------|-------------------|
| string          | ` =`         | `string`          |
| glob            | `*`          | `glob`            |
| regexp          | `~`          | `regexp`          |
| simple patterns |              | `simple_patterns` |

Example:

- `* pattern`: It will use the `glob` matcher to find the `pattern` in the string.

### Syntax

**Tip**: Read `::=` as `is defined as`.

```
Short Syntax
     [ <not> ] <format> <space> <expr>
     
     <not>       ::= '!'
                       negative expression
     <format>    ::= [ '=', '~', '*' ]
                       '=' means string match
                       '~' means regexp match
                       '*' means glob match
     <space>     ::= { ' ' | '\t' | '\n' | '\n' | '\r' }
     <expr>      ::= any string

 Long Syntax
     [ <not> ] <format> <separator> <expr>
     
     <format>    ::= [ 'string' | 'glob' | 'regexp' | 'simple_patterns' ]
     <not>       ::= '!'
                       negative expression
     <separator> ::= ':'
     <expr>      ::= any string
```

When using the short syntax, you can enable the glob format by starting the string with a `*`, while in the long syntax
you need to define it more explicitly. The following examples are identical. `simple_patterns` can be used **only** with
the long syntax.

Examples:

- Short Syntax: `'* * '`
- Long Syntax: `'glob:*'`

### String matcher

The string matcher reports whether the given value equals to the string.

Examples:

- `'= foo'` matches only if the string is `foo`.
- `'!= bar'` matches any string that is not `bar`.

String matcher means **exact match** of the `string`. There are other string match related cases:

- string has prefix `something`
- string has suffix `something`
- string contains `something`

This is achievable using the `glob` matcher:

- `* PREFIX*`, means that it matches with any string that *starts* with `PREFIX`, e.g `PREFIXnetdata`
- `* *SUFFIX`, means that it matches with any string that *ends* with `SUFFIX`, e.g `netdataSUFFIX`
- `* *SUBSTRING*`, means that it matches with any string that *contains* `SUBSTRING`, e.g `netdataSUBSTRINGnetdata`

### Glob matcher

The glob matcher reports whether the given value matches the wildcard pattern. It uses the standard `golang`
library `path`. You can read more about the library in the [golang documentation](https://golang.org/pkg/path/#Match),
where you can also practice with the library in order to learn the syntax and use it in your Netdata configuration.

The pattern syntax is:

```
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
```

Examples:

- `* ?` matches any string that is a single character.
- `'?a'` matches any 2 character string that starts with any character and the second character is `a`, like `ba` but
  not `bb` or `bba`.
- `'[^abc]'` matches any character that is NOT a,b,c. `'[abc]'` matches only a, b, c.
- `'*[a-d]'` matches any string (`*`) that ends with a character that is between `a` and `d` (i.e `a,b,c,d`).

### Regexp matcher

The regexp matcher reports whether the given value matches the RegExp pattern ( use regexp.Match ).

The RegExp syntax is described at https://golang.org/pkg/regexp/syntax/.

Learn more about regular expressions at [RegexOne](https://regexone.com/).

### Simple patterns matcher

The simple patterns matcher reports whether the given value matches the simple patterns.

Simple patterns are a space separated list of words. Each word may use any number of wildcards `*`. Simple patterns
allow negative matches by prefixing a word with `!`.

Examples:

- `!*bad* *` matches anything, except all those that contain the word bad.
- `*foobar* !foo* !*bar *` matches everything containing foobar, except strings that start with foo or end with bar.




