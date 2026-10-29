// SPDX-License-Identifier: GPL-3.0-or-later

#include "pluginsd_internals.h"
#include "health/health_internals.h"
#include "health/health-alert-entry.h"
#include "streaming/stream-receiver-internals.h"

static bool vnode_test_command(PARSER *parser, const char *command) {
    char *input = strdupz(command);
    int rc = parser_action(parser, input);
    freez(input);
    if(rc)
        netdata_log_error("VNODE unittest: command failed: %s", command);
    return rc == 0;
}

static bool vnode_test_online(RRDHOST *host) {
    return rrdhost_flag_check(host, RRDHOST_FLAG_VIRTUAL_HOST) &&
           rrdhost_flag_check(host, RRDHOST_FLAG_COLLECTOR_ONLINE);
}

static bool vnode_test_offline(RRDHOST *host) {
    return !rrdhost_flag_check(host, RRDHOST_FLAG_VIRTUAL_HOST | RRDHOST_FLAG_COLLECTOR_ONLINE);
}

static int vnode_test_timeouts(void) {
    const char *timed_guid = "a11ce000-0000-4000-8000-000000000001";
    const char *omitted_guid = "a11ce000-0000-4000-8000-000000000002";
    const char *zero_guid = "a11ce000-0000-4000-8000-000000000003";
    struct {
        const char *name;
        const char *guid;
        const char *hostname;
        const char *timeout;
        uint32_t expected;
    } cases[] = {
        { "positive timeout", timed_guid, "vnode-timeout-test", "300", 300 },
        { "omitted timeout on next host", omitted_guid, "vnode-omitted-test", NULL, 0 },
        { "explicit zero on next host", zero_guid, "vnode-zero-test", "0", 0 },
        { "omission clears previous timeout", timed_guid, "vnode-timeout-test", NULL, 0 },
        { "replacement positive timeout", timed_guid, "vnode-timeout-test", "600", 600 },
        { "zero clears previous timeout", timed_guid, "vnode-timeout-test", "0", 0 },
        { "maximum timeout", timed_guid, "vnode-timeout-test", "4294967295", UINT32_MAX },
        { "short timeout for expiry", timed_guid, "vnode-timeout-test", "1", 1 },
    };

    struct parser_user_object user = { .host = localhost, .enabled = 1 };
    PARSER *parser = parser_init(&user, -1, -1, PARSER_INPUT_SPLIT, NULL);
    pluginsd_keywords_init(parser, PARSER_INIT_PLUGINSD);
    int result = 1;
    char command[256];

    for(size_t i = 0; i < _countof(cases); i++) {
        snprintfz(command, sizeof(command), "HOST_DEFINE %s %s", cases[i].guid, cases[i].hostname);
        if(!vnode_test_command(parser, command))
            goto cleanup;

        if(cases[i].timeout) {
            snprintfz(command, sizeof(command), "HOST_LABEL _node_stale_after_seconds %s", cases[i].timeout);
            if(!vnode_test_command(parser, command))
                goto cleanup;
        }
        if(!vnode_test_command(parser, "HOST_DEFINE_END"))
            goto cleanup;

        RRDHOST *host = rrdhost_find_by_guid(cases[i].guid);
        if(!host || host != parser->user.host || host->node_stale_after_seconds != cases[i].expected ||
           !vnode_test_online(host)) {
            netdata_log_error("VNODE unittest '%s': expected online host with timeout %u, got %u",
                              cases[i].name, cases[i].expected, host ? host->node_stale_after_seconds : 0);
            goto cleanup;
        }
        if(parser->user.host_define.parsing_host || parser->user.host_define.hostname ||
           parser->user.host_define.rrdlabels || parser->user.host_define.node_stale_after_seconds) {
            netdata_log_error("VNODE unittest '%s': temporary definition state was not cleared", cases[i].name);
            goto cleanup;
        }
        if(cases[i].expected == UINT32_MAX) {
            time_t earliest_check = nd_time_t_add_saturating(now_realtime_sec(), UINT32_MAX);
            if(!vnode_test_command(parser, "HOST localhost") ||
               parser->user.vnodes.last_host_stale_check < earliest_check) {
                netdata_log_error("VNODE unittest: maximum timeout was treated as an absent timeout");
                goto cleanup;
            }
        }
    }

    RRDHOST *timed = rrdhost_find_by_guid(timed_guid);
    RRDHOST *omitted = rrdhost_find_by_guid(omitted_guid);
    RRDHOST *zero = rrdhost_find_by_guid(zero_guid);

    // Exercise the real inactivity clock and HOST-triggered scan, including its check interval.
    usec_t deadline = now_monotonic_usec() + 5 * USEC_PER_SEC;
    while(rrdhost_flag_check(timed, RRDHOST_FLAG_VIRTUAL_HOST | RRDHOST_FLAG_COLLECTOR_ONLINE) &&
          now_monotonic_usec() < deadline) {
        if(!vnode_test_command(parser, "HOST localhost"))
            goto cleanup;
        sleep_usec(100 * USEC_PER_MS);
    }
    if(rrdhost_flag_check(timed, RRDHOST_FLAG_VIRTUAL_HOST | RRDHOST_FLAG_COLLECTOR_ONLINE) ||
       !vnode_test_online(omitted) || !vnode_test_online(zero)) {
        netdata_log_error("VNODE unittest: only the positive-timeout host should become stale");
        goto cleanup;
    }

    snprintfz(command, sizeof(command), "HOST %s", timed_guid);
    if(!vnode_test_command(parser, command) || parser->user.host != timed || !vnode_test_online(timed)) {
        netdata_log_error("VNODE unittest: HOST did not reactivate the stale vnode");
        goto cleanup;
    }
    result = 0;

cleanup:;
    // Destroy the parser first: its selected host is pinned and its index owns GUID strings.
    pluginsd_process_cleanup(parser);
    const char *guids[] = {timed_guid, omitted_guid, zero_guid};
    rrd_wrlock();
    for(size_t i = 0; i < _countof(guids); i++) {
        RRDHOST *host = rrdhost_find_by_guid(guids[i]);
        if(host)
            rrdhost_free___while_having_rrd_wrlock(host);
    }
    rrd_wrunlock();
    return result;
}

