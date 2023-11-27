// SPDX-License-Identifier: GPL-3.0-or-later

#include "log2journal.h"

// ----------------------------------------------------------------------------

static char *rewrite_value(struct log_job *jb, const char *key, XXH64_hash_t hash, const char *value, size_t value_len) {
    static __thread char rewritten_value[MAX_VALUE_LEN + 1];

    for (size_t i = 0; i < jb->rewrites.used; i++) {
        struct key_rewrite *rw = &jb->rewrites.array[i];

        if (rw->hash == hash && strcmp(rw->key, key) == 0) {
            if (!jb_pcre2_match(rw->re, rw->match_data, (char *)value, value_len, false)) {
                continue; // No match found, skip to next rewrite rule
            }

            PCRE2_SIZE *ovector = pcre2_get_ovector_pointer(rw->match_data);

            char *buffer = rewritten_value;
            size_t buffer_remaining = sizeof(rewritten_value);

            // Iterate through the linked list of replacement nodes
            for (struct replacement_node *node = rw->nodes; node != NULL; node = node->next) {
                if (node->is_variable) {
                    uint32_t groupnumber = pcre2_substring_number_from_name(rw->re, (PCRE2_SPTR)node->s);
                    PCRE2_SIZE start_offset = ovector[2 * groupnumber];
                    PCRE2_SIZE end_offset = ovector[2 * groupnumber + 1];
                    PCRE2_SIZE length = end_offset - start_offset;

                    size_t copied = copy_to_buffer(buffer, buffer_remaining, value + start_offset, length);
                    buffer += copied;
                    buffer_remaining -= copied;
                }
                else {
                    size_t len = node->len;
                    size_t copied = copy_to_buffer(buffer, buffer_remaining, node->s, len);
                    buffer += copied;
                    buffer_remaining -= copied;
                }
            }

            return rewritten_value;
        }
    }

    return NULL;
}

static inline const char *rename_key(struct log_job *jb, const char *key, XXH64_hash_t hash, XXH64_hash_t *new_hash) {
    for(size_t i = 0; i < jb->renames.used ;i++) {
        struct key_rename *rn = &jb->renames.array[i];

        if(rn->old_hash == hash && strcmp(rn->old_key, key) == 0) {
            *new_hash = rn->new_hash;
            return rn->new_key;
        }
    }

    *new_hash = hash;
    return key;
}

// ----------------------------------------------------------------------------

static inline void send_key_value_error(const char *key, const char *format, ...) __attribute__ ((format(__printf__, 2, 3)));
static inline void send_key_value_error(const char *key, const char *format, ...) {
    printf("%s=", key);
    va_list args;
    va_start(args, format);
    vprintf(format, args);
    va_end(args);
    printf("\n");
}

inline void jb_send_key_value_and_rewrite(struct log_job *jb, const char *key, XXH64_hash_t hash, const char *value, size_t len) {
    char *rewritten = rewrite_value(jb, key, hash, value, len);
    if(!rewritten)
        printf("%s=%.*s\n", key, (int)len, value);
    else
        printf("%s=%s\n", key, rewritten);
}

inline void jb_send_extracted_key_value(struct log_job *jb, const char *key, const char *value, size_t len) {
    XXH64_hash_t hash = XXH3_64bits(key, strlen(key));

    // process renames (changing the key)
    XXH64_hash_t new_hash;
    const char *new_key = rename_key(jb, key, hash, &new_hash);

    // process rewrites (changing the value)
    // and send it to output
    jb_send_key_value_and_rewrite(jb, new_key, new_hash, value, len);

    // process the duplications (using the original key)
    // and send them to output
    jb_send_duplications_for_key(jb, key, hash, value, len);
}

static inline void send_key_value_constant(struct log_job *jb, const char *key, const char *value) {
    printf("%s=%s\n", key, value);
}

// ----------------------------------------------------------------------------
// injection of constant fields

static void jb_select_which_injections_should_be_injected_on_unmatched(struct log_job *jb) {
    // mark all injections to be added to unmatched logs
    for(size_t i = 0; i < jb->injections.used ; i++)
        jb->injections.keys[i].on_unmatched = true;

    if(jb->injections.used && jb->unmatched.injections.used) {
        // we have both injections and injections on unmatched

        // we find all the injections that are also configured as injections on unmatched,
        // and we disable them, so that the output will not have the same key twice

        for(size_t i = 0; i < jb->injections.used ;i++) {
            for(size_t u = 0; u < jb->unmatched.injections.used ; u++) {
                if(strcmp(jb->injections.keys[i].key, jb->unmatched.injections.keys[u].key) == 0)
                    jb->injections.keys[i].on_unmatched = false;
            }
        }
    }
}


