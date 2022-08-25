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
 *    the output of this work is kept in a BUFFER in RRDHOST
 *    the sender thread is signalled via a pipe (also in RRDHOST)
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
        if(netdata_validate_server == NETDATA_SSL_VALID_CERTIFICATE){
            info("Netdata is configured to accept invalid SSL certificate.");
            netdata_validate_server = NETDATA_SSL_INVALID_CERTIFICATE;
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


static inline int should_send_chart_matching(RRDSET *st) {
    // Do not stream anomaly rates charts.
    if (unlikely(st->state->is_ar_chart))
        return false;

    if (rrdset_flag_check(st, RRDSET_FLAG_ANOMALY_DETECTION))
        return ml_streaming_enabled();

    if(!rrdset_flag_check(st, RRDSET_FLAG_UPSTREAM_SEND|RRDSET_FLAG_UPSTREAM_IGNORE)) {
        RRDHOST *host = st->rrdhost;

        if(simple_pattern_matches(host->rrdpush_send_charts_matching, st->id) ||
            simple_pattern_matches(host->rrdpush_send_charts_matching, st->name)) {
            rrdset_flag_clear(st, RRDSET_FLAG_UPSTREAM_IGNORE);
            rrdset_flag_set(st, RRDSET_FLAG_UPSTREAM_SEND);
        }
        else {
            rrdset_flag_clear(st, RRDSET_FLAG_UPSTREAM_SEND);
            rrdset_flag_set(st, RRDSET_FLAG_UPSTREAM_IGNORE);
        }
    }

    return(rrdset_flag_check(st, RRDSET_FLAG_UPSTREAM_SEND));
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

// checks if the current chart definition has been sent
static inline int need_to_send_chart_definition(RRDSET *st) {
    rrdset_check_rdlock(st);

    if(unlikely(!(rrdset_flag_check(st, RRDSET_FLAG_UPSTREAM_EXPOSED))))
        return 1;

    RRDDIM *rd;
    rrddim_foreach_read(rd, st) {
        if(unlikely(!rd->exposed)) {
            #ifdef NETDATA_INTERNAL_CHECKS
            info("host '%s', chart '%s', dimension '%s' flag 'exposed' triggered chart refresh to upstream", st->rrdhost->hostname, st->id, rrddim_id(rd));
            #endif
            return 1;
        }
    }

    return 0;
}

// chart labels
static int send_clabels_callback(const char *name, const char *value, RRDLABEL_SRC ls, void *data) {
    BUFFER *wb = (BUFFER *)data;
    buffer_sprintf(wb, "CLABEL \"%s\" \"%s\" %d\n", name, value, ls);
    return 1;
}
void rrdpush_send_clabels(RRDHOST *host, RRDSET *st) {
    if (st->state && st->state->chart_labels) {
        if(rrdlabels_walkthrough_read(st->state->chart_labels, send_clabels_callback, host->sender->build) > 0)
            buffer_sprintf(host->sender->build,"CLABEL_COMMIT\n");
    }
}

// Send the current chart definition.
// Assumes that collector thread has already called sender_start for mutex / buffer state.
static inline void rrdpush_send_chart_definition_nolock(RRDSET *st) {
    RRDHOST *host = st->rrdhost;

    rrdset_flag_set(st, RRDSET_FLAG_UPSTREAM_EXPOSED);

    // properly set the name for the remote end to parse it
    char *name = "";
    if(likely(st->name)) {
        if(unlikely(strcmp(st->id, st->name))) {
            // they differ
            name = strchr(st->name, '.');
            if(name)
                name++;
            else
                name = "";
        }
    }

    // send the chart
    buffer_sprintf(
            host->sender->build
            , "CHART \"%s\" \"%s\" \"%s\" \"%s\" \"%s\" \"%s\" \"%s\" %ld %d \"%s %s %s %s\" \"%s\" \"%s\"\n"
            , st->id
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
    if (host->sender->version >= STREAM_VERSION_CLABELS)
        rrdpush_send_clabels(host, st);

    // send the dimensions
    RRDDIM *rd;
    rrddim_foreach_read(rd, st) {
        buffer_sprintf(
                host->sender->build
                , "DIMENSION \"%s\" \"%s\" \"%s\" " COLLECTED_NUMBER_FORMAT " " COLLECTED_NUMBER_FORMAT " \"%s %s %s\"\n"
                , rrddim_id(rd)
                , rrddim_name(rd)
                , rrd_algorithm_name(rd->algorithm)
                , rd->multiplier
                , rd->divisor
                , rrddim_flag_check(rd, RRDDIM_FLAG_OBSOLETE)?"obsolete":""
                , rrddim_flag_check(rd, RRDDIM_FLAG_HIDDEN)?"hidden":""
                , rrddim_flag_check(rd, RRDDIM_FLAG_DONT_DETECT_RESETS_OR_OVERFLOWS)?"noreset":""
        );
        rd->exposed = 1;
    }

    // send the chart local custom variables
    RRDSETVAR *rs;
    for(rs = st->variables; rs ;rs = rs->next) {
        if(unlikely(rs->type == RRDVAR_TYPE_CALCULATED && rs->options & RRDVAR_OPTION_CUSTOM_CHART_VAR)) {
            NETDATA_DOUBLE *value = (NETDATA_DOUBLE *) rs->value;

            buffer_sprintf(
                    host->sender->build
                    , "VARIABLE CHART %s = " NETDATA_DOUBLE_FORMAT "\n"
                    , rs->variable
                    , *value
            );
        }
    }

    st->upstream_resync_time = st->last_collected_time.tv_sec + (remote_clock_resync_iterations * st->update_every);
}

// sends the current chart dimensions
static inline void rrdpush_send_chart_metrics_nolock(RRDSET *st, struct sender_state *s) {
    RRDHOST *host = st->rrdhost;
    buffer_sprintf(host->sender->build, "BEGIN \"%s\" %llu", st->id, (st->last_collected_time.tv_sec > st->upstream_resync_time)?st->usec_since_last_update:0);
    if (s->version >= VERSION_GAP_FILLING)
        buffer_sprintf(host->sender->build, " %"PRId64"\n", (int64_t)st->last_collected_time.tv_sec);
    else
        buffer_strcat(host->sender->build, "\n");

    RRDDIM *rd;
    rrddim_foreach_read(rd, st) {
        if(rd->updated && rd->exposed)
            buffer_sprintf(host->sender->build
                           , "SET \"%s\" = " COLLECTED_NUMBER_FORMAT "\n"
                           , rrddim_id(rd)
                           , rd->collected_value
        );
    }
    buffer_strcat(host->sender->build, "END\n");
}

static void rrdpush_sender_thread_spawn(RRDHOST *host);

// Called from the internal collectors to mark a chart obsolete.
void rrdset_push_chart_definition_now(RRDSET *st) {
    RRDHOST *host = st->rrdhost;

    if(unlikely(!host->rrdpush_send_enabled || !should_send_chart_matching(st)))
        return;

    rrdset_rdlock(st);
    sender_start(host->sender);
    rrdpush_send_chart_definition_nolock(st);
    sender_commit(host->sender);
    rrdset_unlock(st);
}

void rrdset_done_push(RRDSET *st) {
    if(unlikely(!should_send_chart_matching(st)))
        return;

    RRDHOST *host = st->rrdhost;

    if(unlikely(host->rrdpush_send_enabled && !host->rrdpush_sender_spawn))
        rrdpush_sender_thread_spawn(host);

    // Handle non-connected case
    if(unlikely(!__atomic_load_n(&host->rrdpush_sender_connected, __ATOMIC_SEQ_CST))) {
        if(unlikely(!host->rrdpush_sender_error_shown))
            error("STREAM %s [send]: not ready - discarding collected metrics.", host->hostname);
        host->rrdpush_sender_error_shown = 1;
        return;
    }
    else if(unlikely(host->rrdpush_sender_error_shown)) {
        info("STREAM %s [send]: sending metrics...", host->hostname);
        host->rrdpush_sender_error_shown = 0;
    }

    sender_start(host->sender);

    if(need_to_send_chart_definition(st))
        rrdpush_send_chart_definition_nolock(st);

    rrdpush_send_chart_metrics_nolock(st, host->sender);

    // signal the sender there are more data
    if(host->rrdpush_sender_pipe[PIPE_WRITE] != -1 && write(host->rrdpush_sender_pipe[PIPE_WRITE], " ", 1) == -1)
        error("STREAM %s [send]: cannot write to internal pipe", host->hostname);

    sender_commit(host->sender);
}

// labels
static int send_labels_callback(const char *name, const char *value, RRDLABEL_SRC ls, void *data) {
    BUFFER *wb = (BUFFER *)data;
    buffer_sprintf(wb, "LABEL \"%s\" = %d \"%s\"\n", name, ls, value);
    return 1;
}
void rrdpush_send_labels(RRDHOST *host) {
    if (!host->host_labels || !rrdhost_flag_check(host, RRDHOST_FLAG_STREAM_LABELS_UPDATE) || (rrdhost_flag_check(host, RRDHOST_FLAG_STREAM_LABELS_STOP)))
        return;

    sender_start(host->sender);

    rrdlabels_walkthrough_read(host->host_labels, send_labels_callback, host->sender->build);
    buffer_sprintf(host->sender->build, "OVERWRITE %s\n", "labels");
    sender_commit(host->sender);

    if(host->rrdpush_sender_pipe[PIPE_WRITE] != -1 && write(host->rrdpush_sender_pipe[PIPE_WRITE], " ", 1) == -1)
        error("STREAM %s [send]: cannot write to internal pipe", host->hostname);

    rrdhost_flag_clear(host, RRDHOST_FLAG_STREAM_LABELS_UPDATE);
}

void rrdpush_claimed_id(RRDHOST *host)
{
    if(unlikely(!host->rrdpush_send_enabled || !__atomic_load_n(&host->rrdpush_sender_connected, __ATOMIC_SEQ_CST)))
        return;
    
    if(host->sender->version < STREAM_VERSION_CLAIM)
        return;

    sender_start(host->sender);
    rrdhost_aclk_state_lock(host);

    buffer_sprintf(host->sender->build, "CLAIMED_ID %s %s\n", host->machine_guid, (host->aclk_state.claimed_id ? host->aclk_state.claimed_id : "NULL") );

    rrdhost_aclk_state_unlock(host);
    sender_commit(host->sender);

    // signal the sender there are more data
    if(host->rrdpush_sender_pipe[PIPE_WRITE] != -1 && write(host->rrdpush_sender_pipe[PIPE_WRITE], " ", 1) == -1)
        error("STREAM %s [send]: cannot write to internal pipe", host->hostname);
}

int connect_to_one_of_destinations(
    struct rrdpush_destinations *destinations,
    int default_port,
    struct timeval *timeout,
    size_t *reconnects_counter,
    char *connected_to,
    size_t connected_to_size,
    struct rrdpush_destinations **destination)
{
    int sock = -1;

    for (struct rrdpush_destinations *d = destinations; d; d = d->next) {
        if (d->disabled_no_proper_reply) {
            d->disabled_no_proper_reply = 0;
            continue;
        } else if (d->disabled_because_of_localhost) {
            continue;
        } else if (d->disabled_already_streaming && (d->disabled_already_streaming + 30 > now_realtime_sec())) {
            continue;
        } else if (d->disabled_because_of_denied_access) {
            d->disabled_because_of_denied_access = 0;
            continue;
        }

        if (reconnects_counter)
            *reconnects_counter += 1;
        sock = connect_to_this(d->destination, default_port, timeout);
        if (sock != -1) {
            if (connected_to && connected_to_size) {
                strncpy(connected_to, d->destination, connected_to_size);
                connected_to[connected_to_size - 1] = '\0';
            }
            *destination = d;
            break;
        }
    }

    return sock;
}

struct rrdpush_destinations *destinations_init(const char *dests) {
    const char *s = dests;
    struct rrdpush_destinations *destinations = NULL, *prev = NULL;
    while(*s) {
        const char *e = s;

        // skip path, moving both s(tart) and e(nd)
        if(*e == '/')
            while(!isspace(*e) && *e != ',') s = ++e;

        // skip separators, moving both s(tart) and e(nd)
        while(isspace(*e) || *e == ',') s = ++e;

        // move e(nd) to the first separator
        while(*e && !isspace(*e) && *e != ',' && *e != '/') e++;

        // is there anything?
        if(!*s || s == e) break;

        char buf[e - s + 1];
        strncpyz(buf, s, e - s);
        struct rrdpush_destinations *d = callocz(1, sizeof(struct rrdpush_destinations));
        strncpyz(d->destination, buf, sizeof(d->destination)-1);
        d->disabled_no_proper_reply = 0;
        d->disabled_because_of_localhost = 0;
        d->disabled_already_streaming = 0;
        d->disabled_because_of_denied_access = 0;
        d->next = NULL;
        if (!destinations) {
            destinations = d;
        } else {
            prev->next = d;
        }
        prev = d;

        s = e;
    }
    return destinations;
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

    if(host->rrdpush_sender_spawn) {
        info("STREAM %s [send]: signaling sending thread to stop...", host->hostname);

        // signal the thread that we want to join it
        host->rrdpush_sender_join = 1;

        // copy the thread id, so that we will be waiting for the right one
        // even if a new one has been spawn
        thr = host->rrdpush_sender_thread;

        // signal it to cancel
        netdata_thread_cancel(host->rrdpush_sender_thread);
    }

    netdata_mutex_unlock(&host->sender->mutex);

    if(thr != 0) {
        info("STREAM %s [send]: waiting for the sending thread to stop...", host->hostname);
        void *result;
        netdata_thread_join(thr, &result);
        info("STREAM %s [send]: sending thread has exited.", host->hostname);
    }
}


// ----------------------------------------------------------------------------
// rrdpush receiver thread

void log_stream_connection(const char *client_ip, const char *client_port, const char *api_key, const char *machine_guid, const char *host, const char *msg) {
    log_access("STREAM: %d '[%s]:%s' '%s' host '%s' api key '%s' machine guid '%s'", gettid(), client_ip, client_port, msg, host, api_key, machine_guid);
}


static void rrdpush_sender_thread_spawn(RRDHOST *host) {
    netdata_mutex_lock(&host->sender->mutex);

    if(!host->rrdpush_sender_spawn) {
        char tag[NETDATA_THREAD_TAG_MAX + 1];
        snprintfz(tag, NETDATA_THREAD_TAG_MAX, "STREAM_SENDER[%s]", host->hostname);

        if(netdata_thread_create(&host->rrdpush_sender_thread, tag, NETDATA_THREAD_OPTION_JOINABLE, rrdpush_sender_thread, (void *) host->sender))
            error("STREAM %s [send]: failed to create new thread for client.", host->hostname);
        else
            host->rrdpush_sender_spawn = 1;
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
            stream_version = MIN((uint32_t) strtoul(value, NULL, 0), STREAMING_PROTOCOL_CURRENT_VERSION);
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
                stream_version = 1;
            }

            if (unlikely(rrdhost_set_system_info_variable(system_info, name, value))) {
                info("STREAM [receive from [%s]:%s]: request has parameter '%s' = '%s', which is not used.",
                     w->client_ip, w->client_port, name, value);
            }
        }
    }

    if (stream_version == UINT_MAX)
        stream_version = 0;

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
    RRDHOST *host = rrdhost_find_by_guid(machine_guid, 0);
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
                    host->hostname,
                    w->client_ip,
                    w->client_port,
                    (int64_t)age);
            }
            else {
                netdata_mutex_unlock(&host->receiver_lock);
                rrdhost_unlock(host);
                rrd_unlock();
                log_stream_connection(w->client_ip, w->client_port, key, host->machine_guid, host->hostname,
                                      "REJECTED - ALREADY CONNECTED");
                info(
                    "STREAM %s [receive from [%s]:%s]: multiple connections for same host detected - "
                    "existing connection is active (within last %"PRId64" sec), rejecting new connection.",
                    host->hostname,
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
    rpt->stream_version    = stream_version;
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
