// SPDX-License-Identifier: GPL-3.0-or-later

#include "log2journal.h"

void rewrite_cleanup(REWRITE *rw) {
    hashed_key_cleanup(&rw->key);
    search_pattern_cleanup(&rw->search);
    replace_pattern_cleanup(&rw->replace);
}

bool log_job_rewrite_add(LOG_JOB *jb, const char *key, const char *search_pattern, const char *replace_pattern) {
    if(jb->rewrites.used >= MAX_REWRITES) {
        log2stderr("Error: too many rewrites. You can add up to %d rewrite rules.", MAX_REWRITES);
        return false;
    }

    REWRITE *rw = &jb->rewrites.array[jb->rewrites.used++];
    hashed_key_set(&rw->key, key);

    if(!search_pattern_set(&rw->search, search_pattern, strlen(search_pattern)) ||
        !replace_pattern_set(&rw->replace, replace_pattern)) {
        rewrite_cleanup(rw);
        jb->rewrites.used--;
        return false;
    }

    return true;
}
