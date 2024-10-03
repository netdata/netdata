// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_UTF8_SANITIZER_H
#define NETDATA_UTF8_SANITIZER_H

#include "../libnetdata.h"

size_t text_sanitize(unsigned char *dst, const unsigned char *src, size_t dst_size, const unsigned char *char_map, bool utf, const char *empty, size_t *multibyte_length);

#endif //NETDATA_UTF8_SANITIZER_H
