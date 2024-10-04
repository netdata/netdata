// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_CHART_ID_AND_NAME_H
#define NETDATA_CHART_ID_AND_NAME_H

#include "../libnetdata.h"

void netdata_fix_chart_id(char *s);
void netdata_fix_chart_name(char *s);
char *rrdset_strncpyz_name(char *dst, const char *src, size_t dst_size_minus_1);
bool rrdvar_fix_name(char *variable);

extern unsigned char chart_names_allowed_chars[256];
static inline bool is_netdata_api_valid_character(char c) {
    return (IS_UTF8_BYTE(c) || chart_names_allowed_chars[(unsigned char)c] == (unsigned char)c);
}

#endif //NETDATA_CHART_ID_AND_NAME_H
