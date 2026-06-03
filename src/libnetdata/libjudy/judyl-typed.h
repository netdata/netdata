// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_JUDYL_TYPED_H
#define NETDATA_JUDYL_TYPED_H

#include <Judy.h>

#ifdef __cplusplus
#define DEFINED_JUDYL_CHECK_SIZE(TYPE, NAME)
#else
// Vendored Judy types Word_t as `unsigned long`. On Linux/macOS LP64
// that is pointer-sized; on Windows LLP64 it is 32 bits while pointers
// are 64. JudyL values are actually stored through `Pvoid_t` (a `void *`
// slot), so the relevant capacity check is `sizeof(TYPE) <= sizeof(void *)`.
// Using sizeof(Word_t) here trips the assertion on 64-bit Windows for any
// pointer-typed value, even though the storage slot is wide enough.
#define DEFINED_JUDYL_CHECK_SIZE(TYPE, NAME) _Static_assert(sizeof(TYPE) <= sizeof(void *), #NAME "_type_must_fit_in_a_pointer_slot")
#endif


// Advanced macro for types requiring conversion
#define DEFINE_JUDYL_TYPED_ADVANCED(NAME, TYPE, PACK_MACRO, UNPACK_MACRO, EXTRA_MEMBERS)                \
    DEFINED_JUDYL_CHECK_SIZE(TYPE, NAME);                                                               \
                                                                                                        \
    typedef struct {                                                                                    \
        Pvoid_t judyl;                                                                                  \
        EXTRA_MEMBERS                                                                                   \
    } NAME##_JudyLSet;                                                                                  \
    ALWAYS_INLINE                                                                                       \
    static __attribute__((unused)) void NAME##_INIT(NAME##_JudyLSet *set) {                             \
        set->judyl = NULL;                                                                              \
    }                                                                                                   \
                                                                                                        \
    ALWAYS_INLINE                                                                                       \
    static bool __attribute__((unused)) NAME##_SET(NAME##_JudyLSet *set, Word_t index, TYPE value) {    \
        Pvoid_t *pValue = JudyLIns(&set->judyl, index, PJE0);                                           \
        if (pValue == PJERR) return false;                                                              \
        *pValue = (void *)PACK_MACRO(value);                                                            \
        return true;                                                                                    \
    }                                                                                                   \
                                                                                                        \
    ALWAYS_INLINE                                                                                       \
    static TYPE __attribute__((unused)) NAME##_GET(NAME##_JudyLSet *set, Word_t index) {                \
        Pvoid_t *pValue = JudyLGet(set->judyl, index, PJE0);                                            \
        return (pValue != NULL) ? (TYPE)UNPACK_MACRO(*pValue) : (TYPE){0};                              \
    }                                                                                                   \
                                                                                                        \
    ALWAYS_INLINE                                                                                       \
    static bool __attribute__((unused)) NAME##_DEL(NAME##_JudyLSet *set, Word_t index) {                \
        return JudyLDel(&set->judyl, index, PJE0) == 1;                                                 \
    }                                                                                                   \
                                                                                                        \
    ALWAYS_INLINE                                                                                       \
    static TYPE __attribute__((unused)) NAME##_FIRST(NAME##_JudyLSet *set, Word_t *index) {             \
        Pvoid_t *pValue = JudyLFirst(set->judyl, index, PJE0);                                          \
        return (pValue != NULL) ? (TYPE)UNPACK_MACRO(*pValue) : (TYPE){0};                              \
    }                                                                                                   \
                                                                                                        \
    ALWAYS_INLINE                                                                                       \
    static TYPE __attribute__((unused)) NAME##_NEXT(NAME##_JudyLSet *set, Word_t *index) {              \
        Pvoid_t *pValue = JudyLNext(set->judyl, index, PJE0);                                           \
        return (pValue != NULL) ? (TYPE)UNPACK_MACRO(*pValue) : (TYPE){0};                              \
    }                                                                                                   \
                                                                                                        \
    ALWAYS_INLINE                                                                                       \
    static TYPE __attribute__((unused)) NAME##_LAST(NAME##_JudyLSet *set, Word_t *index) {              \
        Pvoid_t *pValue = JudyLLast(set->judyl, index, PJE0);                                           \
        return (pValue != NULL) ? (TYPE)UNPACK_MACRO(*pValue) : (TYPE){0};                              \
    }                                                                                                   \
                                                                                                        \
    ALWAYS_INLINE                                                                                       \
    static TYPE __attribute__((unused)) NAME##_PREV(NAME##_JudyLSet *set, Word_t *index) {              \
        Pvoid_t *pValue = JudyLPrev(set->judyl, index, PJE0);                                           \
        return (pValue != NULL) ? (TYPE)UNPACK_MACRO(*pValue) : (TYPE){0};                              \
    }                                                                                                   \
                                                                                                        \
    ALWAYS_INLINE                                                                                       \
    static void __attribute__((unused)) NAME##_FREE(NAME##_JudyLSet *set, void (*callback)(Word_t, TYPE, void *), void *data) { \
        Word_t index = 0;                                                                               \
        Pvoid_t *pValue;                                                                                \
        if (callback) {                                                                                 \
            for (pValue = JudyLFirst(set->judyl, &index, PJE0);                                         \
                 pValue != NULL;                                                                        \
                 pValue = JudyLNext(set->judyl, &index, PJE0)) {                                        \
                callback(index, (TYPE)UNPACK_MACRO(*pValue), data);                                     \
            }                                                                                           \
        }                                                                                               \
        JudyLFreeArray(&set->judyl, PJE0);                                                              \
    }

// Basic macro for types with no conversion
#define JUDYL_TYPED_NO_CONVERSION(value) (uintptr_t)(value)

#define DEFINE_JUDYL_TYPED(NAME, TYPE)                                                                  \
    DEFINE_JUDYL_TYPED_ADVANCED(NAME, TYPE, JUDYL_TYPED_NO_CONVERSION, JUDYL_TYPED_NO_CONVERSION,)

#endif //NETDATA_JUDYL_TYPED_H
