// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"
#include "buildinfo.h"
#include "daemon-shutdown-watcher.h"
#include "status-file.h"
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
bool netdata_anonymous_statistics_enabled = true;

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
            " |.-.   .-.   .-.   .-.   .  Netdata                                         \n"
            " |   '-'   '-'   '-'   '-'   X-Ray Vision for your infrastructure!           \n"
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
            "  -W cmakecache            Print the cmake cache used for building this agent\n"
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

#if defined(FSANITIZE_ADDRESS)
#define LOG_TO_STDERR(...) fprintf(stdout, __VA_ARGS__)
#else
#define LOG_TO_STDERR(...)
#endif

#define delta_startup_time(msg)                                     \
    do {                                                            \
        usec_t now_ut = now_monotonic_usec();                       \
        if(prev_msg) {                                              \
            netdata_log_info("NETDATA STARTUP: in %7llu ms, %s - next: %s", (now_ut - last_ut) / USEC_PER_MS, prev_msg, msg); \
            LOG_TO_STDERR(" > startup: in %7llu ms, %s - next: %s\n", (now_ut - last_ut) / USEC_PER_MS, prev_msg, msg); \
        }                                                           \
        else {                                                      \
            netdata_log_info("NETDATA STARTUP: next: %s", msg);     \
            LOG_TO_STDERR(" > startup: next: %s\n", msg);           \
        }                                                           \
        last_ut = now_ut;                                           \
        prev_msg = msg;                                             \
        daemon_status_file_startup_step("startup(" msg ")");        \
    } while(0)

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
int eval_unittest(void);
int duration_unittest(void);
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

static void fatal_status_file_save(void) {
    daemon_status_file_update_status(DAEMON_STATUS_NONE);
    exit(1);
}

int netdata_main(int argc, char **argv) {
    libjudy_malloc_init();
    string_init();
    analytics_init();

    netdata_start_time = now_realtime_sec();
    usec_t started_ut = now_monotonic_usec();
    usec_t last_ut = started_ut;
    const char *prev_msg = NULL;

    int i;
    int config_loaded = 0;
    bool close_open_fds = true; (void)close_open_fds;
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
                    inicfg_set(&netdata_config, CONFIG_SECTION_WEB, "bind to", optarg);
                    break;
                case 'P':
                    pidfile = strdupz(optarg);
                    break;
                case 'p':
                    inicfg_set(&netdata_config, CONFIG_SECTION_GLOBAL, "default port", optarg);
                    break;
                case 's':
                    inicfg_set(&netdata_config, CONFIG_SECTION_GLOBAL, "host access prefix", optarg);
                    break;
                case 't':
                    inicfg_set(&netdata_config, CONFIG_SECTION_GLOBAL, "update every", optarg);
                    break;
                case 'u':
                    inicfg_set(&netdata_config, CONFIG_SECTION_GLOBAL, "run as user", optarg);
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
                            inicfg_set(&netdata_config, CONFIG_SECTION_DB, "dbengine page type", "gorilla");
#ifdef ENABLE_DBENGINE
                            default_rrdeng_disk_quota_mb = default_multidb_disk_quota_mb = 256;
#endif

                            if (sqlite_library_init())
                                return 1;
                            rrdlabels_aral_init(false);

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
                            if (eval_unittest()) return 1;
                            if (duration_unittest()) return 1;
                            if (unittest_waiting_queue()) return 1;
                            if (uuidmap_unittest()) return 1;
                            if (stacktrace_unittest()) return 1;
#ifdef OS_WINDOWS
                            if (perflibnamestest_main()) return 1;
#endif
                            sqlite_library_shutdown();
                            rrdlabels_aral_destroy(false);
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
                            rrdlabels_aral_init(true);
                            int rc = rrdlabels_unittest();
                            rrdlabels_aral_destroy(true);
                            return rc;
                        }
                        else if(strcmp(optarg, "buffertest") == 0) {
                            unittest_running = true;
                            return buffer_unittest();
                        }
                        else if(strcmp(optarg, "test_cmd_pool_fifo") == 0) {
                            unittest_running = true;
                            return test_cmd_pool_fifo();
                        }
                        else if(strcmp(optarg, "uuidtest") == 0) {
                            unittest_running = true;
                            return uuid_unittest();
                        }
                        else if(strcmp(optarg, "stacktracetest") == 0) {
                            unittest_running = true;
                            return stacktrace_unittest();
                        }
