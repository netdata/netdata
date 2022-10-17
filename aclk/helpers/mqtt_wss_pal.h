// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef MQTT_WSS_PAL_H
#define MQTT_WSS_PAL_H

#include "libnetdata/libnetdata.h"

#undef OPENSSL_VERSION_095
#undef OPENSSL_VERSION_097
#undef OPENSSL_VERSION_110
#undef OPENSSL_VERSION_111

#define mw_malloc(...) mallocz(__VA_ARGS__)
#define mw_calloc(...) callocz(__VA_ARGS__)
#define mw_free(...) freez(__VA_ARGS__)
#define mw_strdup(...) strdupz(__VA_ARGS__)
#define mw_realloc(...) reallocz(__VA_ARGS__)

#endif /* MQTT_WSS_PAL_H */
