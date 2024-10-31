// SPDX-License-Identifier: GPL-3.0-or-later

#include "log2journal.h"

// ----------------------------------------------------------------------------
// yaml configuration file

#ifdef HAVE_LIBYAML

static const char *yaml_event_name(yaml_event_type_t type) {
    switch (type) {
        case YAML_NO_EVENT:
            return "YAML_NO_EVENT";

        case YAML_SCALAR_EVENT:
            return "YAML_SCALAR_EVENT";

        case YAML_ALIAS_EVENT:
            return "YAML_ALIAS_EVENT";

        case YAML_MAPPING_START_EVENT:
            return "YAML_MAPPING_START_EVENT";

        case YAML_MAPPING_END_EVENT:
            return "YAML_MAPPING_END_EVENT";

        case YAML_SEQUENCE_START_EVENT:
            return "YAML_SEQUENCE_START_EVENT";

        case YAML_SEQUENCE_END_EVENT:
            return "YAML_SEQUENCE_END_EVENT";

        case YAML_STREAM_START_EVENT:
            return "YAML_STREAM_START_EVENT";

        case YAML_STREAM_END_EVENT:
            return "YAML_STREAM_END_EVENT";

        case YAML_DOCUMENT_START_EVENT:
            return "YAML_DOCUMENT_START_EVENT";

        case YAML_DOCUMENT_END_EVENT:
            return "YAML_DOCUMENT_END_EVENT";

        default:
            return "UNKNOWN";
    }
}

#define yaml_error(parser, event, fmt, args...) yaml_error_with_trace(parser, event, __LINE__, __FUNCTION__, __FILE__, fmt, ##args)
static void yaml_error_with_trace(yaml_parser_t *parser, yaml_event_t *event, size_t line, const char *function, const char *file, const char *format, ...) PRINTFLIKE(6, 7);
static void yaml_error_with_trace(yaml_parser_t *parser, yaml_event_t *event, size_t line, const char *function, const char *file, const char *format, ...) {
    char buf[1024] = ""; // Initialize buf to an empty string
    const char *type = "";

    if(event) {
        type = yaml_event_name(event->type);

        switch (event->type) {
            case YAML_SCALAR_EVENT:
                copy_to_buffer(buf, sizeof(buf), (char *)event->data.scalar.value, event->data.scalar.length);
                break;

            case YAML_ALIAS_EVENT:
                snprintf(buf, sizeof(buf), "%s", event->data.alias.anchor);
                break;

            default:
                break;
        }
    }

    fprintf(stderr, "YAML %zu@%s, %s(): (line %d, column %d, %s%s%s): ",
            line, file, function,
            (int)(parser->mark.line + 1), (int)(parser->mark.column + 1),
            type, buf[0]? ", near ": "", buf);

    va_list args;
    va_start(args, format);
    vfprintf(stderr, format, args);
    va_end(args);
    fprintf(stderr, "\n");
}

#define yaml_parse(parser, event) yaml_parse_with_trace(parser, event, __LINE__, __FUNCTION__, __FILE__)
static bool yaml_parse_with_trace(yaml_parser_t *parser, yaml_event_t *event, size_t line __maybe_unused, const char *function __maybe_unused, const char *file __maybe_unused) {
    if (!yaml_parser_parse(parser, event)) {
        yaml_error(parser, NULL, "YAML parser error %u", parser->error);
        return false;
    }

//    fprintf(stderr, ">>> %s >>> %.*s\n",
//            yaml_event_name(event->type),
//            event->type == YAML_SCALAR_EVENT ? event->data.scalar.length : 0,
//            event->type == YAML_SCALAR_EVENT ? (char *)event->data.scalar.value : "");

    return true;
}

