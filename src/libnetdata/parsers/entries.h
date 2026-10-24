// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef LIBNETDATA_PARSERS_ENTRIES_H
#define LIBNETDATA_PARSERS_ENTRIES_H

#include "parsers.h"

bool entries_parse(const char *entries_str, uint64_t *result, const char *default_unit);
#define entries_parse_k(entries_str, k_entries) entries_parse(entries_str, k_entries, "K")
#define entries_parse_m(entries_str, m_entries) entries_parse(entries_str, m_entries, "M")
#define entries_parse_g(entries_str, g_entries) entries_parse(entries_str, g_entries, "G")

ssize_t entries_snprintf(char *dst, size_t dst_size, uint64_t value, const char *unit, bool accurate);
#define entries_snprintf_n(dst, dst_size, value) entries_snprintf(dst, dst_size, value, "", true)
#define entries_snprintf_k(dst, dst_size, value) entries_snprintf(dst, dst_size, value, "K", true)
#define entries_snprintf_m(dst, dst_size, value) entries_snprintf(dst, dst_size, value, "M", true)
#define entries_snprintf_g(dst, dst_size, value) entries_snprintf(dst, dst_size, value, "G", true)

#endif //LIBNETDATA_PARSERS_ENTRIES_H
