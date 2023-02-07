// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdpush.h"
#include "parser/parser.h"

/*
 * rrdpush
 *
 * 3 threads are involved for all stream operations
 *
 * 1. a random data collection thread, calling rrdset_done_push()
 *    this is called for each chart.
 *
 *    the output of this work is kept in a thread BUFFER
 *    the sender thread is signalled via a pipe (in RRDHOST)
 *
 * 2. a sender thread running at the sending netdata
 *    this is spawned automatically on the first chart to be pushed
 *
 *    It tries to push the metrics to the remote netdata, as fast
 *    as possible (i.e. immediately after they are collected).
 *
 * 3. a receiver thread, running at the receiving netdata
 *    this is spawned automatically when the sender connects to
 *    the receiver.
 *
 */

struct config stream_config = {
        .first_section = NULL,
        .last_section = NULL,
        .mutex = NETDATA_MUTEX_INITIALIZER,
        .index = {
                .avl_tree = {
                        .root = NULL,
                        .compar = appconfig_section_compare
                },
                .rwlock = AVL_LOCK_INITIALIZER
        }
};

unsigned int default_rrdpush_enabled = 0;
#ifdef ENABLE_COMPRESSION
unsigned int default_compression_enabled = 1;
#endif
char *default_rrdpush_destination = NULL;
char *default_rrdpush_api_key = NULL;
char *default_rrdpush_send_charts_matching = NULL;
bool default_rrdpush_enable_replication = true;
time_t default_rrdpush_seconds_to_replicate = 86400;
time_t default_rrdpush_replication_step = 600;
#ifdef ENABLE_HTTPS
int netdata_use_ssl_on_stream = NETDATA_SSL_OPTIONAL;
char *netdata_ssl_ca_path = NULL;
char *netdata_ssl_ca_file = NULL;
#endif

static void load_stream_conf() {
    errno = 0;
    char *filename = strdupz_path_subpath(netdata_configured_user_config_dir, "stream.conf");
    if(!appconfig_load(&stream_config, filename, 0, NULL)) {
        info("CONFIG: cannot load user config '%s'. Will try stock config.", filename);
        freez(filename);

        filename = strdupz_path_subpath(netdata_configured_stock_config_dir, "stream.conf");
        if(!appconfig_load(&stream_config, filename, 0, NULL))
            info("CONFIG: cannot load stock config '%s'. Running with internal defaults.", filename);
    }
    freez(filename);
}

bool rrdpush_receiver_needs_dbengine() {
    struct section *co;

    for(co = stream_config.first_section; co; co = co->next) {
        if(strcmp(co->name, "stream") == 0)
            continue; // the first section is not relevant

        char *s;

        s = appconfig_get_by_section(co, "enabled", NULL);
        if(!s || !appconfig_test_boolean_value(s))
            continue;

        s = appconfig_get_by_section(co, "default memory mode", NULL);
        if(s && strcmp(s, "dbengine") == 0)
            return true;

        s = appconfig_get_by_section(co, "memory mode", NULL);
        if(s && strcmp(s, "dbengine") == 0)
            return true;
    }

    return false;
}

int rrdpush_init() {
    // --------------------------------------------------------------------
    // load stream.conf
    load_stream_conf();

    default_rrdpush_enabled     = (unsigned int)appconfig_get_boolean(&stream_config, CONFIG_SECTION_STREAM, "enabled", default_rrdpush_enabled);
    default_rrdpush_destination = appconfig_get(&stream_config, CONFIG_SECTION_STREAM, "destination", "");
    default_rrdpush_api_key     = appconfig_get(&stream_config, CONFIG_SECTION_STREAM, "api key", "");
    default_rrdpush_send_charts_matching      = appconfig_get(&stream_config, CONFIG_SECTION_STREAM, "send charts matching", "*");

    default_rrdpush_enable_replication = config_get_boolean(CONFIG_SECTION_DB, "enable replication", default_rrdpush_enable_replication);
    default_rrdpush_seconds_to_replicate = config_get_number(CONFIG_SECTION_DB, "seconds to replicate", default_rrdpush_seconds_to_replicate);
    default_rrdpush_replication_step = config_get_number(CONFIG_SECTION_DB, "seconds per replication step", default_rrdpush_replication_step);

    rrdhost_free_orphan_time_s    = config_get_number(CONFIG_SECTION_DB, "cleanup orphan hosts after secs", rrdhost_free_orphan_time_s);

#ifdef ENABLE_COMPRESSION
    default_compression_enabled = (unsigned int)appconfig_get_boolean(&stream_config, CONFIG_SECTION_STREAM,
        "enable compression", default_compression_enabled);
#endif

    if(default_rrdpush_enabled && (!default_rrdpush_destination || !*default_rrdpush_destination || !default_rrdpush_api_key || !*default_rrdpush_api_key)) {
        error("STREAM [send]: cannot enable sending thread - information is missing.");
        default_rrdpush_enabled = 0;
    }

#ifdef ENABLE_HTTPS
    if (netdata_use_ssl_on_stream == NETDATA_SSL_OPTIONAL) {
        if (default_rrdpush_destination){
            char *test = strstr(default_rrdpush_destination,":SSL");
            if(test){
                *test = 0X00;
                netdata_use_ssl_on_stream = NETDATA_SSL_FORCE;
            }
        }
    }

    bool invalid_certificate = appconfig_get_boolean(&stream_config, CONFIG_SECTION_STREAM, "ssl skip certificate verification", CONFIG_BOOLEAN_NO);

    if(invalid_certificate == CONFIG_BOOLEAN_YES){
        if(netdata_ssl_validate_server == NETDATA_SSL_VALID_CERTIFICATE){
            info("Netdata is configured to accept invalid SSL certificate.");
            netdata_ssl_validate_server = NETDATA_SSL_INVALID_CERTIFICATE;
        }
    }

    netdata_ssl_ca_path = appconfig_get(&stream_config, CONFIG_SECTION_STREAM, "CApath", NULL);
    netdata_ssl_ca_file = appconfig_get(&stream_config, CONFIG_SECTION_STREAM, "CAfile", NULL);
#endif

    return default_rrdpush_enabled;
}

