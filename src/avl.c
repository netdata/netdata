// SPDX-License-Identifier: LGPL-3.0+

#include "common.h"

/* ------------------------------------------------------------------------- */
/*
 * avl_insert(), avl_remove() and avl_search()
 * are adaptations (by Costa Tsaousis) of the AVL algorithm found in libavl
 * v2.0.3, so that they do not use any memory allocations and their memory
 * footprint is optimized (by eliminating non-necessary data members).
 *
 * libavl - library for manipulation of binary trees.
 * Copyright (C) 1998, 1999, 2000, 2001, 2002, 2004 Free Software
 * Foundation, Inc.
*/


/* Search |tree| for an item matching |item|, and return it if found.
     Otherwise return |NULL|. */
avl *avl_search(avl_tree *tree, avl *item) {
    avl *p;

    // assert (tree != NULL && item != NULL);

    for (p = tree->root; p != NULL; ) {
        int cmp = tree->compar(item, p);

        if (cmp < 0)
            p = p->avl_link[0];
        else if (cmp > 0)
            p = p->avl_link[1];
        else /* |cmp == 0| */
            return p;
    }

    return NULL;
}

/* Inserts |item| into |tree| and returns a pointer to |item|'s address.
     If a duplicate item is found in the tree,
     returns a pointer to the duplicate without inserting |item|.
 */
avl *avl_insert(avl_tree *tree, avl *item) {
    avl *y, *z; /* Top node to update balance factor, and parent. */
    avl *p, *q; /* Iterator, and parent. */
    avl *n;     /* Newly inserted node. */
    avl *w;     /* New root of rebalanced subtree. */
    unsigned char dir; /* Direction to descend. */

    unsigned char da[AVL_MAX_HEIGHT]; /* Cached comparison results. */
    int k = 0;              /* Number of cached results. */

    // assert(tree != NULL && item != NULL);

    z = (avl *) &tree->root;
    y = tree->root;
    dir = 0;
    for (q = z, p = y; p != NULL; q = p, p = p->avl_link[dir]) {
        int cmp = tree->compar(item, p);
        if (cmp == 0)
            return p;

        if (p->avl_balance != 0)
            z = q, y = p, k = 0;
        da[k++] = dir = (unsigned char)(cmp > 0);
    }

    n = q->avl_link[dir] = item;

    // tree->avl_count++;
    n->avl_link[0] = n->avl_link[1] = NULL;
    n->avl_balance = 0;
    if (y == NULL) return n;

    for (p = y, k = 0; p != n; p = p->avl_link[da[k]], k++)
        if (da[k] == 0)
            p->avl_balance--;
        else
            p->avl_balance++;

    if (y->avl_balance == -2) {
        avl *x = y->avl_link[0];
        if (x->avl_balance == -1) {
            w = x;
            y->avl_link[0] = x->avl_link[1];
            x->avl_link[1] = y;
            x->avl_balance = y->avl_balance = 0;
        }
        else {
            // assert (x->avl_balance == +1);
            w = x->avl_link[1];
            x->avl_link[1] = w->avl_link[0];
            w->avl_link[0] = x;
            y->avl_link[0] = w->avl_link[1];
            w->avl_link[1] = y;
            if (w->avl_balance == -1)
                x->avl_balance = 0, y->avl_balance = +1;
            else if (w->avl_balance == 0)
                x->avl_balance = y->avl_balance = 0;
            else /* |w->avl_balance == +1| */
                x->avl_balance = -1, y->avl_balance = 0;
            w->avl_balance = 0;
        }
    }
    else if (y->avl_balance == +2) {
        avl *x = y->avl_link[1];
        if (x->avl_balance == +1) {
            w = x;
            y->avl_link[1] = x->avl_link[0];
            x->avl_link[0] = y;
            x->avl_balance = y->avl_balance = 0;
        }
        else {
            // assert (x->avl_balance == -1);
            w = x->avl_link[0];
            x->avl_link[0] = w->avl_link[1];
            w->avl_link[1] = x;
            y->avl_link[1] = w->avl_link[0];
            w->avl_link[0] = y;
            if (w->avl_balance == +1)
                x->avl_balance = 0, y->avl_balance = -1;
            else if (w->avl_balance == 0)
                x->avl_balance = y->avl_balance = 0;
            else /* |w->avl_balance == -1| */
                x->avl_balance = +1, y->avl_balance = 0;
            w->avl_balance = 0;
        }
    }
    else return n;

    z->avl_link[y != z->avl_link[0]] = w;

    // tree->avl_generation++;
    return n;
}

