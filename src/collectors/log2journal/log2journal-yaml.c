// SPDX-License-Identifier: GPL-3.0-or-later

#include "log2journal.h"
#include "libnetdata/json/json-c-parser-inline.h"
#include "libnetdata/yaml/yaml.h"

// ----------------------------------------------------------------------------
// yaml configuration file

#ifdef HAVE_LIBYAML

// ----------------------------------------------------------------------------
// JSON-C based YAML parsing using libnetdata's YAML parser

static bool log2journal_config_from_json(json_object *jobj, void *data, BUFFER *error) {
    char path[1024]; path[0] = '\0';
    LOG_JOB *jb = data;

    // Parse pattern (optional - despite being conceptually required, we handle it gracefully)
    CLEAN_CHAR_P *pattern = NULL;
    JSONC_PARSE_TXT2STRDUPZ_OR_ERROR_AND_RETURN(jobj, path, "pattern", pattern, error, JSONC_OPTIONAL);
    if(pattern) {
        if(!log_job_pattern_set(jb, pattern, strlen(pattern))) {
            buffer_sprintf(error, "failed to set pattern");
            return false;
        }
    }

    // Parse prefix (optional)
    CLEAN_CHAR_P *prefix = NULL;
    JSONC_PARSE_TXT2STRDUPZ_OR_ERROR_AND_RETURN(jobj, path, "prefix", prefix, error, JSONC_OPTIONAL);
    if(prefix) {
        if(!log_job_key_prefix_set(jb, prefix, strlen(prefix))) {
            buffer_sprintf(error, "failed to set prefix");
            return false;
        }
    }

    // Parse filename injection (optional)
    JSONC_PARSE_SUBOBJECT(jobj, path, "filename", error, JSONC_STRICT, {
        CLEAN_CHAR_P *key = NULL;
        JSONC_PARSE_TXT2STRDUPZ_OR_ERROR_AND_RETURN(jobj, path, "key", key, error, JSONC_REQUIRED);
        if(key) {
            if(!log_job_filename_key_set(jb, key, strlen(key))) {
                buffer_sprintf(error, "failed to set filename key");
                return false;
            }
        }
    });

    // Parse filter (optional)
    JSONC_PARSE_SUBOBJECT(jobj, path, "filter", error, JSONC_STRICT, {
        CLEAN_CHAR_P *include = NULL;
        JSONC_PARSE_TXT2STRDUPZ_OR_ERROR_AND_RETURN(jobj, path, "include", include, error, JSONC_OPTIONAL);
        if(include) {
            if(!log_job_include_pattern_set(jb, include, strlen(include))) {
                buffer_sprintf(error, "failed to set include pattern");
                return false;
            }
        }

        CLEAN_CHAR_P *exclude = NULL;
        JSONC_PARSE_TXT2STRDUPZ_OR_ERROR_AND_RETURN(jobj, path, "exclude", exclude, error, JSONC_OPTIONAL);
        if(exclude) {
            if(!log_job_exclude_pattern_set(jb, exclude, strlen(exclude))) {
                buffer_sprintf(error, "failed to set exclude pattern");
                return false;
            }
        }
    });

    // Parse injections array (optional)
    JSONC_PARSE_ARRAY(jobj, path, "inject", error, JSONC_STRICT, {
        size_t i;
        JSONC_PARSE_ARRAY_ITEM_OBJECT(jobj, path, i, JSONC_REQUIRED, {
            CLEAN_CHAR_P *key = NULL;
            CLEAN_CHAR_P *value = NULL;
            JSONC_PARSE_TXT2STRDUPZ_OR_ERROR_AND_RETURN(jobj, path, "key", key, error, JSONC_REQUIRED);
            JSONC_PARSE_SCALAR2STRDUPZ_OR_ERROR_AND_RETURN(jobj, path, "value", value, error, JSONC_REQUIRED);
            if(key && value) {
                if(!log_job_injection_add(jb, key, strlen(key), value, strlen(value), false)) {
                    buffer_sprintf(error, "failed to add injection for '%s.inject'", path);
                    return false;
                }
            }
        });
    });

    // Parse rename array (optional)
    JSONC_PARSE_ARRAY(jobj, path, "rename", error, JSONC_STRICT, {
        size_t i;
        JSONC_PARSE_ARRAY_ITEM_OBJECT(jobj, path, i, JSONC_REQUIRED, {
            CLEAN_CHAR_P *new_key = NULL;
            CLEAN_CHAR_P *old_key = NULL;
            JSONC_PARSE_TXT2STRDUPZ_OR_ERROR_AND_RETURN(jobj, path, "new_key", new_key, error, JSONC_REQUIRED);
            JSONC_PARSE_TXT2STRDUPZ_OR_ERROR_AND_RETURN(jobj, path, "old_key", old_key, error, JSONC_REQUIRED);
            if(new_key && old_key) {
                if(!log_job_rename_add(jb, new_key, strlen(new_key), old_key, strlen(old_key))) {
                    buffer_sprintf(error, "failed to add rename for '%s.rename'", path);
                    return false;
                }
            }
        });
    });

    // Parse rewrite array (optional)
    JSONC_PARSE_ARRAY(jobj, path, "rewrite", error, JSONC_STRICT, {
        size_t i;
        JSONC_PARSE_ARRAY_ITEM_OBJECT(jobj, path, i, JSONC_REQUIRED, {
            CLEAN_CHAR_P *key = NULL;
            CLEAN_CHAR_P *match = NULL;
            CLEAN_CHAR_P *not_empty = NULL;
            CLEAN_CHAR_P *value = NULL;
            RW_FLAGS flags = RW_NONE;

            JSONC_PARSE_TXT2STRDUPZ_OR_ERROR_AND_RETURN(jobj, path, "key", key, error, JSONC_REQUIRED);
            JSONC_PARSE_TXT2STRDUPZ_OR_ERROR_AND_RETURN(jobj, path, "match", match, error, JSONC_OPTIONAL);
            JSONC_PARSE_TXT2STRDUPZ_OR_ERROR_AND_RETURN(jobj, path, "not_empty", not_empty, error, JSONC_OPTIONAL);
            JSONC_PARSE_SCALAR2STRDUPZ_OR_ERROR_AND_RETURN(jobj, path, "value", value, error, JSONC_REQUIRED);

            bool stop = true;
            bool inject = false;
            JSONC_PARSE_BOOL_OR_ERROR_AND_RETURN(jobj, path, "stop", stop, error, JSONC_OPTIONAL);
            JSONC_PARSE_BOOL_OR_ERROR_AND_RETURN(jobj, path, "inject", inject, error, JSONC_OPTIONAL);
            
            if(match) flags |= RW_MATCH_PCRE2;
            else if(not_empty) flags |= RW_MATCH_NON_EMPTY;
            if(!stop) flags |= RW_DONT_STOP;
            if(inject) flags |= RW_INJECT;
            
            if(key && value) {
                if(!log_job_rewrite_add(jb, key, flags, match ? match : not_empty, value)) {
                    buffer_sprintf(error, "failed to add rewrite for '%s.rewrite'", path);
                    return false;
                }
            }
        });
    });

    // Parse unmatched section (optional)
    JSONC_PARSE_SUBOBJECT(jobj, path, "unmatched", error, JSONC_STRICT, {
        CLEAN_CHAR_P *key = NULL;
        JSONC_PARSE_TXT2STRDUPZ_OR_ERROR_AND_RETURN(jobj, path, "key", key, error, JSONC_OPTIONAL);
        if(key) {
            hashed_key_set(&jb->unmatched.key, key, strlen(key));
        }

        // Parse unmatched injections
        JSONC_PARSE_ARRAY(jobj, path, "inject", error, JSONC_STRICT, {
            size_t i;
            JSONC_PARSE_ARRAY_ITEM_OBJECT(jobj, path, i, JSONC_REQUIRED, {
                CLEAN_CHAR_P *inj_key = NULL;
                CLEAN_CHAR_P *inj_value = NULL;
                JSONC_PARSE_TXT2STRDUPZ_OR_ERROR_AND_RETURN(jobj, path, "key", inj_key, error, JSONC_REQUIRED);
                JSONC_PARSE_SCALAR2STRDUPZ_OR_ERROR_AND_RETURN(jobj, path, "value", inj_value, error, JSONC_REQUIRED);
                if(inj_key && inj_value) {
                    if(!log_job_injection_add(jb, inj_key, strlen(inj_key), inj_value, strlen(inj_value), true)) {
                        buffer_sprintf(error, "failed to add unmatched injection for '%s.unmatched.inject'", path);
                        return false;
                    }
                }
            });
        });
    });

    return true;
}