static int vnode_test_shorter_deadline(bool redefine) {
    const char *long_guid = "a11ce000-0000-4000-8000-000000000004";
    const char *short_guid = redefine ? long_guid : "a11ce000-0000-4000-8000-000000000005";
    struct parser_user_object user = { .host = localhost, .enabled = 1 };
    PARSER *parser = parser_init(&user, -1, -1, PARSER_INPUT_SPLIT, NULL);
    pluginsd_keywords_init(parser, PARSER_INIT_PLUGINSD);
    int result = 1;
    char command[256];

    snprintfz(command, sizeof(command), "HOST_DEFINE %s vnode-long-deadline-test", long_guid);
    if(!vnode_test_command(parser, command) ||
       !vnode_test_command(parser, "HOST_LABEL _node_stale_after_seconds 300") ||
       !vnode_test_command(parser, "HOST_DEFINE_END") ||
       !vnode_test_command(parser, "HOST localhost"))
        goto cleanup;

    // Cache the long interval through the real scan, then commit a new or reduced timeout.
    if(parser->user.vnodes.last_host_stale_check < now_realtime_sec() + 290) {
        netdata_log_error("VNODE unittest: long scan interval was not established");
        goto cleanup;
    }
    snprintfz(command, sizeof(command), "HOST_DEFINE %s vnode-short-deadline-test", short_guid);
    if(!vnode_test_command(parser, command) ||
       !vnode_test_command(parser, "HOST_LABEL _node_stale_after_seconds 1") ||
       !vnode_test_command(parser, "HOST_DEFINE_END"))
        goto cleanup;

    RRDHOST *host = rrdhost_find_by_guid(short_guid);
    usec_t deadline = now_monotonic_usec() + USEC_PER_SEC + 500 * USEC_PER_MS;
    while(!vnode_test_offline(host) && now_monotonic_usec() < deadline) {
        if(!vnode_test_command(parser, "HOST localhost"))
            goto cleanup;
        sleep_usec(50 * USEC_PER_MS);
    }
    if(!vnode_test_offline(host)) {
        netdata_log_error("VNODE unittest: %s timeout waited for the old scan deadline",
                          redefine ? "reduced" : "new");
        goto cleanup;
    }
    result = 0;

cleanup:;
    pluginsd_process_cleanup(parser);
    const char *guids[] = {long_guid, short_guid};
    rrd_wrlock();
    for(size_t i = 0; i < _countof(guids); i++) {
        RRDHOST *host = rrdhost_find_by_guid(guids[i]);
        if(host)
            rrdhost_free___while_having_rrd_wrlock(host);
    }
    rrd_wrunlock();
    return result;
}

