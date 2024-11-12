// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_JUDYL_TYPED_H
#define NETDATA_JUDYL_TYPED_H

#include <Judy.h>

#define DEFINE_JUDYL_TYPED(NAME, TYPE)                                           \
    typedef struct {                                                             \
        Pvoid_t judyl;                                                           \
    } NAME##_JudyLSet;                                                           \
                                                                                 \
    static inline void NAME##_Init(NAME##_JudyLSet *set) {                       \
        set->judyl = NULL;                                                       \
    }                                                                            \
                                                                                 \
    static inline bool NAME##_Add(NAME##_JudyLSet *set, Word_t index, TYPE value) { \
        Pvoid_t *pValue = JudyLIns(&set->judyl, index, PJE0);                    \
        if (pValue == PJERR) return false;                                       \
        *pValue = (void *)value;                                                 \
        return true;                                                             \
    }                                                                            \
                                                                                 \
    static inline TYPE NAME##_Get(NAME##_JudyLSet *set, Word_t index) {          \
        Pvoid_t *pValue = JudyLGet(set->judyl, index, PJE0);                     \
        return (pValue != NULL) ? (TYPE)(*pValue) : NULL;                        \
    }                                                                            \
                                                                                 \
    static inline bool NAME##_Remove(NAME##_JudyLSet *set, Word_t index) {       \
        int Rc;                                                                  \
        PPvoid_t ppJudy = &set->judyl;                                           \
        Rc = JudyLDel(ppJudy, index, PJE0);                                      \
        return Rc == 1;                                                          \
    }                                                                            \
                                                                                 \
    static inline TYPE NAME##_First(NAME##_JudyLSet *set, Word_t *index) {       \
        Pvoid_t *pValue = JudyLFirst(set->judyl, index, PJE0);                   \
        return (pValue != NULL) ? (TYPE)(*pValue) : NULL;                        \
    }                                                                            \
                                                                                 \
    static inline TYPE NAME##_Next(NAME##_JudyLSet *set, Word_t *index) {        \
        Pvoid_t *pValue = JudyLNext(set->judyl, index, PJE0);                    \
        return (pValue != NULL) ? (TYPE)(*pValue) : NULL;                        \
    }                                                                            \
                                                                                 \
    static inline TYPE NAME##_Last(NAME##_JudyLSet *set, Word_t *index) {        \
        Pvoid_t *pValue = JudyLLast(set->judyl, index, PJE0);                    \
        return (pValue != NULL) ? (TYPE)(*pValue) : NULL;                        \
    }                                                                            \
                                                                                 \
    static inline TYPE NAME##_Prev(NAME##_JudyLSet *set, Word_t *index) {        \
        Pvoid_t *pValue = JudyLPrev(set->judyl, index, PJE0);                    \
        return (pValue != NULL) ? (TYPE)(*pValue) : NULL;                        \
    }                                                                            \
                                                                                 \
    static inline void NAME##_Free(NAME##_JudyLSet *set, void (*callback)(TYPE)) { \
        Word_t index = 0;                                                        \
        Pvoid_t *pValue;                                                         \
        if (callback) {                                                          \
            for (pValue = JudyLFirst(set->judyl, &index, PJE0);                  \
                 pValue != NULL;                                                 \
                 pValue = JudyLNext(set->judyl, &index, PJE0)) {                 \
                callback((TYPE)(*pValue));                                       \
            }                                                                    \
        }                                                                        \
        JudyLFreeArray(&set->judyl, PJE0);                                       \
    }



#endif //NETDATA_JUDYL_TYPED_H
