<!--
title: "Simple patterns"
description: "Netdata supports simple patterns, which are less cryptic versions of regular expressions. Use familiar notation for powerful results."
custom_edit_url: https://github.com/netdata/netdata/edit/master/libnetdata/simple_pattern/README.md
sidebar_label: "Simple patterns"
learn_status: "Published"
learn_topic_type: "Tasks"
learn_rel_path: "Developers"
-->

# Simple patterns

Unix prefers regular expressions. But they are just too hard, too cryptic
to use, write and understand.

So, Netdata supports **simple patterns**.

Simple patterns are a space separated list of words, that can have `*`
as a wildcard. Each world may use any number of `*`. Simple patterns
allow **negative** matches by prefixing a word with `!`.

So, `pattern = !*bad* *` will match anything, except all those that
contain the word `bad`. 

Simple patterns are quite powerful: `pattern = *foobar* !foo* !*bar *`
matches everything containing `foobar`, except strings that start
with `foo` or end with `bar`.

You can use the Netdata command line to check simple patterns,
like this:

```sh
# netdata -W simple-pattern '*foobar* !foo* !*bar *' 'hello world'
RESULT: MATCHED - pattern '*foobar* !foo* !*bar *' matches 'hello world'

# netdata -W simple-pattern '*foobar* !foo* !*bar *' 'hello world bar'
RESULT: NOT MATCHED - pattern '*foobar* !foo* !*bar *' does not match 'hello world bar'

# netdata -W simple-pattern '*foobar* !foo* !*bar *' 'hello world foobar'
RESULT: MATCHED - pattern '*foobar* !foo* !*bar *' matches 'hello world foobar'
```

Netdata stops processing to the first positive or negative match
(left to right). If it is not matched by either positive or negative
patterns, it is denied at the end.


