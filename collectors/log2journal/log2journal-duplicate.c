// SPDX-License-Identifier: GPL-3.0-or-later

#include "log2journal.h"

void duplication_cleanup(DUPLICATION *dp) {
    hashed_key_cleanup(&dp->target);

    for(size_t j = 0; j < dp->used ; j++) {
        hashed_key_cleanup(&dp->keys[j]);
        txt_cleanup(&dp->values[j]);
    }
}

DUPLICATION *log_job_duplication_add(LOG_JOB *jb, const char *target, size_t target_len) {
    if (jb->dups.used >= MAX_KEY_DUPS) {
        log2stderr("ERROR: Too many duplicates defined. Maximum allowed is %d.", MAX_KEY_DUPS);
        return NULL;
    }

    if(target_len > JOURNAL_MAX_KEY_LEN) {
        log2stderr("WARNING: key of duplicate '%.*s' is too long for journals. Will be truncated.", (int)target_len, target);
        target_len = JOURNAL_MAX_KEY_LEN;
    }

    DUPLICATION *kd = &jb->dups.array[jb->dups.used++];
    hashed_key_len_set(&kd->target, target, target_len);
    kd->used = 0;
    kd->exposed = false;

    // Initialize values array
    for (size_t i = 0; i < MAX_KEY_DUPS_KEYS; i++) {
        kd->values[i].txt = NULL;
        kd->values[i].size = 0;
    }

    return kd;
}

bool log_job_duplication_key_add(DUPLICATION *kd, const char *key, size_t key_len) {
    if (kd->used >= MAX_KEY_DUPS_KEYS) {
        log2stderr("ERROR: Too many keys in duplication of target '%s'.", kd->target.key);
        return false;
    }

    hashed_key_len_set(&kd->keys[kd->used++], key, key_len);

    return true;
}

