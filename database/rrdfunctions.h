#ifndef NETDATA_RRDFUNCTIONS_H
#define NETDATA_RRDFUNCTIONS_H 1

// ----------------------------------------------------------------------------

typedef DICTIONARY RRDFUNCTION_INFLIGHT_INDEX;
typedef const DICTIONARY_ITEM RRDFUNCTION_INFLIGHT_TRANSACTION;

RRDFUNCTION_INFLIGHT_INDEX *rrdfunction_inflight_create_index(void);
void rrdfunction_inflight_destroy_index(RRDFUNCTION_INFLIGHT_INDEX *idx);
RRDFUNCTION_INFLIGHT_TRANSACTION *rrdfunction_inflight_transaction_register_by_id(RRDFUNCTION_INFLIGHT_INDEX *idx, const char *transaction);
RRDFUNCTION_INFLIGHT_TRANSACTION *rrdfunction_inflight_transaction_register(RRDFUNCTION_INFLIGHT_INDEX *idx);
void rrdfunction_inflight_transaction_done(RRDFUNCTION_INFLIGHT_INDEX *idx, RRDFUNCTION_INFLIGHT_TRANSACTION *ftr);
void rrdfunction_inflight_transaction_done_by_id(RRDFUNCTION_INFLIGHT_INDEX *idx, const char *transaction);
void rrdfunction_inflight_transaction_cancel(RRDFUNCTION_INFLIGHT_TRANSACTION *ftr);
void rrdfunction_inflight_transaction_cancel_by_id(RRDFUNCTION_INFLIGHT_INDEX *idx, const char *transaction);
bool rrdfunction_inflight_transaction_is_cancelled(const void *is_cancelled_cb_data);

#include "rrd.h"

void rrdfunctions_init(RRDHOST *host);
void rrdfunctions_destroy(RRDHOST *host);

void rrd_collector_started(void);
void rrd_collector_finished(void);

typedef void (*rrdfunction_result_callback_t)(BUFFER *wb, int code, void *result_cb_data);
typedef bool (*rrdfunction_is_cancelled_cb_t)(const void *is_cancelled_cb_data);

typedef int (*rrdfunction_execute_cb_t)(BUFFER *wb, int timeout, const char *function, void *collector_data,
                                        rrdfunction_result_callback_t result_cb, void *result_cb_data,
                                        rrdfunction_is_cancelled_cb_t is_cancelled_cb, const void *is_cancelled_cb_data);


void rrd_collector_add_function(RRDHOST *host, RRDSET *st, const char *name, int timeout, const char *help,
                                bool sync, rrdfunction_execute_cb_t execute_cb, void *execute_cb_data);

int rrd_call_function_and_wait_from_api(RRDHOST *host, BUFFER *wb, int timeout, const char *name,
                                        rrdfunction_is_cancelled_cb_t is_cancelled_cb, const void *is_cancelled_cb_data);

int rrd_call_function_async_from_streaming(RRDHOST *host, BUFFER *wb, int timeout, const char *name,
                                           rrdfunction_result_callback_t result_cb, void *result_cb_data,
                                           rrdfunction_is_cancelled_cb_t is_cancelled_cb, const void *is_cancelled_cb_data);

void rrd_functions_expose_rrdpush(RRDSET *st, BUFFER *wb);
void rrd_functions_expose_global_rrdpush(RRDHOST *host, BUFFER *wb);

void chart_functions2json(RRDSET *st, BUFFER *wb, int tabs, const char *kq, const char *sq);
void chart_functions_to_dict(DICTIONARY *rrdset_functions_view, DICTIONARY *dst, void *value, size_t value_size);
void host_functions_to_dict(RRDHOST *host, DICTIONARY *dst, void *value, size_t value_size, STRING **help);
void host_functions2json(RRDHOST *host, BUFFER *wb);

uint8_t functions_format_to_content_type(const char *format);
const char *functions_content_type_to_format(HTTP_CONTENT_TYPE content_type);
int rrd_call_function_error(BUFFER *wb, const char *msg, int code);

int rrdhost_function_streaming(BUFFER *wb, int timeout, const char *function, void *collector_data,
                               rrdfunction_result_callback_t result_cb, void *result_cb_data,
                               rrdfunction_is_cancelled_cb_t is_cancelled_cb, const void *is_cancelled_cb_data);

#define RRDFUNCTIONS_STREAMING_HELP "Streaming status for parents and children."

// ----------------------------------------------------------------------------

#endif // NETDATA_RRDFUNCTIONS_H
