<!--
title: "AVL"
custom_edit_url: https://github.com/netdata/netdata/edit/master/libnetdata/avl/README.md
sidebar_label: "AVL"
learn_status: "Published"
learn_topic_type: "Tasks"
learn_rel_path: "Developers/libnetdata"
-->

# AVL

AVL is a library indexing objects in B-Trees.

`avl_insert()`, `avl_remove()` and `avl_search()` are adaptations
of the AVL algorithm found in `libavl` v2.0.3, so that they do not
use any memory allocations and their memory footprint is optimized
(by eliminating non-necessary data members).

In addition to the above, this version of AVL, provides versions using locks
and traversal functions.