bool yaml_parse_file(const char *config_file_path, LOG_JOB *jb) {
    if(!config_file_path || !*config_file_path) {
        l2j_log("yaml configuration filename cannot be empty.");
        return false;
    }

    BUFFER *error = buffer_create(0, NULL);
    
    // Parse YAML to JSON-C using libnetdata's YAML parser with all values as strings
    struct json_object *json = yaml_parse_filename(config_file_path, error, YAML2JSON_ALL_VALUES_AS_STRINGS);
    if (!json) {
        l2j_log("Error parsing YAML file %s: %s", config_file_path, buffer_tostring(error));
        buffer_free(error);
        return false;
    }

    // Parse JSON-C to LOG_JOB structure
    buffer_flush(error);
    bool success = log2journal_config_from_json(json, jb, error);
    
    if(!success) {
        l2j_log("Error parsing configuration: %s", buffer_tostring(error));
    }
    
    json_object_put(json);
    buffer_free(error);
    
    return success;
}

bool yaml_parse_config(const char *config_name, LOG_JOB *jb) {
    char filename[FILENAME_MAX + 1];

    snprintf(filename, sizeof(filename), "%s/%s.yaml", LOG2JOURNAL_CONFIG_PATH, config_name);
    return yaml_parse_file(filename, jb);
}

#endif // HAVE_LIBYAML

// ----------------------------------------------------------------------------
// printing yaml