// data collection happens from multiple threads
// each of these threads calls rrdset_done()
// which in turn calls rrdset_done_push()
// which uses this pipe to notify the streaming thread
// that there are more data ready to be sent
#define PIPE_READ 0
#define PIPE_WRITE 1

// to have the remote netdata re-sync the charts
// to its current clock, we send for this many
// iterations a BEGIN line without microseconds
// this is for the first iterations of each chart
unsigned int remote_clock_resync_iterations = 60;

static inline bool should_send_chart_matching(RRDSET *st, RRDSET_FLAGS flags) {
    if(!(flags & RRDSET_FLAG_RECEIVER_REPLICATION_FINISHED))
        return false;

    if(unlikely(!(flags & (RRDSET_FLAG_UPSTREAM_SEND | RRDSET_FLAG_UPSTREAM_IGNORE)))) {
        RRDHOST *host = st->rrdhost;

        if (flags & RRDSET_FLAG_ANOMALY_DETECTION) {
            if(ml_streaming_enabled())
                rrdset_flag_set(st, RRDSET_FLAG_UPSTREAM_SEND);
            else
                rrdset_flag_set(st, RRDSET_FLAG_UPSTREAM_IGNORE);
        }
        else if(simple_pattern_matches(host->rrdpush_send_charts_matching, rrdset_id(st)) ||
            simple_pattern_matches(host->rrdpush_send_charts_matching, rrdset_name(st)))

            rrdset_flag_set(st, RRDSET_FLAG_UPSTREAM_SEND);
        else
            rrdset_flag_set(st, RRDSET_FLAG_UPSTREAM_IGNORE);

        // get the flags again, to know how to respond
        flags = rrdset_flag_check(st, RRDSET_FLAG_UPSTREAM_SEND|RRDSET_FLAG_UPSTREAM_IGNORE);
    }

    return flags & RRDSET_FLAG_UPSTREAM_SEND;
}

int configured_as_parent() {
    struct section *section = NULL;
    int is_parent = 0;

    appconfig_wrlock(&stream_config);
    for (section = stream_config.first_section; section; section = section->next) {
        uuid_t uuid;

        if (uuid_parse(section->name, uuid) != -1 &&
                appconfig_get_boolean_by_section(section, "enabled", 0)) {
            is_parent = 1;
            break;
        }
    }
    appconfig_unlock(&stream_config);

    return is_parent;
}

// chart labels
static int send_clabels_callback(const char *name, const char *value, RRDLABEL_SRC ls, void *data) {
    BUFFER *wb = (BUFFER *)data;
    buffer_sprintf(wb, "CLABEL \"%s\" \"%s\" %d\n", name, value, ls);
    return 1;
}

static void rrdpush_send_clabels(BUFFER *wb, RRDSET *st) {
    if (st->rrdlabels) {
        if(rrdlabels_walkthrough_read(st->rrdlabels, send_clabels_callback, wb) > 0)
            buffer_sprintf(wb, "CLABEL_COMMIT\n");
    }
}

// Send the current chart definition.
// Assumes that collector thread has already called sender_start for mutex / buffer state.
static inline bool rrdpush_send_chart_definition(BUFFER *wb, RRDSET *st) {
    bool replication_progress = false;

    RRDHOST *host = st->rrdhost;

    rrdset_flag_set(st, RRDSET_FLAG_UPSTREAM_EXPOSED);

    // properly set the name for the remote end to parse it
    char *name = "";
    if(likely(st->name)) {
        if(unlikely(st->id != st->name)) {
            // they differ
            name = strchr(rrdset_name(st), '.');
            if(name)
                name++;
            else
                name = "";
        }
    }

    // send the chart
    buffer_sprintf(
            wb
            , "CHART \"%s\" \"%s\" \"%s\" \"%s\" \"%s\" \"%s\" \"%s\" %ld %d \"%s %s %s %s\" \"%s\" \"%s\"\n"
            , rrdset_id(st)
            , name
            , rrdset_title(st)
            , rrdset_units(st)
            , rrdset_family(st)
            , rrdset_context(st)
            , rrdset_type_name(st->chart_type)
            , st->priority
            , st->update_every
            , rrdset_flag_check(st, RRDSET_FLAG_OBSOLETE)?"obsolete":""
            , rrdset_flag_check(st, RRDSET_FLAG_DETAIL)?"detail":""
            , rrdset_flag_check(st, RRDSET_FLAG_STORE_FIRST)?"store_first":""
            , rrdset_flag_check(st, RRDSET_FLAG_HIDDEN)?"hidden":""
            , rrdset_plugin_name(st)
            , rrdset_module_name(st)
    );

    // send the chart labels
    if (stream_has_capability(host->sender, STREAM_CAP_CLABELS))
        rrdpush_send_clabels(wb, st);

    // send the dimensions
    RRDDIM *rd;
    rrddim_foreach_read(rd, st) {
        buffer_sprintf(
                wb
                , "DIMENSION \"%s\" \"%s\" \"%s\" " COLLECTED_NUMBER_FORMAT " " COLLECTED_NUMBER_FORMAT " \"%s %s %s\"\n"
                , rrddim_id(rd)
                , rrddim_name(rd)
                , rrd_algorithm_name(rd->algorithm)
                , rd->multiplier
                , rd->divisor
                , rrddim_flag_check(rd, RRDDIM_FLAG_OBSOLETE)?"obsolete":""
                , rrddim_option_check(rd, RRDDIM_OPTION_HIDDEN)?"hidden":""
                , rrddim_option_check(rd, RRDDIM_OPTION_DONT_DETECT_RESETS_OR_OVERFLOWS)?"noreset":""
        );
        rd->exposed = 1;
    }
    rrddim_foreach_done(rd);

    // send the chart functions
    if(stream_has_capability(host->sender, STREAM_CAP_FUNCTIONS))
        rrd_functions_expose_rrdpush(st, wb);

    // send the chart local custom variables
    rrdsetvar_print_to_streaming_custom_chart_variables(st, wb);

    if (stream_has_capability(host->sender, STREAM_CAP_REPLICATION)) {
        time_t db_first_time_t, db_last_time_t;

        time_t now = now_realtime_sec();
        rrdset_get_retention_of_tier_for_collected_chart(st, &db_first_time_t, &db_last_time_t, now, 0);

        buffer_sprintf(wb, PLUGINSD_KEYWORD_CHART_DEFINITION_END " %llu %llu %llu\n",
                       (unsigned long long)db_first_time_t,
                       (unsigned long long)db_last_time_t,
                       (unsigned long long)now);

        rrdset_flag_set(st, RRDSET_FLAG_SENDER_REPLICATION_IN_PROGRESS);
        rrdset_flag_clear(st, RRDSET_FLAG_SENDER_REPLICATION_FINISHED);
        rrdhost_sender_replicating_charts_plus_one(st->rrdhost);
        replication_progress = true;

#ifdef NETDATA_LOG_REPLICATION_REQUESTS
        internal_error(true, "REPLAY: 'host:%s/chart:%s' replication starts",
                       rrdhost_hostname(st->rrdhost), rrdset_id(st));
#endif
    }

    st->upstream_resync_time_s = st->last_collected_time.tv_sec + (remote_clock_resync_iterations * st->update_every);
    return replication_progress;
}

