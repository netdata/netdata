// SPDX-License-Identifier: GPL-3.0-or-later

#include "log2journal.h"

// ----------------------------------------------------------------------------

void log_job_init(LOG_JOB *jb) {
    memset(jb, 0, sizeof(*jb));
    simple_hashtable_init_KEY(&jb->hashtable, 32);
    hashed_key_set(&jb->line.key, "LINE", -1);
}

static void simple_hashtable_cleanup_allocated_keys(SIMPLE_HASHTABLE_KEY *ht) {
    SIMPLE_HASHTABLE_FOREACH_READ_ONLY(ht, sl, _KEY) {
        HASHED_KEY *k = SIMPLE_HASHTABLE_FOREACH_READ_ONLY_VALUE(sl);
        if(k && k->flags & HK_HASHTABLE_ALLOCATED) {
            // the order of these statements is important!
            simple_hashtable_del_slot_KEY(ht, sl); // remove any references to n
            hashed_key_cleanup(k); // cleanup the internals of n
            freez(k); // free n
        }
    }
}

void log_job_cleanup(LOG_JOB *jb) {
    hashed_key_cleanup(&jb->line.key);

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

    for(size_t i = 0; i < jb->rewrites.used; i++)
        rewrite_cleanup(&jb->rewrites.array[i]);

    search_pattern_cleanup(&jb->filter.include);
    search_pattern_cleanup(&jb->filter.exclude);

    hashed_key_cleanup(&jb->filename.key);
    hashed_key_cleanup(&jb->unmatched.key);

    txt_l2j_cleanup(&jb->rewrites.tmp);
    txt_l2j_cleanup(&jb->filename.current);

    simple_hashtable_cleanup_allocated_keys(&jb->hashtable);
    simple_hashtable_destroy_KEY(&jb->hashtable);

    // remove references to everything else, to reveal them in valgrind
    memset(jb, 0, sizeof(*jb));
}

// ----------------------------------------------------------------------------

bool log_job_filename_key_set(LOG_JOB *jb, const char *key, size_t key_len) {
    if(!key || !*key) {
        l2j_log("filename key cannot be empty.");
        return false;
    }

    hashed_key_set(&jb->filename.key, key, key_len);

    return true;
}

bool log_job_key_prefix_set(LOG_JOB *jb, const char *prefix, size_t prefix_len) {
    if(!prefix || !*prefix) {
        l2j_log("filename key cannot be empty.");
        return false;
    }

    if(jb->prefix)
        freez((char*)jb->prefix);

    jb->prefix = strndupz(prefix, prefix_len);

    return true;
}

bool log_job_pattern_set(LOG_JOB *jb, const char *pattern, size_t pattern_len) {
    if(!pattern || !*pattern) {
        l2j_log("filename key cannot be empty.");
        return false;
    }

    if(jb->pattern)
        freez((char*)jb->pattern);

    jb->pattern = strndupz(pattern, pattern_len);

    return true;
}

bool log_job_include_pattern_set(LOG_JOB *jb, const char *pattern, size_t pattern_len) {
    if(jb->filter.include.re) {
        l2j_log("FILTER INCLUDE: there is already an include filter set");
        return false;
    }

    if(!search_pattern_set(&jb->filter.include, pattern, pattern_len)) {
        l2j_log("FILTER INCLUDE: failed: %s", jb->filter.include.error.txt);
        return false;
    }

    return true;
}

bool log_job_exclude_pattern_set(LOG_JOB *jb, const char *pattern, size_t pattern_len) {
    if(jb->filter.exclude.re) {
        l2j_log("FILTER INCLUDE: there is already an exclude filter set");
        return false;
    }

    if(!search_pattern_set(&jb->filter.exclude, pattern, pattern_len)) {
        l2j_log("FILTER EXCLUDE: failed: %s", jb->filter.exclude.error.txt);
        return false;
    }

    return true;
}

// ----------------------------------------------------------------------------

