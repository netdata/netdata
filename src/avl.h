
#ifndef _AVL_H
#define _AVL_H 1

/**
 * @file avl.h
 * @brief This file holds the API fo AVL Trees.
 * @author Costa Tsaousis
 *
 * There are two versions of every method. The ones with suffix `_lock` are thread save.
 * 
 * To initialize an AVL tree use avl_init().
 * 
 * avl_insert(), avl_remove() and avl_search()
 * are adaptations (by Costa Tsaousis) of the AVL algorithm found in libavl
 * v2.0.3, so that they do not use any memory allocations and their memory
 * footprint is optimized (by eliminating non-necessary data members).
 *
 * libavl - library for manipulation of binary trees.
 * Copyright (C) 1998, 1999, 2000, 2001, 2002, 2004 Free Software
 * Foundation, Inc.
 * GNU Lesser General Public License
*/

#ifndef AVL_MAX_HEIGHT
#define AVL_MAX_HEIGHT 92 ///<  Maximum AVL tree height

#endif

#ifndef AVL_WITHOUT_PTHREADS
#include <pthread.h>

// #define AVL_LOCK_WITH_MUTEX 1

#ifdef AVL_LOCK_WITH_MUTEX
/// Initializes avl_tree_lock->mutex
#define AVL_LOCK_INITIALIZER PTHREAD_MUTEX_INITIALIZER
#else /* AVL_LOCK_WITH_MUTEX */
/// Initializes avl_tree_lock->rwlock
#define AVL_LOCK_INITIALIZER PTHREAD_RWLOCK_INITIALIZER
#endif /* AVL_LOCK_WITH_MUTEX */

#else /* AVL_WITHOUT_PTHREADS */
#define AVL_LOCK_INITIALIZER
#endif /* AVL_WITHOUT_PTHREADS */

// ----------------------------------------------------------------------------
// Data structures

/** One element of the AVL tree. */
typedef struct avl {
    struct avl *avl_link[2];  ///< Subtrees
    signed char avl_balance;  ///< Balance factor
} avl;

/** An AVL tree */
typedef struct avl_tree {
    avl *root; ///< root element
    int (*compar)(void *a, void *b); ///< Sort function
} avl_tree;

/** A synchronized AVL tree. */
typedef struct avl_tree_lock {
    avl_tree avl_tree; ///< An AVL tree

#ifndef AVL_WITHOUT_PTHREADS
#ifdef AVL_LOCK_WITH_MUTEX
    pthread_mutex_t mutex;
#else /* AVL_LOCK_WITH_MUTEX */
    pthread_rwlock_t rwlock; ///< read write lock
#endif /* AVL_LOCK_WITH_MUTEX */
#endif /* AVL_WITHOUT_PTHREADS */
} avl_tree_lock;

// ----------------------------------------------------------------------------
// Public methods

/** 
 * Insert `a` into AVL tree `t`.
 *
 * Insert element `a` into the AVL tree `t`.
 * If an element equal (as returned by `t->compar()`) to `a` exits in the tree, `a` is not added.
 * `a` is linked directly to the tree, so it has to be properly allocated by the caller.
 *
 * @param t avl tree
 * @param a element to insert
 * @return `a` or an equal element
 */
avl *avl_insert_lock(avl_tree_lock *t, avl *a) NEVERNULL WARNUNUSED;
/** 
 * Insert `a` into AVL tree `t`.
 *
 * Insert element `a` into the AVL tree `t`.
 * If an element equal (as returned by `t->compar()`) to `a` exits in the tree, `a` is not added.
 * `a` is linked directly to the tree, so it has to be properly allocated by the caller.
 *
 * @param t AVL tree
 * @param a element to insert
 * @return `a` or an equal element
 */
avl *avl_insert(avl_tree *t, avl *a) NEVERNULL WARNUNUSED;

/** 
 * Remove `a` from AVL tree `t`.
 *
 * Remove an element equal (as returned by `t->compar()`) to `a` from the AVL tree `t`.
 *
 * @param t AVL tree
 * @param a element to remove
 * @return the removed element or NULL if not found
 */
avl *avl_remove_lock(avl_tree_lock *t, avl *a) WARNUNUSED;
/** 
 * Remove `a` from AVL tree `t`.
 *
 * Remove an element equal (as returned by `t->compar()`) to `a` from the AVL tree `t`.
 *
 * @param t AVL tree
 * @param a element to remove
 * @return the removed element or NULL if not found
 */
avl *avl_remove(avl_tree *t, avl *a) WARNUNUSED;

/** 
 * Find `a` in AVL tree `t`.
 * 
 * Find a element equal (as returned by `t->compar()`) to `a` in AVL tree `t`.
 *
 * @param a element to find
 * @param t AVL tree
 * @return found element or NULL
 */
avl *avl_search_lock(avl_tree_lock *t, avl *a);
/** 
 * Find `a` in AVL tree `t`.
 * 
 * Find a element equal (as returned by `t->compar()`) to `a` in AVL tree `t`.
 *
 * @param a element to find
 * @param t AVL tree
 * @return found element or NULL
 */
avl *avl_search(avl_tree *t, avl *a);

/**
 * Initialize a avl_tree_lock.
 *
 * @param t AVL tree
 * @param compar() Comparison function
 */
void avl_init_lock(avl_tree_lock *t, int (*compar)(void *a, void *b));
/**
 * Initialize a avl_tree_lock.
 *
 * @param t AVL tree
 * @param compar() Comparison function
 */
void avl_init(avl_tree *t, int (*compar)(void *a, void *b));


/**
 * Traverse `avl_tree_lock` while `callback` is positive.
 *
 * Walk through all entries in the AVL tree. 
 * The function `int callback(entry, data)` will be called for each node (`data` is the `data` parameter to the traverse function).
 *
 * The return value of callback() is interpreted as follows:
 * - zero, or positive = add it to `t`.
 * - negative = stop traversing the tree.
 *
 * If no call to callback returns a negative number, the return value of the traverse function is `t`.
 * If any call to callback returns a negative number, the return value is the number returned by that callback function.
 *
 * Nobody verified, if this calls the callback function in a sorted order.
 * It should, but since netdata uses these AVL trees with string hashes - so sorting them based on the hashes is meaningless.
 *
 * The tree is locked for the entire walk through. Use this with care.
 *
 * @author Costa Tsaousis
 *
 * @param t Destination.
 * @param callback function called on each node
 * @param data second argument of `callback(entry, data)`
 * @return `t` on success, negative number on failure.
 */
int avl_traverse_lock(avl_tree_lock *t, int (*callback)(void *entry, void *data), void *data);
/**
 * Traverse `avl_tree_lock` while `callback` is positive.
 *
 * Walk through all entries in the AVL tree. 
 * The function `int callback(entry, data)` will be called for each node (`data` is the `data` parameter to the traverse function).
 *
 * The return value of callback() is interpreted as follows:
 * - zero, or positive = add it to `t`.
 * - negative = stop traversing the tree.
 *
 * If no call to callback returns a negative number, the return value of the traverse function is `t`.
 * If any call to callback returns a negative number, the return value is the number returned by that callback function.
 *
 * Nobody verified, if this calls the callback function in a sorted order.
 * It should, but since netdata uses these AVL trees with string hashes - so sorting them based on the hashes is meaningless.
 *
 * @author Costa Tsaousis
 *
 * @param t Destination.
 * @param callback function called on each node
 * @param data second argument of `callback(entry, data)`
 * @return `t` on success, negative number on failure.
 */
int avl_traverse(avl_tree *t, int (*callback)(void *entry, void *data), void *data);

#endif /* avl.h */