// sends the current chart dimensions
static void rrdpush_send_chart_metrics(BUFFER *wb, RRDSET *st, struct sender_state *s __maybe_unused, RRDSET_FLAGS flags) {
    buffer_fast_strcat(wb, "BEGIN \"", 7);
    buffer_fast_strcat(wb, rrdset_id(st), string_strlen(st->id));
    buffer_fast_strcat(wb, "\" ", 2);

    if(st->last_collected_time.tv_sec > st->upstream_resync_time_s)
        buffer_print_llu(wb, st->usec_since_last_update);
    else
        buffer_fast_strcat(wb, "0", 1);

    buffer_fast_strcat(wb, "\n", 1);

    RRDDIM *rd;
    rrddim_foreach_read(rd, st) {
        if(unlikely(!rd->updated))
            continue;

        if(likely(rd->exposed)) {
            buffer_fast_strcat(wb, "SET \"", 5);
            buffer_fast_strcat(wb, rrddim_id(rd), string_strlen(rd->id));
            buffer_fast_strcat(wb, "\" = ", 4);
            buffer_print_ll(wb, rd->collected_value);
            buffer_fast_strcat(wb, "\n", 1);
        }
        else {
            internal_error(true, "STREAM: 'host:%s/chart:%s/dim:%s' flag 'exposed' is updated but not exposed",
                           rrdhost_hostname(st->rrdhost), rrdset_id(st), rrddim_id(rd));
            // we will include it in the next iteration
            rrdset_flag_clear(st, RRDSET_FLAG_UPSTREAM_EXPOSED);
        }
    }
    rrddim_foreach_done(rd);

    if(unlikely(flags & RRDSET_FLAG_UPSTREAM_SEND_VARIABLES))
        rrdsetvar_print_to_streaming_custom_chart_variables(st, wb);

    buffer_fast_strcat(wb, "END\n", 4);
}

static void rrdpush_sender_thread_spawn(RRDHOST *host);

// Called from the internal collectors to mark a chart obsolete.
bool rrdset_push_chart_definition_now(RRDSET *st) {
    RRDHOST *host = st->rrdhost;

    if(unlikely(!rrdhost_can_send_definitions_to_parent(host)
        || !should_send_chart_matching(st, __atomic_load_n(&st->flags, __ATOMIC_SEQ_CST))))
        return false;

    BUFFER *wb = sender_start(host->sender);
    rrdpush_send_chart_definition(wb, st);
    sender_commit(host->sender, wb);
    sender_thread_buffer_free();

    return true;
}

