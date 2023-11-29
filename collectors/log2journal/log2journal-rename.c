// SPDX-License-Identifier: GPL-3.0-or-later

#include "log2journal.h"

void rename_cleanup(RENAME *rn) {
    if(rn->new_key) {
        freez(rn->new_key);
        rn->new_key = NULL;
    }

    if(rn->old_key) {
        freez(rn->old_key);
        rn->old_key = NULL;
    }
}

bool log_job_rename_add(LOG_JOB *jb, const char *new_key, size_t new_key_len, const char *old_key, size_t old_key_len) {
    if(jb->renames.used >= MAX_RENAMES) {
        log2stderr("Error: too many renames. You can rename up to %d fields.", MAX_RENAMES);
        return false;
    }

    RENAME *rn = &jb->renames.array[jb->renames.used++];
    rn->new_key = strndupz(new_key, new_key_len);
    rn->new_hash = XXH3_64bits(rn->new_key, strlen(rn->new_key));
    rn->old_key = strndupz(old_key, old_key_len);
    rn->old_hash = XXH3_64bits(rn->old_key, strlen(rn->old_key));

    return true;
}
