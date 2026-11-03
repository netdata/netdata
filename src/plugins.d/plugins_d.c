// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugins_d.h"
#include "pluginsd_parser.h"

char *plugin_directories[PLUGINSD_MAX_DIRECTORIES] = { [0] = PLUGINS_DIR, };
struct plugind *pluginsd_root = NULL;

static inline void pluginsd_sleep(const int seconds) {
    int timeout_ms = seconds * 1000;
    int waited_ms = 0;
    while(waited_ms < timeout_ms) {
        if(!service_running(SERVICE_COLLECTORS)) break;
        sleep_usec(ND_CHECK_CANCELLABILITY_WHILE_WAITING_EVERY_MS * USEC_PER_MS);
        waited_ms += ND_CHECK_CANCELLABILITY_WHILE_WAITING_EVERY_MS;
    }
}

inline size_t pluginsd_initialize_plugin_directories()
{
    char plugins_dirs[(FILENAME_MAX * 2) + 1];
    static char *plugins_dir_list = NULL;

    // Get the configuration entry
    if (likely(!plugins_dir_list)) {
        snprintfz(plugins_dirs, FILENAME_MAX * 2, "\"%s\" \"%s/custom-plugins.d\"", PLUGINS_DIR, CONFIG_DIR);
        plugins_dir_list = strdupz(inicfg_get_quoted_path_list(&netdata_config, CONFIG_SECTION_DIRECTORIES, "plugins", plugins_dirs));
    }

    // Parse it and store it to plugin directories
    return quoted_strings_splitter_config(plugins_dir_list, plugin_directories, PLUGINSD_MAX_DIRECTORIES);
}

static inline void plugin_set_disabled(struct plugind *cd) {
    spinlock_lock(&cd->unsafe.spinlock);
    cd->unsafe.enabled = false;
    spinlock_unlock(&cd->unsafe.spinlock);
}

bool plugin_is_enabled(struct plugind *cd) {
    spinlock_lock(&cd->unsafe.spinlock);
    bool ret = cd->unsafe.enabled;
    spinlock_unlock(&cd->unsafe.spinlock);
    return ret;
}

static inline void plugin_set_running(struct plugind *cd) {
    spinlock_lock(&cd->unsafe.spinlock);
    cd->unsafe.running = true;
    spinlock_unlock(&cd->unsafe.spinlock);
}

static inline bool plugin_is_running(struct plugind *cd) {
    spinlock_lock(&cd->unsafe.spinlock);
    bool ret = cd->unsafe.running;
    spinlock_unlock(&cd->unsafe.spinlock);
    return ret;
}

#define SERIAL_FAILURES_THRESHOLD 10
static void pluginsd_worker_thread_handle_success(struct plugind *cd) {
    if (likely(cd->successful_collections)) {
        pluginsd_sleep(cd->update_every);
        return;
    }

    if (likely(cd->serial_failures <= SERIAL_FAILURES_THRESHOLD)) {
        netdata_log_info("PLUGINSD: 'host:%s', '%s' (pid %d) does not generate useful output but it reports success (exits with 0). %s.",
             rrdhost_hostname(cd->host), string2str(cd->fullfilename), cd->unsafe.pid,
             plugin_is_enabled(cd) ? "Waiting a bit before starting it again." : "Will not start it again - it is now disabled.");

        pluginsd_sleep(cd->update_every * 10);
        return;
    }

    if (cd->serial_failures > SERIAL_FAILURES_THRESHOLD) {
        netdata_log_error("PLUGINSD: 'host:'%s', '%s' (pid %d) does not generate useful output, "
              "although it reports success (exits with 0)."
              "We have tried to collect something %zu times - unsuccessfully. Disabling it.",
              rrdhost_hostname(cd->host), string2str(cd->fullfilename), cd->unsafe.pid, cd->serial_failures);
        plugin_set_disabled(cd);
        return;
    }
}

