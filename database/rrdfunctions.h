#ifndef NETDATA_RRDFUNCTIONS_H
#define NETDATA_RRDFUNCTIONS_H 1

#include "rrd.h"

void rrdfunctions_init(RRDHOST *host);
void rrdfunctions_destroy(RRDHOST *host);

void rrd_collector_started(void);
void rrd_collector_finished(void);

typedef void (*function_data_ready_callback)(BUFFER *wb, int code, void *callback_data);

typedef int (*function_execute_at_collector)(BUFFER *wb, int timeout, const char *function, void *collector_data,
                                             function_data_ready_callback callback, void *callback_data);

void rrd_collector_add_function(RRDHOST *host, RRDSET *st, const char *name, int timeout, const char *help,
                                       bool sync, function_execute_at_collector function, void *collector_data);

int rrd_call_function_and_wait(RRDHOST *host, BUFFER *wb, int timeout, const char *name);

typedef void (*rrd_call_function_async_callback)(BUFFER *wb, int code, void *callback_data);
int rrd_call_function_async(RRDHOST *host, BUFFER *wb, int timeout, const char *name, rrd_call_function_async_callback, void *callback_data);

void rrd_functions_expose_rrdpush(RRDSET *st, BUFFER *wb);

void chart_functions2json(RRDSET *st, BUFFER *wb, int tabs, const char *kq, const char *sq);
void chart_functions_to_dict(DICTIONARY *rrdset_functions_view, DICTIONARY *dst);
void host_functions2json(RRDHOST *host, BUFFER *wb);

uint8_t functions_format_to_content_type(const char *format);
const char *functions_content_type_to_format(HTTP_CONTENT_TYPE content_type);
int rrd_call_function_error(BUFFER *wb, const char *msg, int code);

#endif // NETDATA_RRDFUNCTIONS_H
