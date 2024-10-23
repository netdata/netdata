// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_LINKED_LISTS_H
#define NETDATA_LINKED_LISTS_H

// ---------------------------------------------------------------------------------------------
// double linked list management
// inspired by https://github.com/troydhanson/uthash/blob/master/src/utlist.h

#define DOUBLE_LINKED_LIST_PREPEND_ITEM_UNSAFE(head, item, prev, next)                         \
    do {                                                                                       \
        (item)->next = (head);                                                                 \
                                                                                               \
        if(likely(head)) {                                                                     \
            (item)->prev = (head)->prev;                                                       \
            (head)->prev = (item);                                                             \
        }                                                                                      \
        else                                                                                   \
            (item)->prev = (item);                                                             \
                                                                                               \
        (head) = (item);                                                                       \
    } while (0)

#define DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(head, item, prev, next)                          \
    do {                                                                                       \
                                                                                               \
        (item)->next = NULL;                                                                   \
                                                                                               \
        if(likely(head)) {                                                                     \
            (item)->prev = (head)->prev;                                                       \
            (head)->prev->next = (item);                                                       \
            (head)->prev = (item);                                                             \
        }                                                                                      \
        else {                                                                                 \
            (item)->prev = (item);                                                             \
            (head) = (item);                                                                   \
        }                                                                                      \
                                                                                               \
    } while (0)

#define DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(head, item, prev, next)                          \
    do {                                                                                       \
        fatal_assert((head) != NULL);                                                          \
        fatal_assert((item)->prev != NULL);                                                    \
                                                                                               \
        if((item)->prev == (item))                                                             \
            /* it is the only item in the list */                                              \
            (head) = NULL;                                                                     \
                                                                                               \
        else if((item) == (head)) {                                                            \
            /* it is the first item */                                                         \
            fatal_assert((item)->next != NULL);                                                \
            (item)->next->prev = (item)->prev;                                                 \
            (head) = (item)->next;                                                             \
        }                                                                                      \
        else {                                                                                 \
            /* it is any other item */                                                         \
            (item)->prev->next = (item)->next;                                                 \
                                                                                               \
            if ((item)->next)                                                                  \
                (item)->next->prev = (item)->prev;                                             \
            else                                                                               \
                (head)->prev = (item)->prev;                                                   \
        }                                                                                      \
                                                                                               \
        (item)->next = NULL;                                                                   \
        (item)->prev = NULL;                                                                   \
    } while (0)

#define DOUBLE_LINKED_LIST_INSERT_ITEM_BEFORE_UNSAFE(head, existing, item, prev, next)         \
    do {                                                                                       \
        if (existing) {                                                                        \
            fatal_assert((head) != NULL);                                                      \
            fatal_assert((item) != NULL);                                                      \
                                                                                               \
            (item)->next = (existing);                                                         \
            (item)->prev = (existing)->prev;                                                   \
            (existing)->prev = (item);                                                         \
                                                                                               \
            if ((head) == (existing))                                                          \
                (head) = (item);                                                               \
            else                                                                               \
                (item)->prev->next = (item);                                                   \
                                                                                               \
        }                                                                                      \
        else                                                                                   \
            DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(head, item, prev, next);                     \
                                                                                               \
    } while (0)

#define DOUBLE_LINKED_LIST_INSERT_ITEM_AFTER_UNSAFE(head, existing, item, prev, next)          \
    do {                                                                                       \
        if (existing) {                                                                        \
            fatal_assert((head) != NULL);                                                      \
            fatal_assert((item) != NULL);                                                      \
                                                                                               \
            (item)->next = (existing)->next;                                                   \
            (item)->prev = (existing);                                                         \
            (existing)->next = (item);                                                         \
                                                                                               \
            if ((item)->next)                                                                  \
                (item)->next->prev = (item);                                                   \
            else                                                                               \
                (head)->prev = (item);                                                         \
        }                                                                                      \
        else                                                                                   \
            DOUBLE_LINKED_LIST_PREPEND_ITEM_UNSAFE(head, item, prev, next);                    \
                                                                                               \
    } while (0)

#define DOUBLE_LINKED_LIST_APPEND_LIST_UNSAFE(head, head2, prev, next)                         \
    do {                                                                                       \
        if (head2) {                                                                           \
            if (head) {                                                                        \
                __typeof(head2) _head2_last_item = (head2)->prev;                              \
                                                                                               \
                (head2)->prev = (head)->prev;                                                  \
                (head)->prev->next = (head2);                                                  \
                                                                                               \
                (head)->prev = _head2_last_item;                                               \
            }                                                                                  \
            else                                                                               \
                (head) = (head2);                                                              \
        }                                                                                      \
    } while (0)

#define DOUBLE_LINKED_LIST_FOREACH_FORWARD(head, var, prev, next)                              \
    for ((var) = (head); (var) ; (var) = (var)->next)

#define DOUBLE_LINKED_LIST_FOREACH_BACKWARD(head, var, prev, next)                             \
    for ((var) = (head) ? (head)->prev : NULL ; (var) ; (var) = ((var) == (head)) ? NULL : (var)->prev)

#endif //NETDATA_LINKED_LISTS_H
