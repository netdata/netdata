// SPDX-License-Identifier: LGPL-3.0-or-later

#ifndef _AVL_H
#define _AVL_H 1

#include "../libnetdata.h"

/* Maximum AVL tree height. */
#ifndef AVL_MAX_HEIGHT
#define AVL_MAX_HEIGHT 92
#endif

#if defined(AVL_LOCK_WITH_RWLOCK)
#define AVL_LOCK_INITIALIZER NETDATA_RWLOCK_INITIALIZER
#else
#define AVL_LOCK_INITIALIZER RW_SPINLOCK_INITIALIZER
#endif

/* Data structures */

/* One element of the AVL tree */
typedef struct avl_element {
    struct avl_element *avl_link[2];  /* Subtrees. */
    signed char avl_balance;       /* Balance factor. */
} avl_t;

typedef struct __attribute__((packed)) avl_element_packed {
    struct avl_element *avl_link[2];  /* Subtrees. */
    signed char avl_balance;       /* Balance factor. */
} avl_t_packed;

/* An AVL tree */
typedef struct avl_tree_type {
    avl_t *root;
    int (*compar)(void *a, void *b);
} avl_tree_type;

typedef struct avl_tree_lock {
    avl_tree_type avl_tree;

#if defined(AVL_LOCK_WITH_RWLOCK)
    netdata_rwlock_t rwlock;
#else
    RW_SPINLOCK rwlock;
#endif
} avl_tree_lock;

/* Public methods */

/* Insert element a into the AVL tree t
 * returns the added element a, or a pointer the
 * element that is equal to a (as returned by t->compar())
 * a is linked directly to the tree, so it has to
 * be properly allocated by the caller.
 */
avl_t *avl_insert_lock(avl_tree_lock *tree, avl_t *item) NEVERNULL WARNUNUSED;
avl_t *avl_insert(avl_tree_type *tree, avl_t *item) NEVERNULL WARNUNUSED;

/* Remove an element a from the AVL tree t
 * returns a pointer to the removed element
 * or NULL if an element equal to a is not found
 * (equal as returned by t->compar())
 */
avl_t *avl_remove_lock(avl_tree_lock *tree, avl_t *item) WARNUNUSED;
avl_t *avl_remove(avl_tree_type *tree, avl_t *item) WARNUNUSED;

/* Find the element into the tree that equal to a
 * (equal as returned by t->compar())
 * returns NULL is no element is equal to a
 */
avl_t *avl_search_lock(avl_tree_lock *tree, avl_t *item);
avl_t *avl_search(avl_tree_type *tree, avl_t *item);

/* Initialize the avl_tree_lock
 */
void avl_init_lock(avl_tree_lock *tree, int (*compar)(void *a, void *b));
void avl_init(avl_tree_type *tree, int (*compar)(void *a, void *b));

/* Destroy the avl_tree_lock locks
 */
void avl_destroy_lock(avl_tree_lock *tree);

int avl_traverse_lock(avl_tree_lock *tree, int (*callback)(void *entry, void *data), void *data);
int avl_traverse(avl_tree_type *tree, int (*callback)(void *entry, void *data), void *data);

#endif /* avl.h */
