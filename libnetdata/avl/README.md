# AVL

AVL is a library indexing objects in B-Trees.

`avl_insert()`, `avl_remove()` and `avl_search()` are adaptations
of the AVL algorithm found in `libavl` v2.0.3, so that they do not
use any memory allocations and their memory footprint is optimized
(by eliminating non-necessary data members).

In addition to the above, this version of AVL, provides versions using locks
and traversal functions.