#ifndef NETDATA_RRDFUNCTIONS_H
#define NETDATA_RRDFUNCTIONS_H 1

#include "rrd.h"

extern void rrdfunctions_init(RRDHOST *host);
extern void rrdfunctions_destroy(RRDHOST *host);

extern void rrd_collector_started(void);
extern void rrd_collector_finished(void);

extern void rrd_collector_add_function(RRDSET *st, const char *name, const char *help, const char *format, int timeout, bool sync, int (*function)(BUFFER *wb, int timeout, const char *function, void *collector_data, void (*callback)(BUFFER *wb, int code, void *callback_data), void *callback_data), void *collector_data);

extern int rrd_call_function_and_wait(RRDHOST *host, BUFFER *wb, int timeout, const char *name);
extern int rrd_call_function_async(RRDHOST *host, BUFFER *wb, int timeout, const char *name, void (*callback)(BUFFER *wb, int code, void *callback_data), void *callback_data);

extern void rrd_functions_expose(RRDSET *st, BUFFER *wb);

extern void chart_functions2json(RRDSET *st, BUFFER *wb, int tabs, const char *kq, const char *sq);
extern void chart_functions_to_dict(RRDSET *st, DICTIONARY *dict);
extern void host_functions2json(RRDHOST *host, BUFFER *wb, int tabs, const char *kq, const char *sq);

#endif // NETDATA_RRDFUNCTIONS_H