#define yaml_parse_expect_event(parser, type) yaml_parse_expect_event_with_trace(parser, type, __LINE__, __FUNCTION__, __FILE__)
static bool yaml_parse_expect_event_with_trace(yaml_parser_t *parser, yaml_event_type_t type, size_t line, const char *function, const char *file) {
    yaml_event_t event;
    if (!yaml_parse(parser, &event))
        return false;

    bool ret = true;
    if(event.type != type) {
        yaml_error_with_trace(parser, &event, line, function, file, "unexpected event - expecting: %s", yaml_event_name(type));
        ret = false;
    }
//    else
//        fprintf(stderr, "OK (%zu@%s, %s()\n", line, file, function);

    yaml_event_delete(&event);
    return ret;
}

#define yaml_scalar_matches(event, s, len) yaml_scalar_matches_with_trace(event, s, len, __LINE__, __FUNCTION__, __FILE__)
static bool yaml_scalar_matches_with_trace(yaml_event_t *event, const char *s, size_t len, size_t line __maybe_unused, const char *function __maybe_unused, const char *file __maybe_unused) {
    if(event->type != YAML_SCALAR_EVENT)
        return false;

    if(len != event->data.scalar.length)
        return false;
//    else
//        fprintf(stderr, "OK (%zu@%s, %s()\n", line, file, function);

    return strcmp((char *)event->data.scalar.value, s) == 0;
}

// ----------------------------------------------------------------------------

static size_t yaml_parse_filename_injection(yaml_parser_t *parser, LOG_JOB *jb) {
    yaml_event_t event;
    size_t errors = 0;

    if(!yaml_parse_expect_event(parser, YAML_MAPPING_START_EVENT))
        return 1;

    if (!yaml_parse(parser, &event))
        return 1;

    if (yaml_scalar_matches(&event, "key", strlen("key"))) {
        yaml_event_t sub_event;
        if (!yaml_parse(parser, &sub_event))
            errors++;

        else {
            if (sub_event.type == YAML_SCALAR_EVENT) {
                if(!log_job_filename_key_set(jb, (char *) sub_event.data.scalar.value,
                                             sub_event.data.scalar.length))
                    errors++;
            }

            else {
                yaml_error(parser, &sub_event, "expected the filename as %s", yaml_event_name(YAML_SCALAR_EVENT));
                errors++;
            }

            yaml_event_delete(&sub_event);
        }
    }

    if(!yaml_parse_expect_event(parser, YAML_MAPPING_END_EVENT))
        errors++;

    yaml_event_delete(&event);
    return errors;
}

static size_t yaml_parse_filters(yaml_parser_t *parser, LOG_JOB *jb) {
    if(!yaml_parse_expect_event(parser, YAML_MAPPING_START_EVENT))
        return 1;

    size_t errors = 0;
    bool finished = false;

    while(!errors && !finished) {
        yaml_event_t event;

        if(!yaml_parse(parser, &event))
            return 1;

        if(event.type == YAML_SCALAR_EVENT) {
            if(yaml_scalar_matches(&event, "include", strlen("include"))) {
                yaml_event_t sub_event;
                if(!yaml_parse(parser, &sub_event))
                    errors++;

                else {
                    if(sub_event.type == YAML_SCALAR_EVENT) {
                        if(!log_job_include_pattern_set(jb, (char *) sub_event.data.scalar.value,
                                                        sub_event.data.scalar.length))
                            errors++;
                    }

                    else {
                        yaml_error(parser, &sub_event, "expected the include as %s",
                                   yaml_event_name(YAML_SCALAR_EVENT));
                        errors++;
                    }

                    yaml_event_delete(&sub_event);
                }
            }
            else if(yaml_scalar_matches(&event, "exclude", strlen("exclude"))) {
                yaml_event_t sub_event;
                if(!yaml_parse(parser, &sub_event))
                    errors++;

                else {
                    if(sub_event.type == YAML_SCALAR_EVENT) {
                        if(!log_job_exclude_pattern_set(jb,(char *) sub_event.data.scalar.value,
                                                        sub_event.data.scalar.length))
                            errors++;
                    }

                    else {
                        yaml_error(parser, &sub_event, "expected the exclude as %s",
                                   yaml_event_name(YAML_SCALAR_EVENT));
                        errors++;
                    }

                    yaml_event_delete(&sub_event);
                }
            }
        }
        else if(event.type == YAML_MAPPING_END_EVENT)
            finished = true;
        else {
            yaml_error(parser, &event, "expected %s or %s",
                       yaml_event_name(YAML_SCALAR_EVENT),
                       yaml_event_name(YAML_MAPPING_END_EVENT));
            errors++;
        }

        yaml_event_delete(&event);
    }

    return errors;
}

