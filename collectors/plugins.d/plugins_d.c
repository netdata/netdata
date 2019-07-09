// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugins_d.h"

char *plugin_directories[PLUGINSD_MAX_DIRECTORIES] = { NULL };
struct plugind *pluginsd_root = NULL;

static inline int pluginsd_space(char c) {
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

inline int config_isspace(char c) {
    switch(c) {
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
static inline int quoted_strings_splitter(char *str, char **words, int max_words, int (*custom_isspace)(char)) {
    char *s = str, quote = 0;
    int i = 0, j;

    // skip all white space
    while(unlikely(custom_isspace(*s))) s++;

    // check for quote
    if(unlikely(*s == '\'' || *s == '"')) {
        quote = *s; // remember the quote
        s++;        // skip the quote
    }

    // store the first word
    words[i++] = s;

    // while we have something
    while(likely(*s)) {
        // if it is escape
        if(unlikely(*s == '\\' && s[1])) {
            s += 2;
            continue;
        }

        // if it is quote
        else if(unlikely(*s == quote)) {
            quote = 0;
            *s = ' ';
            continue;
        }

        // if it is a space
        else if(unlikely(quote == 0 && custom_isspace(*s))) {

            // terminate the word
            *s++ = '\0';

            // skip all white space
            while(likely(custom_isspace(*s))) s++;

            // check for quote
            if(unlikely(*s == '\'' || *s == '"')) {
                quote = *s; // remember the quote
                s++;        // skip the quote
            }

            // if we reached the end, stop
            if(unlikely(!*s)) break;

            // store the next word
            if(likely(i < max_words)) words[i++] = s;
            else break;
        }

        // anything else
        else s++;
    }

    // terminate the words
    j = i;
    while(likely(j < max_words)) words[j++] = NULL;

    return i;
}

inline int pluginsd_initialize_plugin_directories() {
    char plugins_dirs[(FILENAME_MAX * 2) + 1];
    static char *plugins_dir_list = NULL;

    // Get the configuration entry
    if(likely(!plugins_dir_list)) {
        snprintfz(plugins_dirs, FILENAME_MAX * 2, "\"%s\" \"%s/custom-plugins.d\"", PLUGINS_DIR, CONFIG_DIR);
        plugins_dir_list = strdupz(config_get(CONFIG_SECTION_GLOBAL, "plugins directory",  plugins_dirs));
    }

    // Parse it and store it to plugin directories
    return quoted_strings_splitter(plugins_dir_list, plugin_directories, PLUGINSD_MAX_DIRECTORIES, config_isspace);
}

inline int pluginsd_split_words(char *str, char **words, int max_words) {
    return quoted_strings_splitter(str, words, max_words, pluginsd_space);
}

inline size_t pluginsd_process(RRDHOST *host, struct plugind *cd, FILE *fp, int trust_durations) {
    int enabled = cd->enabled;

    if(!fp || !enabled) {
        cd->enabled = 0;
        return 0;
    }

    size_t count = 0;

    char line[PLUGINSD_LINE_MAX + 1];

    char *words[PLUGINSD_MAX_WORDS] = { NULL };
    uint32_t BEGIN_HASH = simple_hash(PLUGINSD_KEYWORD_BEGIN);
    uint32_t END_HASH = simple_hash(PLUGINSD_KEYWORD_END);
    uint32_t FLUSH_HASH = simple_hash(PLUGINSD_KEYWORD_FLUSH);
    uint32_t CHART_HASH = simple_hash(PLUGINSD_KEYWORD_CHART);
    uint32_t DIMENSION_HASH = simple_hash(PLUGINSD_KEYWORD_DIMENSION);
    uint32_t DISABLE_HASH = simple_hash(PLUGINSD_KEYWORD_DISABLE);
    uint32_t VARIABLE_HASH = simple_hash(PLUGINSD_KEYWORD_VARIABLE);

    RRDSET *st = NULL;
    uint32_t hash;

    errno = 0;
    clearerr(fp);

    if(unlikely(fileno(fp) == -1)) {
        error("file descriptor given is not a valid stream");
        goto cleanup;
    }

    while(!ferror(fp)) {
        if(unlikely(netdata_exit)) break;

        char *r = fgets(line, PLUGINSD_LINE_MAX, fp);
        if(unlikely(!r)) {
            if(feof(fp))
                error("read failed: end of file");
            else if(ferror(fp))
                error("read failed: input error");
            else
                error("read failed: unknown error");
            break;
        }

        if(unlikely(netdata_exit)) break;

        line[PLUGINSD_LINE_MAX] = '\0';

        int w = pluginsd_split_words(line, words, PLUGINSD_MAX_WORDS);
        char *s = words[0];
        if(unlikely(!s || !*s || !w)) {
            continue;
        }

        // debug(D_PLUGINSD, "PLUGINSD: words 0='%s' 1='%s' 2='%s' 3='%s' 4='%s' 5='%s' 6='%s' 7='%s' 8='%s' 9='%s'", words[0], words[1], words[2], words[3], words[4], words[5], words[6], words[7], words[8], words[9]);

        if(likely(!simple_hash_strcmp(s, "SET", &hash))) {
            char *dimension = words[1];
            char *value = words[2];

            if(unlikely(!dimension || !*dimension)) {
                error("requested a SET on chart '%s' of host '%s', without a dimension. Disabling it.", st->id, host->hostname);
                enabled = 0;
                break;
            }

            if(unlikely(!value || !*value)) value = NULL;

            if(unlikely(!st)) {
                error("requested a SET on dimension %s with value %s on host '%s', without a BEGIN. Disabling it.", dimension, value?value:"<nothing>", host->hostname);
                enabled = 0;
                break;
            }

            if(unlikely(rrdset_flag_check(st, RRDSET_FLAG_DEBUG)))
                debug(D_PLUGINSD, "is setting dimension %s/%s to %s", st->id, dimension, value?value:"<nothing>");

            if(value) {
                RRDDIM *rd = rrddim_find(st, dimension);
                if(unlikely(!rd)) {
                    error("requested a SET to dimension with id '%s' on stats '%s' (%s) on host '%s', which does not exist. Disabling it.", dimension, st->name, st->id, st->rrdhost->hostname);
                    enabled = 0;
                    break;
                }
                else
                    rrddim_set_by_pointer(st, rd, strtoll(value, NULL, 0));
            }
        }
        else if(likely(hash == BEGIN_HASH && !strcmp(s, PLUGINSD_KEYWORD_BEGIN))) {
            char *id = words[1];
            char *microseconds_txt = words[2];

            if(unlikely(!id)) {
                error("requested a BEGIN without a chart id for host '%s'. Disabling it.", host->hostname);
                enabled = 0;
                break;
            }

            st = rrdset_find(host, id);
            if(unlikely(!st)) {
                error("requested a BEGIN on chart '%s', which does not exist on host '%s'. Disabling it.", id, host->hostname);
                enabled = 0;
                break;
            }

            if(likely(st->counter_done)) {
                usec_t microseconds = 0;
                if(microseconds_txt && *microseconds_txt) microseconds = str2ull(microseconds_txt);

                if(likely(microseconds)) {
                    if(trust_durations)
                        rrdset_next_usec_unfiltered(st, microseconds);
                    else
                        rrdset_next_usec(st, microseconds);
                }
                else rrdset_next(st);
            }
        }
        else if(likely(hash == END_HASH && !strcmp(s, PLUGINSD_KEYWORD_END))) {
            if(unlikely(!st)) {
                error("requested an END, without a BEGIN on host '%s'. Disabling it.", host->hostname);
                enabled = 0;
                break;
            }

            if(unlikely(rrdset_flag_check(st, RRDSET_FLAG_DEBUG)))
                debug(D_PLUGINSD, "requested an END on chart %s", st->id);

            rrdset_done(st);
            st = NULL;

            count++;
        }
        else if(likely(hash == CHART_HASH && !strcmp(s, PLUGINSD_KEYWORD_CHART))) {
            st = NULL;

            char *type           = words[1];
            char *name           = words[2];
            char *title          = words[3];
            char *units          = words[4];
            char *family         = words[5];
            char *context        = words[6];
            char *chart          = words[7];
            char *priority_s     = words[8];
            char *update_every_s = words[9];
            char *options        = words[10];
            char *plugin         = words[11];
            char *module         = words[12];

            // parse the id from type
            char *id = NULL;
            if(likely(type && (id = strchr(type, '.')))) {
                *id = '\0';
                id++;
            }

            // make sure we have the required variables
            if(unlikely(!type || !*type || !id || !*id)) {
                error("requested a CHART, without a type.id, on host '%s'. Disabling it.", host->hostname);
                enabled = 0;
                break;
            }

            // parse the name, and make sure it does not include 'type.'
            if(unlikely(name && *name)) {
                // when data are coming from slaves
                // name will be type.name
                // so we have to remove 'type.' from name too
                size_t len = strlen(type);
                if(strncmp(type, name, len) == 0 && name[len] == '.')
                    name = &name[len + 1];

                // if the name is the same with the id,
                // or is just 'NULL', clear it.
                if(unlikely(strcmp(name, id) == 0 || strcasecmp(name, "NULL") == 0 || strcasecmp(name, "(NULL)") == 0))
                    name = NULL;
            }

            int priority = 1000;
            if(likely(priority_s && *priority_s)) priority = str2i(priority_s);

            int update_every = cd->update_every;
            if(likely(update_every_s && *update_every_s)) update_every = str2i(update_every_s);
            if(unlikely(!update_every)) update_every = cd->update_every;

            RRDSET_TYPE chart_type = RRDSET_TYPE_LINE;
            if(unlikely(chart)) chart_type = rrdset_type_id(chart);

            if(unlikely(name && !*name)) name = NULL;
            if(unlikely(family && !*family)) family = NULL;
            if(unlikely(context && !*context)) context = NULL;
            if(unlikely(!title)) title = "";
            if(unlikely(!units)) units = "unknown";

            debug(D_PLUGINSD, "creating chart type='%s', id='%s', name='%s', family='%s', context='%s', chart='%s', priority=%d, update_every=%d"
                  , type, id
                  , name?name:""
                  , family?family:""
                  , context?context:""
                  , rrdset_type_name(chart_type)
                  , priority
                  , update_every
            );

            st = rrdset_create(
                    host
                    , type
                    , id
                    , name
                    , family
                    , context
                    , title
                    , units
                    , (plugin && *plugin)?plugin:cd->filename
                    , module
                    , priority
                    , update_every
                    , chart_type
            );

            if(options && *options) {
                if(strstr(options, "obsolete"))
                    rrdset_is_obsolete(st);
                else
                    rrdset_isnot_obsolete(st);

                if(strstr(options, "detail"))
                    rrdset_flag_set(st, RRDSET_FLAG_DETAIL);
                else
                    rrdset_flag_clear(st, RRDSET_FLAG_DETAIL);

                if(strstr(options, "hidden"))
                    rrdset_flag_set(st, RRDSET_FLAG_HIDDEN);
                else
                    rrdset_flag_clear(st, RRDSET_FLAG_HIDDEN);

                if(strstr(options, "store_first"))
                    rrdset_flag_set(st, RRDSET_FLAG_STORE_FIRST);
                else
                    rrdset_flag_clear(st, RRDSET_FLAG_STORE_FIRST);
            }
            else {
                rrdset_isnot_obsolete(st);
                rrdset_flag_clear(st, RRDSET_FLAG_DETAIL);
                rrdset_flag_clear(st, RRDSET_FLAG_STORE_FIRST);
            }
        }
        else if(likely(hash == DIMENSION_HASH && !strcmp(s, PLUGINSD_KEYWORD_DIMENSION))) {
            char *id = words[1];
            char *name = words[2];
            char *algorithm = words[3];
            char *multiplier_s = words[4];
            char *divisor_s = words[5];
            char *options = words[6];

            if(unlikely(!id || !*id)) {
                error("requested a DIMENSION, without an id, host '%s' and chart '%s'. Disabling it.", host->hostname, st?st->id:"UNSET");
                enabled = 0;
                break;
            }

            if(unlikely(!st)) {
                error("requested a DIMENSION, without a CHART, on host '%s'. Disabling it.", host->hostname);
                enabled = 0;
                break;
            }

            long multiplier = 1;
            if(multiplier_s && *multiplier_s) multiplier = strtol(multiplier_s, NULL, 0);
            if(unlikely(!multiplier)) multiplier = 1;

            long divisor = 1;
            if(likely(divisor_s && *divisor_s)) divisor = strtol(divisor_s, NULL, 0);
            if(unlikely(!divisor)) divisor = 1;

            if(unlikely(!algorithm || !*algorithm)) algorithm = "absolute";

            if(unlikely(rrdset_flag_check(st, RRDSET_FLAG_DEBUG)))
                debug(D_PLUGINSD, "creating dimension in chart %s, id='%s', name='%s', algorithm='%s', multiplier=%ld, divisor=%ld, hidden='%s'"
                      , st->id
                      , id
                      , name?name:""
                      , rrd_algorithm_name(rrd_algorithm_id(algorithm))
                      , multiplier
                      , divisor
                      , options?options:""
                );

            RRDDIM *rd = rrddim_add(st, id, name, multiplier, divisor, rrd_algorithm_id(algorithm));
            rrddim_flag_clear(rd, RRDDIM_FLAG_HIDDEN);
            rrddim_flag_clear(rd, RRDDIM_FLAG_DONT_DETECT_RESETS_OR_OVERFLOWS);
            if(options && *options) {
                if(strstr(options, "obsolete") != NULL)
                    rrddim_is_obsolete(st, rd);
                else
                    rrddim_isnot_obsolete(st, rd);
                if(strstr(options, "hidden") != NULL) rrddim_flag_set(rd, RRDDIM_FLAG_HIDDEN);
                if(strstr(options, "noreset") != NULL) rrddim_flag_set(rd, RRDDIM_FLAG_DONT_DETECT_RESETS_OR_OVERFLOWS);
                if(strstr(options, "nooverflow") != NULL) rrddim_flag_set(rd, RRDDIM_FLAG_DONT_DETECT_RESETS_OR_OVERFLOWS);
            }
            else {
                rrddim_isnot_obsolete(st, rd);
            }
        }
        else if(likely(hash == VARIABLE_HASH && !strcmp(s, PLUGINSD_KEYWORD_VARIABLE))) {
            char *name = words[1];
            char *value = words[2];
            int global = (st)?0:1;

            if(name && *name) {
                if((strcmp(name, "GLOBAL") == 0 || strcmp(name, "HOST") == 0)) {
                    global = 1;
                    name = words[2];
                    value  = words[3];
                }
                else if((strcmp(name, "LOCAL") == 0 || strcmp(name, "CHART") == 0)) {
                    global = 0;
                    name = words[2];
                    value  = words[3];
                }
            }

            if(unlikely(!name || !*name)) {
                error("requested a VARIABLE on host '%s', without a variable name. Disabling it.", host->hostname);
                enabled = 0;
                break;
            }

            if(unlikely(!value || !*value))
                value = NULL;

            if(value) {
                char *endptr = NULL;
                calculated_number v = (calculated_number)str2ld(value, &endptr);

                if(unlikely(endptr && *endptr)) {
                    if(endptr == value)
                        error("the value '%s' of VARIABLE '%s' on host '%s' cannot be parsed as a number", value, name, host->hostname);
                    else
                        error("the value '%s' of VARIABLE '%s' on host '%s' has leftovers: '%s'", value, name, host->hostname, endptr);
                }

                if(global) {
                    RRDVAR *rv = rrdvar_custom_host_variable_create(host, name);
                    if (rv) rrdvar_custom_host_variable_set(host, rv, v);
                    else error("cannot find/create HOST VARIABLE '%s' on host '%s'", name, host->hostname);
                }
                else if(st) {
                    RRDSETVAR *rs = rrdsetvar_custom_chart_variable_create(st, name);
                    if (rs) rrdsetvar_custom_chart_variable_set(rs, v);
                    else error("cannot find/create CHART VARIABLE '%s' on host '%s', chart '%s'", name, host->hostname, st->id);
                }
                else
                    error("cannot find/create CHART VARIABLE '%s' on host '%s' without a chart", name, host->hostname);
            }
            else
                error("cannot set %s VARIABLE '%s' on host '%s' to an empty value", (global)?"HOST":"CHART", name, host->hostname);
        }
        else if(likely(hash == FLUSH_HASH && !strcmp(s, PLUGINSD_KEYWORD_FLUSH))) {
            debug(D_PLUGINSD, "requested a FLUSH");
            st = NULL;
        }
        else if(unlikely(hash == DISABLE_HASH && !strcmp(s, PLUGINSD_KEYWORD_DISABLE))) {
            info("called DISABLE. Disabling it.");
            enabled = 0;
            break;
        }
        else {
            error("sent command '%s' which is not known by netdata, for host '%s'. Disabling it.", s, host->hostname);
            enabled = 0;
            break;
        }
    }

cleanup:
    cd->enabled = enabled;

    if(likely(count)) {
        cd->successful_collections += count;
        cd->serial_failures = 0;
    }
    else
        cd->serial_failures++;

    return count;
}

static void pluginsd_worker_thread_cleanup(void *arg) {
    struct plugind *cd = (struct plugind *)arg;

    if(cd->enabled && !cd->obsolete) {
        cd->obsolete = 1;

        info("data collection thread exiting");

        if (cd->pid) {
            siginfo_t info;
            info("killing child process pid %d", cd->pid);
            if (killpid(cd->pid, SIGTERM) != -1) {
                info("waiting for child process pid %d to exit...", cd->pid);
                waitid(P_PID, (id_t) cd->pid, &info, WEXITED);
            }
            cd->pid = 0;
        }
    }
}

#define SERIAL_FAILURES_THRESHOLD 10
static void pluginsd_worker_thread_handle_success(struct plugind *cd) {
    if (likely(cd->successful_collections)) {
        sleep((unsigned int) cd->update_every);
        return;
    }

    if(likely(cd->serial_failures <= SERIAL_FAILURES_THRESHOLD)) {
        info("'%s' (pid %d) does not generate useful output but it reports success (exits with 0). %s.",
            cd->fullfilename, cd->pid,
            cd->enabled ?
                "Waiting a bit before starting it again." :
                "Will not start it again - it is now disabled.");
        sleep((unsigned int) (cd->update_every * 10));
        return;
    }

    if (cd->serial_failures > SERIAL_FAILURES_THRESHOLD) {
        error("'%s' (pid %d) does not generate useful output, although it reports success (exits with 0)."
            "We have tried to collect something %zu times - unsuccessfully. Disabling it.",
            cd->fullfilename, cd->pid, cd->serial_failures);
        cd->enabled = 0;
        return;
    }

    return;
}

static void pluginsd_worker_thread_handle_error(struct plugind *cd, int worker_ret_code) {
    if (worker_ret_code == -1) {
        info("'%s' (pid %d) was killed with SIGTERM. Disabling it.", cd->fullfilename, cd->pid);
        cd->enabled = 0;
        return;
    }

    if (!cd->successful_collections) {
        error("'%s' (pid %d) exited with error code %d and haven't collected any data. Disabling it.",
            cd->fullfilename, cd->pid, worker_ret_code);
        cd->enabled = 0;
        return;
    }

    if (cd->serial_failures <= SERIAL_FAILURES_THRESHOLD) {
        error("'%s' (pid %d) exited with error code %d, but has given useful output in the past (%zu times). %s",
            cd->fullfilename, cd->pid, worker_ret_code, cd->successful_collections,
            cd->enabled ?
                "Waiting a bit before starting it again." :
                "Will not start it again - it is disabled.");
        sleep((unsigned int) (cd->update_every * 10));
        return;
    }

    if (cd->serial_failures > SERIAL_FAILURES_THRESHOLD) {
        error("'%s' (pid %d) exited with error code %d, but has given useful output in the past (%zu times)."
            "We tried to restart it %zu times, but it failed to generate data. Disabling it.",
            cd->fullfilename, cd->pid, worker_ret_code, cd->successful_collections, cd->serial_failures);
        cd->enabled = 0;
        return;
    }

    return;
}
#undef SERIAL_FAILURES_THRESHOLD

void *pluginsd_worker_thread(void *arg) {
    netdata_thread_cleanup_push(pluginsd_worker_thread_cleanup, arg);

    struct plugind *cd = (struct plugind *)arg;

    cd->obsolete = 0;
    size_t count = 0;

    while(!netdata_exit) {
        FILE *fp = mypopen(cd->cmd, &cd->pid);
        if(unlikely(!fp)) {
            error("Cannot popen(\"%s\", \"r\").", cd->cmd);
            break;
        }

        info("connected to '%s' running on pid %d", cd->fullfilename, cd->pid);
        count = pluginsd_process(localhost, cd, fp, 0);
        error("'%s' (pid %d) disconnected after %zu successful data collections (ENDs).", cd->fullfilename, cd->pid, count);
        killpid(cd->pid, SIGTERM);

        int worker_ret_code = mypclose(fp, cd->pid);

        if (likely(worker_ret_code == 0))
            pluginsd_worker_thread_handle_success(cd);
        else
            pluginsd_worker_thread_handle_error(cd, worker_ret_code);

        cd->pid = 0;
        if(unlikely(!cd->enabled)) break;
    }

    netdata_thread_cleanup_pop(1);
    return NULL;
}

static void pluginsd_main_cleanup(void *data) {
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

void *pluginsd_main(void *ptr) {
    netdata_thread_cleanup_push(pluginsd_main_cleanup, ptr);

    int automatic_run = config_get_boolean(CONFIG_SECTION_PLUGINS, "enable running new plugins", 1);
    int scan_frequency = (int) config_get_number(CONFIG_SECTION_PLUGINS, "check for new plugins every", 60);
    if(scan_frequency < 1) scan_frequency = 1;

    // store the errno for each plugins directory
    // so that we don't log broken directories on each loop
    int directory_errors[PLUGINSD_MAX_DIRECTORIES] =  { 0 };

    while(!netdata_exit) {
        int idx;
        const char *directory_name;

        for( idx = 0; idx < PLUGINSD_MAX_DIRECTORIES && (directory_name = plugin_directories[idx]) ; idx++ ) {
            if(unlikely(netdata_exit)) break;

            errno = 0;
            DIR *dir = opendir(directory_name);
            if(unlikely(!dir)) {
                if(directory_errors[idx] != errno) {
                    directory_errors[idx] = errno;
                    error("cannot open plugins directory '%s'", directory_name);
                }
                continue;
            }

            struct dirent *file = NULL;
            while(likely((file = readdir(dir)))) {
                if(unlikely(netdata_exit)) break;

                debug(D_PLUGINSD, "examining file '%s'", file->d_name);

                if(unlikely(strcmp(file->d_name, ".") == 0 || strcmp(file->d_name, "..") == 0)) continue;

                int len = (int) strlen(file->d_name);
                if(unlikely(len <= (int)PLUGINSD_FILE_SUFFIX_LEN)) continue;
                if(unlikely(strcmp(PLUGINSD_FILE_SUFFIX, &file->d_name[len - (int)PLUGINSD_FILE_SUFFIX_LEN]) != 0)) {
                    debug(D_PLUGINSD, "file '%s' does not end in '%s'", file->d_name, PLUGINSD_FILE_SUFFIX);
                    continue;
                }

                char pluginname[CONFIG_MAX_NAME + 1];
                snprintfz(pluginname, CONFIG_MAX_NAME, "%.*s", (int)(len - PLUGINSD_FILE_SUFFIX_LEN), file->d_name);
                int enabled = config_get_boolean(CONFIG_SECTION_PLUGINS, pluginname, automatic_run);

                if(unlikely(!enabled)) {
                    debug(D_PLUGINSD, "plugin '%s' is not enabled", file->d_name);
                    continue;
                }

                // check if it runs already
                struct plugind *cd;
                for(cd = pluginsd_root ; cd ; cd = cd->next)
                    if(unlikely(strcmp(cd->filename, file->d_name) == 0)) break;

                if(likely(cd && !cd->obsolete)) {
                    debug(D_PLUGINSD, "plugin '%s' is already running", cd->filename);
                    continue;
                }

                // it is not running
                // allocate a new one, or use the obsolete one
                if(unlikely(!cd)) {
                    cd = callocz(sizeof(struct plugind), 1);

                    snprintfz(cd->id, CONFIG_MAX_NAME, "plugin:%s", pluginname);

                    strncpyz(cd->filename, file->d_name, FILENAME_MAX);
                    snprintfz(cd->fullfilename, FILENAME_MAX, "%s/%s", directory_name, cd->filename);

                    cd->enabled = enabled;
                    cd->update_every = (int) config_get_number(cd->id, "update every", localhost->rrd_update_every);
                    cd->started_t = now_realtime_sec();

                    char *def = "";
                    snprintfz(cd->cmd, PLUGINSD_CMD_MAX, "exec %s %d %s", cd->fullfilename, cd->update_every, config_get(cd->id, "command options", def));

                    // link it
                    if(likely(pluginsd_root)) cd->next = pluginsd_root;
                    pluginsd_root = cd;

                    // it is not currently running
                    cd->obsolete = 1;

                    if(cd->enabled) {
                        char tag[NETDATA_THREAD_TAG_MAX + 1];
                        snprintfz(tag, NETDATA_THREAD_TAG_MAX, "PLUGINSD[%s]", pluginname);
                        // spawn a new thread for it
                        netdata_thread_create(&cd->thread, tag, NETDATA_THREAD_OPTION_DEFAULT, pluginsd_worker_thread, cd);
                    }
                }
            }

            closedir(dir);
        }

        sleep((unsigned int) scan_frequency);
    }

    netdata_thread_cleanup_pop(1);
    return NULL;
}
