// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"
#include "buildinfo.h"
#include "daemon/daemon-shutdown-watcher.h"
#include "static_threads.h"
#include "web/api/queries/backfill.h"

#include "database/engine/page_test.h"
#include <curl/curl.h>

#ifdef OS_WINDOWS
#include "win_system-info.h"
#endif

#ifdef ENABLE_SENTRY
#include "sentry-native/sentry-native.h"
#endif

#if defined(ENV32BIT)
#warning COMPILING 32BIT NETDATA
#endif

bool unittest_running = false;
int netdata_anonymous_statistics_enabled;

int libuv_worker_threads = MIN_LIBUV_WORKER_THREADS;
bool ieee754_doubles = false;
time_t netdata_start_time = 0;
struct netdata_static_thread *static_threads;

static void set_nofile_limit(struct rlimit *rl) {
    // get the num files allowed
    if(getrlimit(RLIMIT_NOFILE, rl) != 0) {
        netdata_log_error("getrlimit(RLIMIT_NOFILE) failed");
        return;
    }

    netdata_log_info("resources control: allowed file descriptors: soft = %zu, max = %zu",
                     (size_t) rl->rlim_cur, (size_t) rl->rlim_max);

    // make the soft/hard limits equal
    rl->rlim_cur = rl->rlim_max;
    if (setrlimit(RLIMIT_NOFILE, rl) != 0) {
        netdata_log_error("setrlimit(RLIMIT_NOFILE, { %zu, %zu }) failed", (size_t)rl->rlim_cur, (size_t)rl->rlim_max);
    }

    // sanity check to make sure we have enough file descriptors available to open
    if (getrlimit(RLIMIT_NOFILE, rl) != 0) {
        netdata_log_error("getrlimit(RLIMIT_NOFILE) failed");
        return;
    }

    if (rl->rlim_cur < 1024)
        netdata_log_error("Number of open file descriptors allowed for this process is too low (RLIMIT_NOFILE=%zu)", (size_t)rl->rlim_cur);
}

static const struct option_def {
    const char val;
    const char *description;
    const char *arg_name;
    const char *default_value;
} option_definitions[] = {
    {'c', "Configuration file to load.", "filename", CONFIG_DIR "/" CONFIG_FILENAME},
    {'D', "Do not fork. Run in the foreground.", NULL, "run in the background"},
    {'d', "Fork. Run in the background.", NULL, "run in the background"},
    {'h', "Display this help message.", NULL, NULL},
    {'P', "File to save a pid while running.", "filename", "do not save pid to a file"},
    {'i', "The IP address to listen to.", "IP", "all IP addresses IPv4 and IPv6"},
    {'p', "API/Web port to use.", "port", "19999"},
    {'s', "Prefix for /proc and /sys (for containers).", "path", "no prefix"},
    {'t', "The internal clock of netdata.", "seconds", "1"},
    {'u', "Run as user.", "username", "netdata"},
    {'v', "Print netdata version and exit.", NULL, NULL},
    {'V', "Print netdata version and exit.", NULL, NULL},
    {'W', "See Advanced options below.", "options", NULL},
};

