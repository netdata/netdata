// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef LIBNETDATA_BASE_H
#define LIBNETDATA_BASE_H

#include "config.h"
#include <stdbool.h>
#include <stddef.h>
#if defined(HAVE_INTTYPES_H)
#include <inttypes.h>
#elif defined(HAVE_STDINT_H)
#include <stdint.h>
#endif

#if defined(NETDATA_DEV_MODE) && !defined(NETDATA_INTERNAL_CHECKS)
#define NETDATA_INTERNAL_CHECKS 1
#endif

#ifndef SIZEOF_VOID_P
#error SIZEOF_VOID_P is not defined
#endif

#if SIZEOF_VOID_P == 4
#define ENV32BIT 1
#else
#define ENV64BIT 1
#endif

#ifdef HAVE_LIBDATACHANNEL
#define ENABLE_WEBRTC 1
#endif

#define STRINGIFY(x) #x
#define TOSTRING(x) STRINGIFY(x)

#define _cleanup_(x) __attribute__((__cleanup__(x)))

#ifdef HAVE_FUNC_ATTRIBUTE_RETURNS_NONNULL
#define NEVERNULL __attribute__((returns_nonnull))
#else
#define NEVERNULL
#endif

#ifdef HAVE_FUNC_ATTRIBUTE_NOINLINE
#define NOINLINE __attribute__((noinline))
#else
#define NOINLINE
#endif

#ifdef HAVE_FUNC_ATTRIBUTE_MALLOC
#define MALLOCLIKE __attribute__((malloc))
#else
#define MALLOCLIKE
#endif

#if defined(HAVE_FUNC_ATTRIBUTE_FORMAT_GNU_PRINTF)
#define PRINTFLIKE(f, a) __attribute__ ((format(gnu_printf, f, a)))
#elif defined(HAVE_FUNC_ATTRIBUTE_FORMAT_PRINTF)
#define PRINTFLIKE(f, a) __attribute__ ((format(printf, f, a)))
#else
#define PRINTFLIKE(f, a)
#endif

#ifdef HAVE_FUNC_ATTRIBUTE_NORETURN
#define NORETURN __attribute__ ((noreturn))
#else
#define NORETURN
#endif

#ifdef HAVE_FUNC_ATTRIBUTE_WARN_UNUSED_RESULT
#define WARNUNUSED __attribute__ ((warn_unused_result))
#else
#define WARNUNUSED
#endif

#define UNUSED(x) (void)(x)

#if defined(__GNUC__) && !defined(FSANITIZE_ADDRESS)
#define UNUSED_FUNCTION(x) __attribute__((unused)) UNUSED_##x
#define ALWAYS_INLINE_ONLY __attribute__((always_inline))
#define ALWAYS_INLINE inline __attribute__((always_inline))
#define ALWAYS_INLINE_HOT inline __attribute__((hot, always_inline))
#define ALWAYS_INLINE_HOT_FLATTEN inline __attribute__((hot, always_inline, flatten))
#define NOT_INLINE_HOT __attribute__((hot))
#define NEVER_INLINE __attribute__((noinline))
#else
#define UNUSED_FUNCTION(x) UNUSED_##x
#define ALWAYS_INLINE_ONLY
#define ALWAYS_INLINE inline
#define ALWAYS_INLINE_HOT inline
#define ALWAYS_INLINE_HOT_FLATTEN inline
#define NOT_INLINE_HOT
#define NEVER_INLINE
#endif

#define ABS(x) (((x) < 0)? (-(x)) : (x))
#define MIN(a,b) (((a)<(b))?(a):(b))
#define MAX(a,b) (((a)>(b))?(a):(b))
#define SWAP(a, b) do { \
    typeof(a) _tmp = b; \
    b = a;              \
    a = _tmp;           \
} while(0)

#define HOWMANY(total, divider) ({ \
    typeof(total) _t = (total);    \
    typeof(total) _d = (divider);  \
    _d = _d ? _d : 1;              \
    (_t + (_d - 1)) / _d;          \
})

#define FIT_IN_RANGE(value, min, max) ({ \
    typeof(value) _v = (value);          \
    typeof(min) _min = (min);            \
    typeof(max) _max = (max);            \
    (_v < _min) ? _min : ((_v > _max) ? _max : _v); \
})

#define PIPE_READ 0
#define PIPE_WRITE 1

#define BUILD_BUG_ON(condition) ((void)sizeof(char[1 - 2*!!(condition)]))

#define CONCAT_INDIRECT(a, b) a##b
#define CONCAT(a, b) CONCAT_INDIRECT(a, b)
#define PAD64(type) uint8_t CONCAT(padding, __COUNTER__)[64 - sizeof(type)]; type

#endif // LIBNETDATA_BASE_H
