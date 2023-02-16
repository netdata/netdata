<!--
title: "BUFFER"
custom_edit_url: https://github.com/netdata/netdata/edit/master/libnetdata/buffer/README.md
sidebar_label: "BUFFER library"
learn_status: "Published"
learn_topic_type: "Tasks"
learn_rel_path: "Developers/libnetdata"
-->

# BUFFER

`BUFFER` is a convenience library for working with strings in `C`.
Mainly, `BUFFER`s eliminate the need for tracking the string length, thus providing
a safe alternative for string operations.

Also, they are super fast in printing and appending data to the string and its `buffer_strlen()`
is just a lookup (it does not traverse the string).

Netdata uses `BUFFER`s for preparing web responses and buffering data to be sent upstream or
to external databases.