static size_t yaml_parse_prefix(yaml_parser_t *parser, LOG_JOB *jb) {
    yaml_event_t event;
    size_t errors = 0;

    if (!yaml_parse(parser, &event))
        return 1;

    if (event.type == YAML_SCALAR_EVENT) {
        if(!log_job_key_prefix_set(jb, (char *) event.data.scalar.value, event.data.scalar.length))
            errors++;
    }

    yaml_event_delete(&event);
    return errors;
}

static bool yaml_parse_constant_field_injection(yaml_parser_t *parser, LOG_JOB *jb, bool unmatched) {
    yaml_event_t event;
    if (!yaml_parse(parser, &event) || event.type != YAML_SCALAR_EVENT) {
        yaml_error(parser, &event, "Expected scalar for constant field injection key");
        yaml_event_delete(&event);
        return false;
    }

    char *key = strndupz((char *)event.data.scalar.value, event.data.scalar.length);
    char *value = NULL;
    bool ret = false;

    yaml_event_delete(&event);

    if (!yaml_parse(parser, &event) || event.type != YAML_SCALAR_EVENT) {
        yaml_error(parser, &event, "Expected scalar for constant field injection value");
        goto cleanup;
    }

    if(!yaml_scalar_matches(&event, "value", strlen("value"))) {
        yaml_error(parser, &event, "Expected scalar 'value'");
        goto cleanup;
    }

    yaml_event_delete(&event);

    if (!yaml_parse(parser, &event) || event.type != YAML_SCALAR_EVENT) {
        yaml_error(parser, &event, "Expected scalar for constant field injection value");
        goto cleanup;
    }

    value = strndupz((char *)event.data.scalar.value, event.data.scalar.length);

    if(!log_job_injection_add(jb, key, strlen(key), value, strlen(value), unmatched))
        ret = false;
    else
        ret = true;

    ret = true;

cleanup:
    yaml_event_delete(&event);
    freez(key);
    freez(value);
    return !ret ? 1 : 0;
}

static bool yaml_parse_injection_mapping(yaml_parser_t *parser, LOG_JOB *jb, bool unmatched) {
    yaml_event_t event;
    size_t errors = 0;
    bool finished = false;

    while (!errors && !finished) {
        if (!yaml_parse(parser, &event)) {
            errors++;
            continue;
        }

        switch (event.type) {
            case YAML_SCALAR_EVENT:
                if (yaml_scalar_matches(&event, "key", strlen("key"))) {
                    errors += yaml_parse_constant_field_injection(parser, jb, unmatched) ? 1 : 0;
                } else {
                    yaml_error(parser, &event, "Unexpected scalar in injection mapping");
                    errors++;
                }
                break;

            case YAML_MAPPING_END_EVENT:
                finished = true;
                break;

            default:
                yaml_error(parser, &event, "Unexpected event in injection mapping");
                errors++;
                break;
        }

        yaml_event_delete(&event);
    }

    return errors == 0;
}

