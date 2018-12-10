## netdata simple patterns

Unix prefers regular expressions. But they are just too hard, too cryptic
to use, write and understand.

So, netdata supports **simple patterns**.

Simple patterns are a space separated list of words, that can have `*`
as a wildcard. Each world may use any number of `*`. Simple patterns
allow **negative** matches by prefixing a word with `!`.

So, `pattern = !*bad* *` will match anything, except all those that
contain the word `bad`. 

Simple patterns are quite powerful: `pattern = *foobar* !foo* !*bar *`
matches everything containing `foobar`, except strings that start
with `foo` or end with `bar`.

You can use the netdata command line to check simple patterns,
like this:

```sh
# netdata -W simple-pattern '*foobar* !foo* !*bar *' 'hello world'
RESULT: MATCHED - pattern '*foobar* !foo* !*bar *' matches 'hello world'

# netdata -W simple-pattern '*foobar* !foo* !*bar *' 'hello world bar'
RESULT: NOT MATCHED - pattern '*foobar* !foo* !*bar *' does not match 'hello world bar'

# netdata -W simple-pattern '*foobar* !foo* !*bar *' 'hello world foobar'
RESULT: MATCHED - pattern '*foobar* !foo* !*bar *' matches 'hello world foobar'
```

netdata stops processing to the first positive or negative match
(left to right). If it is not matched by either positive or negative
patterns, it is denied at the end.


[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Flibnetdata%2Fsimple_pattern%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()
