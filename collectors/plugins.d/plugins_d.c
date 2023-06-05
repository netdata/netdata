// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugins_d.h"
#include "pluginsd_parser.h"

char *plugin_directories[PLUGINSD_MAX_DIRECTORIES] = { NULL };
struct plugind *pluginsd_root = NULL;

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
    return quoted_strings_splitter(plugins_dir_list, plugin_directories, PLUGINSD_MAX_DIRECTORIES, config_isspace);
}

static inline void plugin_set_disabled(struct plugind *cd) {
    netdata_spinlock_lock(&cd->unsafe.spinlock);
    cd->unsafe.enabled = false;
    netdata_spinlock_unlock(&cd->unsafe.spinlock);
}

bool plugin_is_enabled(struct plugind *cd) {
    netdata_spinlock_lock(&cd->unsafe.spinlock);
    bool ret = cd->unsafe.enabled;
    netdata_spinlock_unlock(&cd->unsafe.spinlock);
    return ret;
}

static inline void plugin_set_running(struct plugind *cd) {
    netdata_spinlock_lock(&cd->unsafe.spinlock);
    cd->unsafe.running = true;
    netdata_spinlock_unlock(&cd->unsafe.spinlock);
}

static inline bool plugin_is_running(struct plugind *cd) {
    netdata_spinlock_lock(&cd->unsafe.spinlock);
    bool ret = cd->unsafe.running;
    netdata_spinlock_unlock(&cd->unsafe.spinlock);
    return ret;
}

static void pluginsd_worker_thread_cleanup(void *arg)
{
    struct plugind *cd = (struct plugind *)arg;

    worker_unregister();

    netdata_spinlock_lock(&cd->unsafe.spinlock);

    cd->unsafe.running = false;
    cd->unsafe.thread = 0;

    pid_t pid = cd->unsafe.pid;
    cd->unsafe.pid = 0;

    netdata_spinlock_unlock(&cd->unsafe.spinlock);

    if (pid) {
        siginfo_t info;
        info("PLUGINSD: 'host:%s', killing data collection child process with pid %d",
             rrdhost_hostname(cd->host), pid);

        if (killpid(pid) != -1) {
            info("PLUGINSD: 'host:%s', waiting for data collection child process pid %d to exit...",
                 rrdhost_hostname(cd->host), pid);

            netdata_waitid(P_PID, (id_t)pid, &info, WEXITED);
        }
    }
}

#define SERIAL_FAILURES_THRESHOLD 10
static void pluginsd_worker_thread_handle_success(struct plugind *cd) {
    if (likely(cd->successful_collections)) {
        sleep((unsigned int)cd->update_every);
        return;
    }

    if (likely(cd->serial_failures <= SERIAL_FAILURES_THRESHOLD)) {
        info("PLUGINSD: 'host:%s', '%s' (pid %d) does not generate useful output but it reports success (exits with 0). %s.",
             rrdhost_hostname(cd->host), cd->fullfilename, cd->unsafe.pid,
             plugin_is_enabled(cd) ? "Waiting a bit before starting it again." : "Will not start it again - it is now disabled.");

        sleep((unsigned int)(cd->update_every * 10));
        return;
    }

    if (cd->serial_failures > SERIAL_FAILURES_THRESHOLD) {
        error("PLUGINSD: 'host:'%s', '%s' (pid %d) does not generate useful output, "
              "although it reports success (exits with 0)."
              "We have tried to collect something %zu times - unsuccessfully. Disabling it.",
              rrdhost_hostname(cd->host), cd->fullfilename, cd->unsafe.pid, cd->serial_failures);
        plugin_set_disabled(cd);
        return;
    }
}

static void pluginsd_worker_thread_handle_error(struct plugind *cd, int worker_ret_code) {
    if (worker_ret_code == -1) {
        info("PLUGINSD: 'host:%s', '%s' (pid %d) was killed with SIGTERM. Disabling it.",
             rrdhost_hostname(cd->host), cd->fullfilename, cd->unsafe.pid);
        plugin_set_disabled(cd);
        return;
    }

    if (!cd->successful_collections) {
        error("PLUGINSD: 'host:%s', '%s' (pid %d) exited with error code %d and haven't collected any data. Disabling it.",
              rrdhost_hostname(cd->host), cd->fullfilename, cd->unsafe.pid, worker_ret_code);
        plugin_set_disabled(cd);
        return;
    }

    if (cd->serial_failures <= SERIAL_FAILURES_THRESHOLD) {
        error("PLUGINSD: 'host:%s', '%s' (pid %d) exited with error code %d, but has given useful output in the past (%zu times). %s",
              rrdhost_hostname(cd->host), cd->fullfilename, cd->unsafe.pid, worker_ret_code, cd->successful_collections,
              plugin_is_enabled(cd) ? "Waiting a bit before starting it again." : "Will not start it again - it is disabled.");
        sleep((unsigned int)(cd->update_every * 10));
        return;
    }

    if (cd->serial_failures > SERIAL_FAILURES_THRESHOLD) {
        error("PLUGINSD: 'host:%s', '%s' (pid %d) exited with error code %d, but has given useful output in the past (%zu times)."
              "We tried to restart it %zu times, but it failed to generate data. Disabling it.",
              rrdhost_hostname(cd->host), cd->fullfilename, cd->unsafe.pid, worker_ret_code,
              cd->successful_collections, cd->serial_failures);
        plugin_set_disabled(cd);
        return;
    }
}

#undef SERIAL_FAILURES_THRESHOLD