/* Deletes from |tree| and returns an item matching |item|.
     Returns a null pointer if no matching item found. */
avl *avl_remove(avl_tree *tree, avl *item) {
    /* Stack of nodes. */
    avl *pa[AVL_MAX_HEIGHT]; /* Nodes. */
    unsigned char da[AVL_MAX_HEIGHT];    /* |avl_link[]| indexes. */
    int k;                               /* Stack pointer. */

    avl *p;   /* Traverses tree to find node to delete. */
    int cmp;              /* Result of comparison between |item| and |p|. */

    // assert (tree != NULL && item != NULL);

    k = 0;
    p = (avl *) &tree->root;
    for(cmp = -1; cmp != 0; cmp = tree->compar(item, p)) {
        unsigned char dir = (unsigned char)(cmp > 0);

        pa[k] = p;
        da[k++] = dir;

        p = p->avl_link[dir];
        if(p == NULL) return NULL;
    }

    item = p;

    if (p->avl_link[1] == NULL)
        pa[k - 1]->avl_link[da[k - 1]] = p->avl_link[0];
    else {
        avl *r = p->avl_link[1];
        if (r->avl_link[0] == NULL) {
            r->avl_link[0] = p->avl_link[0];
            r->avl_balance = p->avl_balance;
            pa[k - 1]->avl_link[da[k - 1]] = r;
            da[k] = 1;
            pa[k++] = r;
        }
        else {
            avl *s;
            int j = k++;

            for (;;) {
                da[k] = 0;
                pa[k++] = r;
                s = r->avl_link[0];
                if (s->avl_link[0] == NULL) break;

                r = s;
            }

            s->avl_link[0] = p->avl_link[0];
            r->avl_link[0] = s->avl_link[1];
            s->avl_link[1] = p->avl_link[1];
            s->avl_balance = p->avl_balance;

            pa[j - 1]->avl_link[da[j - 1]] = s;
            da[j] = 1;
            pa[j] = s;
        }
    }

    // assert (k > 0);
    while (--k > 0) {
        avl *y = pa[k];

        if (da[k] == 0) {
            y->avl_balance++;
            if (y->avl_balance == +1) break;
            else if (y->avl_balance == +2) {
                avl *x = y->avl_link[1];
                if (x->avl_balance == -1) {
                    avl *w;
                    // assert (x->avl_balance == -1);
                    w = x->avl_link[0];
                    x->avl_link[0] = w->avl_link[1];
                    w->avl_link[1] = x;
                    y->avl_link[1] = w->avl_link[0];
                    w->avl_link[0] = y;
                    if (w->avl_balance == +1)
                        x->avl_balance = 0, y->avl_balance = -1;
                    else if (w->avl_balance == 0)
                        x->avl_balance = y->avl_balance = 0;
                    else /* |w->avl_balance == -1| */
                        x->avl_balance = +1, y->avl_balance = 0;
                    w->avl_balance = 0;
                    pa[k - 1]->avl_link[da[k - 1]] = w;
                }
                else {
                    y->avl_link[1] = x->avl_link[0];
                    x->avl_link[0] = y;
                    pa[k - 1]->avl_link[da[k - 1]] = x;
                    if (x->avl_balance == 0) {
                        x->avl_balance = -1;
                        y->avl_balance = +1;
                        break;
                    }
                    else x->avl_balance = y->avl_balance = 0;
                }
            }
        }
        else
        {
            y->avl_balance--;
            if (y->avl_balance == -1) break;
            else if (y->avl_balance == -2) {
                avl *x = y->avl_link[0];
                if (x->avl_balance == +1) {
                    avl *w;
                    // assert (x->avl_balance == +1);
                    w = x->avl_link[1];
                    x->avl_link[1] = w->avl_link[0];
                    w->avl_link[0] = x;
                    y->avl_link[0] = w->avl_link[1];
                    w->avl_link[1] = y;
                    if (w->avl_balance == -1)
                        x->avl_balance = 0, y->avl_balance = +1;
                    else if (w->avl_balance == 0)
                        x->avl_balance = y->avl_balance = 0;
                    else /* |w->avl_balance == +1| */
                        x->avl_balance = -1, y->avl_balance = 0;
                    w->avl_balance = 0;
                    pa[k - 1]->avl_link[da[k - 1]] = w;
                }
                else {
                    y->avl_link[0] = x->avl_link[1];
                    x->avl_link[1] = y;
                    pa[k - 1]->avl_link[da[k - 1]] = x;
                    if (x->avl_balance == 0) {
                        x->avl_balance = +1;
                        y->avl_balance = -1;
                        break;
                    }
                    else x->avl_balance = y->avl_balance = 0;
                }
            }
        }
    }

    // tree->avl_count--;
    // tree->avl_generation++;
    return item;
}

