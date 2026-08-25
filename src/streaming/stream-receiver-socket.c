// SPDX-License-Identifier: GPL-3.0-or-later

#include "stream-receiver-socket.h"

#if defined(__APPLE__) && defined(TCP_KEEPALIVE)
#define STREAM_RECEIVER_TCP_KEEPIDLE TCP_KEEPALIVE
#elif defined(TCP_KEEPIDLE)
#define STREAM_RECEIVER_TCP_KEEPIDLE TCP_KEEPIDLE
#endif

static bool stream_receiver_socket_keepalive_enable(int fd, bool enabled) {
    int value = enabled ? 1 : 0;
    return setsockopt(fd, SOL_SOCKET, SO_KEEPALIVE, (char *)&value, sizeof(value)) == 0;
}

static bool stream_receiver_socket_keepalive_base(int fd) {
    int interval = STREAM_RECEIVER_KEEPALIVE_INTERVAL_SECONDS;
    int count = STREAM_RECEIVER_KEEPALIVE_PROBE_COUNT;
    bool success = stream_receiver_socket_keepalive_enable(fd, true);

#ifdef TCP_KEEPINTVL
    if(setsockopt(fd, IPPROTO_TCP, TCP_KEEPINTVL, (char *)&interval, sizeof(interval)) != 0)
        success = false;
#else
    (void)interval;
#endif

#ifdef TCP_KEEPCNT
    if(setsockopt(fd, IPPROTO_TCP, TCP_KEEPCNT, (char *)&count, sizeof(count)) != 0)
        success = false;
#else
    (void)count;
#endif

    return success;
}

STREAM_RECEIVER_KEEPALIVE_RESULT stream_receiver_socket_keepalive_reconcile(
    int fd,
    bool enabled,
    uint32_t idle_s,
    STREAM_RECEIVER_KEEPALIVE_STATE *state) {

    if(!state)
        return STREAM_RECEIVER_KEEPALIVE_FAILED;

    if(state->non_tcp)
        return STREAM_RECEIVER_KEEPALIVE_UNCHANGED;

    if(state->base_attempted && state->attempted_enabled == enabled &&
       (!enabled || state->attempted_idle_s == idle_s))
        return STREAM_RECEIVER_KEEPALIVE_UNCHANGED;

    if(state->option_unsupported && enabled && state->base_applied) {
        state->attempted_enabled = true;
        state->attempted_idle_s = idle_s;
        return STREAM_RECEIVER_KEEPALIVE_UNCHANGED;
    }

    if(fd < 0) {
        state->base_attempted = true;
        state->attempted_enabled = enabled;
        state->attempted_idle_s = idle_s;
        return STREAM_RECEIVER_KEEPALIVE_FAILED;
    }

    if(!state->base_attempted) {
        state->base_attempted = true;

        struct sockaddr_storage address = { 0 };
        socklen_t address_length = sizeof(address);
        if(getsockname(fd, (struct sockaddr *)&address, &address_length) != 0) {
            state->attempted_enabled = enabled;
            state->attempted_idle_s = idle_s;
            return STREAM_RECEIVER_KEEPALIVE_FAILED;
        }

        if(address.ss_family != AF_INET && address.ss_family != AF_INET6) {
            state->non_tcp = true;
            state->attempted_enabled = enabled;
            state->attempted_idle_s = idle_s;
            return STREAM_RECEIVER_KEEPALIVE_NON_TCP;
        }
    }

    // Remember failed attempts separately from successful application so periodic reconciliation does not create syscall/log storms.
    state->attempted_enabled = enabled;
    state->attempted_idle_s = idle_s;

    if(!enabled) {
        if(!stream_receiver_socket_keepalive_enable(fd, false))
            return STREAM_RECEIVER_KEEPALIVE_FAILED;

        state->base_applied = false;
        state->applied_enabled = false;
        state->applied_idle_s = 0;
        return STREAM_RECEIVER_KEEPALIVE_APPLIED;
    }

    if(!state->base_applied)
        state->base_applied = stream_receiver_socket_keepalive_base(fd);

    if(state->option_unsupported) {
        state->applied_enabled = state->base_applied;
        return state->base_applied ? STREAM_RECEIVER_KEEPALIVE_OPTION_UNSUPPORTED :
                                     STREAM_RECEIVER_KEEPALIVE_BASE_FAILED;
    }

#ifdef STREAM_RECEIVER_TCP_KEEPIDLE
    int value = (int)idle_s;
    if(setsockopt(fd, IPPROTO_TCP, STREAM_RECEIVER_TCP_KEEPIDLE, (char *)&value, sizeof(value)) != 0)
        return STREAM_RECEIVER_KEEPALIVE_FAILED;

    if(!state->base_applied)
        return STREAM_RECEIVER_KEEPALIVE_BASE_FAILED;

    state->applied_enabled = true;
    state->applied_idle_s = idle_s;
    return STREAM_RECEIVER_KEEPALIVE_APPLIED;
#else
    state->option_unsupported = true;
    state->applied_enabled = state->base_applied;
    return STREAM_RECEIVER_KEEPALIVE_OPTION_UNSUPPORTED;
#endif
}