static size_t yaml_parse_injections(yaml_parser_t *parser, LOG_JOB *jb, bool unmatched) {
    yaml_event_t event;
    size_t errors = 0;
    bool finished = false;

    if (!yaml_parse_expect_event(parser, YAML_SEQUENCE_START_EVENT))
        return 1;

    while (!errors && !finished) {
        if (!yaml_parse(parser, &event)) {
            errors++;
            continue;
        }

        switch (event.type) {
            case YAML_MAPPING_START_EVENT:
                if (!yaml_parse_injection_mapping(parser, jb, unmatched))
                    errors++;
                break;

            case YAML_SEQUENCE_END_EVENT:
                finished = true;
                break;

            default:
                yaml_error(parser, &event, "Unexpected event in injections sequence");
                errors++;
                break;
        }

        yaml_event_delete(&event);
    }

    return errors;
}

static size_t yaml_parse_unmatched(yaml_parser_t *parser, LOG_JOB *jb) {
    size_t errors = 0;
    bool finished = false;

    if (!yaml_parse_expect_event(parser, YAML_MAPPING_START_EVENT))
        return 1;

    while (!errors && !finished) {
        yaml_event_t event;
        if (!yaml_parse(parser, &event)) {
            errors++;
            continue;
        }

        switch (event.type) {
            case YAML_SCALAR_EVENT:
                if (yaml_scalar_matches(&event, "key", strlen("key"))) {
                    yaml_event_t sub_event;
                    if (!yaml_parse(parser, &sub_event)) {
                        errors++;
                    } else {
                        if (sub_event.type == YAML_SCALAR_EVENT) {
                            hashed_key_set(
                                &jb->unmatched.key, (char *)sub_event.data.scalar.value, sub_event.data.scalar.length);
                        } else {
                            yaml_error(parser, &sub_event, "expected a scalar value for 'key'");
                            errors++;
                        }
                        yaml_event_delete(&sub_event);
                    }
                } else if (yaml_scalar_matches(&event, "inject", strlen("inject"))) {
                    errors += yaml_parse_injections(parser, jb, true);
                } else {
                    yaml_error(parser, &event, "Unexpected scalar in unmatched section");
                    errors++;
                }
                break;

            case YAML_MAPPING_END_EVENT:
                finished = true;
                break;

            default:
                yaml_error(parser, &event, "Unexpected event in unmatched section");
                errors++;
                break;
        }

        yaml_event_delete(&event);
    }

    return errors;
}

static bool yaml_parse_scalar_boolean(yaml_parser_t *parser, bool def, const char *where, size_t *errors) {
    bool rc = def;

    yaml_event_t value_event;
    if (!yaml_parse(parser, &value_event)) {
        (*errors)++;
        return rc;
    }

    if (value_event.type != YAML_SCALAR_EVENT) {
        yaml_error(parser, &value_event, "Expected scalar for %s boolean", where);
        (*errors)++;
    }
    else if(strncmp((char*)value_event.data.scalar.value, "yes", 3) == 0 ||
             strncmp((char*)value_event.data.scalar.value, "true", 4) == 0)
        rc = true;
    else if(strncmp((char*)value_event.data.scalar.value, "no", 2) == 0 ||
             strncmp((char*)value_event.data.scalar.value, "false", 5) == 0)
        rc = false;
    else {
        yaml_error(parser, &value_event, "Expected scalar for %s boolean: invalid value %s", where, value_event.data.scalar.value);
        rc = def;
    }

    yaml_event_delete(&value_event);
    return rc;
}

