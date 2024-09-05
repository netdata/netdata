// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef LIBNETDATA_PARSERS_SIZE_H
#define LIBNETDATA_PARSERS_SIZE_H

#include "parsers.h"

bool size_parse(const char *size_str, uint64_t *result, const char *default_unit);
#define size_parse_bytes(size_str, bytes) size_parse(size_str, bytes, "B")
#define size_parse_kb(size_str, kb) size_parse(size_str, kb, "KiB")
#define size_parse_mb(size_str, mb) size_parse(size_str, mb, "MiB")
#define size_parse_gb(size_str, gb) size_parse(size_str, gb, "GiB")

ssize_t size_snprintf(char *dst, size_t dst_size, uint64_t value, const char *unit, bool accurate);
#define size_snprintf_bytes(dst, dst_size, value) size_snprintf(dst, dst_size, value, "B", true)
#define size_snprintf_kb(dst, dst_size, value) size_snprintf(dst, dst_size, value, "KiB", true)
#define size_snprintf_mb(dst, dst_size, value) size_snprintf(dst, dst_size, value, "MiB", true)
#define size_snprintf_gb(dst, dst_size, value) size_snprintf(dst, dst_size, value, "GiB", true)

#endif //LIBNETDATA_PARSERS_SIZE_H
