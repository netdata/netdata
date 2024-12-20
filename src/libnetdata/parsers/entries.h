// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef LIBNETDATA_PARSERS_ENTRIES_H
#define LIBNETDATA_PARSERS_ENTRIES_H

#include "parsers.h"

bool entries_parse(const char *entries_str, uint64_t *result, const char *default_unit);
#define entries_parse_k(size_str, kb) size_parse(size_str, kb, "K")
#define entries_parse_m(size_str, mb) size_parse(size_str, mb, "M")
#define entries_parse_g(size_str, gb) size_parse(size_str, gb, "G")

ssize_t entries_snprintf(char *dst, size_t dst_size, uint64_t value, const char *unit, bool accurate);
#define entries_snprintf_n(dst, dst_size, value) size_snprintf(dst, dst_size, value, "", true)
#define entries_snprintf_k(dst, dst_size, value) size_snprintf(dst, dst_size, value, "K", true)
#define entries_snprintf_m(dst, dst_size, value) size_snprintf(dst, dst_size, value, "M", true)
#define entries_snprintf_g(dst, dst_size, value) size_snprintf(dst, dst_size, value, "G", true)

#endif //LIBNETDATA_PARSERS_ENTRIES_H