static int vnode_test_removal(bool scan_before_destroy) {
    const char *guid = "a11ce000-0000-4000-8000-000000000006";
    struct parser_user_object user = { .host = localhost, .enabled = 1 };
    PARSER *parser = parser_init(&user, -1, -1, PARSER_INPUT_SPLIT, NULL);
    pluginsd_keywords_init(parser, PARSER_INIT_PLUGINSD);
    int result = 1;
    char command[256];

    snprintfz(command, sizeof(command), "HOST_DEFINE %s vnode-removal-test", guid);
    if(!vnode_test_command(parser, command) ||
       !vnode_test_command(parser, "HOST_LABEL _node_stale_after_seconds 1") ||
       !vnode_test_command(parser, "HOST_DEFINE_END"))
        goto cleanup;

    RRDHOST *host = rrdhost_find_by_guid(guid);
    usec_t deadline = now_monotonic_usec() + 3 * USEC_PER_SEC;
    while(!vnode_test_offline(host) && now_monotonic_usec() < deadline) {
        if(!vnode_test_command(parser, "HOST localhost"))
            goto cleanup;
        sleep_usec(50 * USEC_PER_MS);
    }
    if(!vnode_test_offline(host)) {
        netdata_log_error("VNODE unittest: host did not expire before removal");
        goto cleanup;
    }

    // Use the same lifetime guard and destruction path as remove-stale-node.
    // The parser switched to localhost above, so its selection pin must be released.
    if(!rw_spinlock_trywrite_lock(&host->metadata_lifetime_lock)) {
        netdata_log_error("VNODE unittest: expired host is still pinned");
        goto cleanup;
    }
    rrdhost_free___consume_metadata_lifetime_writelock(host);
    if(rrdhost_find_by_guid(guid)) {
        netdata_log_error("VNODE unittest: removed host is still indexed");
        goto cleanup;
    }

    if(scan_before_destroy) {
        deadline = now_monotonic_usec() + 3 * USEC_PER_SEC;
        while(JudyLCount(parser->user.vnodes.JudyL, 0, -1, PJE0) && now_monotonic_usec() < deadline) {
            if(!vnode_test_command(parser, "HOST localhost"))
                goto cleanup;
            sleep_usec(50 * USEC_PER_MS);
        }
        if(JudyLCount(parser->user.vnodes.JudyL, 0, -1, PJE0)) {
            netdata_log_error("VNODE unittest: stale scan retained the removed host's GUID");
            goto cleanup;
        }
    }
    result = 0;

cleanup:;
    // Also exercise destruction with an unresolved GUID still in the parser's index.
    pluginsd_process_cleanup(parser);
    rrd_wrlock();
    host = rrdhost_find_by_guid(guid);
    if(host)
        rrdhost_free___while_having_rrd_wrlock(host);
    rrd_wrunlock();
    return result;
}

struct vnode_test_receiver_drain {
    struct receiver_state *receiver;
    bool cancel;
    bool signalled;
};

static void vnode_test_drain_receiver(void *data) {
    struct vnode_test_receiver_drain *drain = data;
    usec_t deadline = now_monotonic_usec() + 3 * USEC_PER_SEC;
    while(!__atomic_load_n(&drain->receiver->exit.shutdown, __ATOMIC_ACQUIRE) &&
          !__atomic_load_n(&drain->cancel, __ATOMIC_ACQUIRE) && now_monotonic_usec() < deadline)
        sleep_usec(USEC_PER_MS);

    drain->signalled = __atomic_load_n(&drain->receiver->exit.shutdown, __ATOMIC_ACQUIRE);
    rrdhost_clear_receiver(drain->receiver, STREAM_HANDSHAKE_DISCONNECT_SIGNALED_TO_STOP);
}

static bool vnode_test_live_state(RRDHOST *host, bool health_enabled) {
    bool acquired = object_state_acquire(&host->state_id, object_state_id(&host->state_id));
    if(acquired)
        object_state_release(&host->state_id);

    return acquired && vnode_test_online(host) &&
           !rrdhost_flag_check(host, RRDHOST_FLAG_ORPHAN | RRDHOST_FLAG_ARCHIVED) &&
           host->health.enabled == health_enabled && host->rrdcalc_root_index && !host->receiver;
}

