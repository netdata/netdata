// SPDX-License-Identifier: GPL-3.0-or-later

#include "log2journal.h"

void rename_cleanup(RENAME *rn) {
    hashed_key_cleanup(&rn->new_key);
    hashed_key_cleanup(&rn->old_key);
}

bool log_job_rename_add(LOG_JOB *jb, const char *new_key, size_t new_key_len, const char *old_key, size_t old_key_len) {
    if(jb->renames.used >= MAX_RENAMES) {
        l2j_log("Error: too many renames. You can rename up to %d fields.", MAX_RENAMES);
        return false;
    }

    RENAME *rn = &jb->renames.array[jb->renames.used++];
    hashed_key_set(&rn->new_key, new_key, new_key_len);
    hashed_key_set(&rn->old_key, old_key, old_key_len);

    return true;
}