// The policy for a failed run, whether the plugin exited with an error or we
// disconnected it because we could not parse its output: disable it if it never
// produced useful output, otherwise restart it with backoff until too many
// consecutive runs fail without useful output.
static void pluginsd_worker_thread_handle_failure(struct plugind *cd, const char *failure) {
    if (!cd->successful_collections) {
        netdata_log_error("PLUGINSD: 'host:%s', '%s' (pid %d) %s and haven't collected any data. Disabling it.",
              rrdhost_hostname(cd->host), string2str(cd->fullfilename), cd->unsafe.pid, failure);
        plugin_set_disabled(cd);
        return;
    }

    if (cd->serial_failures <= SERIAL_FAILURES_THRESHOLD) {
        netdata_log_error("PLUGINSD: 'host:%s', '%s' (pid %d) %s, but has given useful output in the past (%zu times). %s",
              rrdhost_hostname(cd->host), string2str(cd->fullfilename), cd->unsafe.pid, failure, cd->successful_collections,
              plugin_is_enabled(cd) ? "Waiting a bit before starting it again." : "Will not start it again - it is disabled.");

        pluginsd_sleep(cd->update_every * 10);
        return;
    }

    netdata_log_error("PLUGINSD: 'host:%s', '%s' (pid %d) %s, but has given useful output in the past (%zu times)."
          "We tried to restart it %zu times, but it failed to generate data. Disabling it.",
          rrdhost_hostname(cd->host), string2str(cd->fullfilename), cd->unsafe.pid, failure,
          cd->successful_collections, cd->serial_failures);
    plugin_set_disabled(cd);
}

static void pluginsd_worker_thread_handle_error(struct plugind *cd, int worker_ret_code) {
    if (worker_ret_code == -1) {
        netdata_log_info("PLUGINSD: 'host:%s', '%s' (pid %d) exited abnormally. Disabling it.",
             rrdhost_hostname(cd->host), string2str(cd->fullfilename), cd->unsafe.pid);
        plugin_set_disabled(cd);
        return;
    }

    char failure[50];
    snprintfz(failure, sizeof(failure), "exited with error code %d", worker_ret_code);
    pluginsd_worker_thread_handle_failure(cd, failure);
}

#undef SERIAL_FAILURES_THRESHOLD