void rrdset_done_push(RRDSET *st) {
    RRDHOST *host = st->rrdhost;

    // fetch the flags we need to check with one atomic operation
    RRDHOST_FLAGS host_flags = __atomic_load_n(&host->flags, __ATOMIC_SEQ_CST);

    // check if we are not connected
    if(unlikely(!(host_flags & RRDHOST_FLAG_RRDPUSH_SENDER_READY_4_METRICS))) {

        if(unlikely(!(host_flags & (RRDHOST_FLAG_RRDPUSH_SENDER_SPAWN | RRDHOST_FLAG_RRDPUSH_RECEIVER_DISCONNECTED))))
            rrdpush_sender_thread_spawn(host);

        if(unlikely(!(host_flags & RRDHOST_FLAG_RRDPUSH_SENDER_LOGGED_STATUS))) {
            rrdhost_flag_set(host, RRDHOST_FLAG_RRDPUSH_SENDER_LOGGED_STATUS);
            error("STREAM %s [send]: not ready - collected metrics are not sent to parent.", rrdhost_hostname(host));
        }

        return;
    }
    else if(unlikely(host_flags & RRDHOST_FLAG_RRDPUSH_SENDER_LOGGED_STATUS)) {
        info("STREAM %s [send]: sending metrics to parent...", rrdhost_hostname(host));
        rrdhost_flag_clear(host, RRDHOST_FLAG_RRDPUSH_SENDER_LOGGED_STATUS);
    }

    RRDSET_FLAGS rrdset_flags = __atomic_load_n(&st->flags, __ATOMIC_SEQ_CST);
    bool exposed_upstream = (rrdset_flags & RRDSET_FLAG_UPSTREAM_EXPOSED);
    bool replication_in_progress = !(rrdset_flags & RRDSET_FLAG_SENDER_REPLICATION_FINISHED);

    if(unlikely((exposed_upstream && replication_in_progress) ||
                !should_send_chart_matching(st, rrdset_flags)))
        return;

    BUFFER *wb = sender_start(host->sender);

    if(unlikely(!exposed_upstream))
        replication_in_progress = rrdpush_send_chart_definition(wb, st);

    if (likely(!replication_in_progress))
        rrdpush_send_chart_metrics(wb, st, host->sender, rrdset_flags);

    sender_commit(host->sender, wb);
}

// labels
static int send_labels_callback(const char *name, const char *value, RRDLABEL_SRC ls, void *data) {
    BUFFER *wb = (BUFFER *)data;
    buffer_sprintf(wb, "LABEL \"%s\" = %d \"%s\"\n", name, ls, value);
    return 1;
}
void rrdpush_send_host_labels(RRDHOST *host) {
    if(unlikely(!rrdhost_can_send_definitions_to_parent(host)
                 || !stream_has_capability(host->sender, STREAM_CAP_HLABELS)))
        return;

    BUFFER *wb = sender_start(host->sender);

    rrdlabels_walkthrough_read(host->rrdlabels, send_labels_callback, wb);
    buffer_sprintf(wb, "OVERWRITE %s\n", "labels");

    sender_commit(host->sender, wb);

    sender_thread_buffer_free();
}

void rrdpush_claimed_id(RRDHOST *host)
{
    if(!stream_has_capability(host->sender, STREAM_CAP_CLAIM))
        return;

    if(unlikely(!rrdhost_can_send_definitions_to_parent(host)))
        return;
    
    BUFFER *wb = sender_start(host->sender);
    rrdhost_aclk_state_lock(host);

    buffer_sprintf(wb, "CLAIMED_ID %s %s\n", host->machine_guid, (host->aclk_state.claimed_id ? host->aclk_state.claimed_id : "NULL") );

    rrdhost_aclk_state_unlock(host);
    sender_commit(host->sender, wb);

    sender_thread_buffer_free();
}

int connect_to_one_of_destinations(
    RRDHOST *host,
    int default_port,
    struct timeval *timeout,
    size_t *reconnects_counter,
    char *connected_to,
    size_t connected_to_size,
    struct rrdpush_destinations **destination)
{
    int sock = -1;

    for (struct rrdpush_destinations *d = host->destinations; d; d = d->next) {
        time_t now = now_realtime_sec();

        if(d->postpone_reconnection_until > now)
            continue;

        info(
            "STREAM %s: connecting to '%s' (default port: %d)...",
            rrdhost_hostname(host),
            string2str(d->destination),
            default_port);

        if (reconnects_counter)
            *reconnects_counter += 1;

        sock = connect_to_this(string2str(d->destination), default_port, timeout);

        if (sock != -1) {
            if (connected_to && connected_to_size)
                strncpyz(connected_to, string2str(d->destination), connected_to_size);

            *destination = d;

            // move the current item to the end of the list
            // without this, this destination will break the loop again and again
            // not advancing the destinations to find one that may work
            DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(host->destinations, d, prev, next);
            DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(host->destinations, d, prev, next);

            break;
        }
    }

    return sock;
}

struct destinations_init_tmp {
    RRDHOST *host;
    struct rrdpush_destinations *list;
    int count;
};

bool destinations_init_add_one(char *entry, void *data) {
    struct destinations_init_tmp *t = data;

    struct rrdpush_destinations *d = callocz(1, sizeof(struct rrdpush_destinations));
    d->destination = string_strdupz(entry);

    __atomic_add_fetch(&netdata_buffers_statistics.rrdhost_senders, sizeof(struct rrdpush_destinations), __ATOMIC_RELAXED);

    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(t->list, d, prev, next);

    t->count++;
    info("STREAM: added streaming destination No %d: '%s' to host '%s'", t->count, string2str(d->destination), rrdhost_hostname(t->host));

    return false; // we return false, so that we will get all defined destinations
}

void rrdpush_destinations_init(RRDHOST *host) {
    if(!host->rrdpush_send_destination) return;

    rrdpush_destinations_free(host);

    struct destinations_init_tmp t = {
        .host = host,
        .list = NULL,
        .count = 0,
    };

    foreach_entry_in_connection_string(host->rrdpush_send_destination, destinations_init_add_one, &t);

    host->destinations = t.list;
}

void rrdpush_destinations_free(RRDHOST *host) {
    while (host->destinations) {
        struct rrdpush_destinations *tmp = host->destinations;
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(host->destinations, tmp, prev, next);
        string_freez(tmp->destination);
        freez(tmp);
        __atomic_sub_fetch(&netdata_buffers_statistics.rrdhost_senders, sizeof(struct rrdpush_destinations), __ATOMIC_RELAXED);
    }

    host->destinations = NULL;
}

// ----------------------------------------------------------------------------
// rrdpush sender thread

