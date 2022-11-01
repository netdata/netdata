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
    rrdhost_free_orphan_time    = config_get_number(CONFIG_SECTION_DB, "cleanup orphan hosts after secs", rrdhost_free_orphan_time);

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

    netdata_ssl_ca_path = appconfig_get(&stream_config, CONFIG_SECTION_STREAM, "CApath", "/etc/ssl/certs/");
    netdata_ssl_ca_file = appconfig_get(&stream_config, CONFIG_SECTION_STREAM, "CAfile", "/etc/ssl/certs/certs.pem");
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


static inline bool should_send_chart_matching(RRDSET *st) {
    // get all the flags we need to check, with one atomic operation
    RRDSET_FLAGS flags = rrdset_flag_check(st,
             RRDSET_FLAG_UPSTREAM_SEND
            |RRDSET_FLAG_UPSTREAM_IGNORE
            |RRDSET_FLAG_ANOMALY_RATE_CHART
            |RRDSET_FLAG_ANOMALY_DETECTION);

    if(unlikely(!flags)) {
        RRDHOST *host = st->rrdhost;

        // Do not stream anomaly rates charts.
        if (unlikely(flags & RRDSET_FLAG_ANOMALY_RATE_CHART))
            rrdset_flag_set(st, RRDSET_FLAG_UPSTREAM_IGNORE);

        else if (flags & RRDSET_FLAG_ANOMALY_DETECTION) {
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

#define need_to_send_chart_definition(st) (!rrdset_flag_check(st, RRDSET_FLAG_UPSTREAM_EXPOSED))

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
static inline void rrdpush_send_chart_definition(BUFFER *wb, RRDSET *st) {
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

    st->upstream_resync_time = st->last_collected_time.tv_sec + (remote_clock_resync_iterations * st->update_every);
}

// sends the current chart dimensions
static inline void rrdpush_send_chart_metrics(BUFFER *wb, RRDSET *st, struct sender_state *s) {
    buffer_fast_strcat(wb, "BEGIN \"", 7);
    buffer_fast_strcat(wb, rrdset_id(st), string_strlen(st->id));
    buffer_fast_strcat(wb, "\" ", 2);
    buffer_print_llu(wb, (st->last_collected_time.tv_sec > st->upstream_resync_time)?st->usec_since_last_update:0);

    if (stream_has_capability(s, STREAM_CAP_GAP_FILLING)) {
        buffer_fast_strcat(wb, " ", 1);
        buffer_print_ll(wb, st->last_collected_time.tv_sec);
    }

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
            internal_error(true, "host '%s', chart '%s', dimension '%s' flag 'exposed' is updated but not exposed", rrdhost_hostname(st->rrdhost), rrdset_id(st), rrddim_id(rd));
            // we will include it in the next iteration
            rrdset_flag_clear(st, RRDSET_FLAG_UPSTREAM_EXPOSED);
        }
    }
    rrddim_foreach_done(rd);
    buffer_fast_strcat(wb, "END\n", 4);
}

static void rrdpush_sender_thread_spawn(RRDHOST *host);

// Called from the internal collectors to mark a chart obsolete.
bool rrdset_push_chart_definition_now(RRDSET *st) {
    RRDHOST *host = st->rrdhost;

    if(unlikely(!rrdhost_can_send_definitions_to_parent(host) || !should_send_chart_matching(st)))
        return false;

    BUFFER *wb = sender_start(host->sender);
    rrdpush_send_chart_definition(wb, st);
    sender_commit(host->sender, wb);

    return true;
}

bool rrdpush_incremental_transmission_of_chart_definitions(RRDHOST *host, DICTFE *dictfe, bool restart, bool stop) {
    if(stop || restart)
        dictionary_foreach_done(dictfe);

    if(stop)
        return false;

    RRDSET *st = NULL;

    if(unlikely(!dictfe->dict)) {
        st = dictionary_foreach_start_rw(dictfe, host->rrdset_root_index, DICTIONARY_LOCK_REENTRANT);
    }
    else
        st = dictionary_foreach_next(dictfe);

    do {
        while(st && !need_to_send_chart_definition(st))
            st = dictionary_foreach_next(dictfe);

        if(st && rrdset_push_chart_definition_now(st))
            break;

    } while((st = dictionary_foreach_next(dictfe)));

    if (!st) {
        dictionary_foreach_done(dictfe);
        return false;
    }

    return true;
}

void rrdset_done_push(RRDSET *st) {
    RRDHOST *host = st->rrdhost;

    // fetch the flags we need to check with one atomic operation
    RRDHOST_FLAGS flags = rrdhost_flag_check(host,
              RRDHOST_FLAG_RRDPUSH_SENDER_READY_4_METRICS
            | RRDHOST_FLAG_RRDPUSH_SENDER_LOGGED_STATUS
            | RRDHOST_FLAG_RRDPUSH_SENDER_SPAWN
        );

    // check if we are not connected
    if(unlikely(!(flags & RRDHOST_FLAG_RRDPUSH_SENDER_READY_4_METRICS))) {

        if(unlikely(!(flags & RRDHOST_FLAG_RRDPUSH_SENDER_SPAWN)))
            rrdpush_sender_thread_spawn(host);

        if(unlikely(!(flags & RRDHOST_FLAG_RRDPUSH_SENDER_LOGGED_STATUS))) {
            rrdhost_flag_set(host, RRDHOST_FLAG_RRDPUSH_SENDER_LOGGED_STATUS);
            error("STREAM %s [send]: not ready - collected metrics are not sent to parent.", rrdhost_hostname(host));
        }

        return;
    }
    else if(unlikely(flags & RRDHOST_FLAG_RRDPUSH_SENDER_LOGGED_STATUS)) {
        info("STREAM %s [send]: sending metrics to parent...", rrdhost_hostname(host));
        rrdhost_flag_clear(host, RRDHOST_FLAG_RRDPUSH_SENDER_LOGGED_STATUS);
    }

    if(unlikely(!should_send_chart_matching(st)))
        return;

    BUFFER *wb = sender_start(host->sender);

    if(unlikely(need_to_send_chart_definition(st)))
        rrdpush_send_chart_definition(wb, st);

    rrdpush_send_chart_metrics(wb, st, host->sender);

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

        if(d->postpone_reconnection_until > now) {
            info(
                "STREAM %s: skipping destination '%s' (default port: %d) due to last error (code: %d, %s), will retry it in %d seconds",
                rrdhost_hostname(host),
                string2str(d->destination),
                default_port,
                d->last_handshake, d->last_error?d->last_error:"unset reason description",
                (int)(d->postpone_reconnection_until - now));

            continue;
        }

        info(
            "STREAM %s: attempting to connect to '%s' (default port: %d)...",
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
            DOUBLE_LINKED_LIST_REMOVE_UNSAFE(host->destinations, d, prev, next);
            DOUBLE_LINKED_LIST_APPEND_UNSAFE(host->destinations, d, prev, next);

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

    DOUBLE_LINKED_LIST_APPEND_UNSAFE(t->list, d, prev, next);

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
        DOUBLE_LINKED_LIST_REMOVE_UNSAFE(host->destinations, tmp, prev, next);
        string_freez(tmp->destination);
        freez(tmp);
    }

    host->destinations = NULL;
}

// ----------------------------------------------------------------------------
// rrdpush sender thread

// Either the receiver lost the connection or the host is being destroyed.
// The sender mutex guards thread creation, any spurious data is wiped on reconnection.
void rrdpush_sender_thread_stop(RRDHOST *host) {

    if (!host->sender)
        return;

    netdata_mutex_lock(&host->sender->mutex);
    netdata_thread_t thr = 0;

    if(rrdhost_flag_check(host, RRDHOST_FLAG_RRDPUSH_SENDER_SPAWN)) {
        rrdhost_flag_clear(host, RRDHOST_FLAG_RRDPUSH_SENDER_SPAWN);

        info("STREAM %s [send]: signaling sending thread to stop...", rrdhost_hostname(host));

        // signal the thread that we want to join it
        rrdhost_flag_set(host, RRDHOST_FLAG_RRDPUSH_SENDER_JOIN);

        // copy the thread id, so that we will be waiting for the right one
        // even if a new one has been spawn
        thr = host->rrdpush_sender_thread;

        // signal it to cancel
        netdata_thread_cancel(host->rrdpush_sender_thread);
    }

    netdata_mutex_unlock(&host->sender->mutex);

    if(thr != 0) {
        info("STREAM %s [send]: waiting for the sending thread to stop...", rrdhost_hostname(host));
        void *result;
        netdata_thread_join(thr, &result);
        info("STREAM %s [send]: sending thread has exited.", rrdhost_hostname(host));
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
        snprintfz(tag, NETDATA_THREAD_TAG_MAX, "STREAM_SENDER[%s]", rrdhost_hostname(host));

        if(netdata_thread_create(&host->rrdpush_sender_thread, tag, NETDATA_THREAD_OPTION_JOINABLE, rrdpush_sender_thread, (void *) host->sender))
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
    return 401;
}

int rrdpush_receiver_too_busy_now(struct web_client *w) {
    // we always respond with the same message and error code
    // to prevent an attacker from gaining info about the error
    buffer_flush(w->response.data);
    buffer_sprintf(w->response.data, "The server is too busy now to accept this request. Try later.");
    return 503;
}

void *rrdpush_receiver_thread(void *ptr);
int rrdpush_receiver_thread_spawn(struct web_client *w, char *url) {
    info("clients wants to STREAM metrics.");

    char *key = NULL, *hostname = NULL, *registry_hostname = NULL, *machine_guid = NULL, *os = "unknown", *timezone = "unknown", *abbrev_timezone = "UTC", *tags = NULL;
    int32_t utc_offset = 0;
    int update_every = default_rrd_update_every;
    uint32_t stream_version = UINT_MAX;
    char buf[GUID_LEN + 1];

    struct rrdhost_system_info *system_info = callocz(1, sizeof(struct rrdhost_system_info));
    system_info->hops = 1;
    while(url) {
        char *value = mystrsep(&url, "&");
        if(!value || !*value) continue;

        char *name = mystrsep(&value, "=");
        if(!name || !*name) continue;
        if(!value || !*value) continue;

        if(!strcmp(name, "key"))
            key = value;
        else if(!strcmp(name, "hostname"))
            hostname = value;
        else if(!strcmp(name, "registry_hostname"))
            registry_hostname = value;
        else if(!strcmp(name, "machine_guid"))
            machine_guid = value;
        else if(!strcmp(name, "update_every"))
            update_every = (int)strtoul(value, NULL, 0);
        else if(!strcmp(name, "os"))
            os = value;
        else if(!strcmp(name, "timezone"))
            timezone = value;
        else if(!strcmp(name, "abbrev_timezone"))
            abbrev_timezone = value;
        else if(!strcmp(name, "utc_offset"))
            utc_offset = (int32_t)strtol(value, NULL, 0);
        else if(!strcmp(name, "hops"))
            system_info->hops = (uint16_t) strtoul(value, NULL, 0);
        else if(!strcmp(name, "ml_capable"))
            system_info->ml_capable = strtoul(value, NULL, 0);
        else if(!strcmp(name, "ml_enabled"))
            system_info->ml_enabled = strtoul(value, NULL, 0);
        else if(!strcmp(name, "mc_version"))
            system_info->mc_version = strtoul(value, NULL, 0);
        else if(!strcmp(name, "tags"))
            tags = value;
        else if(!strcmp(name, "ver"))
            stream_version = convert_stream_version_to_capabilities(strtoul(value, NULL, 0));
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
            else if(!strcmp(name, "NETDATA_PROTOCOL_VERSION") && stream_version == UINT_MAX) {
                stream_version = convert_stream_version_to_capabilities(1);
            }

            if (unlikely(rrdhost_set_system_info_variable(system_info, name, value))) {
                info("STREAM [receive from [%s]:%s]: request has parameter '%s' = '%s', which is not used.",
                     w->client_ip, w->client_port, name, value);
            }
        }
    }

    if (stream_version == UINT_MAX)
        stream_version = convert_stream_version_to_capabilities(0);

    if(!key || !*key) {
        rrdhost_system_info_free(system_info);
        log_stream_connection(w->client_ip, w->client_port, (key && *key)?key:"-", (machine_guid && *machine_guid)?machine_guid:"-", (hostname && *hostname)?hostname:"-", "ACCESS DENIED - NO KEY");
        error("STREAM [receive from [%s]:%s]: request without an API key. Forbidding access.", w->client_ip, w->client_port);
        return rrdpush_receiver_permission_denied(w);
    }

    if(!hostname || !*hostname) {
        rrdhost_system_info_free(system_info);
        log_stream_connection(w->client_ip, w->client_port, (key && *key)?key:"-", (machine_guid && *machine_guid)?machine_guid:"-", (hostname && *hostname)?hostname:"-", "ACCESS DENIED - NO HOSTNAME");
        error("STREAM [receive from [%s]:%s]: request without a hostname. Forbidding access.", w->client_ip, w->client_port);
        return rrdpush_receiver_permission_denied(w);
    }

    if(!machine_guid || !*machine_guid) {
        rrdhost_system_info_free(system_info);
        log_stream_connection(w->client_ip, w->client_port, (key && *key)?key:"-", (machine_guid && *machine_guid)?machine_guid:"-", (hostname && *hostname)?hostname:"-", "ACCESS DENIED - NO MACHINE GUID");
        error("STREAM [receive from [%s]:%s]: request without a machine GUID. Forbidding access.", w->client_ip, w->client_port);
        return rrdpush_receiver_permission_denied(w);
    }

    if(regenerate_guid(key, buf) == -1) {
        rrdhost_system_info_free(system_info);
        log_stream_connection(w->client_ip, w->client_port, (key && *key)?key:"-", (machine_guid && *machine_guid)?machine_guid:"-", (hostname && *hostname)?hostname:"-", "ACCESS DENIED - INVALID KEY");
        error("STREAM [receive from [%s]:%s]: API key '%s' is not valid GUID (use the command uuidgen to generate one). Forbidding access.", w->client_ip, w->client_port, key);
        return rrdpush_receiver_permission_denied(w);
    }

    if(regenerate_guid(machine_guid, buf) == -1) {
        rrdhost_system_info_free(system_info);
        log_stream_connection(w->client_ip, w->client_port, (key && *key)?key:"-", (machine_guid && *machine_guid)?machine_guid:"-", (hostname && *hostname)?hostname:"-", "ACCESS DENIED - INVALID MACHINE GUID");
        error("STREAM [receive from [%s]:%s]: machine GUID '%s' is not GUID. Forbidding access.", w->client_ip, w->client_port, machine_guid);
        return rrdpush_receiver_permission_denied(w);
    }

    if(!appconfig_get_boolean(&stream_config, key, "enabled", 0)) {
        rrdhost_system_info_free(system_info);
        log_stream_connection(w->client_ip, w->client_port, (key && *key)?key:"-", (machine_guid && *machine_guid)?machine_guid:"-", (hostname && *hostname)?hostname:"-", "ACCESS DENIED - KEY NOT ENABLED");
        error("STREAM [receive from [%s]:%s]: API key '%s' is not allowed. Forbidding access.", w->client_ip, w->client_port, key);
        return rrdpush_receiver_permission_denied(w);
    }

    {
        SIMPLE_PATTERN *key_allow_from = simple_pattern_create(appconfig_get(&stream_config, key, "allow from", "*"), NULL, SIMPLE_PATTERN_EXACT);
        if(key_allow_from) {
            if(!simple_pattern_matches(key_allow_from, w->client_ip)) {
                simple_pattern_free(key_allow_from);
                rrdhost_system_info_free(system_info);
                log_stream_connection(w->client_ip, w->client_port, (key && *key)?key:"-", (machine_guid && *machine_guid)?machine_guid:"-", (hostname && *hostname) ? hostname : "-", "ACCESS DENIED - KEY NOT ALLOWED FROM THIS IP");
                error("STREAM [receive from [%s]:%s]: API key '%s' is not permitted from this IP. Forbidding access.", w->client_ip, w->client_port, key);
                return rrdpush_receiver_permission_denied(w);
            }
            simple_pattern_free(key_allow_from);
        }
    }

    if(!appconfig_get_boolean(&stream_config, machine_guid, "enabled", 1)) {
        rrdhost_system_info_free(system_info);
        log_stream_connection(w->client_ip, w->client_port, (key && *key)?key:"-", (machine_guid && *machine_guid)?machine_guid:"-", (hostname && *hostname)?hostname:"-", "ACCESS DENIED - MACHINE GUID NOT ENABLED");
        error("STREAM [receive from [%s]:%s]: machine GUID '%s' is not allowed. Forbidding access.", w->client_ip, w->client_port, machine_guid);
        return rrdpush_receiver_permission_denied(w);
    }

    {
        SIMPLE_PATTERN *machine_allow_from = simple_pattern_create(appconfig_get(&stream_config, machine_guid, "allow from", "*"), NULL, SIMPLE_PATTERN_EXACT);
        if(machine_allow_from) {
            if(!simple_pattern_matches(machine_allow_from, w->client_ip)) {
                simple_pattern_free(machine_allow_from);
                rrdhost_system_info_free(system_info);
                log_stream_connection(w->client_ip, w->client_port, (key && *key)?key:"-", (machine_guid && *machine_guid)?machine_guid:"-", (hostname && *hostname) ? hostname : "-", "ACCESS DENIED - MACHINE GUID NOT ALLOWED FROM THIS IP");
                error("STREAM [receive from [%s]:%s]: Machine GUID '%s' is not permitted from this IP. Forbidding access.", w->client_ip, w->client_port, machine_guid);
                return rrdpush_receiver_permission_denied(w);
            }
            simple_pattern_free(machine_allow_from);
        }
    }

    if(unlikely(web_client_streaming_rate_t > 0)) {
        static netdata_mutex_t stream_rate_mutex = NETDATA_MUTEX_INITIALIZER;
        static volatile time_t last_stream_accepted_t = 0;

        netdata_mutex_lock(&stream_rate_mutex);
        time_t now = now_realtime_sec();

        if(unlikely(last_stream_accepted_t == 0))
            last_stream_accepted_t = now;

        if(now - last_stream_accepted_t < web_client_streaming_rate_t) {
            netdata_mutex_unlock(&stream_rate_mutex);
            rrdhost_system_info_free(system_info);
            error("STREAM [receive from [%s]:%s]: too busy to accept new streaming request. Will be allowed in %ld secs.", w->client_ip, w->client_port, (long)(web_client_streaming_rate_t - (now - last_stream_accepted_t)));
            return rrdpush_receiver_too_busy_now(w);
        }

        last_stream_accepted_t = now;
        netdata_mutex_unlock(&stream_rate_mutex);
    }

    /*
     * Quick path for rejecting multiple connections. The lock taken is fine-grained - it only protects the receiver
     * pointer within the host (if a host exists). This protects against multiple concurrent web requests hitting
     * separate threads within the web-server and landing here. The lock guards the thread-shutdown sequence that
     * detaches the receiver from the host. If the host is being created (first time-access) then we also use the
     * lock to prevent race-hazard (two threads try to create the host concurrently, one wins and the other does a
     * lookup to the now-attached structure).
     */
    struct receiver_state *rpt = callocz(1, sizeof(*rpt));

    rrd_rdlock();
    RRDHOST *host = rrdhost_find_by_guid(machine_guid);
    if (unlikely(host && rrdhost_flag_check(host, RRDHOST_FLAG_ARCHIVED))) /* Ignore archived hosts. */
        host = NULL;
    if (host) {
        rrdhost_wrlock(host);
        netdata_mutex_lock(&host->receiver_lock);
        rrdhost_flag_clear(host, RRDHOST_FLAG_ORPHAN);
        host->senders_disconnected_time = 0;
        if (host->receiver != NULL) {
            time_t age = now_realtime_sec() - host->receiver->last_msg_t;
            if (age > 30) {
                host->receiver->shutdown = 1;
                shutdown(host->receiver->fd, SHUT_RDWR);
                host->receiver = NULL;      // Thread holds reference to structure
                info(
                    "STREAM %s [receive from [%s]:%s]: multiple connections for same host detected - "
                    "existing connection is dead (%"PRId64" sec), accepting new connection.",
                    rrdhost_hostname(host),
                    w->client_ip,
                    w->client_port,
                    (int64_t)age);
            }
            else {
                netdata_mutex_unlock(&host->receiver_lock);
                rrdhost_unlock(host);
                rrd_unlock();
                log_stream_connection(w->client_ip, w->client_port, key, host->machine_guid, rrdhost_hostname(host),
                                      "REJECTED - ALREADY CONNECTED");
                info(
                    "STREAM %s [receive from [%s]:%s]: multiple connections for same host detected - "
                    "existing connection is active (within last %"PRId64" sec), rejecting new connection.",
                    rrdhost_hostname(host),
                    w->client_ip,
                    w->client_port,
                    (int64_t)age);
                // Have not set WEB_CLIENT_FLAG_DONT_CLOSE_SOCKET - caller should clean up
                buffer_flush(w->response.data);
                buffer_strcat(w->response.data, "This GUID is already streaming to this server");
                freez(rpt);
                return 409;
            }
        }
        host->receiver = rpt;
        netdata_mutex_unlock(&host->receiver_lock);
        rrdhost_unlock(host);
    }
    rrd_unlock();

    rpt->last_msg_t = now_realtime_sec();

    rpt->host              = host;
    rpt->fd                = w->ifd;
    rpt->key               = strdupz(key);
    rpt->hostname          = strdupz(hostname);
    rpt->registry_hostname = strdupz((registry_hostname && *registry_hostname)?registry_hostname:hostname);
    rpt->machine_guid      = strdupz(machine_guid);
    rpt->os                = strdupz(os);
    rpt->timezone          = strdupz(timezone);
    rpt->abbrev_timezone   = strdupz(abbrev_timezone);
    rpt->utc_offset        = utc_offset;
    rpt->tags              = (tags)?strdupz(tags):NULL;
    rpt->client_ip         = strdupz(w->client_ip);
    rpt->client_port       = strdupz(w->client_port);
    rpt->update_every      = update_every;
    rpt->system_info       = system_info;
    rpt->capabilities = stream_version;
#ifdef ENABLE_HTTPS
    rpt->ssl.conn          = w->ssl.conn;
    rpt->ssl.flags         = w->ssl.flags;

    w->ssl.conn = NULL;
    w->ssl.flags = NETDATA_SSL_START;
#endif

    if(w->user_agent && w->user_agent[0]) {
        char *t = strchr(w->user_agent, '/');
        if(t && *t) {
            *t = '\0';
            t++;
        }

        rpt->program_name = strdupz(w->user_agent);
        if(t && *t) rpt->program_version = strdupz(t);
    }



    debug(D_SYSTEM, "starting STREAM receive thread.");

    char tag[FILENAME_MAX + 1];
    snprintfz(tag, FILENAME_MAX, "STREAM_RECEIVER[%s,[%s]:%s]", rpt->hostname, w->client_ip, w->client_port);

    if(netdata_thread_create(&rpt->thread, tag, NETDATA_THREAD_OPTION_DEFAULT, rrdpush_receiver_thread, (void *)rpt))
        error("Failed to create new STREAM receive thread for client.");

    // prevent the caller from closing the streaming socket
    if(web_server_mode == WEB_SERVER_MODE_STATIC_THREADED) {
        web_client_flag_set(w, WEB_CLIENT_FLAG_DONT_CLOSE_SOCKET);
    }
    else {
        if(w->ifd == w->ofd)
            w->ifd = w->ofd = -1;
        else
            w->ifd = -1;
    }

    buffer_flush(w->response.data);
    return 200;
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
    if(caps & STREAM_CAP_GAP_FILLING) buffer_strcat(wb, "GAP_FILLING ");
}

void log_receiver_capabilities(struct receiver_state *rpt) {
    BUFFER *wb = buffer_create(100);
    stream_capabilities_to_string(wb, rpt->capabilities);

    info("STREAM %s [receive from [%s]:%s]: established link with negotiated capabilities: %s",
         rrdhost_hostname(rpt->host), rpt->client_ip, rpt->client_port, buffer_tostring(wb));

    buffer_free(wb);
}

void log_sender_capabilities(struct sender_state *s) {
    BUFFER *wb = buffer_create(100);
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