static void pluginsd_worker_thread(void *arg) {
    struct plugind *cd = (struct plugind *) arg;

    worker_register("PLUGINSD");

    plugin_set_running(cd);

    size_t count = 0;

    while(service_running(SERVICE_COLLECTORS)) {
        cd->unsafe.pi = spawn_popen_run(string2str(cd->cmd));
        if(!cd->unsafe.pi) {
            netdata_log_error("PLUGINSD: 'host:%s', cannot popen(\"%s\", \"r\").",
                              rrdhost_hostname(cd->host), string2str(cd->cmd));
            break;
        }
        cd->unsafe.pid = spawn_popen_pid(cd->unsafe.pi);

        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "PLUGINSD: 'host:%s' connected to '%s' running on pid %d",
               rrdhost_hostname(cd->host),
               string2str(cd->fullfilename), cd->unsafe.pid);

        const char *plugin = strrchr(string2str(cd->fullfilename), '/');
        if(plugin)
            plugin++;
        else
            plugin = string2str(cd->fullfilename);

        char module[100];
        snprintfz(module, sizeof(module), "plugins.d[%s]", plugin);
        ND_LOG_STACK lgs[] = {
                ND_LOG_FIELD_TXT(NDF_MODULE, module),
                ND_LOG_FIELD_TXT(NDF_NIDL_NODE, rrdhost_hostname(cd->host)),
                ND_LOG_FIELD_TXT(NDF_SRC_TRANSPORT, "pluginsd"),
                ND_LOG_FIELD_END(),
        };
        ND_LOG_STACK_PUSH(lgs);

        bool retry = false, protocol_error = false;
        count = pluginsd_process(cd->host, cd,
                                 spawn_popen_read_fd(cd->unsafe.pi),
                                 spawn_popen_write_fd(cd->unsafe.pi),
                                 0, &retry, &protocol_error);

        nd_log(NDLS_COLLECTORS, NDLP_WARNING,
               "PLUGINSD: 'host:%s', '%s' (pid %d) disconnected after %zu successful data collections.",
               rrdhost_hostname(cd->host), string2str(cd->fullfilename), cd->unsafe.pid, count);

        int worker_ret_code = spawn_popen_kill(cd->unsafe.pi, 3 * MSEC_PER_SEC);
        cd->unsafe.pi = NULL;

        if (retry && worker_ret_code != -1)
            pluginsd_sleep(cd->update_every);
        else if (protocol_error && worker_ret_code != -1)
            // after the disconnect the plugin usually exits cleanly or on SIGTERM/SIGPIPE,
            // so its exit code cannot tell that the run failed
            pluginsd_worker_thread_handle_failure(cd, "sent output that could not be parsed");
        else if(likely(worker_ret_code == 0))
            pluginsd_worker_thread_handle_success(cd);
        else
            pluginsd_worker_thread_handle_error(cd, worker_ret_code);

        cd->unsafe.pid = 0;

        if(unlikely(!plugin_is_enabled(cd)))
            break;
    }

    spinlock_lock(&cd->unsafe.spinlock);

    cd->unsafe.running = false;
    cd->unsafe.pid = 0;

    POPEN_INSTANCE *pi = cd->unsafe.pi;
    cd->unsafe.pi = NULL;

    spinlock_unlock(&cd->unsafe.spinlock);

    if (pi)
        spawn_popen_kill(pi, 3 * MSEC_PER_SEC);

    worker_unregister();
}

static void pluginsd_main_cleanup(void *pptr) {
    struct netdata_static_thread *static_thread = CLEANUP_FUNCTION_GET_PTR(pptr);
    if(!static_thread) return;

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;
    netdata_log_info("PLUGINSD: cleaning up...");

    struct plugind *cd = pluginsd_root;
    while(cd) {
        struct plugind *next = cd->next;

        spinlock_lock(&cd->unsafe.spinlock);
        if (cd->unsafe.enabled && cd->unsafe.running && cd->unsafe.thread != 0) {
            netdata_log_info("PLUGINSD: 'host:%s', stopping plugin thread: %s",
                 rrdhost_hostname(cd->host), string2str(cd->id));

            nd_thread_signal_cancel(cd->unsafe.thread);
        }

        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(pluginsd_root, cd, prev, next);
        spinlock_unlock(&cd->unsafe.spinlock);

        if(cd->unsafe.thread) {
            nd_thread_signal_cancel(cd->unsafe.thread);
            nd_thread_join(cd->unsafe.thread);
            cd->unsafe.thread = NULL;
        }

        string_freez(cd->fullfilename);
        string_freez(cd->filename);
        string_freez(cd->id);
        string_freez(cd->cmd);
        freez(cd);

        cd = next;
    }

    netdata_log_info("PLUGINSD: cleanup completed.");
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;

    worker_unregister();
}

static bool is_plugin(char *dst, size_t dst_size, const char *filename) {
#if defined(OS_WINDOWS)
    const char *suffixes[] = {
        ".plugin.exe",
        "_plugin.exe",
        "-plugin.exe",
        ".plugin",
        NULL
    };
#else
    const char *suffixes[] = {
        ".plugin",
        "_plugin",
        "-plugin",
        NULL
    };
#endif

    size_t filename_len = strlen(filename);

    for (int i = 0; suffixes[i] != NULL; i++) {
        size_t suffix_len = strlen(suffixes[i]);

        if (filename_len > suffix_len &&
            strcmp(suffixes[i], &filename[filename_len - suffix_len]) == 0) {
            snprintfz(dst, dst_size, "%.*s", (int)(filename_len - suffix_len), filename);
            return true;
        }
    }

    return false;
}

