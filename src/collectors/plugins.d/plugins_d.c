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
        plugins_dir_list = strdupz(config_get(CONFIG_SECTION_DIRECTORIES, "plugins", plugins_dirs));
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

static void pluginsd_worker_thread_cleanup(void *pptr) {
    struct plugind *cd = CLEANUP_FUNCTION_GET_PTR(pptr);
    if(!cd) return;

    worker_unregister();

    spinlock_lock(&cd->unsafe.spinlock);

    cd->unsafe.running = false;
    cd->unsafe.thread = 0;

    cd->unsafe.pid = 0;

    POPEN_INSTANCE *pi = cd->unsafe.pi;
    cd->unsafe.pi = NULL;

    spinlock_unlock(&cd->unsafe.spinlock);

    if (pi)
        spawn_popen_kill(pi);
}

#define SERIAL_FAILURES_THRESHOLD 10
static void pluginsd_worker_thread_handle_success(struct plugind *cd) {
    if (likely(cd->successful_collections)) {
        pluginsd_sleep(cd->update_every);
        return;
    }

    if (likely(cd->serial_failures <= SERIAL_FAILURES_THRESHOLD)) {
        netdata_log_info("PLUGINSD: 'host:%s', '%s' (pid %d) does not generate useful output but it reports success (exits with 0). %s.",
             rrdhost_hostname(cd->host), cd->fullfilename, cd->unsafe.pid,
             plugin_is_enabled(cd) ? "Waiting a bit before starting it again." : "Will not start it again - it is now disabled.");

        pluginsd_sleep(cd->update_every * 10);
        return;
    }

    if (cd->serial_failures > SERIAL_FAILURES_THRESHOLD) {
        netdata_log_error("PLUGINSD: 'host:'%s', '%s' (pid %d) does not generate useful output, "
              "although it reports success (exits with 0)."
              "We have tried to collect something %zu times - unsuccessfully. Disabling it.",
              rrdhost_hostname(cd->host), cd->fullfilename, cd->unsafe.pid, cd->serial_failures);
        plugin_set_disabled(cd);
        return;
    }
}

static void pluginsd_worker_thread_handle_error(struct plugind *cd, int worker_ret_code) {
    if (worker_ret_code == -1) {
        netdata_log_info("PLUGINSD: 'host:%s', '%s' (pid %d) was killed with SIGTERM. Disabling it.",
             rrdhost_hostname(cd->host), cd->fullfilename, cd->unsafe.pid);
        plugin_set_disabled(cd);
        return;
    }

    if (!cd->successful_collections) {
        netdata_log_error("PLUGINSD: 'host:%s', '%s' (pid %d) exited with error code %d and haven't collected any data. Disabling it.",
              rrdhost_hostname(cd->host), cd->fullfilename, cd->unsafe.pid, worker_ret_code);
        plugin_set_disabled(cd);
        return;
    }

    if (cd->serial_failures <= SERIAL_FAILURES_THRESHOLD) {
        netdata_log_error("PLUGINSD: 'host:%s', '%s' (pid %d) exited with error code %d, but has given useful output in the past (%zu times). %s",
              rrdhost_hostname(cd->host), cd->fullfilename, cd->unsafe.pid, worker_ret_code, cd->successful_collections,
              plugin_is_enabled(cd) ? "Waiting a bit before starting it again." : "Will not start it again - it is disabled.");

        pluginsd_sleep(cd->update_every * 10);
        return;
    }

    if (cd->serial_failures > SERIAL_FAILURES_THRESHOLD) {
        netdata_log_error("PLUGINSD: 'host:%s', '%s' (pid %d) exited with error code %d, but has given useful output in the past (%zu times)."
              "We tried to restart it %zu times, but it failed to generate data. Disabling it.",
              rrdhost_hostname(cd->host), cd->fullfilename, cd->unsafe.pid, worker_ret_code,
              cd->successful_collections, cd->serial_failures);
        plugin_set_disabled(cd);
        return;
    }
}

#undef SERIAL_FAILURES_THRESHOLD

static void *pluginsd_worker_thread(void *arg) {
    struct plugind *cd = (struct plugind *) arg;
    CLEANUP_FUNCTION_REGISTER(pluginsd_worker_thread_cleanup) cleanup_ptr = cd;

    worker_register("PLUGINSD");

    plugin_set_running(cd);

    size_t count = 0;

    while(service_running(SERVICE_COLLECTORS)) {
        cd->unsafe.pi = spawn_popen_run(cd->cmd);
        if(!cd->unsafe.pi) {
            netdata_log_error("PLUGINSD: 'host:%s', cannot popen(\"%s\", \"r\").",
                              rrdhost_hostname(cd->host), cd->cmd);
            break;
        }
        cd->unsafe.pid = spawn_server_instance_pid(cd->unsafe.pi->si);

        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "PLUGINSD: 'host:%s' connected to '%s' running on pid %d",
               rrdhost_hostname(cd->host),
               cd->fullfilename, cd->unsafe.pid);

        const char *plugin = strrchr(cd->fullfilename, '/');
        if(plugin)
            plugin++;
        else
            plugin = cd->fullfilename;

        char module[100];
        snprintfz(module, sizeof(module), "plugins.d[%s]", plugin);
        ND_LOG_STACK lgs[] = {
                ND_LOG_FIELD_TXT(NDF_MODULE, module),
                ND_LOG_FIELD_TXT(NDF_NIDL_NODE, rrdhost_hostname(cd->host)),
                ND_LOG_FIELD_TXT(NDF_SRC_TRANSPORT, "pluginsd"),
                ND_LOG_FIELD_END(),
        };
        ND_LOG_STACK_PUSH(lgs);

        count = pluginsd_process(cd->host, cd, cd->unsafe.pi->child_stdin_fp, cd->unsafe.pi->child_stdout_fp, 0);

        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "PLUGINSD: 'host:%s', '%s' (pid %d) disconnected after %zu successful data collections (ENDs).",
               rrdhost_hostname(cd->host), cd->fullfilename, cd->unsafe.pid, count);

        int worker_ret_code = spawn_popen_kill(cd->unsafe.pi);
        cd->unsafe.pi = NULL;

        if(likely(worker_ret_code == 0))
            pluginsd_worker_thread_handle_success(cd);
        else
            pluginsd_worker_thread_handle_error(cd, worker_ret_code);

        cd->unsafe.pid = 0;

        if(unlikely(!plugin_is_enabled(cd)))
            break;
    }
    return NULL;
}

