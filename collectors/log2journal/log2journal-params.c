// SPDX-License-Identifier: GPL-3.0-or-later

#include "log2journal.h"

// ----------------------------------------------------------------------------

void nd_log_cleanup(LOG_JOB *jb) {
    if(jb->prefix) {
        freez((void *) jb->prefix);
        jb->prefix = NULL;
    }

    if(jb->pattern) {
        freez((void *) jb->pattern);
        jb->pattern = NULL;
    }

    for(size_t i = 0; i < jb->injections.used ;i++)
        injection_cleanup(&jb->injections.keys[i]);

    for(size_t i = 0; i < jb->unmatched.injections.used ;i++)
        injection_cleanup(&jb->unmatched.injections.keys[i]);

    for(size_t i = 0; i < jb->renames.used ;i++)
        rename_cleanup(&jb->renames.array[i]);

    for(size_t i = 0; i < jb->dups.used ;i++)
        duplication_cleanup(&jb->dups.array[i]);

    for(size_t i = 0; i < jb->rewrites.used; i++)
        rewrite_cleanup(&jb->rewrites.array[i]);

    // remove references to everything else, to reveal them in valgrind
    memset(jb, 0, sizeof(*jb));
}

// ----------------------------------------------------------------------------

bool log_job_filename_key_set(LOG_JOB *jb, const char *key, size_t key_len) {
    if(!key || !*key) {
        log2stderr("filename key cannot be empty.");
        return false;
    }

    if(jb->filename.key)
        freez((char*)jb->filename.key);

    jb->filename.key = strndupz(key, key_len);

    return true;
}

bool log_job_key_prefix_set(LOG_JOB *jb, const char *prefix, size_t prefix_len) {
    if(!prefix || !*prefix) {
        log2stderr("filename key cannot be empty.");
        return false;
    }

    if(jb->prefix)
        freez((char*)jb->prefix);

    jb->prefix = strndupz(prefix, prefix_len);

    return true;
}

bool log_job_pattern_set(LOG_JOB *jb, const char *pattern, size_t pattern_len) {
    if(!pattern || !*pattern) {
        log2stderr("filename key cannot be empty.");
        return false;
    }

    if(jb->pattern)
        freez((char*)jb->pattern);

    jb->pattern = strndupz(pattern, pattern_len);

    return true;
}

bool log_job_include_pattern_set(LOG_JOB *jb, const char *pattern, size_t pattern_len) {
    if(jb->filter.include.re) {
        log2stderr("FILTER INCLUDE: there is already an include filter set");
        return false;
    }

    if(!search_pattern_set(&jb->filter.include, pattern, pattern_len)) {
        log2stderr("FILTER INCLUDE: failed: %s", jb->filter.include.error.txt);
        return false;
    }

    return true;
}

bool log_job_exclude_pattern_set(LOG_JOB *jb, const char *pattern, size_t pattern_len) {
    if(jb->filter.exclude.re) {
        log2stderr("FILTER INCLUDE: there is already an exclude filter set");
        return false;
    }

    if(!search_pattern_set(&jb->filter.exclude, pattern, pattern_len)) {
        log2stderr("FILTER EXCLUDE: failed: %s", jb->filter.exclude.error.txt);
        return false;
    }

    return true;
}

// ----------------------------------------------------------------------------

static bool parse_rename(LOG_JOB *jb, const char *param) {
    // Search for '=' in param
    const char *equal_sign = strchr(param, '=');
    if (!equal_sign || equal_sign == param) {
        log2stderr("Error: Invalid rename format, '=' not found in %s", param);
        return false;
    }

    const char *new_key = param;
    size_t new_key_len = equal_sign - new_key;

    const char *old_key = equal_sign + 1;
    size_t old_key_len = strlen(old_key);

    return log_job_rename_add(jb, new_key, new_key_len, old_key, old_key_len);
}

static bool is_symbol(char c) {
    return !isalpha(c) && !isdigit(c) && !iscntrl(c);
}

static bool parse_rewrite(LOG_JOB *jb, const char *param) {
    // Search for '=' in param
    const char *equal_sign = strchr(param, '=');
    if (!equal_sign || equal_sign == param) {
        log2stderr("Error: Invalid rewrite format, '=' not found in %s", param);
        return false;
    }

    // Get the next character as the separator
    char separator = *(equal_sign + 1);
    if (!separator || !is_symbol(separator)) {
        log2stderr("Error: rewrite separator not found after '=', or is not one of /\\|-# in: %s", param);
        return false;
    }

    // Find the next occurrence of the separator
    const char *second_separator = strchr(equal_sign + 2, separator);
    if (!second_separator) {
        log2stderr("Error: rewrite second separator not found in: %s", param);
        return false;
    }

    // Check if the search pattern is empty
    if (equal_sign + 1 == second_separator) {
        log2stderr("Error: rewrite search pattern is empty in: %s", param);
        return false;
    }

    // Check if the replacement pattern is empty
    if (*(second_separator + 1) == '\0') {
        log2stderr("Error: rewrite replacement pattern is empty in: %s", param);
        return false;
    }

    // Reserve a slot in rewrites
    if (jb->rewrites.used >= MAX_REWRITES) {
        log2stderr("Error: Exceeded maximum number of rewrite rules, while processing: %s", param);
        return false;
    }

    // Extract key, search pattern, and replacement pattern
    char *key = strndupz(param, equal_sign - param);
    char *search_pattern = strndupz(equal_sign + 2, second_separator - (equal_sign + 2));
    char *replace_pattern = strdupz(second_separator + 1);

    bool ret = log_job_rewrite_add(jb, key, search_pattern, replace_pattern);

    freez(key);
    freez(search_pattern);
    freez(replace_pattern);

    return ret;
}