// Either the receiver lost the connection or the host is being destroyed.
// The sender mutex guards thread creation, any spurious data is wiped on reconnection.
void rrdpush_sender_thread_stop(RRDHOST *host, const char *reason, bool wait) {
    if (!host->sender)
        return;

    netdata_mutex_lock(&host->sender->mutex);

    if(rrdhost_flag_check(host, RRDHOST_FLAG_RRDPUSH_SENDER_SPAWN)) {

        host->sender->exit.shutdown = true;
        host->sender->exit.reason = reason;

        // signal it to cancel
        netdata_thread_cancel(host->rrdpush_sender_thread);
    }

    netdata_mutex_unlock(&host->sender->mutex);

    if(wait) {
        netdata_mutex_lock(&host->sender->mutex);
        while(host->sender->tid) {
            netdata_mutex_unlock(&host->sender->mutex);
            sleep_usec(10 * USEC_PER_MS);
            netdata_mutex_lock(&host->sender->mutex);
        }
        netdata_mutex_unlock(&host->sender->mutex);
    }
}


// ----------------------------------------------------------------------------
// rrdpush receiver thread

void log_stream_connection(const char *client_ip, const char *client_port, const char *api_key, const char *machine_guid, const char *host, const char *msg) {
    log_access("STREAM: %d '[%s]:%s' '%s' host '%s' api key '%s' machine guid '%s'", gettid(), client_ip, client_port, msg, host, api_key, machine_guid);
}


static void rrdpush_sender_thread_spawn(RRDHOST *host) {
    netdata_mutex_lock(&host->sender->mutex);

    if(!rrdhost_flag_check(host, RRDHOST_FLAG_RRDPUSH_SENDER_SPAWN)) {
        char tag[NETDATA_THREAD_TAG_MAX + 1];
        snprintfz(tag, NETDATA_THREAD_TAG_MAX, THREAD_TAG_STREAM_SENDER "[%s]", rrdhost_hostname(host));

        if(netdata_thread_create(&host->rrdpush_sender_thread, tag, NETDATA_THREAD_OPTION_DEFAULT, rrdpush_sender_thread, (void *) host->sender))
            error("STREAM %s [send]: failed to create new thread for client.", rrdhost_hostname(host));
        else
            rrdhost_flag_set(host, RRDHOST_FLAG_RRDPUSH_SENDER_SPAWN);
    }

    netdata_mutex_unlock(&host->sender->mutex);
}

int rrdpush_receiver_permission_denied(struct web_client *w) {
    // we always respond with the same message and error code
    // to prevent an attacker from gaining info about the error
    buffer_flush(w->response.data);
    buffer_sprintf(w->response.data, "You are not permitted to access this. Check the logs for more info.");
    return HTTP_RESP_UNAUTHORIZED;
}

int rrdpush_receiver_too_busy_now(struct web_client *w) {
    // we always respond with the same message and error code
    // to prevent an attacker from gaining info about the error
    buffer_flush(w->response.data);
    buffer_sprintf(w->response.data, "The server is too busy now to accept this request. Try later.");
    return HTTP_RESP_SERVICE_UNAVAILABLE;
}

