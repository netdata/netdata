<!--
---
title: "BUFFER"
custom_edit_url: https://github.com/netdata/netdata/edit/master/libnetdata/buffer/README.md
---
-->

# BUFFER

`BUFFER` is a convenience library for working with strings in `C`.
Mainly, `BUFFER`s eliminate the need for tracking the string length, thus providing
a safe alternative for string operations.

Also, they are super fast in printing and appending data to the string and its `buffer_strlen()`
is just a lookup (it does not traverse the string).

Netdata uses `BUFFER`s for preparing web responses and buffering data to be sent upstream or
to backend databases.
[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Flibnetdata%2Fbuffer%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