static bool handle_rewrite_event(yaml_parser_t *parser, yaml_event_t *event,
                                 char **key, char **search_pattern, char **replace_pattern,
                                 RW_FLAGS *flags, bool *mapping_finished,
                                 LOG_JOB *jb, size_t *errors) {
    switch (event->type) {
        case YAML_SCALAR_EVENT:
            if (yaml_scalar_matches(event, "key", strlen("key"))) {
                yaml_event_t value_event;
                if (!yaml_parse(parser, &value_event)) {
                    (*errors)++;
                    return false;
                }

                if (value_event.type != YAML_SCALAR_EVENT) {
                    yaml_error(parser, &value_event, "Expected scalar for rewrite key");
                    (*errors)++;
                } else {
                    freez(*key);
                    *key = strndupz((char *)value_event.data.scalar.value, value_event.data.scalar.length);
                }
                yaml_event_delete(&value_event);
            }
            else if (yaml_scalar_matches(event, "match", strlen("match"))) {
                yaml_event_t value_event;
                if (!yaml_parse(parser, &value_event)) {
                    (*errors)++;
                    return false;
                }

                if (value_event.type != YAML_SCALAR_EVENT) {
                    yaml_error(parser, &value_event, "Expected scalar for rewrite match PCRE2 pattern");
                    (*errors)++;
                }
                else {
                    freez(*search_pattern);
                    *flags |= RW_MATCH_PCRE2;
                    *flags &= ~RW_MATCH_NON_EMPTY;
                    *search_pattern = strndupz((char *)value_event.data.scalar.value, value_event.data.scalar.length);
                }
                yaml_event_delete(&value_event);
            }
            else if (yaml_scalar_matches(event, "not_empty", strlen("not_empty"))) {
                yaml_event_t value_event;
                if (!yaml_parse(parser, &value_event)) {
                    (*errors)++;
                    return false;
                }

                if (value_event.type != YAML_SCALAR_EVENT) {
                    yaml_error(parser, &value_event, "Expected scalar for rewrite not empty condition");
                    (*errors)++;
                }
                else {
                    freez(*search_pattern);
                    *flags |= RW_MATCH_NON_EMPTY;
                    *flags &= ~RW_MATCH_PCRE2;
                    *search_pattern = strndupz((char *)value_event.data.scalar.value, value_event.data.scalar.length);
                }
                yaml_event_delete(&value_event);
            }
            else if (yaml_scalar_matches(event, "value", strlen("value"))) {
                yaml_event_t value_event;
                if (!yaml_parse(parser, &value_event)) {
                    (*errors)++;
                    return false;
                }

                if (value_event.type != YAML_SCALAR_EVENT) {
                    yaml_error(parser, &value_event, "Expected scalar for rewrite value");
                    (*errors)++;
                } else {
                    freez(*replace_pattern);
                    *replace_pattern = strndupz((char *)value_event.data.scalar.value, value_event.data.scalar.length);
                }
                yaml_event_delete(&value_event);
            }
            else if (yaml_scalar_matches(event, "stop", strlen("stop"))) {
                if(yaml_parse_scalar_boolean(parser, true, "rewrite stop", errors))
                    *flags &= ~RW_DONT_STOP;
                else
                    *flags |= RW_DONT_STOP;
            }
            else if (yaml_scalar_matches(event, "inject", strlen("inject"))) {
                if(yaml_parse_scalar_boolean(parser, false, "rewrite inject", errors))
                    *flags |= RW_INJECT;
                else
                    *flags &= ~RW_INJECT;
            }
            else {
                yaml_error(parser, event, "Unexpected scalar in rewrite mapping");
                (*errors)++;
            }
            break;

        case YAML_MAPPING_END_EVENT:
            if(*key) {
                if (!log_job_rewrite_add(jb, *key, *flags, *search_pattern, *replace_pattern))
                    (*errors)++;
            }

            freez(*key);
            freez(*search_pattern);
            freez(*replace_pattern);
            *mapping_finished = true;
            break;

        default:
            yaml_error(parser, event, "Unexpected event in rewrite mapping");
            (*errors)++;
            break;
    }

    return true;
}