void *rrdpush_receiver_thread(void *ptr);
int rrdpush_receiver_thread_spawn(struct web_client *w, char *url) {

    if(!service_running(ABILITY_STREAMING_CONNECTIONS))
        return rrdpush_receiver_too_busy_now(w);

    struct receiver_state *rpt = callocz(1, sizeof(*rpt));
    rpt->last_msg_t = now_realtime_sec();
    rpt->capabilities = STREAM_CAP_INVALID;
    rpt->hops = 1;

    __atomic_add_fetch(&netdata_buffers_statistics.rrdhost_receivers, sizeof(*rpt), __ATOMIC_RELAXED);
    __atomic_add_fetch(&netdata_buffers_statistics.rrdhost_allocations_size, sizeof(struct rrdhost_system_info), __ATOMIC_RELAXED);

    rpt->system_info = callocz(1, sizeof(struct rrdhost_system_info));
    rpt->system_info->hops = rpt->hops;

    rpt->fd                = w->ifd;
    rpt->client_ip         = strdupz(w->client_ip);
    rpt->client_port       = strdupz(w->client_port);

    rpt->config.update_every = default_rrd_update_every;

#ifdef ENABLE_HTTPS
    rpt->ssl.conn          = w->ssl.conn;
    rpt->ssl.flags         = w->ssl.flags;

    w->ssl.conn = NULL;
    w->ssl.flags = NETDATA_SSL_START;
#endif

    // parse the parameters and fill rpt and rpt->system_info

    while(url) {
        char *value = mystrsep(&url, "&");
        if(!value || !*value) continue;

        char *name = mystrsep(&value, "=");
        if(!name || !*name) continue;
        if(!value || !*value) continue;

        if(!strcmp(name, "key") && !rpt->key)
            rpt->key = strdupz(value);

        else if(!strcmp(name, "hostname") && !rpt->hostname)
            rpt->hostname = strdupz(value);

        else if(!strcmp(name, "registry_hostname") && !rpt->registry_hostname)
            rpt->registry_hostname = strdupz(value);

        else if(!strcmp(name, "machine_guid") && !rpt->machine_guid)
            rpt->machine_guid = strdupz(value);

        else if(!strcmp(name, "update_every"))
            rpt->config.update_every = (int)strtoul(value, NULL, 0);

        else if(!strcmp(name, "os") && !rpt->os)
            rpt->os = strdupz(value);

        else if(!strcmp(name, "timezone") && !rpt->timezone)
            rpt->timezone = strdupz(value);

        else if(!strcmp(name, "abbrev_timezone") && !rpt->abbrev_timezone)
            rpt->abbrev_timezone = strdupz(value);

        else if(!strcmp(name, "utc_offset"))
            rpt->utc_offset = (int32_t)strtol(value, NULL, 0);

        else if(!strcmp(name, "hops"))
            rpt->hops = rpt->system_info->hops = (uint16_t) strtoul(value, NULL, 0);

        else if(!strcmp(name, "ml_capable"))
            rpt->system_info->ml_capable = strtoul(value, NULL, 0);

        else if(!strcmp(name, "ml_enabled"))
            rpt->system_info->ml_enabled = strtoul(value, NULL, 0);

        else if(!strcmp(name, "mc_version"))
            rpt->system_info->mc_version = strtoul(value, NULL, 0);

        else if(!strcmp(name, "tags") && !rpt->tags)
            rpt->tags = strdupz(value);

        else if(!strcmp(name, "ver") && (rpt->capabilities & STREAM_CAP_INVALID))
            rpt->capabilities = convert_stream_version_to_capabilities(strtoul(value, NULL, 0));

        else {
            // An old Netdata child does not have a compatible streaming protocol, map to something sane.
            if (!strcmp(name, "NETDATA_SYSTEM_OS_NAME"))
                name = "NETDATA_HOST_OS_NAME";

            else if (!strcmp(name, "NETDATA_SYSTEM_OS_ID"))
                name = "NETDATA_HOST_OS_ID";

            else if (!strcmp(name, "NETDATA_SYSTEM_OS_ID_LIKE"))
                name = "NETDATA_HOST_OS_ID_LIKE";

            else if (!strcmp(name, "NETDATA_SYSTEM_OS_VERSION"))
                name = "NETDATA_HOST_OS_VERSION";

            else if (!strcmp(name, "NETDATA_SYSTEM_OS_VERSION_ID"))
                name = "NETDATA_HOST_OS_VERSION_ID";

            else if (!strcmp(name, "NETDATA_SYSTEM_OS_DETECTION"))
                name = "NETDATA_HOST_OS_DETECTION";

            else if(!strcmp(name, "NETDATA_PROTOCOL_VERSION") && (rpt->capabilities & STREAM_CAP_INVALID))
                rpt->capabilities = convert_stream_version_to_capabilities(1);

            if (unlikely(rrdhost_set_system_info_variable(rpt->system_info, name, value))) {
                info("STREAM '%s' [receive from [%s]:%s]: "
                     "request has parameter '%s' = '%s', which is not used."
                     , (rpt->hostname && *rpt->hostname) ? rpt->hostname : "-"
                     , rpt->client_ip, rpt->client_port
                     , name, value);
            }
        }
    }

    if (rpt->capabilities & STREAM_CAP_INVALID)
        // no version is supplied, assume version 0;
        rpt->capabilities = convert_stream_version_to_capabilities(0);

    // find the program name and version
    if(w->user_agent && w->user_agent[0]) {
        char *t = strchr(w->user_agent, '/');
        if(t && *t) {
            *t = '\0';
            t++;
        }

        rpt->program_name = strdupz(w->user_agent);
        if(t && *t) rpt->program_version = strdupz(t);
    }

    // check if we should accept this connection

    if(!rpt->key || !*rpt->key) {
        rrdpush_receive_log_status(
                rpt,
                "request without an API key",
                "NO API KEY PERMISSION DENIED");

        receiver_state_free(rpt);
        return rrdpush_receiver_permission_denied(w);
    }

    if(!rpt->hostname || !*rpt->hostname) {
        rrdpush_receive_log_status(
                rpt,
                "request without a hostname",
                "NO HOSTNAME PERMISSION DENIED");

        receiver_state_free(rpt);
        return rrdpush_receiver_permission_denied(w);
    }

    if(!rpt->registry_hostname)
        rpt->registry_hostname = strdupz(rpt->hostname);

    if(!rpt->machine_guid || !*rpt->machine_guid) {
        rrdpush_receive_log_status(
                rpt,
                "request without a machine GUID",
                "NO MACHINE GUID PERMISSION DENIED");

        receiver_state_free(rpt);
        return rrdpush_receiver_permission_denied(w);
    }

    {
        char buf[GUID_LEN + 1];

        if (regenerate_guid(rpt->key, buf) == -1) {
            rrdpush_receive_log_status(
                    rpt,
                    "API key is not a valid UUID (use the command uuidgen to generate one)",
                    "INVALID API KEY PERMISSION DENIED");

            receiver_state_free(rpt);
            return rrdpush_receiver_permission_denied(w);
        }

        if (regenerate_guid(rpt->machine_guid, buf) == -1) {
            rrdpush_receive_log_status(
                    rpt,
                    "machine GUID is not a valid UUID",
                    "INVALID MACHINE GUID PERMISSION DENIED");

            receiver_state_free(rpt);
            return rrdpush_receiver_permission_denied(w);
        }
    }

    const char *api_key_type = appconfig_get(&stream_config, rpt->key, "type", "api");
    if(!api_key_type || !*api_key_type) api_key_type = "unknown";
    if(strcmp(api_key_type, "api") != 0) {
        rrdpush_receive_log_status(
                rpt,
                "API key is a machine GUID",
                "INVALID API KEY PERMISSION DENIED");

        receiver_state_free(rpt);
        return rrdpush_receiver_permission_denied(w);
    }

    if(!appconfig_get_boolean(&stream_config, rpt->key, "enabled", 0)) {
        rrdpush_receive_log_status(
                rpt,
                "API key is not enabled",
                "API KEY DISABLED PERMISSION DENIED");

        receiver_state_free(rpt);
        return rrdpush_receiver_permission_denied(w);
    }

    {
        SIMPLE_PATTERN *key_allow_from = simple_pattern_create(
                appconfig_get(&stream_config, rpt->key, "allow from", "*"),
                NULL, SIMPLE_PATTERN_EXACT);

        if(key_allow_from) {
            if(!simple_pattern_matches(key_allow_from, w->client_ip)) {
                simple_pattern_free(key_allow_from);

                rrdpush_receive_log_status(
                        rpt,
                        "API key is not allowed from this IP",
                        "NOT ALLOWED IP PERMISSION DENIED");

                receiver_state_free(rpt);
                return rrdpush_receiver_permission_denied(w);
            }

            simple_pattern_free(key_allow_from);
        }
    }

    {
        const char *machine_guid_type = appconfig_get(&stream_config, rpt->machine_guid, "type", "machine");
        if (!machine_guid_type || !*machine_guid_type) machine_guid_type = "unknown";

        if (strcmp(machine_guid_type, "machine") != 0) {
            rrdpush_receive_log_status(
                    rpt,
                    "machine GUID is an API key",
                    "INVALID MACHINE GUID PERMISSION DENIED");

            receiver_state_free(rpt);
            return rrdpush_receiver_permission_denied(w);
        }
    }

    if(!appconfig_get_boolean(&stream_config, rpt->machine_guid, "enabled", 1)) {
        rrdpush_receive_log_status(
                rpt,
                "machine GUID is not enabled",
                "MACHINE GUID DISABLED PERMISSION DENIED");

        receiver_state_free(rpt);
        return rrdpush_receiver_permission_denied(w);
    }

    {
        SIMPLE_PATTERN *machine_allow_from = simple_pattern_create(
                appconfig_get(&stream_config, rpt->machine_guid, "allow from", "*"),
                NULL, SIMPLE_PATTERN_EXACT);

        if(machine_allow_from) {
            if(!simple_pattern_matches(machine_allow_from, w->client_ip)) {
                simple_pattern_free(machine_allow_from);

                rrdpush_receive_log_status(
                        rpt,
                        "machine GUID is not allowed from this IP",
                        "NOT ALLOWED IP PERMISSION DENIED");

                receiver_state_free(rpt);
                return rrdpush_receiver_permission_denied(w);
            }

            simple_pattern_free(machine_allow_from);
        }
    }

    if (strcmp(rpt->machine_guid, localhost->machine_guid) == 0) {

        rrdpush_receive_log_status(
                rpt,
                "machine GUID is my own",
                "LOCALHOST PERMISSION DENIED");

        char initial_response[HTTP_HEADER_SIZE + 1];
        snprintfz(initial_response, HTTP_HEADER_SIZE, "%s", START_STREAMING_ERROR_SAME_LOCALHOST);

        if(send_timeout(
#ifdef ENABLE_HTTPS
                &rpt->ssl,
#endif
                rpt->fd, initial_response, strlen(initial_response), 0, 60) != (ssize_t)strlen(initial_response)) {

            error("STREAM '%s' [receive from [%s]:%s]: "
                  "failed to reply."
                  , rpt->hostname
                  , rpt->client_ip, rpt->client_port
            );
        }

        close(rpt->fd);
        receiver_state_free(rpt);
        return web_client_socket_is_now_used_for_streaming(w);
    }

    if(unlikely(web_client_streaming_rate_t > 0)) {
        static SPINLOCK spinlock = NETDATA_SPINLOCK_INITIALIZER;
        static time_t last_stream_accepted_t = 0;

        time_t now = now_realtime_sec();
        netdata_spinlock_lock(&spinlock);

        if(unlikely(last_stream_accepted_t == 0))
            last_stream_accepted_t = now;

        if(now - last_stream_accepted_t < web_client_streaming_rate_t) {
            netdata_spinlock_unlock(&spinlock);

            char msg[100 + 1];
            snprintfz(msg, 100,
                      "rate limit, will accept new connection in %ld secs",
                      (long)(web_client_streaming_rate_t - (now - last_stream_accepted_t)));

            rrdpush_receive_log_status(
                    rpt,
                    msg,
                    "RATE LIMIT TRY LATER");

            receiver_state_free(rpt);
            return rrdpush_receiver_too_busy_now(w);
        }

        last_stream_accepted_t = now;
        netdata_spinlock_unlock(&spinlock);
    }

    /*
     * Quick path for rejecting multiple connections. The lock taken is fine-grained - it only protects the receiver
     * pointer within the host (if a host exists). This protects against multiple concurrent web requests hitting
     * separate threads within the web-server and landing here. The lock guards the thread-shutdown sequence that
     * detaches the receiver from the host. If the host is being created (first time-access) then we also use the
     * lock to prevent race-hazard (two threads try to create the host concurrently, one wins and the other does a
     * lookup to the now-attached structure).
     */

    {
        time_t age = 0;
        bool receiver_stale = false;
        bool receiver_working = false;

        rrd_rdlock();
        RRDHOST *host = rrdhost_find_by_guid(rpt->machine_guid);
        if (unlikely(host && rrdhost_flag_check(host, RRDHOST_FLAG_ARCHIVED))) /* Ignore archived hosts. */
            host = NULL;

        if (host) {
            netdata_mutex_lock(&host->receiver_lock);
            if (host->receiver) {
                age = now_realtime_sec() - host->receiver->last_msg_t;

                if (age < 30)
                    receiver_working = true;
                else
                    receiver_stale = true;
            }
            netdata_mutex_unlock(&host->receiver_lock);
        }
        rrd_unlock();

        if (receiver_stale && stop_streaming_receiver(host, "STALE RECEIVER")) {
            // we stopped the receiver
            // we can proceed with this connection
            receiver_stale = false;

            info("STREAM '%s' [receive from [%s]:%s]: "
                 "stopped previous stale receiver to accept this one."
                 , rpt->hostname
                 , rpt->client_ip, rpt->client_port
            );
        }

        if (receiver_working || receiver_stale) {
            // another receiver is already connected
            // try again later

            char msg[200 + 1];
            snprintfz(msg, 200,
                      "multiple connections for same host, "
                      "old connection was used %ld secs ago%s",
                      age, receiver_stale ? " (signaled old receiver to stop)" : " (new connection not accepted)");

            rrdpush_receive_log_status(
                    rpt,
                    msg,
                    "ALREADY CONNECTED");

            // Have not set WEB_CLIENT_FLAG_DONT_CLOSE_SOCKET - caller should clean up
            buffer_flush(w->response.data);
            buffer_strcat(w->response.data, "This GUID is already streaming to this server");
            receiver_state_free(rpt);
            return HTTP_RESP_CONFLICT;
        }
    }

    debug(D_SYSTEM, "starting STREAM receive thread.");

    char tag[FILENAME_MAX + 1];
    snprintfz(tag, FILENAME_MAX, THREAD_TAG_STREAM_RECEIVER "[%s,[%s]:%s]", rpt->hostname, w->client_ip, w->client_port);

    if(netdata_thread_create(&rpt->thread, tag, NETDATA_THREAD_OPTION_DEFAULT, rrdpush_receiver_thread, (void *)rpt)) {
        rrdpush_receive_log_status(
                rpt,
                "can't create receiver thread",
                "INTERNAL SERVER ERROR");

        buffer_flush(w->response.data);
        buffer_strcat(w->response.data, "Can't handle this request");
        receiver_state_free(rpt);
        return HTTP_RESP_INTERNAL_SERVER_ERROR;
    }

    // prevent the caller from closing the streaming socket
    return web_client_socket_is_now_used_for_streaming(w);
}

