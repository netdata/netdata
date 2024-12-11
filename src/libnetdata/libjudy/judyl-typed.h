// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_JUDYL_TYPED_H
#define NETDATA_JUDYL_TYPED_H

#include <Judy.h>

#define DEFINE_JUDYL_TYPED(NAME, TYPE)                                           \
    _Static_assert(sizeof(TYPE) == sizeof(Word_t),                               \
                   #NAME "_type_must_have_same_size_as_Word_t");                 \
    typedef struct {                                                             \
        Pvoid_t judyl;                                                           \
    } NAME##_JudyLSet;                                                           \
                                                                                 \
    static inline void NAME##_INIT(NAME##_JudyLSet *set) {                       \
        set->judyl = NULL;                                                       \
    }                                                                            \
                                                                                 \
    static inline bool NAME##_SET(NAME##_JudyLSet *set, Word_t index, TYPE value) { \
        Pvoid_t *pValue = JudyLIns(&set->judyl, index, PJE0);                    \
        if (pValue == PJERR) return false;                                       \
        *pValue = (void *)(uintptr_t)value;                                      \
        return true;                                                             \
    }                                                                            \
                                                                                 \
    static inline TYPE NAME##_GET(NAME##_JudyLSet *set, Word_t index) {          \
        Pvoid_t *pValue = JudyLGet(set->judyl, index, PJE0);                     \
        return (pValue != NULL) ? (TYPE)(uintptr_t)(*pValue) : (TYPE)0;          \
    }                                                                            \
                                                                                 \
    static inline TYPE *NAME##_GETPTR(NAME##_JudyLSet *set, Word_t index) {      \
        Pvoid_t *pValue = JudyLGet(set->judyl, index, PJE0);                     \
        return (TYPE *)pValue;                                                   \
    }                                                                            \
                                                                                 \
    static inline bool NAME##_DEL(NAME##_JudyLSet *set, Word_t index) {          \
        int Rc;                                                                  \
        PPvoid_t ppJudy = &set->judyl;                                           \
        Rc = JudyLDel(ppJudy, index, PJE0);                                      \
        return Rc == 1;                                                          \
    }                                                                            \
                                                                                 \
    static inline TYPE NAME##_FIRST(NAME##_JudyLSet *set, Word_t *index) {       \
        Pvoid_t *pValue = JudyLFirst(set->judyl, index, PJE0);                   \
        return (pValue != NULL) ? (TYPE)(uintptr_t)(*pValue) : (TYPE)0;          \
    }                                                                            \
                                                                                 \
    static inline TYPE NAME##_NEXT(NAME##_JudyLSet *set, Word_t *index) {        \
        Pvoid_t *pValue = JudyLNext(set->judyl, index, PJE0);                    \
        return (pValue != NULL) ? (TYPE)(uintptr_t)(*pValue) : (TYPE)0;          \
    }                                                                            \
                                                                                 \
    static inline TYPE NAME##_LAST(NAME##_JudyLSet *set, Word_t *index) {        \
        Pvoid_t *pValue = JudyLLast(set->judyl, index, PJE0);                    \
        return (pValue != NULL) ? (TYPE)(uintptr_t)(*pValue) : (TYPE)0;          \
    }                                                                            \
                                                                                 \
    static inline TYPE NAME##_PREV(NAME##_JudyLSet *set, Word_t *index) {        \
        Pvoid_t *pValue = JudyLPrev(set->judyl, index, PJE0);                    \
        return (pValue != NULL) ? (TYPE)(uintptr_t)(*pValue) : (TYPE)0;          \
    }                                                                            \
                                                                                 \
    static inline void NAME##_FREE(NAME##_JudyLSet *set, void (*callback)(TYPE)) { \
        Word_t index = 0;                                                        \
        Pvoid_t *pValue;                                                         \
        if (callback) {                                                          \
            for (pValue = JudyLFirst(set->judyl, &index, PJE0);                  \
                 pValue != NULL;                                                 \
                 pValue = JudyLNext(set->judyl, &index, PJE0)) {                 \
                callback((TYPE)(uintptr_t)(*pValue));                            \
            }                                                                    \
        }                                                                        \
        JudyLFreeArray(&set->judyl, PJE0);                                       \
    }



#endif //NETDATA_JUDYL_TYPED_H