static inline void jb_finalize_injections(struct log_job *jb, bool line_is_matched) {
    for (size_t j = 0; j < jb->injections.used; j++) {
        if(!line_is_matched && !jb->injections.keys[j].on_unmatched)
            continue;

        send_key_value_constant(jb, jb->injections.keys[j].key, jb->injections.keys[j].value.s);
    }
}

static inline void jb_reset_injections(struct log_job *jb) {
    for(size_t d = 0; d < jb->dups.used ; d++) {
        struct key_dup *kd = &jb->dups.array[d];
        kd->exposed = false;

        for(size_t g = 0; g < kd->used ; g++) {
            if(kd->values[g].s)
                kd->values[g].s[0] = '\0';
        }
    }
}

// ----------------------------------------------------------------------------
// duplications

inline void jb_send_duplications_for_key(struct log_job *jb, const char *key, XXH64_hash_t hash, const char *value, size_t value_len) {
    // IMPORTANT:
    // The 'value' may not be NULL terminated and have more data that the value we need

    for (size_t d = 0; d < jb->dups.used; d++) {
        struct key_dup *kd = &jb->dups.array[d];

        if(kd->exposed || kd->used == 0)
            continue;

        if(kd->used == 1) {
            // just one key to be duplicated
            if(strcmp(kd->keys[0], key) == 0) {
                jb_send_key_value_and_rewrite(jb, kd->target, kd->hash, value, value_len);
                kd->exposed = true;
            }
        }
        else {
            // multiple keys to be duplicated
            for(size_t g = 0; g < kd->used ; g++) {
                if(strcmp(kd->keys[g], key) == 0)
                    txt_replace(&kd->values[g], value, value_len);
            }
        }
    }
}

static inline void jb_send_remaining_duplications(struct log_job *jb) {
    static __thread char buffer[MAX_VALUE_LEN + 1];

    // IMPORTANT:
    // all duplications are exposed, even the ones we haven't found their keys in the source,
    // so that the output always has the same fields for matched entries.

    for(size_t d = 0; d < jb->dups.used ; d++) {
        struct key_dup *kd = &jb->dups.array[d];

        if(kd->exposed || kd->used == 0)
            continue;

        buffer[0] = '\0';
        size_t remaining = sizeof(buffer);
        char *s = buffer;

        for(size_t g = 0; g < kd->used ; g++) {
            if(remaining < 2) {
                log2stderr("Warning: duplicated key '%s' cannot fit the values.", kd->target);
                break;
            }

            if(g > 0) {
                *s++ = ',';
                *s = '\0';
                remaining--;
            }

            char *value = (kd->values[g].s && kd->values[g].s[0]) ? kd->values[g].s : "[unavailable]";
            size_t len = strlen(value);
            size_t copied = copy_to_buffer(s, remaining, value, len);
            remaining -= copied;
            s += copied;

            if(copied != len) {
                log2stderr("Warning: duplicated key '%s' will have truncated value", jb->dups.array[d].target);
                break;
            }
        }
        jb_send_key_value_and_rewrite(jb, kd->target, kd->hash, buffer, s - buffer);
    }
}

// ----------------------------------------------------------------------------
// filename injection

static inline void jb_inject_filename(struct log_job *jb) {
    if (jb->filename.key && jb->filename.current[0])
        send_key_value_constant(jb, jb->filename.key, jb->filename.current);
}

static inline bool jb_switched_filename(struct log_job *jb, const char *line, size_t len) {
    // IMPORTANT:
    // Return TRUE when the caller should skip this line (because it is ours).
    // Unfortunately, we have to consume empty lines too.

    // IMPORTANT:
    // filename may not be NULL terminated and have more data than the filename.

    if (!len) {
        jb->filename.last_line_was_empty = true;
        return true;
    }

    // Check if it's a log file change line
    if (jb->filename.last_line_was_empty && line[0] == '=' && strncmp(line, "==> ", 4) == 0) {
        const char *start = line + 4;
        const char *end = strstr(line, " <==");
        while (*start == ' ') start++;
        if (*start != '\n' && *start != '\0' && end) {
            copy_to_buffer(jb->filename.current, sizeof(jb->filename.current),
                           start, end - start);
            return true;
        }
    }

    jb->filename.last_line_was_empty = false;
    return false;
}

// ----------------------------------------------------------------------------
// input reading

