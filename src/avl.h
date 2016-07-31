
#ifndef _AVL_H
#define _AVL_H 1

/* Maximum AVL tree height. */
#ifndef AVL_MAX_HEIGHT
#define AVL_MAX_HEIGHT 92
#endif

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
    struct avl *avl_link[2];  /* Subtrees. */
    signed char avl_balance;       /* Balance factor. */
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
avl *avl_insert_lock(avl_tree_lock *t, avl *a);
avl *avl_insert(avl_tree *t, avl *a);

/* Remove an element a from the AVL tree t
 * returns -1 if the depth of the tree has shrunk
 * Warning: if the element is not present in the tree,
 *          returns 0 as if it had been removed succesfully.
 */
avl *avl_remove_lock(avl_tree_lock *t, avl *a);
avl *avl_remove(avl_tree *t, avl *a);

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
