// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugins_d.h"
#include "pluginsd_parser.h"

char *plugin_directories[PLUGINSD_MAX_DIRECTORIES] = { NULL };
struct plugind *pluginsd_root = NULL;

inline int pluginsd_space(char c) {
    switch(c) {
    case ' ':
    case '\t':
    case '\r':
    case '\n':
    case '=':
        return 1;

    default:
        return 0;
    }
}

inline int config_isspace(char c)
{
    switch (c) {
        case ' ':
        case '\t':
        case '\r':
        case '\n':
        case ',':
            return 1;

        default:
            return 0;
    }
}

// split a text into words, respecting quotes
static inline int quoted_strings_splitter(char *str, char **words, int max_words, int (*custom_isspace)(char), char *recover_input, char **recover_location, int max_recover)
{
    char *s = str, quote = 0;
    int i = 0, j, rec = 0;
    char *recover = recover_input;

    // skip all white space
    while (unlikely(custom_isspace(*s)))
        s++;

    // check for quote
    if (unlikely(*s == '\'' || *s == '"')) {
        quote = *s; // remember the quote
        s++;        // skip the quote
    }

    // store the first word
    words[i++] = s;

    // while we have something
    while (likely(*s)) {
        // if it is escape
        if (unlikely(*s == '\\' && s[1])) {
            s += 2;
            continue;
        }

        // if it is quote
        else if (unlikely(*s == quote)) {
            quote = 0;
            if (recover && rec < max_recover) {
                recover_location[rec++] = s;
                *recover++ = *s;
            }
            *s = ' ';
            continue;
        }

        // if it is a space
        else if (unlikely(quote == 0 && custom_isspace(*s))) {
            // terminate the word
            if (recover && rec < max_recover) {
                if (!rec || (rec && recover_location[rec-1] != s)) {
                    recover_location[rec++] = s;
                    *recover++ = *s;
                }
            }
            *s++ = '\0';

            // skip all white space
            while (likely(custom_isspace(*s)))
                s++;

            // check for quote
            if (unlikely(*s == '\'' || *s == '"')) {
                quote = *s; // remember the quote
                s++;        // skip the quote
            }

            // if we reached the end, stop
            if (unlikely(!*s))
                break;

            // store the next word
            if (likely(i < max_words))
                words[i++] = s;
            else
                break;
        }

        // anything else
        else
            s++;
    }

    // terminate the words
    j = i;
    while (likely(j < max_words))
        words[j++] = NULL;

    return i;
}

inline int pluginsd_initialize_plugin_directories()
{
    char plugins_dirs[(FILENAME_MAX * 2) + 1];
    static char *plugins_dir_list = NULL;

    // Get the configuration entry
    if (likely(!plugins_dir_list)) {
        snprintfz(plugins_dirs, FILENAME_MAX * 2, "\"%s\" \"%s/custom-plugins.d\"", PLUGINS_DIR, CONFIG_DIR);
        plugins_dir_list = strdupz(config_get(CONFIG_SECTION_GLOBAL, "plugins directory", plugins_dirs));
    }

    // Parse it and store it to plugin directories
    return quoted_strings_splitter(plugins_dir_list, plugin_directories, PLUGINSD_MAX_DIRECTORIES, config_isspace, NULL, NULL, 0);
}

inline int pluginsd_split_words(char *str, char **words, int max_words, char *recover_input, char **recover_location, int max_recover)
{
    return quoted_strings_splitter(str, words, max_words, pluginsd_space, recover_input, recover_location, max_recover);
}


static void pluginsd_worker_thread_cleanup(void *arg)
{
    struct plugind *cd = (struct plugind *)arg;

    if (cd->enabled && !cd->obsolete) {
        cd->obsolete = 1;

        info("data collection thread exiting");

        if (cd->pid) {
            siginfo_t info;
            info("killing child process pid %d", cd->pid);
            if (killpid(cd->pid) != -1) {
                info("waiting for child process pid %d to exit...", cd->pid);
                waitid(P_PID, (id_t)cd->pid, &info, WEXITED);
            }
            cd->pid = 0;
        }
    }
}

#define SERIAL_FAILURES_THRESHOLD 10
static void pluginsd_worker_thread_handle_success(struct plugind *cd)
{
    if (likely(cd->successful_collections)) {
        sleep((unsigned int)cd->update_every);
        return;
    }

    if (likely(cd->serial_failures <= SERIAL_FAILURES_THRESHOLD)) {
        info(
            "'%s' (pid %d) does not generate useful output but it reports success (exits with 0). %s.",
            cd->fullfilename, cd->pid,
            cd->enabled ? "Waiting a bit before starting it again." : "Will not start it again - it is now disabled.");
        sleep((unsigned int)(cd->update_every * 10));
        return;
    }

    if (cd->serial_failures > SERIAL_FAILURES_THRESHOLD) {
        error(
            "'%s' (pid %d) does not generate useful output, although it reports success (exits with 0)."
            "We have tried to collect something %zu times - unsuccessfully. Disabling it.",
            cd->fullfilename, cd->pid, cd->serial_failures);
        cd->enabled = 0;
        return;
    }

    return;
}