static int vnode_test_recovery(bool redefine, unsigned peer, bool health_enabled, RRD_DB_MODE memory_mode) {
    const char *guid = "a11ce000-0000-4000-8000-000000000007";
    struct plugind cd = { .update_every = 1, .filename = string_strdupz("vnode-recovery-test") };
    struct parser_user_object user = { .host = localhost, .enabled = 1, .cd = &cd };
    PARSER *parser = parser_init(&user, -1, -1, PARSER_INPUT_SPLIT, NULL);
    pluginsd_keywords_init(parser, PARSER_INIT_PLUGINSD);
    bool saved_health = health_globals.config.enabled;
    RRD_DB_MODE saved_memory_mode = default_rrd_memory_mode;
    health_globals.config.enabled = health_enabled;
    default_rrd_memory_mode = memory_mode;
    bool expected_health = health_enabled && memory_mode != RRD_DB_MODE_NONE;
    struct receiver_state receiver = { .sock.fd = -1, .config.health.enabled = CONFIG_BOOLEAN_YES };
    struct vnode_test_receiver_drain drain = { .receiver = &receiver };
    ND_THREAD *drainer = NULL;
    RRDHOST *host = NULL;
    int result = 1;
    char command[256];

    snprintfz(command, sizeof(command), "HOST_DEFINE %s vnode-recovery-test", guid);
    if(!vnode_test_command(parser, command) ||
       !vnode_test_command(parser, "HOST_LABEL _node_stale_after_seconds 1") ||
       !vnode_test_command(parser, "HOST_DEFINE_END"))
        goto cleanup;

    host = rrdhost_find_by_guid(guid);
    if(!host || !vnode_test_live_state(host, expected_health))
        goto cleanup;

    usec_t deadline = now_monotonic_usec() + 3 * USEC_PER_SEC;
    while(!vnode_test_offline(host) && now_monotonic_usec() < deadline) {
        if(!vnode_test_command(parser, "HOST localhost"))
            goto cleanup;
        sleep_usec(50 * USEC_PER_MS);
    }
    if(!vnode_test_offline(host))
        goto cleanup;

    if(peer) {
        if(rrdhost_set_receiver(host, &receiver) != RRDHOST_SET_RECEIVER_OK)
            goto cleanup;

        if(peer == 1 || peer == 3)
            rrdhost_clear_receiver(&receiver, STREAM_HANDSHAKE_DISCONNECT_SIGNALED_TO_STOP);
        else {
            // The fixture drains through the real disconnect path only after the parser signals it.
            drainer = nd_thread_create("vnode-receiver", NETDATA_THREAD_OPTION_DONT_LOG,
                                       vnode_test_drain_receiver, &drain);
            if(!drainer)
                goto cleanup;
        }
    }

    bool send_definition = redefine;
    if(peer == 3) {
        // Use the orphan reaper's actual archive transition and lifetime guards, without waiting an hour.
        rrd_wrlock();
        if(!rw_spinlock_trywrite_lock(&host->metadata_lifetime_lock)) {
            rrd_wrunlock();
            goto cleanup;
        }
        rrdhost_cleanup_data_collection_and_health(host);
        rw_spinlock_write_unlock(&host->metadata_lifetime_lock);
        rrd_wrunlock();
        if(!rrdhost_flag_check(host, RRDHOST_FLAG_ARCHIVED) || host->rrdcalc_root_index)
            goto cleanup;

        if(!redefine) {
            snprintfz(command, sizeof(command), "HOST %s", guid);
            if(parser_action(parser, command) != PARSER_RC_ERROR || !parser->user.retry ||
               !parser->user.enabled || parser->user.host || parser->user.host_pin)
                goto cleanup;

            // A restarted collector publishes a fresh definition through the normal unarchive owner.
            pluginsd_process_cleanup(parser);
            parser = parser_init(&user, -1, -1, PARSER_INPUT_SPLIT, NULL);
            pluginsd_keywords_init(parser, PARSER_INIT_PLUGINSD);
            send_definition = true;
        }
    }

    snprintfz(command, sizeof(command), send_definition ? "HOST_DEFINE %s vnode-recovery-test" : "HOST %s", guid);
    if(!vnode_test_command(parser, command))
        goto cleanup;
    if(send_definition && (!vnode_test_command(parser, "HOST_LABEL _node_stale_after_seconds 1") ||
                          !vnode_test_command(parser, "HOST_DEFINE_END")))
        goto cleanup;

    if(drainer) {
        nd_thread_join(drainer);
        drainer = NULL;
        if(!drain.signalled)
            goto cleanup;
    }

    if(!vnode_test_command(parser, "CHART recovery.data '' 'Recovery' units family recovery.data line 1 1") ||
       !vnode_test_command(parser, "DIMENSION value '' absolute 1 1") ||
       !vnode_test_command(parser, "BEGIN recovery.data") ||
       !vnode_test_command(parser, "SET value 1") ||
       !vnode_test_command(parser, "END") ||
       !vnode_test_live_state(host, expected_health))
        goto cleanup;

    struct receiver_state contender = { .sock.fd = -1 };
    RRDHOST_SET_RECEIVER_RESULT attached = rrdhost_set_receiver(host, &contender);
    if(attached != RRDHOST_SET_RECEIVER_VNODE_IS_LOCAL) {
        if(attached == RRDHOST_SET_RECEIVER_OK)
            rrdhost_clear_receiver(&contender, STREAM_HANDSHAKE_DISCONNECT_SIGNALED_TO_STOP);
        goto cleanup;
    }
    result = 0;

cleanup:
    if(drainer) {
        __atomic_store_n(&drain.cancel, true, __ATOMIC_RELEASE);
        nd_thread_join(drainer);
    }
    if(host && host->receiver == &receiver)
        rrdhost_clear_receiver(&receiver, STREAM_HANDSHAKE_DISCONNECT_SIGNALED_TO_STOP);
    if(result)
        netdata_log_error("VNODE unittest: %s recovery failed (peer=%u, health=%d, memory=%d)",
                          redefine ? "HOST_DEFINE_END" : "HOST", peer, health_enabled, (int)memory_mode);
    health_globals.config.enabled = saved_health;
    default_rrd_memory_mode = saved_memory_mode;
    pluginsd_process_cleanup(parser);
    string_freez(cd.filename);
    rrd_wrlock();
    host = rrdhost_find_by_guid(guid);
    if(host)
        rrdhost_free___while_having_rrd_wrlock(host);
    rrd_wrunlock();
    return result;
}

