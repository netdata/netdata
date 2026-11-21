// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"
#include "buildinfo.h"

#define INFO_FIELDS_MAX_HITS 255

// the script normally answers in well under a second; this only bounds a hung script
#define NOTIFICATION_METHODS_TIMEOUT_MS 60000

static size_t hits[INFO_FIELDS_HIT_MAX] = { 0 };

// published once, never freed: readers may hold them while the agent runs
static const char *buildinfo_features = NULL;
static const char *notification_methods = NULL;

void info_fields_count_hit(INFO_FIELDS_HIT hit) {
    if(hit >= INFO_FIELDS_HIT_MAX || !telemetry_enabled())
        return;

    size_t value = __atomic_load_n(&hits[hit], __ATOMIC_RELAXED);
    while(value < INFO_FIELDS_MAX_HITS &&
          !__atomic_compare_exchange_n(&hits[hit], &value, value + 1, false, __ATOMIC_RELAXED, __ATOMIC_RELAXED))
        ;
}

void info_fields_buildinfo_init(void) {
    CLEAN_BUFFER *wb = buffer_create(1000, NULL);
    build_info_features(wb);
    __atomic_store_n(&buildinfo_features, strdupz(buffer_tostring(wb)), __ATOMIC_RELEASE);
}

// ----------------------------------------------------------------------------
// notification methods

typedef enum {
    PROBE_READ_EOF,
    PROBE_READ_TIMEOUT,
    PROBE_READ_ERROR,
    PROBE_READ_CANCELLED,
} PROBE_READ;

static int probe_ms_left(usec_t deadline_ut) {
    usec_t now_ut = now_monotonic_usec();
    return now_ut >= deadline_ut ? 0 : (int)((deadline_ut - now_ut + USEC_PER_MS - 1) / USEC_PER_MS);
}

// Read the child's stdout until EOF, the deadline, or shutdown.
// wait_on_socket_or_cancel_with_timeout() also handles pipes on Windows, where poll() only works on sockets.
static PROBE_READ notification_methods_read(int fd, BUFFER *out, usec_t deadline_ut) {
    char buf[1024];

    while(true) {
        int timeout_ms = probe_ms_left(deadline_ut);
        if(!timeout_ms)
            return PROBE_READ_TIMEOUT;

        int rc = wait_on_socket_or_cancel_with_timeout(NULL, fd, timeout_ms, POLLIN, NULL);
        if(rc == -1)
            return PROBE_READ_CANCELLED;
        if(rc == 1)
            return PROBE_READ_TIMEOUT;

        // rc 0 is data, rc 2 is a hangup or an error: read() tells them apart
        ssize_t bytes = read(fd, buf, sizeof(buf));
        if(bytes > 0) {
            buffer_memcat(out, buf, (size_t)bytes);
            continue;
        }
        if(bytes == 0)
            return PROBE_READ_EOF;
        if(rc == 0 && (errno == EINTR || errno == EAGAIN))
            continue;

        return PROBE_READ_ERROR;
    }
}

// One method per output line; the field joins them with '|'.
static void notification_methods_publish(BUFFER *out) {
    CLEAN_BUFFER *methods = buffer_create(buffer_strlen(out) + 1, NULL);

    const char *s = buffer_tostring(out);
    size_t lines = 0;
    while(*s) {
        const char *eol = strchr(s, '\n');
        size_t len = eol ? (size_t)(eol - s) : strlen(s);

        if(lines++)
            buffer_putc(methods, '|');
        buffer_fast_strcat(methods, s, len);

        s += len;
        if(*s == '\n')
            s++;
    }

    __atomic_store_n(&notification_methods, strdupz(buffer_tostring(methods)), __ATOMIC_RELEASE);
}

static void notification_methods_cleanup(void *pptr) {
    struct netdata_static_thread *static_thread = CLEANUP_FUNCTION_GET_PTR(pptr);
    if(!static_thread) return;

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
}