int help(int exitcode) {
    FILE *stream;
    if(exitcode == 0)
        stream = stdout;
    else
        stream = stderr;

    int num_opts = sizeof(option_definitions) / sizeof(struct option_def);
    int i;
    int max_len_arg = 0;

    // Compute maximum argument length
    for( i = 0; i < num_opts; i++ ) {
        if(option_definitions[i].arg_name) {
            int len_arg = (int)strlen(option_definitions[i].arg_name);
            if(len_arg > max_len_arg) max_len_arg = len_arg;
        }
    }

    if(max_len_arg > 30) max_len_arg = 30;
    if(max_len_arg < 20) max_len_arg = 20;

    fprintf(stream, "%s", "\n"
            " ^\n"
            " |.-.   .-.   .-.   .-.   .  netdata                                         \n"
            " |   '-'   '-'   '-'   '-'   monitoring and troubleshooting, transformed!    \n"
            " +----+-----+-----+-----+-----+-----+-----+-----+-----+-----+-----+-----+--->\n"
            "\n"
            " Copyright 2018-2025 Netdata Inc.\n"
            " Released under GNU General Public License v3 or later.\n"
            "\n"
            " Home Page  : https://netdata.cloud\n"
            " Source Code: https://github.com/netdata/netdata\n"
            " Docs       : https://learn.netdata.cloud\n"
            " Support    : https://github.com/netdata/netdata/issues\n"
            " License    : https://github.com/netdata/netdata/blob/master/LICENSE.md\n"
            "\n"
            " Twitter    : https://twitter.com/netdatahq\n"
            " LinkedIn   : https://linkedin.com/company/netdata-cloud/\n"
            " Facebook   : https://facebook.com/linuxnetdata/\n"
            "\n"
            "\n"
    );

    fprintf(stream, " SYNOPSIS: netdata [options]\n");
    fprintf(stream, "\n");
    fprintf(stream, " Options:\n\n");

    // Output options description.
    for( i = 0; i < num_opts; i++ ) {
        fprintf(stream, "  -%c %-*s  %s", option_definitions[i].val, max_len_arg, option_definitions[i].arg_name ? option_definitions[i].arg_name : "", option_definitions[i].description);
        if(option_definitions[i].default_value) {
            fprintf(stream, "\n   %c %-*s  Default: %s\n", ' ', max_len_arg, "", option_definitions[i].default_value);
        } else {
            fprintf(stream, "\n");
        }
        fprintf(stream, "\n");
    }

    fprintf(stream, "\n Advanced options:\n\n"
            "  -W stacksize=N           Set the stacksize (in bytes).\n\n"
            "  -W debug_flags=N         Set runtime tracing to debug.log.\n\n"
            "  -W unittest              Run internal unittests and exit.\n\n"
            "  -W sqlite-meta-recover   Run recovery on the metadata database and exit.\n\n"
            "  -W sqlite-compact        Reclaim metadata database unused space and exit.\n\n"
            "  -W sqlite-analyze        Run update statistics and exit.\n\n"
            "  -W sqlite-alert-cleanup  Perform maintenance on the alerts table.\n\n"
#ifdef ENABLE_DBENGINE
            "  -W createdataset=N       Create a DB engine dataset of N seconds and exit.\n\n"
            "  -W stresstest=A,B,C,D,E,F,G\n"
            "                           Run a DB engine stress test for A seconds,\n"
            "                           with B writers and C readers, with a ramp up\n"
            "                           time of D seconds for writers, a page cache\n"
            "                           size of E MiB, an optional disk space limit\n"
            "                           of F MiB, G libuv workers (default 16) and exit.\n\n"
#endif
            "  -W set section option value\n"
            "                           set netdata.conf option from the command line.\n\n"
            "  -W buildinfo             Print the version, the configure options,\n"
            "                           a list of optional features, and whether they\n"
            "                           are enabled or not.\n\n"
            "  -W buildinfojson         Print the version, the configure options,\n"
            "                           a list of optional features, and whether they\n"
            "                           are enabled or not, in JSON format.\n\n"
            "  -W simple-pattern pattern string\n"
            "                           Check if string matches pattern and exit.\n\n"
#ifdef OS_WINDOWS
            "  -W perflibdump [key]\n"
            "                           Dump the Windows Performance Counters Registry in JSON.\n\n"
#endif
    );

    fprintf(stream, "\n Signals netdata handles:\n\n"
            "  - HUP                    Close and reopen log files.\n"
            "  - USR2                   Reload health configuration.\n"
            "\n"
    );

    fflush(stream);
    return exitcode;
}

/* Any config setting that can be accessed without a default value i.e. configget(...,...,NULL) *MUST*
   be set in this procedure to be called in all the relevant code paths.
*/

#define delta_startup_time(msg)                         \
    {                                                   \
        usec_t now_ut = now_monotonic_usec();           \
        if(prev_msg)                                    \
            netdata_log_info("NETDATA STARTUP: in %7llu ms, %s - next: %s", (now_ut - last_ut) / USEC_PER_MS, prev_msg, msg); \
        else                                            \
            netdata_log_info("NETDATA STARTUP: next: %s", msg);    \
        last_ut = now_ut;                               \
        prev_msg = msg;                                 \
    }

int buffer_unittest(void);
int pgc_unittest(void);
int mrg_unittest(void);
int pluginsd_parser_unittest(void);
void replication_initialize(void);
void bearer_tokens_init(void);
int unittest_stream_compressions(void);
int uuid_unittest(void);
int progress_unittest(void);
int dyncfg_unittest(void);
bool netdata_random_session_id_generate(void);

#ifdef OS_WINDOWS
int windows_perflib_dump(const char *key);
#endif

int unittest_prepare_rrd(const char **user) {
    netdata_conf_section_global_run_as_user(user);
    netdata_conf_section_global();
    nd_profile.update_every = 1;
    default_rrd_memory_mode = RRD_DB_MODE_RAM;
    health_plugin_disable();
    nd_profile.storage_tiers = 1;
    registry_init();
    if(rrd_init("unittest", NULL, true)) {
        fprintf(stderr, "rrd_init failed for unittest\n");
        return 1;
    }
    stream_send.enabled = false;

    return 0;
}