static void pluginsd_worker_thread_handle_error(struct plugind *cd, int worker_ret_code)
{
    if (worker_ret_code == -1) {
        info("'%s' (pid %d) was killed with SIGTERM. Disabling it.", cd->fullfilename, cd->pid);
        cd->enabled = 0;
        return;
    }

    if (!cd->successful_collections) {
        error(
            "'%s' (pid %d) exited with error code %d and haven't collected any data. Disabling it.", cd->fullfilename,
            cd->pid, worker_ret_code);
        cd->enabled = 0;
        return;
    }

    if (cd->serial_failures <= SERIAL_FAILURES_THRESHOLD) {
        error(
            "'%s' (pid %d) exited with error code %d, but has given useful output in the past (%zu times). %s",
            cd->fullfilename, cd->pid, worker_ret_code, cd->successful_collections,
            cd->enabled ? "Waiting a bit before starting it again." : "Will not start it again - it is disabled.");
        sleep((unsigned int)(cd->update_every * 10));
        return;
    }

    if (cd->serial_failures > SERIAL_FAILURES_THRESHOLD) {
        error(
            "'%s' (pid %d) exited with error code %d, but has given useful output in the past (%zu times)."
            "We tried to restart it %zu times, but it failed to generate data. Disabling it.",
            cd->fullfilename, cd->pid, worker_ret_code, cd->successful_collections, cd->serial_failures);
        cd->enabled = 0;
        return;
    }

    return;
}
#undef SERIAL_FAILURES_THRESHOLD

void *pluginsd_worker_thread(void *arg)
{
    netdata_thread_cleanup_push(pluginsd_worker_thread_cleanup, arg);

    struct plugind *cd = (struct plugind *)arg;

    cd->obsolete = 0;
    size_t count = 0;

    while (!netdata_exit) {
        FILE *fp = mypopen(cd->cmd, &cd->pid);
        if (unlikely(!fp)) {
            error("Cannot popen(\"%s\", \"r\").", cd->cmd);
            break;
        }

        info("connected to '%s' running on pid %d", cd->fullfilename, cd->pid);
        count = pluginsd_process(localhost, cd, fp, 0);
        error("'%s' (pid %d) disconnected after %zu successful data collections (ENDs).", cd->fullfilename, cd->pid, count);
        killpid(cd->pid);

        int worker_ret_code = mypclose(fp, cd->pid);

        if (likely(worker_ret_code == 0))
            pluginsd_worker_thread_handle_success(cd);
        else
            pluginsd_worker_thread_handle_error(cd, worker_ret_code);

        cd->pid = 0;
        if (unlikely(!cd->enabled))
            break;
    }

    netdata_thread_cleanup_pop(1);
    return NULL;
}

static void pluginsd_main_cleanup(void *data)
{
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)data;
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;
    info("cleaning up...");

    struct plugind *cd;
    for (cd = pluginsd_root; cd; cd = cd->next) {
        if (cd->enabled && !cd->obsolete) {
            info("stopping plugin thread: %s", cd->id);
            netdata_thread_cancel(cd->thread);
        }
    }

    info("cleanup completed.");
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
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

    while (!netdata_exit) {
        int idx;
        const char *directory_name;

        for (idx = 0; idx < PLUGINSD_MAX_DIRECTORIES && (directory_name = plugin_directories[idx]); idx++) {
            if (unlikely(netdata_exit))
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
                if (unlikely(netdata_exit))
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

                if (likely(cd && !cd->obsolete)) {
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

                    cd->enabled = enabled;
                    cd->update_every = (int)config_get_number(cd->id, "update every", localhost->rrd_update_every);
                    cd->started_t = now_realtime_sec();

                    char *def = "";
                    snprintfz(
                        cd->cmd, PLUGINSD_CMD_MAX, "exec %s %d %s", cd->fullfilename, cd->update_every,
                        config_get(cd->id, "command options", def));

                    // link it
                    if (likely(pluginsd_root))
                        cd->next = pluginsd_root;
                    pluginsd_root = cd;

                    // it is not currently running
                    cd->obsolete = 1;

                    if (cd->enabled) {
                        char tag[NETDATA_THREAD_TAG_MAX + 1];
                        snprintfz(tag, NETDATA_THREAD_TAG_MAX, "PLUGINSD[%s]", pluginname);
                        // spawn a new thread for it
                        netdata_thread_create(
                            &cd->thread, tag, NETDATA_THREAD_OPTION_DEFAULT, pluginsd_worker_thread, cd);
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
