// SPDX-License-Identifier: GPL-3.0-or-later

#include "log2journal.h"

void injection_cleanup(INJECTION *inj) {
    hashed_key_cleanup(&inj->key);
    replace_pattern_cleanup(&inj->value);
}

static inline bool log_job_injection_replace(INJECTION *inj, const char *key, size_t key_len, const char *value, size_t value_len) {
    if(key_len > JOURNAL_MAX_KEY_LEN)
        l2j_log("WARNING: injection key '%.*s' is too long for journal. Will be truncated.", (int)key_len, key);

    if(value_len > JOURNAL_MAX_VALUE_LEN)
        l2j_log(
            "WARNING: injection value of key '%.*s' is too long for journal. Will be truncated.", (int)key_len, key);

    hashed_key_set(&inj->key, key, key_len);
    char *v = strndupz(value, value_len);
    bool ret = replace_pattern_set(&inj->value, v);
    freez(v);

    return ret;
}

bool log_job_injection_add(LOG_JOB *jb, const char *key, size_t key_len, const char *value, size_t value_len, bool unmatched) {
    if (unmatched) {
        if (jb->unmatched.injections.used >= MAX_INJECTIONS) {
            l2j_log("Error: too many unmatched injections. You can inject up to %d lines.", MAX_INJECTIONS);
            return false;
        }
    }
    else {
        if (jb->injections.used >= MAX_INJECTIONS) {
            l2j_log("Error: too many injections. You can inject up to %d lines.", MAX_INJECTIONS);
            return false;
        }
    }

    bool ret;
    if (unmatched) {
        ret = log_job_injection_replace(&jb->unmatched.injections.keys[jb->unmatched.injections.used++],
                                        key, key_len, value, value_len);
    } else {
        ret = log_job_injection_replace(&jb->injections.keys[jb->injections.used++],
                                        key, key_len, value, value_len);
    }

    return ret;
}
