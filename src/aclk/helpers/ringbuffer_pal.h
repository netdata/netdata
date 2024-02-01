// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef RINGBUFFER_PAL_H
#define RINGBUFFER_PAL_H

#include "libnetdata/libnetdata.h"

#define crbuf_malloc(...) mallocz(__VA_ARGS__)
#define crbuf_free(...) freez(__VA_ARGS__)

#endif /* RINGBUFFER_PAL_H */
