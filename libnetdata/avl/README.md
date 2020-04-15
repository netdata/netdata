<!--
---
title: "AVL"
custom_edit_url: https://github.com/netdata/netdata/edit/master/libnetdata/avl/README.md
---
-->

# AVL

AVL is a library indexing objects in B-Trees.

`avl_insert()`, `avl_remove()` and `avl_search()` are adaptations
of the AVL algorithm found in `libavl` v2.0.3, so that they do not
use any memory allocations and their memory footprint is optimized
(by eliminating non-necessary data members).

In addition to the above, this version of AVL, provides versions using locks
and traversal functions.
[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Flibnetdata%2Favl%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