static void *pluginsd_worker_thread(void *arg) {
    worker_register("PLUGINSD");

    netdata_thread_cleanup_push(pluginsd_worker_thread_cleanup, arg);

    struct plugind *cd = (struct plugind *)arg;
    plugin_set_running(cd);

    size_t count = 0;

    while (service_running(SERVICE_COLLECTORS)) {
        FILE *fp_child_input = NULL;
        FILE *fp_child_output = netdata_popen(cd->cmd, &cd->unsafe.pid, &fp_child_input);

        if (unlikely(!fp_child_input || !fp_child_output)) {
            error("PLUGINSD: 'host:%s', cannot popen(\"%s\", \"r\").", rrdhost_hostname(cd->host), cd->cmd);
            break;
        }

        info("PLUGINSD: 'host:%s' connected to '%s' running on pid %d",
             rrdhost_hostname(cd->host), cd->fullfilename, cd->unsafe.pid);

        count = pluginsd_process(cd->host, cd, fp_child_input, fp_child_output, 0);

        info("PLUGINSD: 'host:%s', '%s' (pid %d) disconnected after %zu successful data collections (ENDs).",
              rrdhost_hostname(cd->host), cd->fullfilename, cd->unsafe.pid, count);

        killpid(cd->unsafe.pid);

        int worker_ret_code = netdata_pclose(fp_child_input, fp_child_output, cd->unsafe.pid);

        if (likely(worker_ret_code == 0))
            pluginsd_worker_thread_handle_success(cd);
        else
            pluginsd_worker_thread_handle_error(cd, worker_ret_code);

        cd->unsafe.pid = 0;
        if (unlikely(!plugin_is_enabled(cd)))
            break;
    }

    netdata_thread_cleanup_pop(1);
    return NULL;
}

static void pluginsd_main_cleanup(void *data) {
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)data;
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;
    info("PLUGINSD: cleaning up...");

    struct plugind *cd;
    for (cd = pluginsd_root; cd; cd = cd->next) {
        netdata_spinlock_lock(&cd->unsafe.spinlock);
        if (cd->unsafe.enabled && cd->unsafe.running && cd->unsafe.thread != 0) {
            info("PLUGINSD: 'host:%s', stopping plugin thread: %s",
                 rrdhost_hostname(cd->host), cd->id);

            netdata_thread_cancel(cd->unsafe.thread);
        }
        netdata_spinlock_unlock(&cd->unsafe.spinlock);
    }

    info("PLUGINSD: cleanup completed.");
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;

    worker_unregister();
}

void *pluginsd_main(void *ptr)
{
    netdata_thread_cleanup_push(pluginsd_main_cleanup, ptr);

    int automatic_run = config_get_boolean(CONFIG_SECTION_PLUGINS, "enable running new plugins", 1);
    int scan_frequency = (int)config_get_number(CONFIG_SECTION_PLUGINS, "check for new plugins every", 60);
    if (scan_frequency < 1)
        scan_frequency = 1;

    // disable some plugins by default
    config_get_boolean(CONFIG_SECTION_PLUGINS, "slabinfo", CONFIG_BOOLEAN_NO);

    // store the errno for each plugins directory
    // so that we don't log broken directories on each loop
    int directory_errors[PLUGINSD_MAX_DIRECTORIES] = { 0 };

    while (service_running(SERVICE_COLLECTORS)) {
        int idx;
        const char *directory_name;

        for (idx = 0; idx < PLUGINSD_MAX_DIRECTORIES && (directory_name = plugin_directories[idx]); idx++) {
            if (unlikely(!service_running(SERVICE_COLLECTORS)))
                break;

            errno = 0;
            DIR *dir = opendir(directory_name);
            if (unlikely(!dir)) {
                if (directory_errors[idx] != errno) {
                    directory_errors[idx] = errno;
                    error("cannot open plugins directory '%s'", directory_name);
                }
                continue;
            }

            struct dirent *file = NULL;
            while (likely((file = readdir(dir)))) {
                if (unlikely(!service_running(SERVICE_COLLECTORS)))
                    break;

                debug(D_PLUGINSD, "examining file '%s'", file->d_name);

                if (unlikely(strcmp(file->d_name, ".") == 0 || strcmp(file->d_name, "..") == 0))
                    continue;

                int len = (int)strlen(file->d_name);
                if (unlikely(len <= (int)PLUGINSD_FILE_SUFFIX_LEN))
                    continue;
                if (unlikely(strcmp(PLUGINSD_FILE_SUFFIX, &file->d_name[len - (int)PLUGINSD_FILE_SUFFIX_LEN]) != 0)) {
                    debug(D_PLUGINSD, "file '%s' does not end in '%s'", file->d_name, PLUGINSD_FILE_SUFFIX);
                    continue;
                }

                char pluginname[CONFIG_MAX_NAME + 1];
                snprintfz(pluginname, CONFIG_MAX_NAME, "%.*s", (int)(len - PLUGINSD_FILE_SUFFIX_LEN), file->d_name);
                int enabled = config_get_boolean(CONFIG_SECTION_PLUGINS, pluginname, automatic_run);

                if (unlikely(!enabled)) {
                    debug(D_PLUGINSD, "plugin '%s' is not enabled", file->d_name);
                    continue;
                }

                // check if it runs already
                struct plugind *cd;
                for (cd = pluginsd_root; cd; cd = cd->next)
                    if (unlikely(strcmp(cd->filename, file->d_name) == 0))
                        break;

                if (likely(cd && plugin_is_running(cd))) {
                    debug(D_PLUGINSD, "plugin '%s' is already running", cd->filename);
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
                        netdata_thread_create(&cd->unsafe.thread,
                                              tag,
                                              NETDATA_THREAD_OPTION_DEFAULT,
                                              pluginsd_worker_thread,
                                              cd);
                    }
                }
            }

            closedir(dir);
        }

        sleep((unsigned int)scan_frequency);
    }

    netdata_thread_cleanup_pop(1);
    return NULL;
}