/* ------------------------------------------------------------------------- */
// below are functions by (C) Costa Tsaousis

// ---------------------------
// traversing

int avl_walker(avl *node, int (*callback)(void * /*entry*/, void * /*data*/), void *data) {
    int total = 0, ret = 0;

    if(node->avl_link[0]) {
        ret = avl_walker(node->avl_link[0], callback, data);
        if(ret < 0) return ret;
        total += ret;
    }

    ret = callback(node, data);
    if(ret < 0) return ret;
    total += ret;

    if(node->avl_link[1]) {
        ret = avl_walker(node->avl_link[1], callback, data);
        if (ret < 0) return ret;
        total += ret;
    }

    return total;
}

int avl_traverse(avl_tree *tree, int (*callback)(void * /*entry*/, void * /*data*/), void *data) {
    if(tree->root)
        return avl_walker(tree->root, callback, data);
    else
        return 0;
}

// ---------------------------
// locks

void avl_read_lock(avl_tree_lock *t) {
#ifndef AVL_WITHOUT_PTHREADS
#ifdef AVL_LOCK_WITH_MUTEX
    netdata_mutex_lock(&t->mutex);
#else
    netdata_rwlock_rdlock(&t->rwlock);
#endif
#endif /* AVL_WITHOUT_PTHREADS */
}

void avl_write_lock(avl_tree_lock *t) {
#ifndef AVL_WITHOUT_PTHREADS
#ifdef AVL_LOCK_WITH_MUTEX
    netdata_mutex_lock(&t->mutex);
#else
    netdata_rwlock_wrlock(&t->rwlock);
#endif
#endif /* AVL_WITHOUT_PTHREADS */
}

void avl_unlock(avl_tree_lock *t) {
#ifndef AVL_WITHOUT_PTHREADS
#ifdef AVL_LOCK_WITH_MUTEX
    netdata_mutex_unlock(&t->mutex);
#else
    netdata_rwlock_unlock(&t->rwlock);
#endif
#endif /* AVL_WITHOUT_PTHREADS */
}

// ---------------------------
// operations with locking

void avl_init_lock(avl_tree_lock *tree, int (*compar)(void * /*a*/, void * /*b*/)) {
    avl_init(&tree->avl_tree, compar);

#ifndef AVL_WITHOUT_PTHREADS
    int lock;

#ifdef AVL_LOCK_WITH_MUTEX
    lock = netdata_mutex_init(&tree->mutex, NULL);
#else
    lock = netdata_rwlock_init(&tree->rwlock);
#endif

    if(lock != 0)
        fatal("Failed to initialize AVL mutex/rwlock, error: %d", lock);

#endif /* AVL_WITHOUT_PTHREADS */
}

avl *avl_search_lock(avl_tree_lock *tree, avl *item) {
    avl_read_lock(tree);
    avl *ret = avl_search(&tree->avl_tree, item);
    avl_unlock(tree);
    return ret;
}

avl * avl_remove_lock(avl_tree_lock *tree, avl *item) {
    avl_write_lock(tree);
    avl *ret = avl_remove(&tree->avl_tree, item);
    avl_unlock(tree);
    return ret;
}

avl *avl_insert_lock(avl_tree_lock *tree, avl *item) {
    avl_write_lock(tree);
    avl * ret = avl_insert(&tree->avl_tree, item);
    avl_unlock(tree);
    return ret;
}

int avl_traverse_lock(avl_tree_lock *tree, int (*callback)(void * /*entry*/, void * /*data*/), void *data) {
    int ret;
    avl_read_lock(tree);
    ret = avl_traverse(&tree->avl_tree, callback, data);
    avl_unlock(tree);
    return ret;
}

void avl_init(avl_tree *tree, int (*compar)(void * /*a*/, void * /*b*/)) {
    tree->root = NULL;
    tree->compar = compar;
}

// ------------------
