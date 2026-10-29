// SPDX-License-Identifier: GPL-3.0-or-later

#include "pluginsd_internals.h"
#include "health/health_internals.h"
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

    return acquired && vnode_test_online(host) && !rrdhost_flag_check(host, RRDHOST_FLAG_ORPHAN) &&
           host->health.enabled == health_enabled && !host->receiver;
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

        if(peer == 1)
            rrdhost_clear_receiver(&receiver, STREAM_HANDSHAKE_DISCONNECT_SIGNALED_TO_STOP);
        else {
            // The fixture drains through the real disconnect path only after the parser signals it.
            drainer = nd_thread_create("vnode-receiver", NETDATA_THREAD_OPTION_DONT_LOG,
                                       vnode_test_drain_receiver, &drain);
            if(!drainer)
                goto cleanup;
        }
    }

    snprintfz(command, sizeof(command), redefine ? "HOST_DEFINE %s vnode-recovery-test" : "HOST %s", guid);
    if(!vnode_test_command(parser, command))
        goto cleanup;
    if(redefine && (!vnode_test_command(parser, "HOST_LABEL _node_stale_after_seconds 1") ||
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

int pluginsd_vnodes_unittest(void) {
    int errors = vnode_test_timeouts();
    errors += vnode_test_shorter_deadline(false);
    errors += vnode_test_shorter_deadline(true);

    errors += vnode_test_removal(true);
    errors += vnode_test_removal(false);
    for(unsigned redefine = 0; redefine < 2; redefine++) {
        // Normal recovery, a peer that already disconnected, and a peer evicted by local recovery.
        for(unsigned peer = 0; peer < 3; peer++)
            errors += vnode_test_recovery(redefine, peer, true, RRD_DB_MODE_RAM);
        errors += vnode_test_recovery(redefine, 2, false, RRD_DB_MODE_RAM);
        errors += vnode_test_recovery(redefine, 2, true, RRD_DB_MODE_NONE);
    }
    if(!errors)
        fprintf(stderr, "VNODE PARSER TESTS PASSED\n");
    return errors;
}
