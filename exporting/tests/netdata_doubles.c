// SPDX-License-Identifier: GPL-3.0-or-later

#include "test_exporting_engine.h"

// Use memomy allocation functions guarded by CMocka in strdupz
const char *__wrap_strdupz(const char *s)
{
    char *duplicate = malloc(sizeof(char) * (strlen(s) + 1));
    strcpy(duplicate, s);

    return duplicate;
}

time_t __wrap_now_realtime_sec(void)
{
    function_called();
    return mock_type(time_t);
}

void __wrap_info_int(const char *file, const char *function, const unsigned long line, const char *fmt, ...)
{
    (void)file;
    (void)function;
    (void)line;

    function_called();

    va_list args;

    va_start(args, fmt);
    vsnprintf(log_line, MAX_LOG_LINE, fmt, args);
    va_end(args);
}

int __wrap_connect_to_one_of(
    const char *destination,
    int default_port,
    struct timeval *timeout,
    size_t *reconnects_counter,
    char *connected_to,
    size_t connected_to_size)
{
    (void)timeout;

    function_called();

    check_expected(destination);
    check_expected_ptr(default_port);
    // TODO: check_expected_ptr(timeout);
    check_expected(reconnects_counter);
    check_expected(connected_to);
    check_expected(connected_to_size);

    return mock_type(int);
}

void __rrdhost_check_rdlock(RRDHOST *host, const char *file, const char *function, const unsigned long line)
{
    (void)host;
    (void)file;
    (void)function;
    (void)line;
}

void __rrdset_check_rdlock(RRDSET *st, const char *file, const char *function, const unsigned long line)
{
    (void)st;
    (void)file;
    (void)function;
    (void)line;
}

void __rrd_check_rdlock(const char *file, const char *function, const unsigned long line)
{
    (void)file;
    (void)function;
    (void)line;
}

const char *rrd_memory_mode_name(RRD_MEMORY_MODE id)
{
    (void)id;
    return RRD_MEMORY_MODE_NONE_NAME;
}

time_t __mock_rrddim_query_oldest_time(RRDDIM *rd)
{
    (void)rd;

    function_called();
    return mock_type(time_t);
}

time_t __mock_rrddim_query_latest_time(RRDDIM *rd)
{
    (void)rd;

    function_called();
    return mock_type(time_t);
}

void __mock_rrddim_query_init(RRDDIM *rd, struct rrddim_query_handle *handle, time_t start_time, time_t end_time)
{
    (void)rd;
    (void)handle;

    function_called();
    check_expected(start_time);
    check_expected(end_time);
}

int __mock_rrddim_query_is_finished(struct rrddim_query_handle *handle)
{
    (void)handle;

    function_called();
    return mock_type(int);
}

storage_number __mock_rrddim_query_next_metric(struct rrddim_query_handle *handle, time_t *current_time)
{
    (void)handle;
    (void)current_time;

    function_called();
    return mock_type(storage_number);
}

void __mock_rrddim_query_finalize(struct rrddim_query_handle *handle)
{
    (void)handle;

    function_called();
}
