// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdhost-labels.h"
#include "rrdhost.h"
#include "streaming/stream.h"

void rrdhost_set_is_parent_label(void) {
    uint32_t count = stream_receivers_currently_connected();

    if (count == 0 || count == 1) {
        RRDLABELS *labels = localhost->rrdlabels;
        rrdlabels_add(labels, "_is_parent", (count) ? "true" : "false", RRDLABEL_SRC_AUTO);

        // queue a node info
        aclk_queue_node_info(localhost, false);
    }
}

// expand ${VAR} and ${VAR:-default} patterns in src, writing result to dst
static void env_expand_labels_value(const char *src, char *dst, size_t dst_size) {
    if(!src || !dst || dst_size < 1) return;

    const char *s = src;
    char *d = dst;
    char *end = dst + dst_size - 1;

    while(*s && d < end) {
        if(s[0] == '$' && s[1] == '{') {
            const char *closing = strchr(s + 2, '}');
            if(!closing) {
                // no closing brace — copy rest literally
                while(*s && d < end)
                    *d++ = *s++;
                break;
            }

            size_t content_len = closing - (s + 2);
            char *var_name = mallocz(content_len + 1);
            memcpy(var_name, s + 2, content_len);
            var_name[content_len] = '\0';

            // check for :- default separator
            char *default_val = NULL;
            char *sep = strstr(var_name, ":-");
            if(sep) {
                *sep = '\0';
                default_val = sep + 2;
            }

            const char *env_val = getenv(var_name);
            const char *resolved;

            if(env_val && *env_val)
                resolved = env_val;
            else if(default_val)
                resolved = default_val;
            else {
                nd_log(NDLS_DAEMON, NDLP_WARNING,
                       "RRDLABEL: environment variable '%s' is not set and no default provided", var_name);
                resolved = "";
            }

            size_t rlen = strlen(resolved);
            size_t available = end - d;
            size_t to_copy = rlen < available ? rlen : available;
            memcpy(d, resolved, to_copy);
            d += to_copy;

            freez(var_name);
            s = closing + 1;
        }
        else {
            *d++ = *s++;
        }
    }

    *d = '\0';
}

// check if value contains any ${...} pattern worth expanding
static bool value_has_env_variables(const char *value) {
    const char *p = value;
    while((p = strchr(p, '$')) != NULL) {
        if(p[1] == '{') return true;
        p++;
    }
    return false;
}

static bool config_label_cb(void *data __maybe_unused, const char *name, const char *value) {
    if(value_has_env_variables(value)) {
        char expanded[RRDLABELS_MAX_VALUE_LENGTH + 1];
        env_expand_labels_value(value, expanded, sizeof(expanded));
        rrdlabels_add(localhost->rrdlabels, name, expanded, RRDLABEL_SRC_CONFIG);
    }
    else
        rrdlabels_add(localhost->rrdlabels, name, value, RRDLABEL_SRC_CONFIG);

    return true;
}

static void rrdhost_load_config_labels(void) {
    int status = inicfg_load(&netdata_config, NULL, 1, CONFIG_SECTION_HOST_LABEL);
    if(!status) {
        char *filename = CONFIG_DIR "/" CONFIG_FILENAME;
        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "RRDLABEL: Cannot reload the configuration file '%s', using labels in memory",
               filename);
    }

    inicfg_foreach_value_in_section(&netdata_config, CONFIG_SECTION_HOST_LABEL, config_label_cb, NULL);
}

static void rrdhost_load_kubernetes_labels(void) {
    char label_script[sizeof(char) * (strlen(netdata_configured_primary_plugins_dir) + strlen("get-kubernetes-labels.sh") + 2)];
    sprintf(label_script, "%s/%s", netdata_configured_primary_plugins_dir, "get-kubernetes-labels.sh");

    if (unlikely(access(label_script, R_OK) != 0)) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "Kubernetes pod label fetching script %s not found.",
               label_script);

        return;
    }

    POPEN_INSTANCE *instance = spawn_popen_run(label_script);
    if(!instance) return;

    char buffer[1000 + 1];
    while (fgets(buffer, 1000, spawn_popen_stdout(instance)) != NULL)
        rrdlabels_add_pair(localhost->rrdlabels, buffer, RRDLABEL_SRC_AUTO|RRDLABEL_SRC_K8S);

    // Non-zero exit code means that all the script output is error messages. We've shown already any message that didn't include a ':'
    // Here we'll inform with an ERROR that the script failed, show whatever (if anything) was added to the list of labels, free the memory and set the return to null
    int rc = spawn_popen_wait(instance);
    if(rc)
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "%s exited abnormally. Failed to get kubernetes labels.",
               label_script);
}

