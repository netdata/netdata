// SPDX-License-Identifier: GPL-3.0-or-later

#include "log2journal.h"

void injection_cleanup(INJECTION *inj) {
    hashed_key_cleanup(&inj->key);
    txt_cleanup(&inj->value);
}

static inline void log_job_injection_replace(INJECTION *inj, const char *key, size_t key_len, const char *value, size_t value_len) {
    if(key_len > JOURNAL_MAX_KEY_LEN)
        log2stderr("WARNING: injection key '%.*s' is too long for journal. Will be truncated.", (int)key_len, key);

    if(value_len > JOURNAL_MAX_VALUE_LEN)
        log2stderr("WARNING: injection value of key '%.*s' is too long for journal. Will be truncated.", (int)key_len, key);

    hashed_key_len_set(&inj->key, key, key_len);
    txt_replace(&inj->value, value, value_len);
}

bool log_job_injection_add(LOG_JOB *jb, const char *key, size_t key_len, const char *value, size_t value_len, bool unmatched) {
    if (unmatched) {
        if (jb->unmatched.injections.used >= MAX_INJECTIONS) {
            log2stderr("Error: too many unmatched injections. You can inject up to %d lines.", MAX_INJECTIONS);
            return false;
        }
    }
    else {
        if (jb->injections.used >= MAX_INJECTIONS) {
            log2stderr("Error: too many injections. You can inject up to %d lines.", MAX_INJECTIONS);
            return false;
        }
    }

    if (unmatched) {
        log_job_injection_replace(&jb->unmatched.injections.keys[jb->unmatched.injections.used++],
                                  key, key_len, value, value_len);
    } else {
        log_job_injection_replace(&jb->injections.keys[jb->injections.used++],
                                  key, key_len, value, value_len);
    }

    return true;
}