static size_t yaml_parse_rewrites(yaml_parser_t *parser, LOG_JOB *jb) {
    size_t errors = 0;

    if (!yaml_parse_expect_event(parser, YAML_SEQUENCE_START_EVENT))
        return 1;

    bool finished = false;
    while (!errors && !finished) {
        yaml_event_t event;
        if (!yaml_parse(parser, &event)) {
            errors++;
            continue;
        }

        switch (event.type) {
            case YAML_MAPPING_START_EVENT:
            {
                RW_FLAGS flags = RW_NONE;
                char *key = NULL;
                char *search_pattern = NULL;
                char *replace_pattern = NULL;

                bool mapping_finished = false;
                while (!errors && !mapping_finished) {
                    yaml_event_t sub_event;
                    if (!yaml_parse(parser, &sub_event)) {
                        errors++;
                        continue;
                    }

                    handle_rewrite_event(parser, &sub_event, &key,
                                         &search_pattern, &replace_pattern,
                                         &flags, &mapping_finished, jb, &errors);

                    yaml_event_delete(&sub_event);
                }
                break;
            }

            case YAML_SEQUENCE_END_EVENT:
                finished = true;
                break;

            default:
                yaml_error(parser, &event, "Unexpected event in rewrites sequence");
                errors++;
                break;
        }

        yaml_event_delete(&event);
    }

    return errors;
}

static size_t yaml_parse_renames(yaml_parser_t *parser, LOG_JOB *jb) {
    size_t errors = 0;

    if (!yaml_parse_expect_event(parser, YAML_SEQUENCE_START_EVENT))
        return 1;

    bool finished = false;
    while (!errors && !finished) {
        yaml_event_t event;
        if (!yaml_parse(parser, &event)) {
            errors++;
            continue;
        }

        switch (event.type) {
            case YAML_MAPPING_START_EVENT:
            {
                struct key_rename rn = { 0 };

                bool mapping_finished = false;
                while (!errors && !mapping_finished) {
                    yaml_event_t sub_event;
                    if (!yaml_parse(parser, &sub_event)) {
                        errors++;
                        continue;
                    }

                    switch (sub_event.type) {
                        case YAML_SCALAR_EVENT:
                            if (yaml_scalar_matches(&sub_event, "new_key", strlen("new_key"))) {
                                yaml_event_t value_event;

                                if (!yaml_parse(parser, &value_event) || value_event.type != YAML_SCALAR_EVENT) {
                                    yaml_error(parser, &value_event, "Expected scalar for rename new_key");
                                    errors++;
                                } else {
                                    hashed_key_set(
                                        &rn.new_key,
                                        (char *)value_event.data.scalar.value,
                                        value_event.data.scalar.length);
                                    yaml_event_delete(&value_event);
                                }
                            } else if (yaml_scalar_matches(&sub_event, "old_key", strlen("old_key"))) {
                                yaml_event_t value_event;

                                if (!yaml_parse(parser, &value_event) || value_event.type != YAML_SCALAR_EVENT) {
                                    yaml_error(parser, &value_event, "Expected scalar for rename old_key");
                                    errors++;
                                } else {
                                    hashed_key_set(
                                        &rn.old_key,
                                        (char *)value_event.data.scalar.value,
                                        value_event.data.scalar.length);
                                    yaml_event_delete(&value_event);
                                }
                            } else {
                                yaml_error(parser, &sub_event, "Unexpected scalar in rewrite mapping");
                                errors++;
                            }

                            break;

                        case YAML_MAPPING_END_EVENT:
                            if(rn.old_key.key && rn.new_key.key) {
                                if (!log_job_rename_add(jb, rn.new_key.key, rn.new_key.len,
                                                        rn.old_key.key, rn.old_key.len))
                                    errors++;
                            }
                            rename_cleanup(&rn);

                            mapping_finished = true;
                            break;

                        default:
                            yaml_error(parser, &sub_event, "Unexpected event in rewrite mapping");
                            errors++;
                            break;
                    }

                    yaml_event_delete(&sub_event);
                }
            }
                break;

            case YAML_SEQUENCE_END_EVENT:
                finished = true;
                break;

            default:
                yaml_error(parser, &event, "Unexpected event in rewrites sequence");
                errors++;
                break;
        }

        yaml_event_delete(&event);
    }

    return errors;
}