#ifdef OS_WINDOWS
                        else if(strcmp(optarg, "perflibdump") == 0) {
                            return windows_perflib_dump(optind + 1 > argc ? NULL : argv[optind]);
                        }
                        else if(strcmp(optarg, "perflibnamestest") == 0) {
                            unittest_running = true;
                            return perflibnamestest_main();
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
                        else if(strcmp(optarg, "evaltest") == 0) {
                            unittest_running = true;
                            return eval_unittest();
                        }
                        else if(strcmp(optarg, "durationtest") == 0) {
                            unittest_running = true;
                            return duration_unittest();
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
                            inicfg_set(&netdata_config, CONFIG_SECTION_GLOBAL, "pthread stack size", optarg);
                        }
                        else if(strncmp(optarg, debug_flags_string, strlen(debug_flags_string)) == 0) {
                            optarg += strlen(debug_flags_string);
                            inicfg_set(&netdata_config, CONFIG_SECTION_LOGS, "debug flags",  optarg);
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
                            inicfg_set_default_raw_value(&netdata_config, section, key, value);

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
                            inicfg_set_default_raw_value(tmp_config, section, key, value);

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
                            const char *value = inicfg_get(&netdata_config, section, key, def);
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
                            const char *value = inicfg_get(tmp_config, section, key, def);
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
                        else if(strcmp(optarg, "cmakecache") == 0) {
                            print_build_info_cmake_cache();
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

#if !defined(FSANITIZE_ADDRESS)
    if (close_open_fds == true) {
        // close all open file descriptors, except the standard ones
        // the caller may have left open files (lxc-attach has this issue)
        os_close_all_non_std_open_fds_except(NULL, 0, 0);
    }
#else
    fprintf(stderr, "Running with a Sanitizer, custom allocators are disabled.\n");
    fprintf(stderr, "Running with a Sanitizer, not closing open fds.\n");
#endif

    if(!config_loaded) {
        netdata_conf_load(NULL, 0, &user);
        cloud_conf_load(0);
    }

    // ----------------------------------------------------------------------------------------------------------------
    // initialize the logging system
    // IMPORTANT: KEEP THIS FIRST SO THAT THE REST OF NETDATA WILL LOG PROPERLY

    netdata_conf_section_directories();
    netdata_conf_section_logs();
    nd_log_limits_unlimited();
    nd_log_initialize();

    // ----------------------------------------------------------------------------------------------------------------
    // this MUST be before anything else - to load the old status file before saving a new one

    daemon_status_file_init(); // this loads the old file
    machine_guid_get(); // after loading the old daemon status file - we may need the machine guid from it
    nd_log_register_fatal_hook_cb(daemon_status_file_register_fatal);
    nd_log_register_fatal_final_cb(fatal_status_file_save);
    exit_initiated_init();

    // ----------------------------------------------------------------------------------------------------------------
    delta_startup_time("signals");

    signals_block_all_except_deadly();
    nd_initialize_signals(false); // catches deadly signals and stores them in the status file

    // ----------------------------------------------------------------------------------------------------------------

    netdata_conf_section_global(); // get hostname, host prefix, profile, etc
    registry_init(); // for machine_guid, must be after netdata_conf_section_global()

    // ----------------------------------------------------------------------------------------------------------------
    delta_startup_time("run dir");

    {
        const char *run_dir = os_run_dir(true);
        if(!run_dir) {
            netdata_log_error("Cannot get/create a run directory.");
            exit(1);
        }
        netdata_log_info("Netdata run directory is '%s'", run_dir);

//        char lock_file[FILENAME_MAX];
//        snprintfz(lock_file, sizeof(lock_file), "%s/netdata.lock", run_dir);
//        FILE_LOCK lock = file_lock_get(lock_file);
//        if(!FILE_LOCK_OK(lock)) {
//            netdata_log_error("Cannot get exclusive lock on file '%s'. Is Netdata already running?", lock_file);
//            exit(1);
//        }
    }

    nd_profile_setup();

    // ----------------------------------------------------------------------------------------------------------------
    delta_startup_time("crash reports");

    daemon_status_file_check_crash();

    // ----------------------------------------------------------------------------------------------------------------
    delta_startup_time("temp spawn server");

    netdata_main_spawn_server_init("init", argc, (const char **)argv);

    // ----------------------------------------------------------------------------------------------------------------
    delta_startup_time("ssl");

    netdata_conf_ssl();

    // ----------------------------------------------------------------------------------------------------------------
    delta_startup_time("environment for plugins");

    // prepare configuration environment variables for the plugins
    set_environment_for_plugins_and_scripts();

    // ----------------------------------------------------------------------------------------------------------------
    delta_startup_time("cd to user config dir");

    // cd into config_dir to allow the plugins refer to their config files using relative filenames
    if(chdir(netdata_configured_user_config_dir) == -1)
        fatal("Cannot cd to '%s'", netdata_configured_user_config_dir);

    // ----------------------------------------------------------------------------------------------------------------
    delta_startup_time("analytics");

    analytics_reset();
    get_system_timezone();
    rrdlabels_aral_init(true);

    // ----------------------------------------------------------------------------------------------------------------
    delta_startup_time("pulse");

#ifdef NETDATA_INTERNAL_CHECKS
    pulse_enabled = true;
    pulse_extended_enabled = true;
#endif

    pulse_extended_enabled =
        inicfg_get_boolean(&netdata_config, CONFIG_SECTION_PULSE, "extended", pulse_extended_enabled);

    if(pulse_extended_enabled)
        // this has to run before starting any other threads that use workers
        workers_utilization_enable();

    // ----------------------------------------------------------------------------------------------------------------
    delta_startup_time("replication");

    replication_initialize();

    // ----------------------------------------------------------------------------------------------------------------
    delta_startup_time("inflight functions");

    rrd_functions_inflight_init();

    // ----------------------------------------------------------------------------------------------------------------
    delta_startup_time("silencers");

    health_set_silencers_filename();
    health_initialize_global_silencers();

    // ----------------------------------------------------------------------------------------------------------------
    delta_startup_time("static threads");

    for (i = 0; static_threads[i].name != NULL ; i++) {
        struct netdata_static_thread *st = &static_threads[i];

        if(st->enable_routine)
            st->enabled = st->enable_routine();

        if(st->config_name)
            st->enabled = inicfg_get_boolean(&netdata_config, st->config_section, st->config_name, st->enabled);

        if(st->enabled && st->init_routine)
            st->init_routine();

        if(st->env_name)
            nd_setenv(st->env_name, st->enabled?"YES":"NO", 1);

        if(st->global_variable)
            *st->global_variable = (st->enabled) ? true : false;
    }

    // ----------------------------------------------------------------------------------------------------------------
    delta_startup_time("web server api");

    // get the certificate and start security
    netdata_conf_web_security_init();
    nd_web_api_init();
    web_server_threading_selection();

    delta_startup_time("web server sockets");
    if(web_server_mode != WEB_SERVER_MODE_NONE)
        web_server_listen_sockets_setup();

    // ----------------------------------------------------------------------------------------------------------------
    delta_startup_time("sqlite");

    if (sqlite_library_init())
        fatal("Failed to initialize sqlite library");

    // ----------------------------------------------------------------------------------------------------------------
    delta_startup_time("ML");

    ml_init();

    // ----------------------------------------------------------------------------------------------------------------
    delta_startup_time("resource limits");

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

    // ----------------------------------------------------------------------------------------------------------------
    delta_startup_time("stop temporary spawn server");

    // stop the old server and later start a new one under the new permissions
    netdata_main_spawn_server_cleanup();

// ----------------------------------------------------------------------------------------------------------------

#if defined(OS_LINUX) || defined(OS_MACOS) || defined(OS_FREEBSD)
    delta_startup_time("become daemon");

    // fork, switch user, create the pid file, set process priority
    if(become_daemon(dont_fork, user) == -1)
        fatal("Cannot daemonize myself.");
#else
    (void)dont_fork;
#endif

    // ----------------------------------------------------------------------------------------------------------------
    delta_startup_time("plugins spawn server");

    netdata_main_spawn_server_init("plugins", argc, (const char **)argv);

#ifdef ENABLE_SENTRY
    // ----------------------------------------------------------------------------------------------------------------
    delta_startup_time("sentry");

    nd_sentry_init();
#endif

    // ----------------------------------------------------------------------------------------------------------------
    delta_startup_time("home");

    // The "HOME" env var points to the root's home dir because Netdata starts as root. Can't use "HOME".
    struct passwd *pw = getpwuid(getuid());
    if (inicfg_exists(&netdata_config, CONFIG_SECTION_DIRECTORIES, "home") || !pw || !pw->pw_dir) {
        netdata_configured_home_dir = inicfg_get(&netdata_config, CONFIG_SECTION_DIRECTORIES, "home", netdata_configured_home_dir);
    }
    else
        netdata_configured_home_dir = inicfg_get(&netdata_config, CONFIG_SECTION_DIRECTORIES, "home", pw->pw_dir);

    nd_setenv("HOME", netdata_configured_home_dir, 1);

    // ----------------------------------------------------------------------------------------------------------------
    delta_startup_time("dyncfg");

    dyncfg_init(true);

    // ----------------------------------------------------------------------------------------------------------------
    delta_startup_time("threads after fork");

    netdata_conf_reset_stack_size();

    // ----------------------------------------------------------------------------------------------------------------
    delta_startup_time("registry");

    registry_load();
    cloud_conf_init_after_registry();
    netdata_random_session_id_generate();

#ifdef ENABLE_SENTRY
    nd_sentry_set_user(machine_guid_get_txt());
#endif

    // ----------------------------------------------------------------------------------------------------------------
    delta_startup_time("system info");

    struct rrdhost_system_info *system_info = rrdhost_system_info_create();
    rrdhost_system_info_detect(system_info);

    get_install_type(system_info);

    set_late_analytics_variables(system_info);

    // ----------------------------------------------------------------------------------------------------------------
    delta_startup_time("RRD structures");

    abort_on_fatal_disable();
    if (rrd_init(netdata_configured_hostname, system_info, false))
        fatal("Cannot initialize localhost instance with name '%s'.", netdata_configured_hostname);

    abort_on_fatal_enable();
    system_info = NULL; // system_info is now freed by rrd_init

    // ----------------------------------------------------------------------------------------------------------------
    delta_startup_time("localhost labels");

    reload_host_labels();

    // ----------------------------------------------------------------------------------------------------------------
    delta_startup_time("saved bearer tokens");

    bearer_tokens_init();

    // ----------------------------------------------------------------------------------------------------------------
    delta_startup_time("claiming info");

    load_claiming_state();

    // ----------------------------------------------------------------------------------------------------------------
    delta_startup_time("static threads");

    nd_log_limits_reset();
    get_agent_event_time_median_init();

    netdata_conf_section_web();

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

    // ----------------------------------------------------------------------------------------------------------------
    delta_startup_time("commands API");

    commands_init();

    // ----------------------------------------------------------------------------------------------------------------
    delta_startup_time("agent start timings");

    usec_t ready_ut = now_monotonic_usec();
    add_agent_event(EVENT_AGENT_START_TIME, (int64_t ) (ready_ut - started_ut));
    usec_t median_start_time = get_agent_event_time_median(EVENT_AGENT_START_TIME);
    netdata_log_info(
        "NETDATA STARTUP: completed in %llu ms (median start up time is %llu ms). "
        "Enjoy X-Ray Vision for your infrastructure!",
        (ready_ut - started_ut) / USEC_PER_MS, median_start_time / USEC_PER_MS);

    cleanup_agent_event_log();
    netdata_ready = true;

    // ----------------------------------------------------------------------------------------------------------------

    {
        if(analytics_check_enabled())
            delta_startup_time("anonymous analytics");
        else // collect data but do not send it (needed in /api/v1/info)
            delta_startup_time("anonymous analytics (disabled)");

        // check if ANALYTICS needs to start
        for (i = 0; static_threads[i].name != NULL; i++) {
            if (!strncmp(static_threads[i].name, "ANALYTICS", 9)) {
                struct netdata_static_thread *st = &static_threads[i];
                st->enabled = 1;
                netdata_log_debug(D_SYSTEM, "Starting thread %s.", st->name);
                st->thread = nd_thread_create(st->name, NETDATA_THREAD_OPTION_DEFAULT, st->start_routine, st);
            }
        }
    }

    // ----------------------------------------------------------------------------------------------------------------

#ifdef HAVE_LIBDATACHANNEL
    delta_startup_time("webrtc");
    webrtc_initialize();
#endif

    // ----------------------------------------------------------------------------------------------------------------
    delta_startup_time("mrg cleanup");

#ifdef ENABLE_DBENGINE
    mrg_metric_prepopulate_cleanup(main_mrg);
#endif

    // ----------------------------------------------------------------------------------------------------------------
    delta_startup_time("done");

    nd_log_register_fatal_final_cb(netdata_exit_fatal);
    daemon_status_file_startup_step(NULL);
    daemon_status_file_update_status(DAEMON_STATUS_RUNNING);
    return 10;
}

#ifndef OS_WINDOWS
int main(int argc, char *argv[])
{
    int rc = netdata_main(argc, argv);
    if (rc != 10)
        return rc;

#if defined(FSANITIZE_ADDRESS)
    fprintf(stdout, "STDOUT: Sanitizers mode enabled...\n");
    fprintf(stderr, "STDERR: Sanitizers mode enabled...\n");
#endif

    nd_process_signals();
    return 1;
}
#endif