static void yaml_print_multiline_value(const char *s, size_t depth) {
    if (!s)
        s = "";

    do {
        const char* next = strchr(s, '\n');
        if(next) next++;

        size_t len = next ? (size_t)(next - s) : strlen(s);
        char buf[len + 1];
        copy_to_buffer(buf, sizeof(buf), s, len);

        fprintf(stderr, "%.*s%s%s",
                (int)(depth * 2), "                    ",
                buf, next ? "" : "\n");

        s = next;
    } while(s && *s);
}

static bool needs_quotes_in_yaml(const char *str) {
    // Lookup table for special YAML characters
    static bool special_chars[256] = { false };
    static bool table_initialized = false;

    if (!table_initialized) {
        // Initialize the lookup table
        const char *special_chars_str = ":{}[],&*!|>'\"%@`^";
        for (const char *c = special_chars_str; *c; ++c) {
            special_chars[(unsigned char)*c] = true;
        }
        table_initialized = true;
    }

    while (*str) {
        if (special_chars[(unsigned char)*str]) {
            return true;
        }
        str++;
    }
    return false;
}

static void yaml_print_node(const char *key, const char *value, size_t depth, bool dash) {
    if(depth > 10) depth = 10;
    const char *quote = "'";

    const char *second_line = NULL;
    if(value && strchr(value, '\n')) {
        second_line = value;
        value = "|";
        quote = "";
    }
    else if(!value || !needs_quotes_in_yaml(value))
        quote = "";

    fprintf(stderr, "%.*s%s%s%s%s%s%s\n",
            (int)(depth * 2), "                    ", dash ? "- ": "",
            key ? key : "", key ? ": " : "",
            quote, value ? value : "", quote);

    if(second_line) {
        yaml_print_multiline_value(second_line, depth + 1);
    }
}

void log_job_configuration_to_yaml(LOG_JOB *jb) {
    if(jb->pattern)
        yaml_print_node("pattern", jb->pattern, 0, false);

    if(jb->prefix) {
        fprintf(stderr, "\n");
        yaml_print_node("prefix", jb->prefix, 0, false);
    }

    if(jb->filename.key.key) {
        fprintf(stderr, "\n");
        yaml_print_node("filename", NULL, 0, false);
        yaml_print_node("key", jb->filename.key.key, 1, false);
    }

    if(jb->filter.include.pattern || jb->filter.exclude.pattern) {
        fprintf(stderr, "\n");
        yaml_print_node("filter", NULL, 0, false);

        if(jb->filter.include.pattern)
            yaml_print_node("include", jb->filter.include.pattern, 1, false);

        if(jb->filter.exclude.pattern)
            yaml_print_node("exclude", jb->filter.exclude.pattern, 1, false);
    }

    if(jb->renames.used) {
        fprintf(stderr, "\n");
        yaml_print_node("rename", NULL, 0, false);

        for(size_t i = 0; i < jb->renames.used ;i++) {
            yaml_print_node("new_key", jb->renames.array[i].new_key.key, 1, true);
            yaml_print_node("old_key", jb->renames.array[i].old_key.key, 2, false);
        }
    }

    if(jb->injections.used) {
        fprintf(stderr, "\n");
        yaml_print_node("inject", NULL, 0, false);

        for (size_t i = 0; i < jb->injections.used; i++) {
            yaml_print_node("key", jb->injections.keys[i].key.key, 1, true);
            yaml_print_node("value", jb->injections.keys[i].value.pattern, 2, false);
        }
    }

    if(jb->rewrites.used) {
        fprintf(stderr, "\n");
        yaml_print_node("rewrite", NULL, 0, false);

        for(size_t i = 0; i < jb->rewrites.used ;i++) {
            REWRITE *rw = &jb->rewrites.array[i];

            yaml_print_node("key", rw->key.key, 1, true);

            if(rw->flags & RW_MATCH_PCRE2)
                yaml_print_node("match", rw->match_pcre2.pattern, 2, false);

            else if(rw->flags & RW_MATCH_NON_EMPTY)
                yaml_print_node("not_empty", rw->match_non_empty.pattern, 2, false);

            yaml_print_node("value", rw->value.pattern, 2, false);

            if(rw->flags & RW_INJECT)
                yaml_print_node("inject", "yes", 2, false);

            if(rw->flags & RW_DONT_STOP)
                yaml_print_node("stop", "no", 2, false);
        }
    }

    if(jb->unmatched.key.key || jb->unmatched.injections.used) {
        fprintf(stderr, "\n");
        yaml_print_node("unmatched", NULL, 0, false);

        if(jb->unmatched.key.key)
            yaml_print_node("key", jb->unmatched.key.key, 1, false);

        if(jb->unmatched.injections.used) {
            fprintf(stderr, "\n");
            yaml_print_node("inject", NULL, 1, false);

            for (size_t i = 0; i < jb->unmatched.injections.used; i++) {
                yaml_print_node("key", jb->unmatched.injections.keys[i].key.key, 2, true);
                yaml_print_node("value", jb->unmatched.injections.keys[i].value.pattern, 3, false);
            }
        }
    }
}