static bool parse_rename(LOG_JOB *jb, const char *param) {
    // Search for '=' in param
    const char *equal_sign = strchr(param, '=');
    if (!equal_sign || equal_sign == param) {
        l2j_log("Error: Invalid rename format, '=' not found in %s", param);
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

struct {
    const char *keyword;
    int action;
    RW_FLAGS flag;
} rewrite_flags[] = {
        {"match",       1, RW_MATCH_PCRE2},
        {"match",       0, RW_MATCH_NON_EMPTY},

        {"regex",       1, RW_MATCH_PCRE2},
        {"regex",       0, RW_MATCH_NON_EMPTY},

        {"pcre2",       1, RW_MATCH_PCRE2},
        {"pcre2",       0, RW_MATCH_NON_EMPTY},

        {"non_empty",   1, RW_MATCH_NON_EMPTY},
        {"non_empty",   0, RW_MATCH_PCRE2},

        {"non-empty",   1, RW_MATCH_NON_EMPTY},
        {"non-empty",   0, RW_MATCH_PCRE2},

        {"not_empty",   1, RW_MATCH_NON_EMPTY},
        {"not_empty",   0, RW_MATCH_PCRE2},

        {"not-empty",   1, RW_MATCH_NON_EMPTY},
        {"not-empty",   0, RW_MATCH_PCRE2},

        {"stop",        0, RW_DONT_STOP},
        {"no-stop",     1, RW_DONT_STOP},
        {"no_stop",     1, RW_DONT_STOP},
        {"dont-stop",   1, RW_DONT_STOP},
        {"dont_stop",   1, RW_DONT_STOP},
        {"continue",    1, RW_DONT_STOP},
        {"inject",      1, RW_INJECT},
        {"existing",    0, RW_INJECT},
};

RW_FLAGS parse_rewrite_flags(const char *options) {
    RW_FLAGS flags = RW_MATCH_PCRE2; // Default option

    // Tokenize the input options using ","
    char *token;
    char *optionsCopy = strdup(options); // Make a copy to avoid modifying the original
    token = strtok(optionsCopy, ",");

    while (token != NULL) {
        // Find the keyword-action mapping
        bool found = false;

        for (size_t i = 0; i < sizeof(rewrite_flags) / sizeof(rewrite_flags[0]); i++) {
            if (strcmp(token, rewrite_flags[i].keyword) == 0) {
                if (rewrite_flags[i].action == 1) {
                    flags |= rewrite_flags[i].flag; // Set the flag
                } else {
                    flags &= ~rewrite_flags[i].flag; // Unset the flag
                }

                found = true;
            }
        }

        if(!found)
            l2j_log("Warning: rewrite options '%s' is not understood.", token);

        // Get the next token
        token = strtok(NULL, ",");
    }

    free(optionsCopy); // Free the copied string

    return flags;
}


static bool parse_rewrite(LOG_JOB *jb, const char *param) {
    // Search for '=' in param
    const char *equal_sign = strchr(param, '=');
    if (!equal_sign || equal_sign == param) {
        l2j_log("Error: Invalid rewrite format, '=' not found in %s", param);
        return false;
    }

    // Get the next character as the separator
    char separator = *(equal_sign + 1);
    if (!separator || !is_symbol(separator)) {
        l2j_log("Error: rewrite separator not found after '=', or is not one of /\\|-# in: %s", param);
        return false;
    }

    // Find the next occurrence of the separator
    const char *second_separator = strchr(equal_sign + 2, separator);
    if (!second_separator) {
        l2j_log("Error: rewrite second separator not found in: %s", param);
        return false;
    }

    // Check if the search pattern is empty
    if (equal_sign + 1 == second_separator) {
        l2j_log("Error: rewrite search pattern is empty in: %s", param);
        return false;
    }

    // Check if the replacement pattern is empty
    if (*(second_separator + 1) == '\0') {
        l2j_log("Error: rewrite replacement pattern is empty in: %s", param);
        return false;
    }

    RW_FLAGS flags = RW_MATCH_PCRE2;
    const char *third_separator = strchr(second_separator + 1, separator);
    if(third_separator)
        flags = parse_rewrite_flags(third_separator + 1);

    // Extract key, search pattern, and replacement pattern
    char *key = strndupz(param, equal_sign - param);
    char *search_pattern = strndupz(equal_sign + 2, second_separator - (equal_sign + 2));
    char *replace_pattern = third_separator ? strndup(second_separator + 1, third_separator - (second_separator + 1)) : strdupz(second_separator + 1);

    if(!*search_pattern)
        flags &= ~RW_MATCH_PCRE2;

    bool ret = log_job_rewrite_add(jb, key, flags, search_pattern, replace_pattern);

    freez(key);
    freez(search_pattern);
    freez(replace_pattern);

    return ret;
}

static bool parse_inject(LOG_JOB *jb, const char *value, bool unmatched) {
    const char *equal = strchr(value, '=');
    if (!equal) {
        l2j_log("Error: injection '%s' does not have an equal sign.", value);
        return false;
    }

    const char *key = value;
    const char *val = equal + 1;
    log_job_injection_add(jb, key, equal - key, val, strlen(val), unmatched);

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
                        l2j_log(
                            "Error: Multiple patterns detected. Specify only one pattern. The first is '%s', the second is '%s'",
                            jb->pattern,
                            arg);
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
            else if (strcmp(param, "-c") == 0 || strcmp(param, "--config") == 0) {
                if (!yaml_parse_config(value, jb))
                    return false;
            }
#endif
            else if (strcmp(param, "--unmatched-key") == 0)
                hashed_key_set(&jb->unmatched.key, value, -1);
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
                    l2j_log(
                        "Error: Multiple patterns detected. Specify only one pattern. The first is '%s', the second is '%s'",
                        jb->pattern,
                        arg);
                    return false;
                }
            }
        }
    }

    // Check if a pattern is set and exactly one pattern is specified
    if (!jb->pattern) {
        l2j_log("Warning: pattern not specified. Try the default config with: -c default");
        log_job_command_line_help(argv[0]);
        return false;
    }

    return true;
}
