// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugins_d.h"
#include "pluginsd_parser.h"

char *plugin_directories[PLUGINSD_MAX_DIRECTORIES] = { [0] = PLUGINS_DIR, };
struct plugind *pluginsd_root = NULL;

static inline void pluginsd_sleep(const int seconds) {
    // usec_t rather than int milliseconds. `seconds * 1000` in an int wrapped negative for large
    // values and skipped the wait entirely - the opposite of the intent. The update_every clamp
    // now keeps callers well inside the range anyway; this is the second line of defence, and it
    // costs nothing.
    usec_t timeout_ut = (seconds > 0) ? (usec_t)seconds * USEC_PER_SEC : 0;
    usec_t waited_ut = 0;
    while(waited_ut < timeout_ut) {
        if(!service_running(SERVICE_COLLECTORS)) break;
        sleep_usec(ND_CHECK_CANCELLABILITY_WHILE_WAITING_EVERY_MS * USEC_PER_MS);
        waited_ut += ND_CHECK_CANCELLABILITY_WHILE_WAITING_EVERY_MS * USEC_PER_MS;
    }
}

// The "wait longer before trying again" delay. Every site that wants it comes through here, so the
// three of them cannot drift - written out by hand as `cd->update_every * 10`, one of them once
// produced no delay at all for values update_every could legitimately hold.
static inline void pluginsd_sleep_backoff(const struct plugind *cd) {
    pluginsd_sleep(cd->update_every * 10);
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

        pluginsd_sleep_backoff(cd);
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

static void pluginsd_worker_thread_handle_error(struct plugind *cd, int worker_ret_code) {
    if (worker_ret_code == -1) {
        netdata_log_info("PLUGINSD: 'host:%s', '%s' (pid %d) exited abnormally. Disabling it.",
             rrdhost_hostname(cd->host), string2str(cd->fullfilename), cd->unsafe.pid);
        plugin_set_disabled(cd);
        return;
    }

    if (!cd->successful_collections) {
        netdata_log_error("PLUGINSD: 'host:%s', '%s' (pid %d) exited with error code %d and haven't collected any data. Disabling it.",
              rrdhost_hostname(cd->host), string2str(cd->fullfilename), cd->unsafe.pid, worker_ret_code);
        plugin_set_disabled(cd);
        return;
    }

    if (cd->serial_failures <= SERIAL_FAILURES_THRESHOLD) {
        netdata_log_error("PLUGINSD: 'host:%s', '%s' (pid %d) exited with error code %d, but has given useful output in the past (%zu times). %s",
              rrdhost_hostname(cd->host), string2str(cd->fullfilename), cd->unsafe.pid, worker_ret_code, cd->successful_collections,
              plugin_is_enabled(cd) ? "Waiting a bit before starting it again." : "Will not start it again - it is disabled.");

        pluginsd_sleep_backoff(cd);
        return;
    }

    if (cd->serial_failures > SERIAL_FAILURES_THRESHOLD) {
        netdata_log_error("PLUGINSD: 'host:%s', '%s' (pid %d) exited with error code %d, but has given useful output in the past (%zu times)."
              "We tried to restart it %zu times, but it failed to generate data. Disabling it.",
              rrdhost_hostname(cd->host), string2str(cd->fullfilename), cd->unsafe.pid, worker_ret_code,
              cd->successful_collections, cd->serial_failures);
        plugin_set_disabled(cd);
        return;
    }
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

        bool retry = false;
        count = pluginsd_process(cd->host, cd,
                                 spawn_popen_read_fd(cd->unsafe.pi),
                                 spawn_popen_write_fd(cd->unsafe.pi),
                                 0, &retry);

        nd_log(NDLS_COLLECTORS, NDLP_WARNING,
               "PLUGINSD: 'host:%s', '%s' (pid %d) disconnected after %zu successful data collections.",
               rrdhost_hostname(cd->host), string2str(cd->fullfilename), cd->unsafe.pid, count);

        SPAWN_KILL_OUTCOME kill_outcome;
        int worker_ret_code = spawn_popen_kill_ex(cd->unsafe.pi, 3 * MSEC_PER_SEC, &kill_outcome);
        cd->unsafe.pi = NULL;

        if(unlikely(kill_outcome == SPAWN_KILL_UNKNOWN)) {
            // We could not confirm the process is gone, so it may still be running and still
            // writing. Starting a replacement would give this collector two live writers for the
            // same charts, which corrupts data rather than merely losing it. Refusing is the only
            // safe answer here, whatever the exit code says.
            netdata_log_error("PLUGINSD: 'host:%s', '%s' (pid %d) could not be confirmed stopped. "
                              "Not starting it again - a second instance would write the same data.",
                              rrdhost_hostname(cd->host), string2str(cd->fullfilename), cd->unsafe.pid);
            plugin_set_disabled(cd);
            cd->unsafe.pid = 0;
            break;
        }

        if (retry) {
            // WE asked for this retry - the parser hit a transient condition (a host being deleted,
            // a receiver it could not evict, a host it could not create) and wants a fresh
            // connection. So honour it, and do NOT ask why the process is dead.
            //
            // That question has no reliable answer, and it is unanswerable differently per platform.
            // On POSIX the exit code maps our own SIGKILL, a crash and an OOM kill all to -1; on
            // Windows they separate, but wrongly for us - our forced termination reports 0, exactly
            // like a clean exit, while a crash reports -1. Nor does the kill outcome help: it says
            // whether WE escalated, never whether our signal is why the child died, because an
            // operator and the OOM killer send the identical SIGKILL. Every rule built on any of
            // that mislabels some real case, so the retry does not depend on it at all.
            //
            // And it retries indefinitely, with no attrition. A plugin is not the right unit to
            // give up on here: go.d.plugin runs many independent jobs over one connection, and the
            // retry conditions are per-vnode - one device whose receiver cannot be evicted aborts
            // the session for all of them. Disabling on a streak would stop every healthy job in
            // the process over one stuck device, and nothing re-enables a disabled collector short
            // of an agent restart. The plugin is not what failed, so the plugin is not what stops.
            // Throttled because this path has no end: a condition that never clears restarts the
            // plugin every backoff, forever, and an unthrottled line here is thousands a day per
            // plugin. Thread-local, and this thread is this plugin, so a noisy one cannot hide
            // another's. The limiter reports how many it suppressed.
            nd_log_limit_static_thread_var(erl_retry, 60, 0);
            nd_log_limit(&erl_retry, NDLS_COLLECTORS, NDLP_WARNING,
                   "PLUGINSD: 'host:%s', '%s' is being restarted for a retry we requested%s.",
                   rrdhost_hostname(cd->host), string2str(cd->fullfilename),
                   kill_outcome == SPAWN_KILL_FORCED_EXITED ? " (it had to be forced to exit)" : "");

            // the condition that triggered the retry can persist for a while - a host deletion
            // holds its lock across two cloud round-trips - so do not spin on it
            pluginsd_sleep_backoff(cd);
        }
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

void *pluginsd_main(void *ptr) {
    CLEANUP_FUNCTION_REGISTER(pluginsd_main_cleanup) cleanup_ptr = ptr;

    int automatic_run = inicfg_get_boolean(&netdata_config, CONFIG_SECTION_PLUGINS, "enable running new plugins", 1);
    int scan_frequency = (int)inicfg_get_duration_seconds(&netdata_config, CONFIG_SECTION_PLUGINS, "check for new plugins every", 60);
    if (scan_frequency < 1)
        scan_frequency = 1;

    // disable some plugins by default
    inicfg_get_boolean(&netdata_config, CONFIG_SECTION_PLUGINS, "slabinfo", CONFIG_BOOLEAN_NO);
    // it crashes (both threads) on Alpine after we made it multi-threaded
    // works with "--device /dev/ipmi0", but this is not default
    // see https://github.com/netdata/netdata/pull/15564 for details
    if (getenv("NETDATA_LISTENER_PORT"))
        inicfg_get_boolean(&netdata_config, CONFIG_SECTION_PLUGINS, "freeipmi", CONFIG_BOOLEAN_NO);

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

                    // clamp on the way in, so nothing downstream has to. Both ends were reachable
                    // and both defeated the backoff delays: 0 made every wait return immediately,
                    // and a large value overflowed the arithmetic that derives them.
                    int64_t update_every_cfg = inicfg_get_duration_seconds(&netdata_config, string2str(cd->id), "update every", localhost->rrd_update_every);
                    int64_t update_every_clamped = update_every_cfg;
                    if(update_every_clamped < UPDATE_EVERY_MIN) update_every_clamped = UPDATE_EVERY_MIN;
                    if(update_every_clamped > UPDATE_EVERY_MAX) update_every_clamped = UPDATE_EVERY_MAX;

                    if(unlikely(update_every_clamped != update_every_cfg))
                        nd_log(NDLS_DAEMON, NDLP_WARNING,
                               "PLUGINSD: '%s' has 'update every = %" PRId64 "', which is outside %d..%d - using %" PRId64 ".",
                                          string2str(cd->id), update_every_cfg,
                                          UPDATE_EVERY_MIN, UPDATE_EVERY_MAX, update_every_clamped);

                    cd->update_every = (int)update_every_clamped;
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