// Plugins that were removed from the agent but may linger on disk after an
// upgrade.
static bool is_obsolete_plugin(const char *pluginname) {
    static const char *obsolete[] = {
        "otel-signal-viewer",
        NULL,
    };

    for (size_t i = 0; obsolete[i]; i++) {
        if (strcmp(pluginname, obsolete[i]) == 0)
            return true;
    }

    return false;
}

static bool pluginsd_find_ipmi(char *path, size_t size, char *const *directories, const char *name) {
    for (size_t i = 0; i < PLUGINSD_MAX_DIRECTORIES && directories[i]; i++) {
        DIR *dir = opendir(directories[i]);
        if (!dir)
            continue; // The normal scan reports directory errors.

        struct dirent *file;
        while ((file = readdir(dir))) {
            char pluginname[CONFIG_MAX_NAME + 1];
            if (!is_plugin(pluginname, sizeof(pluginname), file->d_name) || strcmp(pluginname, name))
                continue;

            char candidate[FILENAME_MAX + 1];
            int len = snprintf(candidate, sizeof(candidate), "%s/%s", directories[i], file->d_name);
            struct stat st;
            if (len < 0 || (size_t)len >= sizeof(candidate) || stat(candidate, &st) ||
                !S_ISREG(st.st_mode) || access(candidate, X_OK))
                continue;

            strncpyz(path, candidate, size - 1);
            closedir(dir);
            return true;
        }
        closedir(dir);
    }
    return false;
}

// Resolve once before spawning workers. A failed plugin or a later directory scan
// must not transfer ownership to the other implementation.
static void pluginsd_select_ipmi(struct config *config, char *const *directories, int default_run,
                                char *path, size_t size) {
    bool legacy_configured = inicfg_exists(config, CONFIG_SECTION_PLUGINS, "freeipmi");
    bool legacy_enabled = inicfg_get_boolean(config, CONFIG_SECTION_PLUGINS, "freeipmi", default_run);
    const char *options = "";
    if (inicfg_exists(config, "plugin:freeipmi", "command options"))
        options = inicfg_get(config, "plugin:freeipmi", "command options", "");
    options += strspn(options, " \t\r\n");

    // Preserve deliberate disablement and legacy settings (possibly remote BMCs).
    // An explicit ipmi=yes overrides this default without interpreting C options.
    int go_default = (legacy_configured && !legacy_enabled) || *options ? CONFIG_BOOLEAN_NO : default_run;
    bool go_enabled = inicfg_get_boolean(config, CONFIG_SECTION_PLUGINS, "ipmi", go_default);

    path[0] = '\0';
    if (go_enabled && pluginsd_find_ipmi(path, size, directories, "ipmi"))
        return;
    if (legacy_enabled)
        pluginsd_find_ipmi(path, size, directories, "freeipmi");
}

static bool pluginsd_ipmi_allowed(const char *selected, const char *pluginname,
                                 const char *directory, const char *filename) {
    if (strcmp(pluginname, "ipmi") && strcmp(pluginname, "freeipmi"))
        return true;

    char path[FILENAME_MAX + 1];
    int len = snprintf(path, sizeof(path), "%s/%s", directory, filename);
    return len >= 0 && (size_t)len < sizeof(path) && !strcmp(selected, path);
}

