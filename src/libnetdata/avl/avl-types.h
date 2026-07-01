// SPDX-License-Identifier: LGPL-3.0-or-later

#ifndef NETDATA_AVL_TYPES_H
#define NETDATA_AVL_TYPES_H 1

/* One element of the AVL tree */
typedef struct avl_element {
    struct avl_element *avl_link[2];  /* Subtrees. */
    signed char avl_balance;       /* Balance factor. */
} avl_t;

typedef struct __attribute__((packed)) avl_element_packed {
    struct avl_element *avl_link[2];  /* Subtrees. */
    signed char avl_balance;       /* Balance factor. */
} avl_t_packed;

#endif /* avl-types.h */
