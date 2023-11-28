// SPDX-License-Identifier: GPL-3.0-or-later

#include "log2journal.h"

static bool parse_replacement_pattern(struct key_rewrite *rw);

// ----------------------------------------------------------------------------

void nd_log_destroy(struct log_job *jb) {
    for(size_t i = 0; i < jb->injections.used ;i++) {
        if(jb->injections.keys[i].value.s)
            freez(jb->injections.keys[i].value.s);
    }

    for(size_t i = 0; i < jb->unmatched.injections.used ;i++) {
        if(jb->unmatched.injections.keys[i].value.s)
            freez(jb->unmatched.injections.keys[i].value.s);
    }

    for(size_t i = 0; i < jb->dups.used ;i++) {
        struct key_dup *kd = &jb->dups.array[i];

        if(kd->target)
            freez(kd->target);

        for(size_t j = 0; j < kd->used ; j++) {
            if (kd->keys[j])
                freez(kd->keys[j]);

            if (kd->values[j].s)
                freez(kd->values[j].s);
        }
    }

    for(size_t i = 0; i < jb->rewrites.used; i++) {
        struct key_rewrite *rw = &jb->rewrites.array[i];

        if (rw->key)
            freez(rw->key);

        if (rw->search_pattern)
            freez(rw->search_pattern);

        if (rw->replace_pattern)
            freez(rw->replace_pattern);

        if(rw->match_data)
            pcre2_match_data_free(rw->match_data);

        if (rw->re)
            pcre2_code_free(rw->re);

        // Cleanup for replacement nodes linked list
        struct replacement_node *current = rw->nodes;
        while (current != NULL) {
            struct replacement_node *next = current->next;

            if (current->s)
                freez((void *)current->s);

            freez(current);
            current = next;
        }
    }

    memset(jb, 0, sizeof(*jb));
}

// ----------------------------------------------------------------------------

bool log_job_add_filename_key(struct log_job *jb, const char *key, size_t key_len) {
    if(!key || !*key) {
        log2stderr("filename key cannot be empty.");
        return false;
    }

    if(jb->filename.key)
        freez((char*)jb->filename.key);

    jb->filename.key = strndupz(key, key_len);

    return true;
}

bool log_job_add_key_prefix(struct log_job *jb, const char *prefix, size_t prefix_len) {
    if(!prefix || !*prefix) {
        log2stderr("filename key cannot be empty.");
        return false;
    }

    if(jb->prefix)
        freez((char*)jb->prefix);

    jb->prefix = strndupz(prefix, prefix_len);

    return true;
}

bool log_job_add_injection(struct log_job *jb, const char *key, size_t key_len, const char *value, size_t value_len, bool unmatched) {
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
        key_value_replace(&jb->unmatched.injections.keys[jb->unmatched.injections.used++],
                key, key_len,
                value, value_len);
    } else {
        key_value_replace(&jb->injections.keys[jb->injections.used++],
                key, key_len,
                value, value_len);
    }

    return true;
}

bool log_job_add_rename(struct log_job *jb, const char *new_key, size_t new_key_len, const char *old_key, size_t old_key_len) {
    if(jb->renames.used >= MAX_RENAMES) {
        log2stderr("Error: too many renames. You can rename up to %d fields.", MAX_RENAMES);
        return false;
    }

    struct key_rename *rn = &jb->renames.array[jb->renames.used++];
    rn->new_key = strndupz(new_key, new_key_len);
    rn->new_hash = XXH3_64bits(rn->new_key, strlen(rn->new_key));
    rn->old_key = strndupz(old_key, old_key_len);
    rn->old_hash = XXH3_64bits(rn->old_key, strlen(rn->old_key));

    return true;
}

bool log_job_add_rewrite(struct log_job *jb, const char *key, const char *search_pattern, const char *replace_pattern) {
    if(jb->rewrites.used >= MAX_REWRITES) {
        log2stderr("Error: too many rewrites. You can add up to %d rewrite rules.", MAX_REWRITES);
        return false;
    }

    pcre2_code *re = jb_compile_pcre2_pattern(search_pattern);
    if (!re) {
        return false;
    }

    struct key_rewrite *rw = &jb->rewrites.array[jb->rewrites.used++];
    rw->key = strdupz(key);
    rw->hash = XXH3_64bits(rw->key, strlen(rw->key));
    rw->search_pattern = strdupz(search_pattern);
    rw->replace_pattern = strdupz(replace_pattern);
    rw->re = re;
    rw->match_data = pcre2_match_data_create_from_pattern(rw->re, NULL);

    // Parse the replacement pattern and create the linked list
    if (!parse_replacement_pattern(rw)) {
        pcre2_match_data_free(rw->match_data);
        pcre2_code_free(rw->re);
        freez(rw->key);
        freez(rw->search_pattern);
        freez(rw->replace_pattern);
        jb->rewrites.used--;
        return false;
    }

    return true;
}

// ----------------------------------------------------------------------------

struct key_dup *log_job_add_duplication_to_job(struct log_job *jb, const char *target, size_t target_len) {
    if (jb->dups.used >= MAX_KEY_DUPS) {
        log2stderr("Error: Too many duplicates defined. Maximum allowed is %d.", MAX_KEY_DUPS);
        return NULL;
    }

    struct key_dup *kd = &jb->dups.array[jb->dups.used++];
    kd->target = strndupz(target, target_len);
    kd->hash = XXH3_64bits(kd->target, target_len);
    kd->used = 0;
    kd->exposed = false;