static void pluginsd_main_cleanup(void *pptr) {
    struct netdata_static_thread *static_thread = CLEANUP_FUNCTION_GET_PTR(pptr);
    if(!static_thread) return;

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;
    netdata_log_info("PLUGINSD: cleaning up...");

    struct plugind *cd;
    for (cd = pluginsd_root; cd; cd = cd->next) {
        spinlock_lock(&cd->unsafe.spinlock);
        if (cd->unsafe.enabled && cd->unsafe.running && cd->unsafe.thread != 0) {
            netdata_log_info("PLUGINSD: 'host:%s', stopping plugin thread: %s",
                 rrdhost_hostname(cd->host), cd->id);

            nd_thread_signal_cancel(cd->unsafe.thread);
        }
        spinlock_unlock(&cd->unsafe.spinlock);
    }

    netdata_log_info("PLUGINSD: cleanup completed.");
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;

    worker_unregister();
}

void *pluginsd_main(void *ptr) {
    CLEANUP_FUNCTION_REGISTER(pluginsd_main_cleanup) cleanup_ptr = ptr;

    int automatic_run = config_get_boolean(CONFIG_SECTION_PLUGINS, "enable running new plugins", 1);
    int scan_frequency = (int)config_get_number(CONFIG_SECTION_PLUGINS, "check for new plugins every", 60);
    if (scan_frequency < 1)
        scan_frequency = 1;

    // disable some plugins by default
    config_get_boolean(CONFIG_SECTION_PLUGINS, "slabinfo", CONFIG_BOOLEAN_NO);
    // it crashes (both threads) on Alpine after we made it multi-threaded
    // works with "--device /dev/ipmi0", but this is not default
    // see https://github.com/netdata/netdata/pull/15564 for details
    if (getenv("NETDATA_LISTENER_PORT"))
        config_get_boolean(CONFIG_SECTION_PLUGINS, "freeipmi", CONFIG_BOOLEAN_NO);

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

                int len = (int)strlen(file->d_name);
                if (unlikely(len <= (int)PLUGINSD_FILE_SUFFIX_LEN))
                    continue;
                if (unlikely(strcmp(PLUGINSD_FILE_SUFFIX, &file->d_name[len - (int)PLUGINSD_FILE_SUFFIX_LEN]) != 0)) {
                    netdata_log_debug(D_PLUGINSD, "file '%s' does not end in '%s'", file->d_name, PLUGINSD_FILE_SUFFIX);
                    continue;
                }

                char pluginname[CONFIG_MAX_NAME + 1];
                snprintfz(pluginname, CONFIG_MAX_NAME, "%.*s", (int)(len - PLUGINSD_FILE_SUFFIX_LEN), file->d_name);
                int enabled = config_get_boolean(CONFIG_SECTION_PLUGINS, pluginname, automatic_run);

                if (unlikely(!enabled)) {
                    netdata_log_debug(D_PLUGINSD, "plugin '%s' is not enabled", file->d_name);
                    continue;
                }

                // check if it runs already
                struct plugind *cd;
                for (cd = pluginsd_root; cd; cd = cd->next)
                    if (unlikely(strcmp(cd->filename, file->d_name) == 0))
                        break;

                if (likely(cd && plugin_is_running(cd))) {
                    netdata_log_debug(D_PLUGINSD, "plugin '%s' is already running", cd->filename);
                    continue;
                }

                // it is not running
                // allocate a new one, or use the obsolete one
                if (unlikely(!cd)) {
                    cd = callocz(sizeof(struct plugind), 1);

                    snprintfz(cd->id, CONFIG_MAX_NAME, "plugin:%s", pluginname);

                    strncpyz(cd->filename, file->d_name, FILENAME_MAX);
                    snprintfz(cd->fullfilename, FILENAME_MAX, "%s/%s", directory_name, cd->filename);

                    cd->host = localhost;
                    cd->unsafe.enabled = enabled;
                    cd->unsafe.running = false;

                    cd->update_every = (int)config_get_number(cd->id, "update every", localhost->rrd_update_every);
                    cd->started_t = now_realtime_sec();

                    char *def = "";
                    snprintfz(
                        cd->cmd, PLUGINSD_CMD_MAX, "exec %s %d %s", cd->fullfilename, cd->update_every,
                        config_get(cd->id, "command options", def));

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

    return NULL;
}