int pluginsd_ipmi_unittest(void) {
#if defined(OS_WINDOWS)
    return 0; // Neither IPMI implementation is shipped on Windows.
#else
    char directory[] = "/tmp/netdata-ipmi-selection-XXXXXX";
    if (!mkdtemp(directory))
        return 1;

    char other[FILENAME_MAX + 1], legacy[FILENAME_MAX + 1], modern[FILENAME_MAX + 1];
    snprintfz(other, sizeof(other), "%s/other", directory);
    snprintfz(legacy, sizeof(legacy), "%s/freeipmi.plugin", directory);
    snprintfz(modern, sizeof(modern), "%s/ipmi.plugin", other);
    int errors = 0;
    if (mkdir(other, 0700)) {
        rmdir(directory);
        return 1;
    }
    const char *files[] = { legacy, modern };
    for (size_t i = 0; i < _countof(files); i++) {
        // Selection only needs executable paths; these fixtures are never launched.
        int fd = open(files[i], O_WRONLY | O_CREAT | O_EXCL, 0700);
        if (fd == -1) {
            errors++;
            goto cleanup;
        }
        if (close(fd) || chmod(files[i], 0700)) {
            errors++;
            goto cleanup;
        }
    }

    char *both[] = { directory, other, NULL };
    char *reversed[] = { other, directory, NULL };
    char *only_legacy[] = { directory, NULL };
    char *only_modern[] = { other, NULL };
    char *neither[] = { NULL };
    struct {
        const char *name, *go, *c, *options;
        int default_run;
        char **directories;
        const char *expected;
    } cases[] = {
        { "Go in later directory", NULL, NULL, NULL, 1, both, modern },
        { "directory order reversed", NULL, NULL, NULL, 1, reversed, modern },
        { "explicitly enabled both", "yes", "yes", NULL, 1, both, modern },
        { "legacy disablement", NULL, "no", NULL, 1, both, "" },
        { "explicit Go overrides disablement", "yes", "no", NULL, 1, both, modern },
        { "rollback", "no", "yes", NULL, 1, both, legacy },
        { "both disabled", "no", "no", NULL, 1, both, "" },
        { "legacy remote config", NULL, NULL, "hostname bmc.example", 1, both, legacy },
        { "disabled legacy config", NULL, "no", "no-sel", 1, both, "" },
        { "explicit migration", "yes", NULL, "no-sel", 1, both, modern },
        { "whitespace options", NULL, NULL, " \t\r\n", 1, both, modern },
        { "automatic run disabled", NULL, NULL, NULL, 0, both, "" },
        { "Go overrides automatic run", "yes", NULL, NULL, 0, both, modern },
        { "C overrides automatic run", NULL, "yes", NULL, 0, both, legacy },
        { "only C installed", NULL, NULL, NULL, 1, only_legacy, legacy },
        { "only Go installed", NULL, NULL, NULL, 1, only_modern, modern },
        { "legacy options without C installed", NULL, NULL, "no-sel", 1, only_modern, "" },
        { "nothing installed", NULL, NULL, NULL, 1, neither, "" },
    };
    for (size_t i = 0; i < _countof(cases); i++) {
        struct config config = APPCONFIG_INITIALIZER;
        if (cases[i].go)
            inicfg_set(&config, CONFIG_SECTION_PLUGINS, "ipmi", cases[i].go);
        if (cases[i].c)
            inicfg_set(&config, CONFIG_SECTION_PLUGINS, "freeipmi", cases[i].c);
        if (cases[i].options)
            inicfg_set(&config, "plugin:freeipmi", "command options", cases[i].options);
        char selected[FILENAME_MAX + 1];
        pluginsd_select_ipmi(&config, cases[i].directories, cases[i].default_run, selected, sizeof(selected));
        if (strcmp(selected, cases[i].expected) ||
            pluginsd_ipmi_allowed(selected, "ipmi", other, "ipmi.plugin") != !strcmp(cases[i].expected, modern) ||
            pluginsd_ipmi_allowed(selected, "freeipmi", directory, "freeipmi.plugin") != !strcmp(cases[i].expected, legacy) ||
            !pluginsd_ipmi_allowed(selected, "go.d", directory, "go.d.plugin") ||
            pluginsd_ipmi_allowed(selected, "ipmi", other, "ipmi-plugin")) {
            fprintf(stderr, "IPMI selection failed: %s\n", cases[i].name);
            errors++;
        }
        inicfg_free(&config);
    }

    // A present but non-executable Go binary must not suppress an available C plugin.
    if (chmod(modern, 0600))
        errors++;
    else {
        struct config config = APPCONFIG_INITIALIZER;
        char selected[FILENAME_MAX + 1];
        pluginsd_select_ipmi(&config, both, 1, selected, sizeof(selected));
        if (strcmp(selected, legacy)) {
            fprintf(stderr, "IPMI selection failed: non-executable Go binary\n");
            errors++;
        }
        inicfg_free(&config);
    }

cleanup:
    unlink(modern);
    unlink(legacy);
    rmdir(other);
    rmdir(directory);
    fprintf(stderr, "IPMI selection: %d errors\n", errors);
    return errors ? 1 : 0;
#endif
}