static bool stream_receiver_socket_unittest_pair(int *listener, int *client, int *accepted) {
    *listener = *client = *accepted = -1;

    *listener = socket(AF_INET, SOCK_STREAM, 0);
    if(*listener < 0)
        return false;

    struct sockaddr_in address = {
        .sin_family = AF_INET,
        .sin_addr.s_addr = htonl(INADDR_LOOPBACK),
        .sin_port = 0,
    };
    if(bind(*listener, (struct sockaddr *)&address, sizeof(address)) != 0 || listen(*listener, 1) != 0)
        return false;

    socklen_t length = sizeof(address);
    if(getsockname(*listener, (struct sockaddr *)&address, &length) != 0)
        return false;

    *client = socket(AF_INET, SOCK_STREAM, 0);
    if(*client < 0 || connect(*client, (struct sockaddr *)&address, sizeof(address)) != 0)
        return false;

    *accepted = accept(*listener, NULL, NULL);
    return *accepted >= 0;
}

int stream_receiver_socket_unittest(void) {
#ifndef STREAM_RECEIVER_TCP_KEEPIDLE
    int listener, client, accepted;
    if(!stream_receiver_socket_unittest_pair(&listener, &client, &accepted)) {
        fprintf(stderr, "STREAM RECEIVER SOCKET TEST: cannot create loopback TCP connection\n");
        if(listener >= 0) close(listener);
        if(client >= 0) close(client);
        if(accepted >= 0) close(accepted);
        return 1;
    }

    STREAM_RECEIVER_KEEPALIVE_STATE state = { 0 };
    int errors = 0;
    if(stream_receiver_socket_keepalive_reconcile(accepted, true, 30, &state) !=
           STREAM_RECEIVER_KEEPALIVE_OPTION_UNSUPPORTED ||
       !state.base_attempted || !state.base_applied || !state.option_unsupported || state.applied_idle_s != 0)
        errors++;
    if(stream_receiver_socket_keepalive_reconcile(accepted, true, 60, &state) != STREAM_RECEIVER_KEEPALIVE_UNCHANGED)
        errors++;
    if(stream_receiver_socket_keepalive_reconcile(accepted, false, 0, &state) != STREAM_RECEIVER_KEEPALIVE_APPLIED ||
       state.base_applied || state.applied_enabled)
        errors++;
    if(stream_receiver_socket_keepalive_reconcile(accepted, true, 60, &state) !=
           STREAM_RECEIVER_KEEPALIVE_OPTION_UNSUPPORTED || !state.base_applied)
        errors++;

    close(accepted);
    close(client);
    close(listener);
    return errors;
#else
    int listener, client, accepted;
    if(!stream_receiver_socket_unittest_pair(&listener, &client, &accepted)) {
        fprintf(stderr, "STREAM RECEIVER SOCKET TEST: cannot create loopback TCP connection\n");
        if(listener >= 0) close(listener);
        if(client >= 0) close(client);
        if(accepted >= 0) close(accepted);
        return 1;
    }

    STREAM_RECEIVER_KEEPALIVE_STATE state = { 0 };
    int errors = 0;
    if(stream_receiver_socket_keepalive_reconcile(accepted, false, 0, &state) != STREAM_RECEIVER_KEEPALIVE_APPLIED ||
       !state.base_attempted || state.base_applied || state.applied_enabled || state.applied_idle_s != 0)
        errors++;

    int actual = 0;
    socklen_t actual_length = sizeof(actual);
    if(getsockopt(accepted, SOL_SOCKET, SO_KEEPALIVE, (char *)&actual, &actual_length) != 0 || actual != 0)
        errors++;

    if(stream_receiver_socket_keepalive_reconcile(accepted, false, 0, &state) != STREAM_RECEIVER_KEEPALIVE_UNCHANGED)
        errors++;

    if(stream_receiver_socket_keepalive_reconcile(accepted, true, 30, &state) != STREAM_RECEIVER_KEEPALIVE_APPLIED ||
       !state.base_attempted || !state.base_applied || !state.applied_enabled || state.applied_idle_s != 30)
        errors++;

    actual = 0;
    actual_length = sizeof(actual);
    if(getsockopt(accepted, IPPROTO_TCP, STREAM_RECEIVER_TCP_KEEPIDLE, (char *)&actual, &actual_length) != 0 || actual != 30)
        errors++;

    actual = 0;
    actual_length = sizeof(actual);
    if(getsockopt(accepted, SOL_SOCKET, SO_KEEPALIVE, (char *)&actual, &actual_length) != 0 || actual != 1)
        errors++;

#ifdef TCP_KEEPINTVL
    actual = 0;
    actual_length = sizeof(actual);
    if(getsockopt(accepted, IPPROTO_TCP, TCP_KEEPINTVL, (char *)&actual, &actual_length) != 0 ||
       actual != STREAM_RECEIVER_KEEPALIVE_INTERVAL_SECONDS)
        errors++;
#endif

#ifdef TCP_KEEPCNT
    actual = 0;
    actual_length = sizeof(actual);
    if(getsockopt(accepted, IPPROTO_TCP, TCP_KEEPCNT, (char *)&actual, &actual_length) != 0 ||
       actual != STREAM_RECEIVER_KEEPALIVE_PROBE_COUNT)
        errors++;
#endif

    if(stream_receiver_socket_keepalive_reconcile(accepted, true, 120, &state) != STREAM_RECEIVER_KEEPALIVE_APPLIED ||
       state.applied_idle_s != 120)
        errors++;

    actual = 0;
    actual_length = sizeof(actual);
    if(getsockopt(accepted, IPPROTO_TCP, STREAM_RECEIVER_TCP_KEEPIDLE, (char *)&actual, &actual_length) != 0 || actual != 120)
        errors++;

    if(stream_receiver_socket_keepalive_reconcile(accepted, true, 120, &state) != STREAM_RECEIVER_KEEPALIVE_UNCHANGED)
        errors++;

    if(stream_receiver_socket_keepalive_reconcile(accepted, false, 0, &state) != STREAM_RECEIVER_KEEPALIVE_APPLIED ||
       state.base_applied || state.applied_enabled || state.applied_idle_s != 0)
        errors++;

    actual = 1;
    actual_length = sizeof(actual);
    if(getsockopt(accepted, SOL_SOCKET, SO_KEEPALIVE, (char *)&actual, &actual_length) != 0 || actual != 0)
        errors++;

    if(stream_receiver_socket_keepalive_reconcile(accepted, true, 120, &state) != STREAM_RECEIVER_KEEPALIVE_APPLIED ||
       !state.base_applied || !state.applied_enabled || state.applied_idle_s != 120)
        errors++;

    STREAM_RECEIVER_KEEPALIVE_STATE failed = { 0 };
    close(accepted);
    accepted = -1;
    if(stream_receiver_socket_keepalive_reconcile(-1, true, 60, &failed) != STREAM_RECEIVER_KEEPALIVE_FAILED ||
       !failed.base_attempted || failed.base_applied || !failed.attempted_enabled ||
       failed.attempted_idle_s != 60 || failed.applied_idle_s != 0)
        errors++;
    if(stream_receiver_socket_keepalive_reconcile(-1, true, 60, &failed) != STREAM_RECEIVER_KEEPALIVE_UNCHANGED)
        errors++;

    STREAM_RECEIVER_KEEPALIVE_STATE failed_disable = { 0 };
    if(stream_receiver_socket_keepalive_reconcile(-1, false, 0, &failed_disable) != STREAM_RECEIVER_KEEPALIVE_FAILED ||
       !failed_disable.base_attempted || failed_disable.base_applied || failed_disable.attempted_enabled ||
       failed_disable.applied_enabled)
        errors++;
    if(stream_receiver_socket_keepalive_reconcile(-1, false, 0, &failed_disable) != STREAM_RECEIVER_KEEPALIVE_UNCHANGED)
        errors++;

#if defined(AF_UNIX) && !defined(OS_WINDOWS)
    int unix_pair[2] = { -1, -1 };
    if(socketpair(AF_UNIX, SOCK_STREAM, 0, unix_pair) != 0)
        errors++;
    else {
        STREAM_RECEIVER_KEEPALIVE_STATE unix_state = { 0 };
        if(stream_receiver_socket_keepalive_reconcile(unix_pair[0], true, 30, &unix_state) !=
               STREAM_RECEIVER_KEEPALIVE_NON_TCP ||
           !unix_state.base_attempted || unix_state.base_applied || !unix_state.non_tcp ||
           unix_state.applied_idle_s != 0)
            errors++;
        if(stream_receiver_socket_keepalive_reconcile(unix_pair[0], false, 0, &unix_state) !=
               STREAM_RECEIVER_KEEPALIVE_UNCHANGED)
            errors++;
        close(unix_pair[0]);
        close(unix_pair[1]);
    }
#endif

    close(client);
    close(listener);

    if(errors)
        fprintf(stderr, "STREAM RECEIVER SOCKET TESTS FAILED: %d error(s)\n", errors);
    else
        fprintf(stderr, "STREAM RECEIVER SOCKET TESTS PASSED\n");

    return errors;
#endif
}