    // Initialize values array
    for (size_t i = 0; i < MAX_KEY_DUPS_KEYS; i++) {
        kd->values[i].s = NULL;
        kd->values[i].size = 0;
    }

    return kd;
}

bool log_job_add_key_to_duplication(struct key_dup *kd, const char *key, size_t key_len) {
    if (kd->used >= MAX_KEY_DUPS_KEYS) {
        log2stderr("Error: Too many keys in duplication of target '%s'.", kd->target);
        return false;
    }

    kd->keys[kd->used++] = strndupz(key, key_len);
    return true;
}

// ----------------------------------------------------------------------------
// command line params

struct replacement_node *add_replacement_node(struct replacement_node **head, bool is_variable, const char *text) {
    struct replacement_node *new_node = mallocz(sizeof(struct replacement_node));
    if (!new_node)
        return NULL;

    new_node->is_variable = is_variable;
    new_node->s = text;
    new_node->len = strlen(text);
    new_node->next = NULL;

    if (*head == NULL)
        *head = new_node;

    else {
        struct replacement_node *current = *head;

        // append it
        while (current->next != NULL)
            current = current->next;

        current->next = new_node;
    }

    return new_node;
}

static bool parse_replacement_pattern(struct key_rewrite *rw) {
    const char *current = rw->replace_pattern;

    while (*current != '\0') {
        if (*current == '$' && *(current + 1) == '{') {
            // Start of a variable
            const char *end = strchr(current, '}');
            if (!end) {
                log2stderr("Error: Missing closing brace in replacement pattern: %s", rw->replace_pattern);
                return false;
            }

            size_t name_length = end - current - 2; // Length of the variable name
            char *variable_name = strndupz(current + 2, name_length);
            if (!variable_name) {
                log2stderr("Error: Memory allocation failed for variable name.");
                return false;
            }

            struct replacement_node *node = add_replacement_node(&(rw->nodes), true, variable_name);
            if (!node) {
                freez(variable_name);
                log2stderr("Error: Failed to add replacement node for variable.");
                return false;
            }

            current = end + 1; // Move past the variable
        }
        else {
            // Start of literal text
            const char *start = current;
            while (*current != '\0' && !(*current == '$' && *(current + 1) == '{')) {
                current++;
            }

            size_t text_length = current - start;
            char *text = strndupz(start, text_length);
            if (!text) {
                log2stderr("Error: Memory allocation failed for literal text.");
                return false;
            }

            struct replacement_node *node = add_replacement_node(&(rw->nodes), false, text);
            if (!node) {
                freez(text);
                log2stderr("Error: Failed to add replacement node for text.");
                return false;
            }
        }
    }

    return true;
}

static bool parse_rename(struct log_job *jb, const char *param) {
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

    return log_job_add_rename(jb, new_key, new_key_len, old_key, old_key_len);
}

static bool is_symbol(char c) {
    return !isalpha(c) && !isdigit(c) && !iscntrl(c);
}

static bool parse_rewrite(struct log_job *jb, const char *param) {
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

    bool ret = log_job_add_rewrite(jb, key, search_pattern, replace_pattern);

    freez(key);
    freez(search_pattern);
    freez(replace_pattern);

    return ret;
}

static bool parse_inject(struct log_job *jb, const char *value, bool unmatched) {
    const char *equal = strchr(value, '=');
    if (!equal) {
        log2stderr("Error: injection '%s' does not have an equal sign.", value);
        return false;
    }

    const char *key = value;
    const char *val = equal + 1;
    log_job_add_injection(jb, key, equal - key, val, strlen(val), unmatched);

    return true;
}

static bool parse_duplicate(struct log_job *jb, const char *value) {
    const char *target = value;
    const char *equal_sign = strchr(value, '=');
    if (!equal_sign || equal_sign == target) {
        log2stderr("Error: Invalid duplicate format, '=' not found or at the start in %s", value);
        return false;
    }

    size_t target_len = equal_sign - target;
    struct key_dup *kd = log_job_add_duplication_to_job(jb, target, target_len);
    if(!kd) return false;

    const char *key = equal_sign + 1;
    while (key) {
        if (kd->used >= MAX_KEY_DUPS_KEYS) {
            log2stderr("Error: too many keys in duplication of target '%s'.", kd->target);
            return false;
        }

        const char *comma = strchr(key, ',');
        size_t key_len;
        if (comma) {
            key_len = comma - key;
            log_job_add_key_to_duplication(kd, key, key_len);
            key = comma + 1;
        }
        else {
            log_job_add_key_to_duplication(kd, key, strlen(key));
            break;  // No more keys
        }
    }

    return true;
}

bool parse_log2journal_parameters(struct log_job *jb, int argc, char **argv) {
    for (int i = 1; i < argc; i++) {
        char *arg = argv[i];
        if (strcmp(arg, "--help") == 0 || strcmp(arg, "-h") == 0) {
            log2journal_command_line_help(argv[0]);
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
                        jb->pattern = arg;
                        continue;
                    } else {
                        log2stderr("Error: Multiple patterns detected. Specify only one pattern. The first is '%s', the second is '%s'", jb->pattern, arg);
                        return false;
                    }
                }
            }

            if (strcmp(param, "--filename-key") == 0) {
                if(!log_job_add_filename_key(jb, value, value ? strlen(value) : 0))
                    return false;
            }
            if (strcmp(param, "--prefix") == 0) {
                if(!log_job_add_key_prefix(jb, value, value ? strlen(value) : 0))
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
            else {
                i--;
                if (!jb->pattern) {
                    jb->pattern = arg;
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
        log2journal_command_line_help(argv[0]);
        return false;
    }

    return true;
}