int netdata_main(int argc, char **argv) {
    string_init();
    analytics_init();

    netdata_start_time = now_realtime_sec();
    usec_t started_ut = now_monotonic_usec();
    usec_t last_ut = started_ut;
    const char *prev_msg = NULL;

    int i;
    int config_loaded = 0;
    bool close_open_fds = true;
    size_t default_stacksize;
    const char *user = NULL;

#ifdef OS_WINDOWS
    int dont_fork = 1;
#else
    int dont_fork = 0;
#endif

    static_threads = static_threads_get();

    netdata_ready = false;
    // set the name for logging
    program_name = "netdata";

    curl_global_init(CURL_GLOBAL_ALL);

    // parse options
    {
        int num_opts = sizeof(option_definitions) / sizeof(struct option_def);
        char optstring[(num_opts * 2) + 1];

        int string_i = 0;
        for( i = 0; i < num_opts; i++ ) {
            optstring[string_i] = option_definitions[i].val;
            string_i++;
            if(option_definitions[i].arg_name) {
                optstring[string_i] = ':';
                string_i++;
            }
        }
        // terminate optstring
        optstring[string_i] ='\0';
        optstring[(num_opts *2)] ='\0';

        int opt;
        while( (opt = getopt(argc, argv, optstring)) != -1 ) {
            switch(opt) {
                case 'c':
                    if(!netdata_conf_load(optarg, 1, &user)) {
                        netdata_log_error("Cannot load configuration file %s.", optarg);
                        return 1;
                    }
                    else {
                        netdata_log_debug(D_OPTIONS, "Configuration loaded from %s.", optarg);
                        cloud_conf_load(1);
                        config_loaded = 1;
                    }
                    break;
                case 'D':
                    dont_fork = 1;
                    break;
                case 'd':
                    dont_fork = 0;
                    break;
                case 'h':
                    return help(0);
                case 'i':
                    config_set(CONFIG_SECTION_WEB, "bind to", optarg);
                    break;
                case 'P':
                    pidfile = strdupz(optarg);
                    break;
                case 'p':
                    config_set(CONFIG_SECTION_GLOBAL, "default port", optarg);
                    break;
                case 's':
                    config_set(CONFIG_SECTION_GLOBAL, "host access prefix", optarg);
                    break;
                case 't':
                    config_set(CONFIG_SECTION_GLOBAL, "update every", optarg);
                    break;
                case 'u':
                    config_set(CONFIG_SECTION_GLOBAL, "run as user", optarg);
                    break;
                case 'v':
                case 'V':
                    printf("%s %s\n", program_name, NETDATA_VERSION);
                    return 0;
                case 'W':
                    {
                        char* stacksize_string = "stacksize=";
                        char* debug_flags_string = "debug_flags=";
#ifdef ENABLE_DBENGINE
                        char* createdataset_string = "createdataset=";
                        char* stresstest_string = "stresstest=";

                        if(strcmp(optarg, "pgd-tests") == 0) {
                            return pgd_test(argc, argv);
                        }
#endif

                        if(strcmp(optarg, "sqlite-meta-recover") == 0) {
                            sql_init_meta_database(DB_CHECK_RECOVER, 0);
                            return 0;
                        }

                        if(strcmp(optarg, "sqlite-compact") == 0) {
                            sql_init_meta_database(DB_CHECK_RECLAIM_SPACE, 0);
                            return 0;
                        }

                        if(strcmp(optarg, "sqlite-analyze") == 0) {
                            sql_init_meta_database(DB_CHECK_ANALYZE, 0);
                            return 0;
                        }

                        if(strcmp(optarg, "sqlite-alert-cleanup") == 0) {
                            sql_alert_cleanup(true);
                            return 0;
                        }

                        if(strcmp(optarg, "unittest") == 0) {
                            unittest_running = true;

                            // set defaults for dbegnine unittest
                            config_set(CONFIG_SECTION_DB, "dbengine page type", "gorilla");
#ifdef ENABLE_DBENGINE
                            default_rrdeng_disk_quota_mb = default_multidb_disk_quota_mb = 256;
#endif

                            if (sqlite_library_init())
                                return 1;

                            if (pluginsd_parser_unittest()) return 1;
                            if (unit_test_static_threads()) return 1;
                            if (unit_test_buffer()) return 1;
                            if (unit_test_str2ld()) return 1;
                            if (buffer_unittest()) return 1;

                            // No call to load the config file on this code-path
                            if (unittest_prepare_rrd(&user)) return 1;
                            if (run_all_mockup_tests()) return 1;
                            if (unit_test_storage()) return 1;
#ifdef ENABLE_DBENGINE
                            if (test_dbengine()) return 1;
#endif
                            if (test_sqlite()) return 1;
                            if (string_unittest(10000)) return 1;
                            if (dictionary_unittest(10000)) return 1;
                            if (aral_unittest(10000)) return 1;
                            if (rrdlabels_unittest()) return 1;
                            if (ctx_unittest()) return 1;
                            if (uuid_unittest()) return 1;
                            if (dyncfg_unittest()) return 1;
                            if (unittest_waiting_queue()) return 1;
                            if (uuidmap_unittest()) return 1;
                            sqlite_library_shutdown();
                            fprintf(stderr, "\n\nALL TESTS PASSED\n\n");
                            return 0;
                        }
                        else if(strcmp(optarg, "escapetest") == 0) {
                            return command_argument_sanitization_tests();
                        }
                        else if(strcmp(optarg, "dicttest") == 0) {
                            unittest_running = true;
                            return dictionary_unittest(10000);
                        }
                        else if(strcmp(optarg, "araltest") == 0) {
                            unittest_running = true;
                            return aral_unittest(10000);
                        }
                        else if(strcmp(optarg, "waitqtest") == 0) {
                            unittest_running = true;
                            return unittest_waiting_queue();
                        }
                        else if(strcmp(optarg, "uuidmaptest") == 0) {
                            unittest_running = true;
                            return uuidmap_unittest();
                        }
                        else if(strcmp(optarg, "lockstest") == 0) {
                            unittest_running = true;
                            return locks_stress_test();
                        }
                        else if(strcmp(optarg, "rwlockstest") == 0) {
                            unittest_running = true;
                            return rwlocks_stress_test();
                        }
                        else if(strcmp(optarg, "stringtest") == 0)  {
                            unittest_running = true;
                            return string_unittest(10000);
                        }
                        else if(strcmp(optarg, "rrdlabelstest") == 0) {
                            unittest_running = true;
                            return rrdlabels_unittest();
                        }
                        else if(strcmp(optarg, "buffertest") == 0) {
                            unittest_running = true;
                            return buffer_unittest();
                        }
                        else if(strcmp(optarg, "uuidtest") == 0) {
                            unittest_running = true;
                            return uuid_unittest();
                        }
#ifdef OS_WINDOWS
                        else if(strcmp(optarg, "perflibdump") == 0) {
                            return windows_perflib_dump(optind + 1 > argc ? NULL : argv[optind]);
                        }
#endif
#ifdef ENABLE_DBENGINE
                        else if(strcmp(optarg, "mctest") == 0) {
                            unittest_running = true;
                            return mc_unittest();
                        }
                        else if(strcmp(optarg, "ctxtest") == 0) {
                            unittest_running = true;
                            return ctx_unittest();
                        }
                        else if(strcmp(optarg, "metatest") == 0) {
                            unittest_running = true;
                            return metadata_unittest();
                        }
                        else if(strcmp(optarg, "pgctest") == 0) {
                            unittest_running = true;
                            return pgc_unittest();
                        }
                        else if(strcmp(optarg, "mrgtest") == 0) {
                            unittest_running = true;
                            return mrg_unittest();
                        }
                        else if(strcmp(optarg, "parsertest") == 0) {
                            unittest_running = true;
                            return pluginsd_parser_unittest();
                        }
                        else if(strcmp(optarg, "stream_compressions_test") == 0) {
                            unittest_running = true;
                            return unittest_stream_compressions();
                        }
                        else if(strcmp(optarg, "progresstest") == 0) {
                            unittest_running = true;
                            return progress_unittest();
                        }
                        else if(strcmp(optarg, "dyncfgtest") == 0) {
                            unittest_running = true;
                            if(unittest_prepare_rrd(&user))
                                return 1;
                            return dyncfg_unittest();
                        }
                        else if(strncmp(optarg, createdataset_string, strlen(createdataset_string)) == 0) {
                            optarg += strlen(createdataset_string);
                            unsigned history_seconds = strtoul(optarg, NULL, 0);
                            netdata_conf_section_global_run_as_user(&user);
                            netdata_conf_section_global();
                            nd_profile.update_every = 1;
                            registry_init();
                            if(rrd_init("dbengine-dataset", NULL, true)) {
                                fprintf(stderr, "rrd_init failed for unittest\n");
                                return 1;
                            }
                            generate_dbengine_dataset(history_seconds);
                            return 0;
                        }
                        else if(strncmp(optarg, stresstest_string, strlen(stresstest_string)) == 0) {
                            char *endptr;
                            unsigned test_duration_sec = 0, dset_charts = 0, query_threads = 0, ramp_up_seconds = 0,
                            page_cache_mb = 0, disk_space_mb = 0, workers = 16;

                            optarg += strlen(stresstest_string);
                            test_duration_sec = (unsigned)strtoul(optarg, &endptr, 0);
                            if (',' == *endptr)
                                dset_charts = (unsigned)strtoul(endptr + 1, &endptr, 0);
                            if (',' == *endptr)
                                query_threads = (unsigned)strtoul(endptr + 1, &endptr, 0);
                            if (',' == *endptr)
                                ramp_up_seconds = (unsigned)strtoul(endptr + 1, &endptr, 0);
                            if (',' == *endptr)
                                page_cache_mb = (unsigned)strtoul(endptr + 1, &endptr, 0);
                            if (',' == *endptr)
                                disk_space_mb = (unsigned)strtoul(endptr + 1, &endptr, 0);
                            if (',' == *endptr)
                                workers = (unsigned)strtoul(endptr + 1, &endptr, 0);

                            if (workers > 1024)
                                workers = 1024;

                            char workers_str[16];
                            snprintf(workers_str, 15, "%u", workers);
                            setenv("UV_THREADPOOL_SIZE", workers_str, 1);
                            dbengine_stress_test(test_duration_sec, dset_charts, query_threads, ramp_up_seconds,
                                                 page_cache_mb, disk_space_mb);
                            return 0;
                        }
#endif
                        else if(strcmp(optarg, "simple-pattern") == 0) {
                            if(optind + 2 > argc) {
                                fprintf(stderr, "%s", "\nUSAGE: -W simple-pattern 'pattern' 'string'\n\n"
                                        " Checks if 'pattern' matches the given 'string'.\n"
                                        " - 'pattern' can be one or more space separated words.\n"
                                        " - each 'word' can contain one or more asterisks.\n"
                                        " - words starting with '!' give negative matches.\n"
                                        " - words are processed left to right\n"
                                        "\n"
                                        "Examples:\n"
                                        "\n"
                                        " > match all veth interfaces, except veth0:\n"
                                        "\n"
                                        "   -W simple-pattern '!veth0 veth*' 'veth12'\n"
                                        "\n"
                                        "\n"
                                        " > match all *.ext files directly in /path/:\n"
                                        "   (this will not match *.ext files in a subdir of /path/)\n"
                                        "\n"
                                        "   -W simple-pattern '!/path/*/*.ext /path/*.ext' '/path/test.ext'\n"
                                        "\n"
                                );
                                return 1;
                            }

                            const char *haystack = argv[optind];
                            const char *needle = argv[optind + 1];
                            size_t len = strlen(needle) + 1;
                            char wildcarded[len];

                            SIMPLE_PATTERN *p = simple_pattern_create(haystack, NULL, SIMPLE_PATTERN_EXACT, true);
                            SIMPLE_PATTERN_RESULT ret = simple_pattern_matches_extract(p, needle, wildcarded, len);
                            simple_pattern_free(p);

                            if(ret == SP_MATCHED_POSITIVE) {
                                fprintf(stdout, "RESULT: POSITIVE MATCHED - pattern '%s' matches '%s', wildcarded '%s'\n", haystack, needle, wildcarded);
                                return 0;
                            }
                            else if(ret == SP_MATCHED_NEGATIVE) {
                                fprintf(stdout, "RESULT: NEGATIVE MATCHED - pattern '%s' matches '%s', wildcarded '%s'\n", haystack, needle, wildcarded);
                                return 0;
                            }
                            else {
                                fprintf(stdout, "RESULT: NOT MATCHED - pattern '%s' does not match '%s', wildcarded '%s'\n", haystack, needle, wildcarded);
                                return 1;
                            }
                        }
                        else if(strncmp(optarg, stacksize_string, strlen(stacksize_string)) == 0) {
                            optarg += strlen(stacksize_string);
                            config_set(CONFIG_SECTION_GLOBAL, "pthread stack size", optarg);
                        }
                        else if(strncmp(optarg, debug_flags_string, strlen(debug_flags_string)) == 0) {
                            optarg += strlen(debug_flags_string);
                            config_set(CONFIG_SECTION_LOGS, "debug flags",  optarg);
                            debug_flags = strtoull(optarg, NULL, 0);
                        }
                        else if(strcmp(optarg, "set") == 0) {
                            if(optind + 3 > argc) {
                                fprintf(stderr, "%s", "\nUSAGE: -W set 'section' 'key' 'value'\n\n"
                                        " Overwrites settings of netdata.conf.\n"
                                        "\n"
                                        " These options interact with: -c netdata.conf\n"
                                        " If -c netdata.conf is given on the command line,\n"
                                        " before -W set... the user may overwrite command\n"
                                        " line parameters at netdata.conf\n"
                                        " If -c netdata.conf is given after (or missing)\n"
                                        " -W set... the user cannot overwrite the command line\n"
                                        " parameters."
                                        "\n"
                                );
                                return 1;
                            }
                            const char *section = argv[optind];
                            const char *key = argv[optind + 1];
                            const char *value = argv[optind + 2];
                            optind += 3;

                            // set this one as the default
                            // only if it is not already set in the config file
                            // so the caller can use -c netdata.conf before or
                            // after this parameter to prevent or allow overwriting
                            // variables at netdata.conf
                            config_set_default_raw_value(section, key, value);

                            // fprintf(stderr, "SET section '%s', key '%s', value '%s'\n", section, key, value);
                        }
                        else if(strcmp(optarg, "set2") == 0) {
                            if(optind + 4 > argc) {
                                fprintf(stderr, "%s", "\nUSAGE: -W set 'conf_file' 'section' 'key' 'value'\n\n"
                                        " Overwrites settings of netdata.conf or cloud.conf\n"
                                        "\n"
                                        " These options interact with: -c netdata.conf\n"
                                        " If -c netdata.conf is given on the command line,\n"
                                        " before -W set... the user may overwrite command\n"
                                        " line parameters at netdata.conf\n"
                                        " If -c netdata.conf is given after (or missing)\n"
                                        " -W set... the user cannot overwrite the command line\n"
                                        " parameters."
                                        " conf_file can be \"cloud\" or \"netdata\".\n"
                                        "\n"
                                );
                                return 1;
                            }
                            const char *conf_file = argv[optind]; /* "cloud" is cloud.conf, otherwise netdata.conf */
                            struct config *tmp_config = strcmp(conf_file, "cloud") ? &netdata_config : &cloud_config;
                            const char *section = argv[optind + 1];
                            const char *key = argv[optind + 2];
                            const char *value = argv[optind + 3];
                            optind += 4;

                            // set this one as the default
                            // only if it is not already set in the config file
                            // so the caller can use -c netdata.conf before or
                            // after this parameter to prevent or allow overwriting
                            // variables at netdata.conf
                            appconfig_set_default_raw_value(tmp_config, section, key, value);

                            // fprintf(stderr, "SET section '%s', key '%s', value '%s'\n", section, key, value);
                        }
                        else if(strcmp(optarg, "get") == 0) {
                            if(optind + 3 > argc) {
                                fprintf(stderr, "%s", "\nUSAGE: -W get 'section' 'key' 'value'\n\n"
                                        " Prints settings of netdata.conf.\n"
                                        "\n"
                                        " These options interact with: -c netdata.conf\n"
                                        " -c netdata.conf has to be given before -W get.\n"
                                        "\n"
                                );
                                return 1;
                            }

                            if(!config_loaded) {
                                fprintf(stderr, "warning: no configuration file has been loaded. Use -c CONFIG_FILE, before -W get. Using default config.\n");
                                netdata_conf_load(NULL, 0, &user);
                            }

                            netdata_conf_section_global();

                            const char *section = argv[optind];
                            const char *key = argv[optind + 1];
                            const char *def = argv[optind + 2];
                            const char *value = config_get(section, key, def);
                            printf("%s\n", value);
                            return 0;
                        }
                        else if(strcmp(optarg, "get2") == 0) {
                            if(optind + 4 > argc) {
                                fprintf(stderr, "%s", "\nUSAGE: -W get2 'conf_file' 'section' 'key' 'value'\n\n"
                                        " Prints settings of netdata.conf or cloud.conf\n"
                                        "\n"
                                        " These options interact with: -c netdata.conf\n"
                                        " -c netdata.conf has to be given before -W get2.\n"
                                        " conf_file can be \"cloud\" or \"netdata\".\n"
                                        "\n"
                                );
                                return 1;
                            }

                            if(!config_loaded) {
                                fprintf(stderr, "warning: no configuration file has been loaded. Use -c CONFIG_FILE, before -W get. Using default config.\n");
                                netdata_conf_load(NULL, 0, &user);
                                cloud_conf_load(1);
                            }

                            netdata_conf_section_global();

                            const char *conf_file = argv[optind]; /* "cloud" is cloud.conf, otherwise netdata.conf */
                            struct config *tmp_config = strcmp(conf_file, "cloud") ? &netdata_config : &cloud_config;
                            const char *section = argv[optind + 1];
                            const char *key = argv[optind + 2];
                            const char *def = argv[optind + 3];
                            const char *value = appconfig_get(tmp_config, section, key, def);
                            printf("%s\n", value);
                            return 0;
                        }
                        else if(strcmp(optarg, "buildinfo") == 0) {
                            print_build_info();
                            return 0;
                        }
                        else if(strcmp(optarg, "buildinfojson") == 0) {
                            print_build_info_json();
                            return 0;
                        }
                        else if(strcmp(optarg, "keepopenfds") == 0) {
                            // Internal dev option to skip closing inherited
                            // open FDs. Useful, when we want to run the agent
                            // under profiling tools that open/maintain their
                            // own FDs.
                            close_open_fds = false;
                        } else {
                            fprintf(stderr, "Unknown -W parameter '%s'\n", optarg);
                            return help(1);
                        }
                    }
                    break;

                default: /* ? */
                    fprintf(stderr, "Unknown parameter '%c'\n", opt);
                    return help(1);
            }
        }
    }

    if (close_open_fds == true) {
        // close all open file descriptors, except the standard ones
        // the caller may have left open files (lxc-attach has this issue)
        os_close_all_non_std_open_fds_except(NULL, 0, 0);
    }

    if(!config_loaded) {
        netdata_conf_load(NULL, 0, &user);
        cloud_conf_load(0);
    }

    // ----------------------------------------------------------------------------------------------------------------
    // initialize the logging system
    // IMPORTANT: KEEP THIS FIRST SO THAT THE REST OF NETDATA WILL LOG PROPERLY

    netdata_conf_section_logs();
    nd_log_limits_unlimited();

    // initialize the log files
    nd_log_initialize();
    {
        ND_LOG_STACK lgs[] = {
            ND_LOG_FIELD_UUID(NDF_MESSAGE_ID, &netdata_startup_msgid),
            ND_LOG_FIELD_END(),
        };
        ND_LOG_STACK_PUSH(lgs);

        netdata_log_info("Netdata agent version '%s' is starting", NETDATA_VERSION);
    }

    // ----------------------------------------------------------------------------------------------------------------
    // global configuration

    netdata_conf_section_global();

    // Get execution path before switching user to avoid permission issues
    get_netdata_execution_path();

    // ----------------------------------------------------------------------------------------------------------------
    // analytics

    analytics_reset();
    get_system_timezone();

    // ----------------------------------------------------------------------------------------------------------------
    // data collection plugins

    // prepare configuration environment variables for the plugins
    set_environment_for_plugins_and_scripts();

    // cd into config_dir to allow the plugins refer to their config files using relative filenames
    if(chdir(netdata_configured_user_config_dir) == -1)
        fatal("Cannot cd to '%s'", netdata_configured_user_config_dir);

    // ----------------------------------------------------------------------------------------------------------------
    // pulse (internal netdata instrumentation)

#ifdef NETDATA_INTERNAL_CHECKS
    pulse_enabled = true;
    pulse_extended_enabled = true;
#endif

    pulse_extended_enabled =
        config_get_boolean(CONFIG_SECTION_PULSE, "extended", pulse_extended_enabled);

    if(pulse_extended_enabled)
        // this has to run before starting any other threads that use workers
        workers_utilization_enable();

    // ----------------------------------------------------------------------------------------------------------------
    // profiles

    nd_profile_setup();

    // ----------------------------------------------------------------------------------------------------------------
    // streaming, replication, functions initialization

    replication_initialize();
    rrd_functions_inflight_init();

    {
        // --------------------------------------------------------------------
        // alerts SILENCERS

        health_set_silencers_filename();
        health_initialize_global_silencers();

        // --------------------------------------------------------------------
        // setup process signals

        // block signals while initializing threads.
        // this causes the threads to block signals.

        delta_startup_time("initialize signals");
        nd_initialize_signals(); // setup the signals we want to use

        // --------------------------------------------------------------------
        // check which threads are enabled and initialize them

        delta_startup_time("initialize static threads");

        // setup threads configs
        default_stacksize = netdata_threads_init();
        // musl default thread stack size is 128k, let's set it to a higher value to avoid random crashes
        if (default_stacksize < 1 * 1024 * 1024)
            default_stacksize = 1 * 1024 * 1024;

        for (i = 0; static_threads[i].name != NULL ; i++) {
            struct netdata_static_thread *st = &static_threads[i];

            if(st->enable_routine)
                st->enabled = st->enable_routine();

            if(st->config_name)
                st->enabled = config_get_boolean(st->config_section, st->config_name, st->enabled);

            if(st->enabled && st->init_routine)
                st->init_routine();

            if(st->env_name)
                nd_setenv(st->env_name, st->enabled?"YES":"NO", 1);

            if(st->global_variable)
                *st->global_variable = (st->enabled) ? true : false;
        }

        // --------------------------------------------------------------------
        // create the listening sockets

        delta_startup_time("initialize web server");

        // get the certificate and start security
        netdata_conf_web_security_init();

        nd_web_api_init();
        web_server_threading_selection();

        if(web_server_mode != WEB_SERVER_MODE_NONE) {
            if (!api_listen_sockets_setup()) {
                netdata_log_error("Cannot setup listen port(s). Is Netdata already running?");
                exit(1);
            }
        }
        if (sqlite_library_init())
            fatal("Failed to initialize sqlite library");

        // --------------------------------------------------------------------
        // Initialize ML configuration

        delta_startup_time("initialize ML");
        ml_init();
    }

    delta_startup_time("set resource limits");

#ifdef NETDATA_INTERNAL_CHECKS
    if(debug_flags != 0) {
        struct rlimit rl = { RLIM_INFINITY, RLIM_INFINITY };
        if(setrlimit(RLIMIT_CORE, &rl) != 0)
            netdata_log_error("Cannot request unlimited core dumps for debugging... Proceeding anyway...");
#ifdef HAVE_SYS_PRCTL_H
        prctl(PR_SET_DUMPABLE, 1, 0, 0, 0);
#endif
    }
#endif /* NETDATA_INTERNAL_CHECKS */

    set_nofile_limit(&rlimit_nofile);

    delta_startup_time("become daemon");

#if defined(OS_LINUX) || defined(OS_MACOS) || defined(OS_FREEBSD)
    // fork, switch user, create pid file, set process priority
    if(become_daemon(dont_fork, user) == -1)
        fatal("Cannot daemonize myself.");
#else
    (void)dont_fork;
#endif

    netdata_main_spawn_server_init("plugins", argc, (const char **)argv);
    watcher_thread_start();

    // init sentry
#ifdef ENABLE_SENTRY
        nd_sentry_init();
#endif

    // The "HOME" env var points to the root's home dir because Netdata starts as root. Can't use "HOME".
    struct passwd *pw = getpwuid(getuid());
    if (config_exists(CONFIG_SECTION_DIRECTORIES, "home") || !pw || !pw->pw_dir) {
        netdata_configured_home_dir = config_get(CONFIG_SECTION_DIRECTORIES, "home", netdata_configured_home_dir);
    } else {
        netdata_configured_home_dir = config_get(CONFIG_SECTION_DIRECTORIES, "home", pw->pw_dir);
    }

    nd_setenv("HOME", netdata_configured_home_dir, 1);

    dyncfg_init(true);

    netdata_log_info("netdata started on pid %d.", getpid());

    delta_startup_time("initialize threads after fork");

    netdata_threads_init_after_fork((size_t)config_get_size_bytes(CONFIG_SECTION_GLOBAL, "pthread stack size", default_stacksize));

    // initialize internal registry
    delta_startup_time("initialize registry");
    registry_init();
    cloud_conf_init_after_registry();
    netdata_random_session_id_generate();

    // ------------------------------------------------------------------------
    // initialize rrd, registry, health, streaming, etc.

    delta_startup_time("collecting system info");

    netdata_anonymous_statistics_enabled=-1;
    struct rrdhost_system_info *system_info = rrdhost_system_info_create();
    rrdhost_system_info_detect(system_info);

    const char *guid = registry_get_this_machine_guid();
#ifdef ENABLE_SENTRY
    nd_sentry_set_user(guid);
#else
    UNUSED(guid);
#endif

    get_install_type(system_info);

    delta_startup_time("initialize RRD structures");

    if(rrd_init(netdata_configured_hostname, system_info, false)) {
        set_late_analytics_variables(system_info);
        fatal("Cannot initialize localhost instance with name '%s'.", netdata_configured_hostname);
    }

    delta_startup_time("check for incomplete shutdown");

    char agent_crash_file[FILENAME_MAX + 1];
    char agent_incomplete_shutdown_file[FILENAME_MAX + 1];
    snprintfz(agent_incomplete_shutdown_file, FILENAME_MAX, "%s/.agent_incomplete_shutdown", netdata_configured_varlib_dir);
    int incomplete_shutdown_detected = (unlink(agent_incomplete_shutdown_file) == 0);
    snprintfz(agent_crash_file, FILENAME_MAX, "%s/.agent_crash", netdata_configured_varlib_dir);
    int crash_detected = (unlink(agent_crash_file) == 0);
    int fd = open(agent_crash_file, O_WRONLY | O_CREAT | O_TRUNC | O_CLOEXEC, 444);
    if (fd >= 0)
        close(fd);

    // ------------------------------------------------------------------------
    // Claim netdata agent to a cloud endpoint

    delta_startup_time("collect claiming info");
    load_claiming_state();

    // ------------------------------------------------------------------------
    // enable log flood protection

    nd_log_limits_reset();

    // Load host labels
    delta_startup_time("collect host labels");
    reload_host_labels();

    // ------------------------------------------------------------------------
    // spawn the threads

    get_agent_event_time_median_init();
    bearer_tokens_init();

    delta_startup_time("start the static threads");

    netdata_conf_section_web();

    set_late_analytics_variables(system_info);
    for (i = 0; static_threads[i].name != NULL ; i++) {
        struct netdata_static_thread *st = &static_threads[i];

        if(st->enabled) {
            netdata_log_debug(D_SYSTEM, "Starting thread %s.", st->name);
            st->thread = nd_thread_create(st->name, NETDATA_THREAD_OPTION_DEFAULT, st->start_routine, st);
        }
        else
            netdata_log_debug(D_SYSTEM, "Not starting thread %s.", st->name);
    }
    ml_start_threads();

    // ------------------------------------------------------------------------
    // Initialize netdata agent command serving from cli and signals

    delta_startup_time("initialize commands API");

    commands_init();

    delta_startup_time("ready");

    usec_t ready_ut = now_monotonic_usec();
    add_agent_event(EVENT_AGENT_START_TIME, (int64_t ) (ready_ut - started_ut));
    usec_t median_start_time = get_agent_event_time_median(EVENT_AGENT_START_TIME);
    netdata_log_info(
        "NETDATA STARTUP: completed in %llu ms (median start up time is %llu ms). Enjoy real-time performance monitoring!",
        (ready_ut - started_ut) / USEC_PER_MS, median_start_time / USEC_PER_MS);

    cleanup_agent_event_log();
    netdata_ready = true;

    analytics_statistic_t start_statistic = { "START", "-",  "-" };
    analytics_statistic_send(&start_statistic);
    if (crash_detected) {
        analytics_statistic_t crash_statistic = { "CRASH", "-",  "-" };
        analytics_statistic_send(&crash_statistic);
    }
    if (incomplete_shutdown_detected) {
        analytics_statistic_t incomplete_shutdown_statistic = { "INCOMPLETE_SHUTDOWN", "-", "-" };
        analytics_statistic_send(&incomplete_shutdown_statistic);
    }

    //check if ANALYTICS needs to start
    if (netdata_anonymous_statistics_enabled == 1) {
        for (i = 0; static_threads[i].name != NULL; i++) {
            if (!strncmp(static_threads[i].name, "ANALYTICS", 9)) {
                struct netdata_static_thread *st = &static_threads[i];
                st->enabled = 1;
                netdata_log_debug(D_SYSTEM, "Starting thread %s.", st->name);
                st->thread = nd_thread_create(st->name, NETDATA_THREAD_OPTION_DEFAULT, st->start_routine, st);
            }
        }
    }

    webrtc_initialize();
    return 10;
}

#ifndef OS_WINDOWS
int main(int argc, char *argv[])
{
    int rc = netdata_main(argc, argv);
    if (rc != 10)
        return rc;

    nd_process_signals();
    return 1;
}
#endif
