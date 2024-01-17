// SPDX-License-Identifier: GPL-3.0-only

#ifndef COMMON_INTERNAL_H
#define COMMON_INTERNAL_H

#include "endian_compat.h"

#ifdef MQTT_WSS_CUSTOM_ALLOC
#include "mqtt_wss_pal.h"
#else
#define mw_malloc(...) malloc(__VA_ARGS__)
#define mw_calloc(...) calloc(__VA_ARGS__)
#define mw_free(...) free(__VA_ARGS__)
#define mw_strdup(...) strdup(__VA_ARGS__)
#define mw_realloc(...) realloc(__VA_ARGS__)
#endif

#ifndef MQTT_WSS_FRAG_MEMALIGN
#define MQTT_WSS_FRAG_MEMALIGN (8)
#endif

#define OPENSSL_VERSION_095 0x00905100L
#define OPENSSL_VERSION_097 0x00907000L
#define OPENSSL_VERSION_110 0x10100000L
#define OPENSSL_VERSION_111 0x10101000L

#endif /* COMMON_INTERNAL_H */