void *pluginsd_main(void *ptr) {
    CLEANUP_FUNCTION_REGISTER(pluginsd_main_cleanup) cleanup_ptr = ptr;

    int automatic_run = inicfg_get_boolean(&netdata_config, CONFIG_SECTION_PLUGINS, "enable running new plugins", 1);
    int scan_frequency = (int)inicfg_get_duration_seconds(&netdata_config, CONFIG_SECTION_PLUGINS, "check for new plugins every", 60);
    if (scan_frequency < 1)
        scan_frequency = 1;

    // disable some plugins by default
    inicfg_get_boolean(&netdata_config, CONFIG_SECTION_PLUGINS, "slabinfo", CONFIG_BOOLEAN_NO);
    // Preserve the container opt-in policy for IPMI. FreeIPMI can crash without
    // an exposed device: https://github.com/netdata/netdata/pull/15564
    char ipmi_path[FILENAME_MAX + 1];
    pluginsd_select_ipmi(&netdata_config, plugin_directories,
                        getenv("NETDATA_LISTENER_PORT") ? CONFIG_BOOLEAN_NO : automatic_run,
                        ipmi_path, sizeof(ipmi_path));
    if (*ipmi_path)
        netdata_log_info("PLUGINSD: using '%s' for IPMI; other IPMI plugin candidates are suppressed until restart", ipmi_path);
    else
        netdata_log_debug(D_PLUGINSD, "no enabled IPMI plugin selected; restart Netdata after installing or enabling one");

    // store the errno for each plugins directory
    // so that we don't log broken directories on each loop
    int directory_errors[PLUGINSD_MAX_DIRECTORIES] = { 0 };

    while (service_running(SERVICE_COLLECTORS)) {
        int idx;
        const char *directory_name;

        for (idx = 0; idx < PLUGINSD_MAX_DIRECTORIES && (directory_name = plugin_directories[idx]); idx++) {
            if (unlikely(!service_running(SERVICE_COLLECTORS)))
                break;

            errno_clear();
            DIR *dir = opendir(directory_name);
            if (unlikely(!dir)) {
                if (directory_errors[idx] != errno) {
                    directory_errors[idx] = errno;
                    netdata_log_error("cannot open plugins directory '%s'", directory_name);
                }
                continue;
            }

            struct dirent *file = NULL;
            while (likely((file = readdir(dir)))) {
                if (unlikely(!service_running(SERVICE_COLLECTORS)))
                    break;

                netdata_log_debug(D_PLUGINSD, "examining file '%s'", file->d_name);

                if (unlikely(strcmp(file->d_name, ".") == 0 || strcmp(file->d_name, "..") == 0))
                    continue;

                char pluginname[CONFIG_MAX_NAME + 1];
                if(!is_plugin(pluginname, sizeof(pluginname), file->d_name)) {
                    netdata_log_debug(D_PLUGINSD, "file '%s' does not look like a plugin", file->d_name);
                    continue;
                }

                if (!pluginsd_ipmi_allowed(ipmi_path, pluginname, directory_name, file->d_name))
                    continue;

                // Skip any obsolete plugins.
                if(is_obsolete_plugin(pluginname)) {
                    // log info level first time, debug level afterwards.
                    static bool logged_obsolete = false;
                    if(!logged_obsolete) {
                        logged_obsolete = true;
                        netdata_log_info("skipping obsolete plugin '%s'; the current agent already provides its function", file->d_name);
                    }
                    else {
                        netdata_log_debug(D_PLUGINSD, "skipping obsolete plugin '%s'", file->d_name);
                    }
                    continue;
                }

                int enabled = inicfg_get_boolean(&netdata_config, CONFIG_SECTION_PLUGINS, pluginname, automatic_run);
                if (unlikely(!enabled)) {
                    netdata_log_debug(D_PLUGINSD, "plugin '%s' is not enabled", file->d_name);
                    continue;
                }

                // check if it runs already
                struct plugind *cd;
                for (cd = pluginsd_root; cd; cd = cd->next) {
                    if (unlikely(strcmp(string2str(cd->filename), file->d_name) == 0)) {
                        break;
                    }
                }

                if(cd) {
                    if (likely(plugin_is_running(cd))) {
                        netdata_log_debug(D_PLUGINSD, "plugin '%s' is already running", string2str(cd->filename));
                        continue;
                    }
                    else if(cd->unsafe.thread) {
                        netdata_log_debug(D_PLUGINSD, "plugin '%s' gave up", string2str(cd->filename));
                        nd_thread_signal_cancel(cd->unsafe.thread);
                        nd_thread_join(cd->unsafe.thread);
                        cd->unsafe.thread = NULL;
                    }
                }

                // it is not running
                // allocate a new one, or use the obsolete one
                if (unlikely(!cd)) {
                    cd = callocz(sizeof(struct plugind), 1);

                    {
                        char buf[CONFIG_MAX_NAME];
                        snprintfz(buf, sizeof(buf), "plugin:%s", pluginname);
                        string_freez(cd->id);
                        cd->id = string_strdupz(buf);
                    }

                    {
                        char buf[FILENAME_MAX + 1];
                        strncpyz(buf, file->d_name, sizeof(buf) - 1);
                        string_freez(cd->filename);
                        cd->filename = string_strdupz(buf);

                        snprintfz(buf, sizeof(buf), "%s/%s", directory_name, string2str(cd->filename));
                        string_freez(cd->fullfilename);
                        cd->fullfilename = string_strdupz(buf);
                    }

                    cd->host = localhost;
                    cd->unsafe.enabled = enabled;
                    cd->unsafe.running = false;

                    cd->update_every = (int)inicfg_get_duration_seconds(&netdata_config, string2str(cd->id), "update every", localhost->rrd_update_every);
                    cd->started_t = now_realtime_sec();

                    {
                        const char *def = "";
                        char buf[PLUGINSD_CMD_MAX + 1];

                        snprintfz(
                            buf, sizeof(buf), "exec %s %d %s", string2str(cd->fullfilename),
                            cd->update_every, inicfg_get(&netdata_config, string2str(cd->id), "command options", def));

                        string_freez(cd->cmd);
                        cd->cmd = string_strdupz(buf);
                    }

                    // link it
                    DOUBLE_LINKED_LIST_PREPEND_ITEM_UNSAFE(pluginsd_root, cd, prev, next);

                    if (plugin_is_enabled(cd)) {
                        char tag[NETDATA_THREAD_TAG_MAX + 1];
                        snprintfz(tag, NETDATA_THREAD_TAG_MAX, "PD[%s]", pluginname);

                        // spawn a new thread for it
                        cd->unsafe.thread = nd_thread_create(tag, NETDATA_THREAD_OPTION_DEFAULT,
                                                             pluginsd_worker_thread, cd);
                    }
                }
            }

            closedir(dir);
        }

        pluginsd_sleep(scan_frequency);
    }

    service_exits();
    return NULL;
}