static bool parse_inject(LOG_JOB *jb, const char *value, bool unmatched) {
    const char *equal = strchr(value, '=');
    if (!equal) {
        log2stderr("Error: injection '%s' does not have an equal sign.", value);
        return false;
    }

    const char *key = value;
    const char *val = equal + 1;
    log_job_injection_add(jb, key, equal - key, val, strlen(val), unmatched);

    return true;
}

static bool parse_duplicate(LOG_JOB *jb, const char *value) {
    const char *target = value;
    const char *equal_sign = strchr(value, '=');
    if (!equal_sign || equal_sign == target) {
        log2stderr("Error: Invalid duplicate format, '=' not found or at the start in %s", value);
        return false;
    }

    size_t target_len = equal_sign - target;
    DUPLICATION *kd = log_job_duplication_add(jb, target, target_len);
    if(!kd) return false;

    const char *key = equal_sign + 1;
    while (key) {
        if (kd->used >= MAX_KEY_DUPS_KEYS) {
            log2stderr("Error: too many keys in duplication of target '%s'.", kd->target.key);
            return false;
        }

        const char *comma = strchr(key, ',');
        size_t key_len;
        if (comma) {
            key_len = comma - key;
            log_job_duplication_key_add(kd, key, key_len);
            key = comma + 1;
        }
        else {
            log_job_duplication_key_add(kd, key, strlen(key));
            break;  // No more keys
        }
    }

    return true;
}

bool log_job_command_line_parse_parameters(LOG_JOB *jb, int argc, char **argv) {
    for (int i = 1; i < argc; i++) {
        char *arg = argv[i];
        if (strcmp(arg, "--help") == 0 || strcmp(arg, "-h") == 0) {
            log_job_command_line_help(argv[0]);
            exit(0);
        }
#if defined(NETDATA_DEV_MODE) || defined(NETDATA_INTERNAL_CHECKS)
        else if(strcmp(arg, "--test") == 0) {
            // logfmt_test();
            json_test();
            exit(1);
        }
#endif
        else if (strcmp(arg, "--show-config") == 0) {
            jb->show_config = true;
        }
        else {
            char buffer[1024];
            char *param = NULL;
            char *value = NULL;

            char *equal_sign = strchr(arg, '=');
            if (equal_sign) {
                copy_to_buffer(buffer, sizeof(buffer), arg, equal_sign - arg);
                param = buffer;
                value = equal_sign + 1;
            }
            else {
                param = arg;
                if (i + 1 < argc) {
                    value = argv[++i];
                }
                else {
                    if (!jb->pattern) {
                        log_job_pattern_set(jb, arg, strlen(arg));
                        continue;
                    } else {
                        log2stderr("Error: Multiple patterns detected. Specify only one pattern. The first is '%s', the second is '%s'", jb->pattern, arg);
                        return false;
                    }
                }
            }

            if (strcmp(param, "--filename-key") == 0) {
                if(!log_job_filename_key_set(jb, value, value ? strlen(value) : 0))
                    return false;
            }
            else if (strcmp(param, "--prefix") == 0) {
                if(!log_job_key_prefix_set(jb, value, value ? strlen(value) : 0))
                    return false;
            }
#ifdef HAVE_LIBYAML
            else if (strcmp(param, "-f") == 0 || strcmp(param, "--file") == 0) {
                if (!yaml_parse_file(value, jb))
                    return false;
            }
            else if (strcmp(param, "--config") == 0) {
                if (!yaml_parse_config(value, jb))
                    return false;
            }
#endif
            else if (strcmp(param, "--unmatched-key") == 0)
                jb->unmatched.key = value;
            else if (strcmp(param, "--duplicate") == 0) {
                if (!parse_duplicate(jb, value))
                    return false;
            }
            else if (strcmp(param, "--inject") == 0) {
                if (!parse_inject(jb, value, false))
                    return false;
            }
            else if (strcmp(param, "--inject-unmatched") == 0) {
                if (!parse_inject(jb, value, true))
                    return false;
            }
            else if (strcmp(param, "--rewrite") == 0) {
                if (!parse_rewrite(jb, value))
                    return false;
            }
            else if (strcmp(param, "--rename") == 0) {
                if (!parse_rename(jb, value))
                    return false;
            }
            else if (strcmp(param, "--include") == 0) {
                if (!log_job_include_pattern_set(jb, value, strlen(value)))
                    return false;
            }
            else if (strcmp(param, "--exclude") == 0) {
                if (!log_job_exclude_pattern_set(jb, value, strlen(value)))
                    return false;
            }
            else {
                i--;
                if (!jb->pattern) {
                    log_job_pattern_set(jb, arg, strlen(arg));
                    continue;
                } else {
                    log2stderr("Error: Multiple patterns detected. Specify only one pattern. The first is '%s', the second is '%s'", jb->pattern, arg);
                    return false;
                }
            }
        }
    }

    // Check if a pattern is set and exactly one pattern is specified
    if (!jb->pattern) {
        log2stderr("Error: Pattern not specified.");
        log_job_command_line_help(argv[0]);
        return false;
    }

    return true;
}
