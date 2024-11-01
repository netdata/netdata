// SPDX-License-Identifier: GPL-3.0-or-later

#include "log2journal.h"

void rewrite_cleanup(REWRITE *rw) {
    hashed_key_cleanup(&rw->key);

    if(rw->flags & RW_MATCH_PCRE2)
        search_pattern_cleanup(&rw->match_pcre2);

    else if(rw->flags & RW_MATCH_NON_EMPTY)
        replace_pattern_cleanup(&rw->match_non_empty);

    replace_pattern_cleanup(&rw->value);
    rw->flags = RW_NONE;
}

bool log_job_rewrite_add(LOG_JOB *jb, const char *key, RW_FLAGS flags, const char *search_pattern, const char *replace_pattern) {
    if(jb->rewrites.used >= MAX_REWRITES) {
        l2j_log("Error: too many rewrites. You can add up to %d rewrite rules.", MAX_REWRITES);
        return false;
    }

    if((flags & (RW_MATCH_PCRE2|RW_MATCH_NON_EMPTY)) && (!search_pattern || !*search_pattern)) {
        l2j_log("Error: rewrite for key '%s' does not specify a search pattern.", key);
        return false;
    }

    REWRITE *rw = &jb->rewrites.array[jb->rewrites.used++];
    rw->flags = flags;

    hashed_key_set(&rw->key, key, -1);

    if((flags & RW_MATCH_PCRE2) && !search_pattern_set(&rw->match_pcre2, search_pattern, strlen(search_pattern))) {
        rewrite_cleanup(rw);
        jb->rewrites.used--;
        return false;
    }
    else if((flags & RW_MATCH_NON_EMPTY) && !replace_pattern_set(&rw->match_non_empty, search_pattern)) {
        rewrite_cleanup(rw);
        jb->rewrites.used--;
        return false;
    }

    if(replace_pattern && *replace_pattern && !replace_pattern_set(&rw->value, replace_pattern)) {
        rewrite_cleanup(rw);
        jb->rewrites.used--;
        return false;
    }

    return true;
}
