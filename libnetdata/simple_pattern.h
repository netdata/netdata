// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SIMPLE_PATTERN_H
#define NETDATA_SIMPLE_PATTERN_H

#include "libnetdata.h"


typedef enum {
    SIMPLE_PATTERN_EXACT,
    SIMPLE_PATTERN_PREFIX,
    SIMPLE_PATTERN_SUFFIX,
    SIMPLE_PATTERN_SUBSTRING
} SIMPLE_PREFIX_MODE;

typedef void SIMPLE_PATTERN;

// create a simple_pattern from the string given
// default_mode is used in cases where EXACT matches, without an asterisk,
// should be considered PREFIX matches.
extern SIMPLE_PATTERN *simple_pattern_create(const char *list, const char *separators, SIMPLE_PREFIX_MODE default_mode);

// test if string str is matched from the pattern and fill 'wildcarded' with the parts matched by '*'
extern int simple_pattern_matches_extract(SIMPLE_PATTERN *list, const char *str, char *wildcarded, size_t wildcarded_size);

// test if string str is matched from the pattern
#define simple_pattern_matches(list, str) simple_pattern_matches_extract(list, str, NULL, 0)

// free a simple_pattern that was created with simple_pattern_create()
// list can be NULL, in which case, this does nothing.
extern void simple_pattern_free(SIMPLE_PATTERN *list);

#endif //NETDATA_SIMPLE_PATTERN_H
