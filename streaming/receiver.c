// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdpush.h"

extern struct config stream_config;

static void rrdpush_receiver_thread_cleanup(void *ptr) {
    static __thread int executed = 0;
    if(!executed) {
        executed = 1;
        struct rrdpush_thread *rpt = (struct rrdpush_thread *) ptr;

        info("STREAM %s [receive from [%s]:%s]: receive thread ended (task id %d)", rpt->hostname, rpt->client_ip, rpt->client_port, gettid());

        freez(rpt->key);
        freez(rpt->hostname);
        freez(rpt->registry_hostname);
        freez(rpt->machine_guid);
        freez(rpt->os);
        freez(rpt->timezone);
        freez(rpt->tags);
        freez(rpt->client_ip);
        freez(rpt->client_port);
        freez(rpt->program_name);
        freez(rpt->program_version);
#ifdef ENABLE_HTTPS
        if(rpt->ssl.conn){
            SSL_free(rpt->ssl.conn);
        }
#endif
        freez(rpt);

    }
}

static int rrdpush_receive(int fd
                           , const char *key
                           , const char *hostname
                           , const char *registry_hostname
                           , const char *machine_guid
                           , const char *os
                           , const char *timezone
                           , const char *tags
                           , const char *program_name
                           , const char *program_version
                           , struct rrdhost_system_info *system_info
                           , int update_every
                           , char *client_ip
                           , char *client_port
                           , uint32_t stream_version
#ifdef ENABLE_HTTPS
                           , struct netdata_ssl *ssl
#endif
) {
    RRDHOST *host;
    int history = default_rrd_history_entries;
    RRD_MEMORY_MODE mode = default_rrd_memory_mode;
    int health_enabled = default_health_enabled;
    int rrdpush_enabled = default_rrdpush_enabled;
    char *rrdpush_destination = default_rrdpush_destination;
    char *rrdpush_api_key = default_rrdpush_api_key;
    char *rrdpush_send_charts_matching = default_rrdpush_send_charts_matching;
    time_t alarms_delay = 60;

    update_every = (int)appconfig_get_number(&stream_config, machine_guid, "update every", update_every);
    if(update_every < 0) update_every = 1;

    history = (int)appconfig_get_number(&stream_config, key, "default history", history);
    history = (int)appconfig_get_number(&stream_config, machine_guid, "history", history);
    if(history < 5) history = 5;

    mode = rrd_memory_mode_id(appconfig_get(&stream_config, key, "default memory mode", rrd_memory_mode_name(mode)));
    mode = rrd_memory_mode_id(appconfig_get(&stream_config, machine_guid, "memory mode", rrd_memory_mode_name(mode)));

    health_enabled = appconfig_get_boolean_ondemand(&stream_config, key, "health enabled by default", health_enabled);
    health_enabled = appconfig_get_boolean_ondemand(&stream_config, machine_guid, "health enabled", health_enabled);

    alarms_delay = appconfig_get_number(&stream_config, key, "default postpone alarms on connect seconds", alarms_delay);
    alarms_delay = appconfig_get_number(&stream_config, machine_guid, "postpone alarms on connect seconds", alarms_delay);

    rrdpush_enabled = appconfig_get_boolean(&stream_config, key, "default proxy enabled", rrdpush_enabled);
    rrdpush_enabled = appconfig_get_boolean(&stream_config, machine_guid, "proxy enabled", rrdpush_enabled);

    rrdpush_destination = appconfig_get(&stream_config, key, "default proxy destination", rrdpush_destination);
    rrdpush_destination = appconfig_get(&stream_config, machine_guid, "proxy destination", rrdpush_destination);

    rrdpush_api_key = appconfig_get(&stream_config, key, "default proxy api key", rrdpush_api_key);
    rrdpush_api_key = appconfig_get(&stream_config, machine_guid, "proxy api key", rrdpush_api_key);

    rrdpush_send_charts_matching = appconfig_get(&stream_config, key, "default proxy send charts matching", rrdpush_send_charts_matching);
    rrdpush_send_charts_matching = appconfig_get(&stream_config, machine_guid, "proxy send charts matching", rrdpush_send_charts_matching);

    tags = appconfig_set_default(&stream_config, machine_guid, "host tags", (tags)?tags:"");
    if(tags && !*tags) tags = NULL;

    if (strcmp(machine_guid, localhost->machine_guid) == 0) {
        log_stream_connection(client_ip, client_port, key, machine_guid, hostname, "DENIED - ATTEMPT TO RECEIVE METRICS FROM MACHINE_GUID IDENTICAL TO MASTER");
        error("STREAM %s [receive from %s:%s]: denied to receive metrics, machine GUID [%s] is my own. Did you copy the master/proxy machine guid to a slave?", hostname, client_ip, client_port, machine_guid);
        close(fd);
        return 1;
    }

    /*
     * Quick path for rejecting multiple connections. Don't take any locks so that progress is made. The same
     * condition will be checked again below, while holding the global and host writer locks. Any potential false
     * positives will not cause harm. Data hazards with host deconstruction will be handled when reference counting
     * is implemented.
     */
    host = rrdhost_find_by_guid(machine_guid, 0);
    if(host && host->connected_senders > 0) {
        log_stream_connection(client_ip, client_port, key, host->machine_guid, host->hostname, "REJECTED - ALREADY CONNECTED");
        info("STREAM %s [receive from [%s]:%s]: multiple streaming connections for the same host detected. Rejecting new connection.", host->hostname, client_ip, client_port);
        close(fd);
        return 0;
    }

    host = rrdhost_find_or_create(
            hostname
            , registry_hostname
            , machine_guid
            , os
            , timezone
            , tags
            , program_name
            , program_version
            , update_every
            , history
            , mode
            , (unsigned int)(health_enabled != CONFIG_BOOLEAN_NO)
            , (unsigned int)(rrdpush_enabled && rrdpush_destination && *rrdpush_destination && rrdpush_api_key && *rrdpush_api_key)
            , rrdpush_destination
            , rrdpush_api_key
            , rrdpush_send_charts_matching
            , system_info
    );

    if(!host) {
        close(fd);
        log_stream_connection(client_ip, client_port, key, machine_guid, hostname, "FAILED - CANNOT ACQUIRE HOST");
        error("STREAM %s [receive from [%s]:%s]: failed to find/create host structure.", hostname, client_ip, client_port);
        return 1;
    }

#ifdef NETDATA_INTERNAL_CHECKS
    info("STREAM %s [receive from [%s]:%s]: client willing to stream metrics for host '%s' with machine_guid '%s': update every = %d, history = %ld, memory mode = %s, health %s, tags '%s'"
         , hostname
         , client_ip
         , client_port
         , host->hostname
         , host->machine_guid
         , host->rrd_update_every
         , host->rrd_history_entries
         , rrd_memory_mode_name(host->rrd_memory_mode)
         , (health_enabled == CONFIG_BOOLEAN_NO)?"disabled":((health_enabled == CONFIG_BOOLEAN_YES)?"enabled":"auto")
         , host->tags?host->tags:""
    );
#endif // NETDATA_INTERNAL_CHECKS


    struct plugind cd = {
            .enabled = 1,
            .update_every = default_rrd_update_every,
            .pid = 0,
            .serial_failures = 0,
            .successful_collections = 0,
            .obsolete = 0,
            .started_t = now_realtime_sec(),
            .next = NULL,
            .version = 0,
    };

    // put the client IP and port into the buffers used by plugins.d
    snprintfz(cd.id,           CONFIG_MAX_NAME,  "%s:%s", client_ip, client_port);
    snprintfz(cd.filename,     FILENAME_MAX,     "%s:%s", client_ip, client_port);
    snprintfz(cd.fullfilename, FILENAME_MAX,     "%s:%s", client_ip, client_port);
    snprintfz(cd.cmd,          PLUGINSD_CMD_MAX, "%s:%s", client_ip, client_port);

    info("STREAM %s [receive from [%s]:%s]: initializing communication...", host->hostname, client_ip, client_port);
    char initial_response[HTTP_HEADER_SIZE];
    if (stream_version > 1) {
        info("STREAM %s [receive from [%s]:%s]: Netdata is using the stream version %u.", host->hostname, client_ip, client_port, stream_version);
        sprintf(initial_response, "%s%u", START_STREAMING_PROMPT_VN, stream_version);
    } else if (stream_version == 1) {
        info("STREAM %s [receive from [%s]:%s]: Netdata is using the stream version %u.", host->hostname, client_ip, client_port, stream_version);
        sprintf(initial_response, "%s", START_STREAMING_PROMPT_V2);
    } else {
        info("STREAM %s [receive from [%s]:%s]: Netdata is using first stream protocol.", host->hostname, client_ip, client_port);
        sprintf(initial_response, "%s", START_STREAMING_PROMPT);
    }
    debug(D_STREAM, "Initial response to %s: %s", client_ip, initial_response);
    #ifdef ENABLE_HTTPS
    host->stream_ssl.conn = ssl->conn;
    host->stream_ssl.flags = ssl->flags;
    if(send_timeout(ssl, fd, initial_response, strlen(initial_response), 0, 60) != (ssize_t)strlen(initial_response)) {
#else
    if(send_timeout(fd, initial_response, strlen(initial_response), 0, 60) != strlen(initial_response)) {
#endif
        log_stream_connection(client_ip, client_port, key, host->machine_guid, host->hostname, "FAILED - CANNOT REPLY");
        error("STREAM %s [receive from [%s]:%s]: cannot send ready command.", host->hostname, client_ip, client_port);
        close(fd);
        return 0;
    }

    // remove the non-blocking flag from the socket
    if(sock_delnonblock(fd) < 0)
        error("STREAM %s [receive from [%s]:%s]: cannot remove the non-blocking flag from socket %d", host->hostname, client_ip, client_port, fd);

    // convert the socket to a FILE *
    FILE *fp = fdopen(fd, "r");
    if(!fp) {
        log_stream_connection(client_ip, client_port, key, host->machine_guid, host->hostname, "FAILED - SOCKET ERROR");
        error("STREAM %s [receive from [%s]:%s]: failed to get a FILE for FD %d.", host->hostname, client_ip, client_port, fd);
        close(fd);
        return 0;
    }

    rrdhost_wrlock(host);
    if(host->connected_senders > 0) {
        rrdhost_unlock(host);
        log_stream_connection(client_ip, client_port, key, host->machine_guid, host->hostname, "REJECTED - ALREADY CONNECTED");
        info("STREAM %s [receive from [%s]:%s]: multiple streaming connections for the same host detected. Rejecting new connection.", host->hostname, client_ip, client_port);
        fclose(fp);
        return 0;
    }

    rrdhost_flag_clear(host, RRDHOST_FLAG_ORPHAN);
    host->connected_senders++;
    host->senders_disconnected_time = 0;
    host->labels_flag = (stream_version > 0)?LABEL_FLAG_UPDATE_STREAM:LABEL_FLAG_STOP_STREAM;

    if(health_enabled != CONFIG_BOOLEAN_NO) {
        if(alarms_delay > 0) {
            host->health_delay_up_to = now_realtime_sec() + alarms_delay;
            info("Postponing health checks for %ld seconds, on host '%s', because it was just connected."
            , alarms_delay
            , host->hostname
            );
        }
    }
    rrdhost_unlock(host);

    // call the plugins.d processor to receive the metrics
    info("STREAM %s [receive from [%s]:%s]: receiving metrics...", host->hostname, client_ip, client_port);
    log_stream_connection(client_ip, client_port, key, host->machine_guid, host->hostname, "CONNECTED");

    cd.version = stream_version;
    // Temporary test message to check the reception inside the sender - this will migrate to a proper state
    // machine that handles replication per-chart as updates are received...
    if (stream_version > 2) {
        char message[128];
        sprintf(message,"REPLICATE dummy 0\n");
        int ret;
#ifdef ENABLE_HTTPS
        SSL *conn = host->stream_ssl.conn ;
        if(conn && !host->stream_ssl.flags) {
            ret = SSL_write(conn, message, strlen(message));
        } else {
            ret = send(fd, message, strlen(message), MSG_DONTWAIT);
        }
#else
        ret = send(fd, message, strlen(message), MSG_DONTWAIT);
#endif
        time_t now = now_realtime_sec(), prev = rrdhost_last_entry_t(host);
        if (prev == 0)
            info("First connection - no gap check");
        else
            info("Checking for gaps %ld vs %ld = %ld-sec gap", now, prev, now-prev);
        info("STREAM %s [receive from [%s]:%s]: Checking for gaps... %d", host->hostname, client_ip, client_port, ret);
    }

    size_t count = pluginsd_process(host, &cd, fp, 1);

    log_stream_connection(client_ip, client_port, key, host->machine_guid, host->hostname, "DISCONNECTED");
    error("STREAM %s [receive from [%s]:%s]: disconnected (completed %zu updates).", host->hostname, client_ip, client_port, count);

    rrdhost_wrlock(host);
    host->senders_disconnected_time = now_realtime_sec();
    host->connected_senders--;
    if(!host->connected_senders) {
        rrdhost_flag_set(host, RRDHOST_FLAG_ORPHAN);
        if(health_enabled == CONFIG_BOOLEAN_AUTO)
            host->health_enabled = 0;
    }
    rrdhost_unlock(host);

    if(host->connected_senders == 0)
        rrdpush_sender_thread_stop(host);

    // cleanup
    fclose(fp);

    return (int)count;
}

void *rrdpush_receiver_thread(void *ptr) {
    netdata_thread_cleanup_push(rrdpush_receiver_thread_cleanup, ptr);

    struct rrdpush_thread *rpt = (struct rrdpush_thread *)ptr;
    info("STREAM %s [%s]:%s: receive thread created (task id %d)", rpt->hostname, rpt->client_ip, rpt->client_port, gettid());

    rrdpush_receive(
	    rpt->fd
	    , rpt->key
	    , rpt->hostname
	    , rpt->registry_hostname
	    , rpt->machine_guid
	    , rpt->os
	    , rpt->timezone
	    , rpt->tags
	    , rpt->program_name
	    , rpt->program_version
        , rpt->system_info
	    , rpt->update_every
	    , rpt->client_ip
	    , rpt->client_port
	    , rpt->stream_version
#ifdef ENABLE_HTTPS
	    , &rpt->ssl
#endif
    );

    netdata_thread_cleanup_pop(1);
    return NULL;
}