static void rrdhost_load_auto_labels(void) {
    RRDLABELS *labels = localhost->rrdlabels;

    rrdhost_system_info_to_rrdlabels(localhost->system_info, labels);
    add_aclk_host_labels();

    // The source should be CONF, but when it is set, these labels are exported by default ('send configured labels' in exporting.conf).
    // Their export seems to break exporting to Graphite, see https://github.com/netdata/netdata/issues/14084.

    int is_ephemeral = inicfg_get_boolean(&netdata_config, CONFIG_SECTION_GLOBAL, "is ephemeral node", CONFIG_BOOLEAN_NO);
    rrdlabels_add(labels, HOST_LABEL_IS_EPHEMERAL, is_ephemeral ? "true" : "false", RRDLABEL_SRC_CONFIG);

    int has_unstable_connection = inicfg_get_boolean(&netdata_config, CONFIG_SECTION_GLOBAL, "has unstable connection", CONFIG_BOOLEAN_NO);
    rrdlabels_add(labels, "_has_unstable_connection", has_unstable_connection ? "true" : "false", RRDLABEL_SRC_AUTO);

    rrdlabels_add(labels, "_is_parent", (stream_receivers_currently_connected() > 0) ? "true" : "false", RRDLABEL_SRC_AUTO);

    rrdlabels_add(labels, "_hostname", string2str(localhost->hostname), RRDLABEL_SRC_AUTO);
    rrdlabels_add(labels, "_os", string2str(localhost->os), RRDLABEL_SRC_AUTO);

    if (localhost->stream.snd.destination)
        rrdlabels_add(labels, "_streams_to", string2str(localhost->stream.snd.destination), RRDLABEL_SRC_AUTO);

    rrdlabels_add(labels, "_timezone", rrdhost_timezone(localhost), RRDLABEL_SRC_AUTO);
    rrdlabels_add(labels, "_abbrev_timezone", rrdhost_abbrev_timezone(localhost), RRDLABEL_SRC_AUTO);
}

void reload_host_labels(void) {
    if(!localhost->rrdlabels)
        localhost->rrdlabels = rrdlabels_create();

    rrdlabels_unmark_all(localhost->rrdlabels);

    // priority is important here
    rrdhost_load_config_labels();
    rrdhost_load_kubernetes_labels();
    rrdhost_load_auto_labels();

    rrdhost_flag_set(localhost,RRDHOST_FLAG_METADATA_LABELS | RRDHOST_FLAG_METADATA_UPDATE);

    stream_send_host_labels(localhost);
}

// ----------------------------------------------------------------------------
// unit tests

static int env_expand_unittest_check(const char *src, const char *expected, const char *test_name) {
    char buf[RRDLABELS_MAX_VALUE_LENGTH + 1];
    env_expand_labels_value(src, buf, sizeof(buf));

    int err = strcmp(buf, expected) != 0;
    fprintf(stderr, "  env_expand(%s): %s, expected '%s', got '%s'\n",
            test_name, err ? "FAILED" : "OK", expected, buf);
    return err;
}