static void stream_capabilities_to_string(BUFFER *wb, STREAM_CAPABILITIES caps) {
    if(caps & STREAM_CAP_V1) buffer_strcat(wb, "V1 ");
    if(caps & STREAM_CAP_V2) buffer_strcat(wb, "V2 ");
    if(caps & STREAM_CAP_VN) buffer_strcat(wb, "VN ");
    if(caps & STREAM_CAP_VCAPS) buffer_strcat(wb, "VCAPS ");
    if(caps & STREAM_CAP_HLABELS) buffer_strcat(wb, "HLABELS ");
    if(caps & STREAM_CAP_CLAIM) buffer_strcat(wb, "CLAIM ");
    if(caps & STREAM_CAP_CLABELS) buffer_strcat(wb, "CLABELS ");
    if(caps & STREAM_CAP_COMPRESSION) buffer_strcat(wb, "COMPRESSION ");
    if(caps & STREAM_CAP_FUNCTIONS) buffer_strcat(wb, "FUNCTIONS ");
    if(caps & STREAM_CAP_REPLICATION) buffer_strcat(wb, "REPLICATION ");
    if(caps & STREAM_CAP_BINARY) buffer_strcat(wb, "BINARY ");
}

void log_receiver_capabilities(struct receiver_state *rpt) {
    BUFFER *wb = buffer_create(100, NULL);
    stream_capabilities_to_string(wb, rpt->capabilities);

    info("STREAM %s [receive from [%s]:%s]: established link with negotiated capabilities: %s",
         rrdhost_hostname(rpt->host), rpt->client_ip, rpt->client_port, buffer_tostring(wb));

    buffer_free(wb);
}

