/*
 * ANSI C Library for maintainance of AVL Balanced Trees
 *
 * ref.:
 *  G. M. Adelson-Velskij & E. M. Landis
 *  Doklady Akad. Nauk SSSR 146 (1962), 263-266
 *
 * see also:
 *  D. E. Knuth: The Art of Computer Programming Vol.3 (Sorting and Searching)
 *
 * (C) 2000 Daniel Nagy, Budapest University of Technology and Economics
 * Released under GNU General Public License (GPL) version 2
 *
 */
#ifndef _AVL_H
#define _AVL_H 1

#ifndef AVL_WITHOUT_PTHREADS
#include <pthread.h>

// #define AVL_LOCK_WITH_MUTEX 1

#ifdef AVL_LOCK_WITH_MUTEX
#define AVL_LOCK_INITIALIZER PTHREAD_MUTEX_INITIALIZER
#else /* AVL_LOCK_WITH_MUTEX */
#define AVL_LOCK_INITIALIZER PTHREAD_RWLOCK_INITIALIZER
#endif /* AVL_LOCK_WITH_MUTEX */

#else /* AVL_WITHOUT_PTHREADS */
#define AVL_LOCK_INITIALIZER
#endif /* AVL_WITHOUT_PTHREADS */

/* Data structures */

/* One element of the AVL tree */
typedef struct avl {
	struct avl* left;
	struct avl* right;
	signed char balance;
} avl;

/* An AVL tree */
typedef struct avl_tree {
	avl *root;

	int (*compar)(void *a, void *b);
} avl_tree;

typedef struct avl_tree_lock {
	avl_tree avl_tree;

#ifndef AVL_WITHOUT_PTHREADS
#ifdef AVL_LOCK_WITH_MUTEX
	pthread_mutex_t mutex;
#else /* AVL_LOCK_WITH_MUTEX */
	pthread_rwlock_t rwlock;
#endif /* AVL_LOCK_WITH_MUTEX */
#endif /* AVL_WITHOUT_PTHREADS */
} avl_tree_lock;

/* Public methods */

/* Insert element a into the AVL tree t
 * returns 1 if the depth of the tree has grown
 * Warning: do not insert elements already present
 */
int avl_insert_lock(avl_tree_lock *t, avl *a);
int avl_insert(avl_tree *t, avl *a);

/* Remove an element a from the AVL tree t
 * returns -1 if the depth of the tree has shrunk
 * Warning: if the element is not present in the tree,
 *          returns 0 as if it had been removed succesfully.
 */
int avl_remove_lock(avl_tree_lock *t, avl *a);
int avl_remove(avl_tree *t, avl *a);

/* Remove the root of the AVL tree t
 * Warning: dumps core if t is empty
 */
int avl_removeroot_lock(avl_tree_lock *t);
int avl_removeroot(avl_tree *t);

/* Iterate through elements in t from a range between a and b (inclusive)
 * for each element calls iter(a) until it returns 0
 * returns the last value returned by iterator or 0 if there were no calls
 * Warning: a<=b must hold
 */
int avl_range_lock(avl_tree_lock *t, avl *a, avl *b, int (*iter)(avl *), avl **ret);
int avl_range(avl_tree *t, avl *a, avl *b, int (*iter)(avl *), avl **ret);

/* Iterate through elements in t equal to a
 * for each element calls iter(a) until it returns 0
 * returns the last value returned by iterator or 0 if there were no calls
 */
avl *avl_search_lock(avl_tree_lock *t, avl *a);
avl *avl_search(avl_tree *t, avl *a);

/* Initialize the avl_tree_lock
 */
void avl_init_lock(avl_tree_lock *t, int (*compar)(void *a, void *b));
void avl_init(avl_tree *t, int (*compar)(void *a, void *b));

#endif /* avl.h */