static char *get_next_line(struct log_job *jb, char *buffer, size_t size, size_t *line_length) {
    if(!fgets(buffer, (int)size, stdin)) {
        *line_length = 0;
        return NULL;
    }

    char *line = buffer;
    size_t len = strlen(line);

    // remove trailing newlines and spaces
    while(len > 1 && (line[len - 1] == '\n' || isspace(line[len - 1])))
        line[--len] = '\0';

    // skip leading spaces
    while(isspace(*line)) {
        line++;
        len--;
    }

    *line_length = len;
    return line;
}

// ----------------------------------------------------------------------------

static inline void jb_traverse_pcre2_named_groups_and_send_keys(struct log_job *jb, pcre2_code *re, pcre2_match_data *match_data, char *line) {
    PCRE2_SIZE *ovector = pcre2_get_ovector_pointer(match_data);
    uint32_t namecount;
    pcre2_pattern_info(re, PCRE2_INFO_NAMECOUNT, &namecount);

    if (namecount > 0) {
        PCRE2_SPTR name_table;
        pcre2_pattern_info(re, PCRE2_INFO_NAMETABLE, &name_table);
        uint32_t name_entry_size;
        pcre2_pattern_info(re, PCRE2_INFO_NAMEENTRYSIZE, &name_entry_size);

        const unsigned char *tabptr = name_table;
        for (uint32_t i = 0; i < namecount; i++) {
            int n = (tabptr[0] << 8) | tabptr[1];
            const char *group_name = (const char *)(tabptr + 2);

            PCRE2_SIZE start_offset = ovector[2 * n];
            PCRE2_SIZE end_offset = ovector[2 * n + 1];
            PCRE2_SIZE group_length = end_offset - start_offset;

            jb_send_extracted_key_value(jb, group_name, line + start_offset, group_length);
            tabptr += name_entry_size;
        }
    }
}

// ----------------------------------------------------------------------------

struct log_job log_job = { 0 };
int main(int argc, char *argv[]) {
    struct log_job *jb = &log_job;

    if(!parse_log2journal_parameters(jb, argc, argv))
        exit(1);

    if(jb->show_config)
        log_job_to_yaml(jb);

    jb_select_which_injections_should_be_injected_on_unmatched(jb);

    pcre2_code *pcre2 = NULL;
    pcre2_match_data *match_data = NULL;
    LOG_JSON_STATE *json = NULL;
    LOGFMT_STATE *logfmt = NULL;
    if(strcmp(jb->pattern, "json") == 0) {
        json = json_parser_create(jb);
    }
    else if(strcmp(jb->pattern, "logfmt") == 0) {
        logfmt = logfmt_parser_create(jb);
    }
    else {
        pcre2 = jb_compile_pcre2_pattern(jb->pattern);
        if(!pcre2)
            return 1;

        match_data = pcre2_match_data_create_from_pattern(pcre2, NULL);
        if(!match_data)
            return 1;
    }

    char buffer[MAX_LINE_LENGTH];
    char *line;
    size_t len;

    while ((line = get_next_line(jb, buffer, sizeof(buffer), &len))) {
        if(jb_switched_filename(jb, line, len))
            continue;

        jb_reset_injections(jb);

        bool line_is_matched;

        if(json)
            line_is_matched = json_parse_document(json, line);
        else if(logfmt)
            line_is_matched = logfmt_parse_document(logfmt, line);
        else
            line_is_matched = jb_pcre2_match(pcre2, match_data, line, len, true);

        if(!line_is_matched) {
            if(json)
                log2stderr("%s", json_parser_error(json));
            else if(logfmt)
                log2stderr("%s", logfmt_parser_error(logfmt));

            if (jb->unmatched.key) {
                // we are sending errors to systemd-journal
                send_key_value_error(jb->unmatched.key, "Parsing error on: %s", line);

                for (size_t j = 0; j < jb->unmatched.injections.used; j++)
                    send_key_value_constant(jb, jb->unmatched.injections.keys[j].key,
                                            jb->unmatched.injections.keys[j].value.s);
            }
            else {
                // we are just logging errors to stderr
                continue;
            }
        }
        else {
            if(pcre2)
                jb_traverse_pcre2_named_groups_and_send_keys(jb, pcre2, match_data, line);

            // print all non-exposed duplications
            jb_send_remaining_duplications(jb);
        }

        jb_inject_filename(jb);
        jb_finalize_injections(jb, line_is_matched);

        printf("\n");
        fflush(stdout);
    }

    if(json)
        json_parser_destroy(json);

    else if(logfmt)
        logfmt_parser_destroy(logfmt);

    else if(pcre2) {
        pcre2_match_data_free(match_data);
        pcre2_code_free(pcre2);
    }

    nd_log_destroy(jb);
    return 0;
}