void info_fields_notification_methods_main(void *ptr) {
    CLEANUP_FUNCTION_REGISTER(notification_methods_cleanup) cleanup_ptr = ptr;

    char script[FILENAME_MAX + 1];
    snprintfz(script, sizeof(script), "%s/alarm-notify.sh", netdata_configured_primary_plugins_dir);
    if(access(script, R_OK) != 0) {
        netdata_log_info("Alarm notify script %s not found.", script);
        return;
    }

    // quoted like health quotes it; exec, so that a kill on timeout reaches the script, not the /bin/sh -c wrapper
    char quoted[FILENAME_MAX * 4 + 1];
    if(!sanitize_command_argument_string(quoted, script, sizeof(quoted)))
        return;

    CLEAN_BUFFER *cmd = buffer_create(FILENAME_MAX, NULL);
    buffer_sprintf(cmd, "exec '%s' dump_methods", quoted);

    POPEN_INSTANCE *pi = spawn_popen_run(buffer_tostring(cmd));
    if(!pi)
        return;

    CLEAN_BUFFER *out = buffer_create(1000, NULL);
    usec_t deadline_ut = now_monotonic_usec() + (usec_t)NOTIFICATION_METHODS_TIMEOUT_MS * USEC_PER_MS;
    PROBE_READ r = notification_methods_read(spawn_popen_read_fd(pi), out, deadline_ut);

    if(r == PROBE_READ_EOF) {
        int code = -1;
        SPAWN_TIMEDWAIT_RESULT w = spawn_popen_timedwait(pi, probe_ms_left(deadline_ut), &code);
        if(w == SPAWN_TIMEDWAIT_EXITED) {
            // dump_methods exits 0 after printing the full list; anything else is not a list
            if(code == 0)
                notification_methods_publish(out);
            else
                nd_log(NDLS_DAEMON, NDLP_WARNING, "%s dump_methods exited with code %d.", script, code);
            return;
        }
        r = (w == SPAWN_TIMEDWAIT_RUNNING) ? PROBE_READ_TIMEOUT : PROBE_READ_ERROR;
    }

    if(r == PROBE_READ_TIMEOUT)
        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "%s dump_methods did not finish within %d ms; killing it.", script, NOTIFICATION_METHODS_TIMEOUT_MS);
    else if(r == PROBE_READ_ERROR)
        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "Cannot read the output of, or wait for, %s dump_methods; killing it.", script);

    spawn_popen_kill(pi, 0);
}

// ----------------------------------------------------------------------------
// /api/v1/info

static void localhost_charts_and_metrics(size_t *charts, size_t *metrics) {
    size_t c = 0, m = 0;

    RRDSET *st;
    rrdset_foreach_read(st, localhost) {
        if(!rrdset_is_available_for_viewers(st))
            continue;

        c++;

        RRDDIM *rd;
        rrddim_foreach_read(rd, st) {
            if(rrddim_option_check(rd, RRDDIM_OPTION_HIDDEN) || rrddim_flag_check(rd, RRDDIM_FLAG_OBSOLETE))
                continue;
            m++;
        }
        rrddim_foreach_done(rd);
    }
    rrdset_foreach_done(st);

    *charts = c;
    *metrics = m;
}

void info_fields_to_json(BUFFER *wb) {
    buffer_json_member_add_string(wb, "buildinfo", __atomic_load_n(&buildinfo_features, __ATOMIC_ACQUIRE));
    buffer_json_member_add_string(wb, "release-channel", get_release_channel());
    buffer_json_member_add_string(wb, "notification-methods", __atomic_load_n(&notification_methods, __ATOMIC_ACQUIRE));

    buffer_json_member_add_boolean(wb, "exporting-enabled", exporting_info_enabled());
    buffer_json_member_add_string(wb, "exporting-connectors", exporting_info_connectors());

    buffer_json_member_add_uint64(wb, "allmetrics-prometheus-used", __atomic_load_n(&hits[INFO_FIELDS_HIT_PROMETHEUS], __ATOMIC_RELAXED));
    buffer_json_member_add_uint64(wb, "allmetrics-shell-used", __atomic_load_n(&hits[INFO_FIELDS_HIT_SHELL], __ATOMIC_RELAXED));
    buffer_json_member_add_uint64(wb, "allmetrics-json-used", __atomic_load_n(&hits[INFO_FIELDS_HIT_JSON], __ATOMIC_RELAXED));
    buffer_json_member_add_uint64(wb, "dashboard-used", __atomic_load_n(&hits[INFO_FIELDS_HIT_DASHBOARD], __ATOMIC_RELAXED));

    size_t charts = 0, metrics = 0;
    if(localhost)
        localhost_charts_and_metrics(&charts, &metrics);
    buffer_json_member_add_uint64(wb, "charts-count", charts);
    buffer_json_member_add_uint64(wb, "metrics-count", metrics);
}