struct vnode_test_alert_insert {
    RRDHOST *host;
    RRDSET *st;
    RRD_ALERT_PROTOTYPE *ap;
    bool started;
    bool inserted;
};

static void vnode_test_late_alert_insert(void *ptr) {
    struct vnode_test_alert_insert *insert = ptr;
    __atomic_store_n(&insert->started, true, __ATOMIC_RELEASE);
    insert->inserted = rrdcalc_add_from_prototype(insert->host, insert->st, insert->ap);
}

static bool vnode_test_offline_dyncfg(RRDHOST *host) {
    CLEAN_BUFFER *payload = buffer_create(0, NULL);
    CLEAN_BUFFER *response = buffer_create(0, NULL);
    buffer_strcat(payload,
        "{\"format_version\":1,\"rules\":[{\"enabled\":true,\"type\":\"instance\","
        "\"config\":{\"match\":{\"on\":\"vnode_alert.data\",\"host_labels\":\"*\",\"instance_labels\":\"*\"},"
        "\"value\":{\"update_every\":1,\"database_lookup\":{\"after\":-1,\"before\":0,"
        "\"time_group\":\"average\",\"dims_group\":\"sum\",\"data_source\":\"samples\","
        "\"options\":[],\"dimensions\":\"value\"}},"
        "\"conditions\":{\"warning_condition\":\"$this > 0\",\"critical_condition\":\"\"}}}]}");

    int code = dyncfg_health_cb(NULL, "health:alert:prototype", DYNCFG_CMD_ADD, "vnode_offline_dyncfg",
                                payload, NULL, NULL, response, HTTP_ACCESS_NONE, "unittest", NULL);
    bool ok = code == DYNCFG_RESP_ACCEPTED && dictionary_entries(host->rrdcalc_root_index) == 0;
    if(!ok)
        netdata_log_error("VNODE unittest: offline DynCfg save: code=%d response='%s'", code, buffer_tostring(response));
    if(code == DYNCFG_RESP_ACCEPTED) {
        buffer_flush(response);
        code = dyncfg_health_cb(NULL, "health:alert:prototype:vnode_offline_dyncfg", DYNCFG_CMD_REMOVE,
                                NULL, NULL, NULL, NULL, response, HTTP_ACCESS_NONE, "unittest", NULL);
        ok = ok && code == HTTP_RESP_OK;
    }
    return ok;
}