void log_sender_capabilities(struct sender_state *s) {
    BUFFER *wb = buffer_create(100, NULL);
    stream_capabilities_to_string(wb, s->capabilities);

    info("STREAM %s [send to %s]: established link with negotiated capabilities: %s",
         rrdhost_hostname(s->host), s->connected_to, buffer_tostring(wb));

    buffer_free(wb);
}

STREAM_CAPABILITIES convert_stream_version_to_capabilities(int32_t version) {
    STREAM_CAPABILITIES caps = 0;

    if(version <= 1) caps = STREAM_CAP_V1;
    else if(version < STREAM_OLD_VERSION_CLAIM) caps = STREAM_CAP_V2 | STREAM_CAP_HLABELS;
    else if(version <= STREAM_OLD_VERSION_CLAIM) caps = STREAM_CAP_VN | STREAM_CAP_HLABELS | STREAM_CAP_CLAIM;
    else if(version <= STREAM_OLD_VERSION_CLABELS) caps = STREAM_CAP_VN | STREAM_CAP_HLABELS | STREAM_CAP_CLAIM | STREAM_CAP_CLABELS;
    else if(version <= STREAM_OLD_VERSION_COMPRESSION) caps = STREAM_CAP_VN | STREAM_CAP_HLABELS | STREAM_CAP_CLAIM | STREAM_CAP_CLABELS | STREAM_HAS_COMPRESSION;
    else caps = version;

    if(caps & STREAM_CAP_VCAPS)
        caps &= ~(STREAM_CAP_V1|STREAM_CAP_V2|STREAM_CAP_VN);

    if(caps & STREAM_CAP_VN)
        caps &= ~(STREAM_CAP_V1|STREAM_CAP_V2);

    if(caps & STREAM_CAP_V2)
        caps &= ~(STREAM_CAP_V1);

    return caps & STREAM_OUR_CAPABILITIES;
}

int32_t stream_capabilities_to_vn(uint32_t caps) {
    if(caps & STREAM_CAP_COMPRESSION) return STREAM_OLD_VERSION_COMPRESSION;
    if(caps & STREAM_CAP_CLABELS) return STREAM_OLD_VERSION_CLABELS;
    return STREAM_OLD_VERSION_CLAIM; // if(caps & STREAM_CAP_CLAIM)
}

