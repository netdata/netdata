# Simple patterns

Unix prefers regular expressions. But they are just too hard, too cryptic to use, write and understand.

So, Netdata supports **simple patterns**.

Simple patterns are a space separated list of words, that can have `*` as a wildcard. Each word may use any number of
`*`. Simple patterns allow **negative** matches by prefixing a word with `!`.

So, `pattern = !*bad* *` will match anything, except all those that contain the word `bad`.

Simple patterns are quite powerful: `pattern = !foo* !*bar *foobar*` matches everything containing `foobar`, except
strings that start with `foo` or end with `bar`.

You can use the Netdata command line to check simple patterns, like this:

```sh
# netdata -W simple-pattern '!foo* !*bar *foobar*' 'hello world'
RESULT: NOT MATCHED - pattern '!foo* !*bar *foobar*' does not match 'hello world', wildcarded ''

# netdata -W simple-pattern '!foo* !*bar *foobar*' 'hello world bar'
RESULT: NEGATIVE MATCHED - pattern '!foo* !*bar *foobar*' matches 'hello world bar', wildcarded 'hello world '

# netdata -W simple-pattern '!foo* !*bar *foobar*' 'hello foobar world'
RESULT: POSITIVE MATCHED - pattern '!foo* !*bar *foobar*' matches 'hello foobar world', wildcarded 'hello  world'
```

Netdata stops processing at the first positive or negative match (left to right). If it is not matched by either
positive or negative patterns, it is denied at the end.

## Indexed simple patterns

`SIMPLE_PATTERN_INDEX` maps each interned `STRING` key to one or more
non-owned user pointers. Exact, case-sensitive literal lists use direct key
lookups. Patterns and case-insensitive expressions scan the indexed keys.

The normal left-to-right SIMPLE_PATTERN result is evaluated independently for
each indexed key. When one user pointer is present under multiple keys, the
index combines those per-key results as follows:

- at least one positive key match is required;
- a negative key match vetoes that user pointer, even when another key matches
  positively; and
- each user pointer is returned at most once.

This makes aliases equal while retaining SIMPLE_PATTERN's first-match behavior
for each individual key.

Callers can replace every key for one user pointer under a single index write
lock. This keeps readers from observing a partially updated alias set during
identity changes.

Short-lived queries can parse patterns and collect index matches in their
caller-owned `ONEWAYALLOC`. Persistent patterns and the permanent index remain
heap-backed. An OWA-backed result must not outlive its arena.
