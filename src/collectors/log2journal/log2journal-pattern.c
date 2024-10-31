// SPDX-License-Identifier: GPL-3.0-or-later

#include "log2journal.h"

void search_pattern_cleanup(SEARCH_PATTERN *sp) {
    if(sp->pattern) {
        freez((void *)sp->pattern);
        sp->pattern = NULL;
    }

    if(sp->re) {
        pcre2_code_free(sp->re);
        sp->re = NULL;
    }

    if(sp->match_data) {
        pcre2_match_data_free(sp->match_data);
        sp->match_data = NULL;
    }

    txt_l2j_cleanup(&sp->error);
}

static void pcre2_error_message(SEARCH_PATTERN *sp, int rc, int pos) {
    char msg[1024];
    pcre2_get_error_in_buffer(msg, sizeof(msg), rc, pos);
    txt_l2j_set(&sp->error, msg, strlen(msg));
}

static inline bool compile_pcre2(SEARCH_PATTERN *sp) {
    int error_number;
    PCRE2_SIZE error_offset;
    PCRE2_SPTR pattern_ptr = (PCRE2_SPTR)sp->pattern;

    sp->re = pcre2_compile(pattern_ptr, PCRE2_ZERO_TERMINATED, 0, &error_number, &error_offset, NULL);
    if (!sp->re) {
        pcre2_error_message(sp, error_number, (int) error_offset);
        return false;
    }

    return true;
}

bool search_pattern_set(SEARCH_PATTERN *sp, const char *search_pattern, size_t search_pattern_len) {
    search_pattern_cleanup(sp);

    sp->pattern = strndupz(search_pattern, search_pattern_len);
    if (!compile_pcre2(sp))
        return false;

    sp->match_data = pcre2_match_data_create_from_pattern(sp->re, NULL);

    return true;
}