static int vnode_test_alert_removal(bool teardown, bool redefine) {
    bool saved_health = health_globals.config.enabled;
    health_globals.config.enabled = true;
    const char *guid = "a11ce000-0000-4000-8000-000000000030";
    struct plugind cd = { .update_every = 1, .filename = string_strdupz("vnode-alert-test") };
    struct parser_user_object user = { .host = localhost, .enabled = 1, .cd = &cd };
    PARSER *parser = parser_init(&user, -1, -1, PARSER_INPUT_SPLIT, NULL);
    pluginsd_keywords_init(parser, PARSER_INIT_PLUGINSD);
    PARSER *other = parser_init(&user, -1, -1, PARSER_INPUT_SPLIT, NULL);
    pluginsd_keywords_init(other, PARSER_INIT_PLUGINSD);
    RRDHOST *host = NULL;
    int result = 1;
    ND_THREAD *inserter = NULL;
    bool alert_lock_held = false;
    struct vnode_test_alert_insert insert = { 0 };
    RRD_ALERT_PROTOTYPE ap = { .config = { .name = string_strdupz("vnode_test_alert"), .update_every = 1 } };
    char command[256];

    snprintfz(command, sizeof(command), "HOST_DEFINE %s vnode-alert-test", guid);
    if(!vnode_test_command(parser, command) ||
       !vnode_test_command(parser, "HOST_LABEL _node_stale_after_seconds 1") ||
       !vnode_test_command(parser, "HOST_DEFINE_END") ||
       !vnode_test_command(parser, "CHART vnode_alert.data '' 'Alerts' units family vnode_alert.data line 1 1") ||
       !vnode_test_command(parser, "DIMENSION value '' absolute 1 1") ||
       !vnode_test_command(parser, "BEGIN vnode_alert.data") ||
       !vnode_test_command(parser, "SET value 1") ||
       !vnode_test_command(parser, "END"))
        goto cleanup;

    host = parser->user.host;
    RRDSET *st = rrdset_find(host, "vnode_alert.data", false);
    if(!st || !rrdcalc_add_from_prototype(host, st, &ap) || dictionary_entries(host->rrdcalc_root_index) != 1)
        goto cleanup;

    // A second plugin keeps its own definition and can resume the same vnode after the first stops.
    snprintfz(command, sizeof(command), "HOST_DEFINE %s vnode-alert-test", guid);
    if(!vnode_test_command(other, command) ||
       !vnode_test_command(other, "HOST_LABEL _node_stale_after_seconds 1") ||
       !vnode_test_command(other, "HOST_DEFINE_END"))
        goto cleanup;

    // Hold admission while a prototype application starts, then disconnect before letting it finish.
    dictionary_write_lock(host->rrdcalc_root_index);
    alert_lock_held = true;
    insert = (struct vnode_test_alert_insert) { .host = host, .st = st, .ap = &ap };
    inserter = nd_thread_create("VNODE-ALERT-ADD", NETDATA_THREAD_OPTION_DEFAULT, vnode_test_late_alert_insert, &insert);
    usec_t insert_deadline = now_monotonic_usec() + 5 * USEC_PER_SEC;
    while(!__atomic_load_n(&insert.started, __ATOMIC_ACQUIRE) && now_monotonic_usec() < insert_deadline)
        sleep_usec(USEC_PER_MS);
    if(!__atomic_load_n(&insert.started, __ATOMIC_ACQUIRE))
        goto cleanup;

    if(teardown) {
        // Exercise EOF through the plugin worker, which owns vnode teardown; parser destruction alone does not.
        FILE *input = tmpfile();
        FILE *output = tmpfile();
        if(!input || !output) {
            if(input) fclose(input);
            if(output) fclose(output);
            goto cleanup;
        }
        fprintf(input, "HOST_DEFINE %s vnode-alert-test\nHOST_LABEL _node_stale_after_seconds 1\nHOST_DEFINE_END\n", guid);
        rewind(input);
        struct plugind exiting = { .update_every = 1, .filename = string_strdupz("vnode-exit-test") };
        exiting.unsafe.enabled = true;
        bool retry, protocol_error;
        pluginsd_process(localhost, &exiting, fileno(input), fileno(output), 0, &retry, &protocol_error);
        string_freez(exiting.filename);
        fclose(input);
        fclose(output);
        if(retry || protocol_error)
            goto cleanup;
    }
    else {
        usec_t deadline = now_monotonic_usec() + 5 * USEC_PER_SEC;
        while(!vnode_test_offline(host) && now_monotonic_usec() < deadline) {
            if(!vnode_test_command(parser, "HOST localhost"))
                goto cleanup;
            sleep_usec(50 * USEC_PER_MS);
        }
    }

    dictionary_write_unlock(host->rrdcalc_root_index);
    alert_lock_held = false;
    nd_thread_join(inserter);
    inserter = NULL;
    if(insert.inserted)
        goto cleanup;

    if(!vnode_test_offline(host) || host->health.enabled || dictionary_entries(host->rrdcalc_root_index)) {
        netdata_log_error("VNODE unittest: %s retained offline alerts or enabled health", teardown ? "teardown" : "expiry");
        goto cleanup;
    }

    unsigned removed = 0;
    for(ALARM_ENTRY *ae = host->health_log.alarms; ae; ae = ae->next) {
        if(ae->new_status == RRDCALC_STATUS_REMOVED) {
            if(ae->flags & HEALTH_ENTRY_FLAG_CHILD_DISCONNECTED)
                goto cleanup;
            removed++;
        }
    }
    if(removed != 1 || !rrdhost_flag_check(host, RRDHOST_FLAG_PENDING_HEALTH_INITIALIZATION) ||
       !rrdset_flag_check(st, RRDSET_FLAG_PENDING_HEALTH_INITIALIZATION))
        goto cleanup;

    // A prototype application admitted before the disconnect must not insert after it.
    if(rrdcalc_add_from_prototype(host, st, &ap) || dictionary_entries(host->rrdcalc_root_index))
        goto cleanup;
    if(!vnode_test_offline_dyncfg(host))
        goto cleanup;

    snprintfz(command, sizeof(command), redefine ? "HOST_DEFINE %s vnode-alert-test" : "HOST %s", guid);
    if(!vnode_test_command(other, command) ||
       (redefine && (!vnode_test_command(other, "HOST_LABEL _node_stale_after_seconds 1") ||
                     !vnode_test_command(other, "HOST_DEFINE_END"))) ||
       !vnode_test_live_state(host, true) || !rrdcalc_add_from_prototype(host, st, &ap) ||
       dictionary_entries(host->rrdcalc_root_index) != 1)
        goto cleanup;
    result = 0;

cleanup:
    if(result)
        netdata_log_error("VNODE unittest: alert lifecycle failed (%s, %s)",
                          teardown ? "teardown" : "expiry", redefine ? "definition" : "HOST");
    if(alert_lock_held)
        dictionary_write_unlock(host->rrdcalc_root_index);
    if(inserter)
        nd_thread_join(inserter);
    if(parser)
        pluginsd_process_cleanup(parser);
    pluginsd_process_cleanup(other);
    health_globals.config.enabled = saved_health;
    rrd_alert_config_cleanup(&ap.config);
    string_freez(cd.filename);
    rrd_wrlock();
    host = rrdhost_find_by_guid(guid);
    if(host)
        rrdhost_free___while_having_rrd_wrlock(host);
    rrd_wrunlock();
    return result;
}

int pluginsd_vnodes_unittest(void) {
    health_plugin_init();
    int errors = vnode_test_timeouts();
    errors += vnode_test_shorter_deadline(false);
    errors += vnode_test_shorter_deadline(true);

    errors += vnode_test_removal(true);
    errors += vnode_test_removal(false);
    for(unsigned teardown = 0; teardown < 2; teardown++)
        for(unsigned redefine = 0; redefine < 2; redefine++)
            errors += vnode_test_alert_removal(teardown, redefine);
    for(unsigned redefine = 0; redefine < 2; redefine++) {
        // No peer, a disconnected peer, an attached peer, and an archived peer-disconnected host.
        for(unsigned peer = 0; peer < 4; peer++)
            errors += vnode_test_recovery(redefine, peer, true, RRD_DB_MODE_RAM);
        errors += vnode_test_recovery(redefine, 2, false, RRD_DB_MODE_RAM);
        errors += vnode_test_recovery(redefine, 2, true, RRD_DB_MODE_NONE);
    }
    if(!errors)
        fprintf(stderr, "VNODE PARSER TESTS PASSED\n");
    return errors;
}
