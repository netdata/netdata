// SPDX-License-Identifier: GPL-3.0-or-later

#include "log2journal.h"

void rewrite_cleanup(REWRITE *rw) {
    if(rw->key) {
        freez(rw->key);
        rw->key = NULL;
    }

    search_pattern_cleanup(&rw->search);
    replace_pattern_cleanup(&rw->replace);
}

static inline pcre2_code *jb_compile_pcre2_pattern(const char *pattern) {
    int error_number;
    PCRE2_SIZE error_offset;
    PCRE2_SPTR pattern_ptr = (PCRE2_SPTR)pattern;

    pcre2_code *re = pcre2_compile(pattern_ptr, PCRE2_ZERO_TERMINATED, 0, &error_number, &error_offset, NULL);
    if (re == NULL) {
        PCRE2_UCHAR buffer[1024];
        pcre2_get_error_message(error_number, buffer, sizeof(buffer));
        log2stderr("PCRE2 compilation failed at offset %d: %s", (int)error_offset, buffer);
        log2stderr("Check for common regex syntax errors or unsupported PCRE2 patterns.");
        return NULL;
    }

    return re;
}

bool log_job_rewrite_add(LOG_JOB *jb, const char *key, const char *search_pattern, const char *replace_pattern) {
    if(jb->rewrites.used >= MAX_REWRITES) {
        log2stderr("Error: too many rewrites. You can add up to %d rewrite rules.", MAX_REWRITES);
        return false;
    }

    pcre2_code *re = jb_compile_pcre2_pattern(search_pattern);
    if (!re) {
        return false;
    }

    REWRITE *rw = &jb->rewrites.array[jb->rewrites.used++];
    rw->key = strdupz(key);
    rw->hash = XXH3_64bits(rw->key, strlen(rw->key));
    rw->search.pattern = strdupz(search_pattern);
    rw->search.re = re;
    rw->search.match_data = pcre2_match_data_create_from_pattern(rw->search.re, NULL);

    // Parse the replacement pattern and create the linked list
    if (!replace_pattern_set(&rw->replace, replace_pattern)) {
        pcre2_match_data_free(rw->search.match_data);
        pcre2_code_free(rw->search.re);
        freez(rw->key);
        freez((char *)rw->search.pattern);
        freez((char *)rw->replace.pattern);
        jb->rewrites.used--;
        return false;
    }

    return true;
}