static size_t yaml_parse_pattern(yaml_parser_t *parser, LOG_JOB *jb) {
    yaml_event_t event;
    size_t errors = 0;

    if (!yaml_parse(parser, &event))
        return 1;

    if(event.type == YAML_SCALAR_EVENT)
        log_job_pattern_set(jb, (char *) event.data.scalar.value, event.data.scalar.length);
    else {
        yaml_error(parser, &event, "unexpected event type");
        errors++;
    }

    yaml_event_delete(&event);
    return errors;
}

static size_t yaml_parse_initialized(yaml_parser_t *parser, LOG_JOB *jb) {
    size_t errors = 0;

    if(!yaml_parse_expect_event(parser, YAML_STREAM_START_EVENT)) {
        errors++;
        goto cleanup;
    }

    if(!yaml_parse_expect_event(parser, YAML_DOCUMENT_START_EVENT)) {
        errors++;
        goto cleanup;
    }

    if(!yaml_parse_expect_event(parser, YAML_MAPPING_START_EVENT)) {
        errors++;
        goto cleanup;
    }

    bool finished = false;
    while (!errors && !finished) {
        yaml_event_t event;
        if(!yaml_parse(parser, &event)) {
            errors++;
            continue;
        }

        switch(event.type) {
            default:
                yaml_error(parser, &event, "unexpected type");
                errors++;
                break;

            case YAML_MAPPING_END_EVENT:
                finished = true;
                break;

            case YAML_SCALAR_EVENT:
                if (yaml_scalar_matches(&event, "pattern", strlen("pattern")))
                    errors += yaml_parse_pattern(parser, jb);

                else if (yaml_scalar_matches(&event, "prefix", strlen("prefix")))
                    errors += yaml_parse_prefix(parser, jb);

                else if (yaml_scalar_matches(&event, "filename", strlen("filename")))
                    errors += yaml_parse_filename_injection(parser, jb);

                else if (yaml_scalar_matches(&event, "filter", strlen("filter")))
                    errors += yaml_parse_filters(parser, jb);

                else if (yaml_scalar_matches(&event, "inject", strlen("inject")))
                    errors += yaml_parse_injections(parser, jb, false);

                else if (yaml_scalar_matches(&event, "unmatched", strlen("unmatched")))
                    errors += yaml_parse_unmatched(parser, jb);

                else if (yaml_scalar_matches(&event, "rewrite", strlen("rewrite")))
                    errors += yaml_parse_rewrites(parser, jb);

                else if (yaml_scalar_matches(&event, "rename", strlen("rename")))
                    errors += yaml_parse_renames(parser, jb);

                else {
                    yaml_error(parser, &event, "unexpected scalar");
                    errors++;
                }
                break;
        }

        yaml_event_delete(&event);
    }

    if(!errors && !yaml_parse_expect_event(parser, YAML_DOCUMENT_END_EVENT)) {
        errors++;
        goto cleanup;
    }

    if(!errors && !yaml_parse_expect_event(parser, YAML_STREAM_END_EVENT)) {
        errors++;
        goto cleanup;
    }

cleanup:
    return errors;
}

bool yaml_parse_file(const char *config_file_path, LOG_JOB *jb) {
    if(!config_file_path || !*config_file_path) {
        l2j_log("yaml configuration filename cannot be empty.");
        return false;
    }

    FILE *fp = fopen(config_file_path, "r");
    if (!fp) {
        l2j_log("Error opening config file: %s", config_file_path);
        return false;
    }

    yaml_parser_t parser;
    if (!yaml_parser_initialize(&parser)) {
        fclose(fp);
        return false;
    }

    yaml_parser_set_input_file(&parser, fp);

    size_t errors = yaml_parse_initialized(&parser, jb);

    yaml_parser_delete(&parser);
    fclose(fp);
    return errors == 0;
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