int rrdhost_labels_unittest(void) {
    fprintf(stderr, "\n%s() tests\n", __FUNCTION__);
    int errors = 0;

    // --- set up test env vars ---
    setenv("ND_TEST_VAR", "hello", 1);
    setenv("ND_TEST_DC", "us-east", 1);
    setenv("ND_TEST_RACK", "rack42", 1);
    setenv("ND_TEST_EMPTY", "", 1);
    unsetenv("ND_TEST_UNSET");

    // no variables — pass through unchanged
    errors += env_expand_unittest_check("plain value", "plain value", "plain text");
    errors += env_expand_unittest_check("", "", "empty string");

    // basic variable expansion
    errors += env_expand_unittest_check("${ND_TEST_VAR}", "hello", "${VAR} set");
    errors += env_expand_unittest_check("prefix-${ND_TEST_VAR}", "prefix-hello", "prefix + ${VAR}");
    errors += env_expand_unittest_check("${ND_TEST_VAR}-suffix", "hello-suffix", "${VAR} + suffix");
    errors += env_expand_unittest_check("pre-${ND_TEST_VAR}-post", "pre-hello-post", "prefix + ${VAR} + suffix");

    // multiple variables
    errors += env_expand_unittest_check("${ND_TEST_DC}-${ND_TEST_RACK}", "us-east-rack42", "two vars adjacent");
    errors += env_expand_unittest_check("${ND_TEST_DC}/${ND_TEST_RACK}/${ND_TEST_VAR}", "us-east/rack42/hello", "three vars");
    errors += env_expand_unittest_check("dc=${ND_TEST_DC} rack=${ND_TEST_RACK}", "dc=us-east rack=rack42", "vars with literal labels");

    // default values — variable is set (default ignored)
    errors += env_expand_unittest_check("${ND_TEST_VAR:-fallback}", "hello", "default ignored when var set");
    errors += env_expand_unittest_check("${ND_TEST_DC:-other}", "us-east", "default ignored when var set (2)");

    // default values — variable is unset
    errors += env_expand_unittest_check("${ND_TEST_UNSET:-fallback}", "fallback", "default used when var unset");
    errors += env_expand_unittest_check("pre-${ND_TEST_UNSET:-fallback}-post", "pre-fallback-post", "default with surrounding text");

    // default values — variable is empty (treated same as unset)
    errors += env_expand_unittest_check("${ND_TEST_EMPTY:-fallback}", "fallback", "default used when var empty");

    // unset variable, no default — empty string
    errors += env_expand_unittest_check("${ND_TEST_UNSET}", "", "unset var no default = empty");
    errors += env_expand_unittest_check("pre-${ND_TEST_UNSET}-post", "pre--post", "unset var no default with text");

    // empty default — should resolve to empty string
    errors += env_expand_unittest_check("${ND_TEST_UNSET:-}", "", "empty default");
    errors += env_expand_unittest_check("pre-${ND_TEST_UNSET:-}-post", "pre--post", "empty default with text");

    // malformed syntax — no closing brace, copy literally
    errors += env_expand_unittest_check("${ND_TEST_UNCLOSED", "${ND_TEST_UNCLOSED", "no closing brace");
    errors += env_expand_unittest_check("pre-${ND_TEST_UNCLOSED", "pre-${ND_TEST_UNCLOSED", "no closing brace with prefix");

    // dollar sign not followed by brace — literal
    errors += env_expand_unittest_check("$notavar", "$notavar", "$ without {");
    errors += env_expand_unittest_check("price is $5", "price is $5", "$ with digit");
    errors += env_expand_unittest_check("$$", "$$", "double dollar");
    errors += env_expand_unittest_check("$", "$", "lone dollar at end");

    // empty variable name: ${} — getenv("") returns NULL, no default → empty
    errors += env_expand_unittest_check("${}", "", "empty var name");
    errors += env_expand_unittest_check("${:-fallback}", "fallback", "empty var name with default");

    // default containing :- (only first :- is the separator)
    errors += env_expand_unittest_check("${ND_TEST_UNSET:-a:-b}", "a:-b", "default containing :-");

    // no recursive expansion — env value containing ${...} is NOT re-expanded
    setenv("ND_TEST_NESTED", "${ND_TEST_VAR}", 1);
    errors += env_expand_unittest_check("${ND_TEST_NESTED}", "${ND_TEST_VAR}", "no recursive expansion");

    // default containing ${...} is NOT re-expanded
    errors += env_expand_unittest_check("${ND_TEST_UNSET:-${ND_TEST_VAR}}", "${ND_TEST_VAR}", "no expansion in default");

    // buffer overflow protection — expand into small buffer
    {
        char tiny[8];
        env_expand_labels_value("${ND_TEST_DC}", tiny, sizeof(tiny));
        // "us-east" is 7 chars, buffer is 8 (7+null) — should fit exactly
        int err = strcmp(tiny, "us-east") != 0;
        fprintf(stderr, "  env_expand(small buffer exact fit): %s, expected 'us-east', got '%s'\n",
                err ? "FAILED" : "OK", tiny);
        errors += err;
    }
    {
        char tiny[5];
        env_expand_labels_value("${ND_TEST_DC}", tiny, sizeof(tiny));
        // "us-east" is 7 chars, buffer is 5 (4+null) — should truncate to "us-e"
        int err = strcmp(tiny, "us-e") != 0;
        fprintf(stderr, "  env_expand(small buffer truncation): %s, expected 'us-e', got '%s'\n",
                err ? "FAILED" : "OK", tiny);
        errors += err;
    }
    {
        char tiny[5];
        env_expand_labels_value("abcdefghij", tiny, sizeof(tiny));
        // plain text truncation — should truncate to "abcd"
        int err = strcmp(tiny, "abcd") != 0;
        fprintf(stderr, "  env_expand(plain text truncation): %s, expected 'abcd', got '%s'\n",
                err ? "FAILED" : "OK", tiny);
        errors += err;
    }

    // value_has_env_variables() tests
    {
        struct {
            const char *input;
            bool expected;
        } detect_tests[] = {
            { "plain",                    false },
            { "",                         false },
            { "$notvar",                  false },
            { "$",                        false },
            { "${VAR}",                   true  },
            { "pre${VAR}post",            true  },
            { "${A}${B}",                 true  },
            { "$${}",                     true  },  // second $ starts ${
            { "${",                       true  },  // has ${ even without closing }
            { NULL, false }
        };
        for(int i = 0; detect_tests[i].input; i++) {
            bool result = value_has_env_variables(detect_tests[i].input);
            int err = result != detect_tests[i].expected;
            fprintf(stderr, "  value_has_env_variables('%s'): %s, expected %s, got %s\n",
                    detect_tests[i].input, err ? "FAILED" : "OK",
                    detect_tests[i].expected ? "true" : "false",
                    result ? "true" : "false");
            errors += err;
        }
    }

    // --- cleanup test env vars ---
    unsetenv("ND_TEST_VAR");
    unsetenv("ND_TEST_DC");
    unsetenv("ND_TEST_RACK");
    unsetenv("ND_TEST_EMPTY");
    unsetenv("ND_TEST_NESTED");

    fprintf(stderr, "%s: %d errors\n", __FUNCTION__, errors);
    return errors;
}
