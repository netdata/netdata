// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef TEST_EXPORTING_ENGINE_H
#define TEST_EXPORTING_ENGINE_H 1

#include "libnetdata/libnetdata.h"

#include "exporting/exporting_engine.h"
#include "exporting/graphite/graphite.h"
#include "exporting/json/json.h"
#include "exporting/opentsdb/opentsdb.h"

#if ENABLE_PROMETHEUS_REMOTE_WRITE
#include "exporting/prometheus/remote_write/remote_write.h"
#endif

#if HAVE_KINESIS
#include "exporting/aws_kinesis/aws_kinesis.h"
#endif

#if ENABLE_EXPORTING_PUBSUB
#include "exporting/pubsub/pubsub.h"
#endif

#if HAVE_MONGOC
#include "exporting/mongodb/mongodb.h"
#endif

#include <stdarg.h>
#include <stddef.h>
#include <setjmp.h>
#include <stdint.h>
#include <cmocka.h>

#define MAX_LOG_LINE 1024
extern char log_line[];

// -----------------------------------------------------------------------
// doubles for Netdata functions

const char *__wrap_strdupz(const char *s);
void __wrap_info_int(const char *file, const char *function, const unsigned long line, const char *fmt, ...);
int __wrap_connect_to_one_of(
    const char *destination,
    int default_port,
    struct timeval *timeout,
    size_t *reconnects_counter,
    char *connected_to,
    size_t connected_to_size);
void __rrdhost_check_rdlock(RRDHOST *host, const char *file, const char *function, const unsigned long line);
void __rrdset_check_rdlock(RRDSET *st, const char *file, const char *function, const unsigned long line);
void __rrd_check_rdlock(const char *file, const char *function, const unsigned long line);
time_t __mock_rrddim_query_oldest_time(RRDDIM *rd);
time_t __mock_rrddim_query_latest_time(RRDDIM *rd);
void __mock_rrddim_query_init(RRDDIM *rd, struct rrddim_query_handle *handle, time_t start_time, time_t end_time);
int __mock_rrddim_query_is_finished(struct rrddim_query_handle *handle);
storage_number __mock_rrddim_query_next_metric(struct rrddim_query_handle *handle, time_t *current_time);
void __mock_rrddim_query_finalize(struct rrddim_query_handle *handle);

// -----------------------------------------------------------------------
// wraps for system functions

void __wrap_uv_thread_create(uv_thread_t thread, void (*worker)(void *arg), void *arg);
void __wrap_uv_mutex_lock(uv_mutex_t *mutex);
void __wrap_uv_mutex_unlock(uv_mutex_t *mutex);
void __wrap_uv_cond_signal(uv_cond_t *cond_var);
void __wrap_uv_cond_wait(uv_cond_t *cond_var, uv_mutex_t *mutex);
ssize_t __wrap_recv(int sockfd, void *buf, size_t len, int flags);
ssize_t __wrap_send(int sockfd, const void *buf, size_t len, int flags);

// -----------------------------------------------------------------------
// doubles and originals for exporting engine functions

struct engine *__real_read_exporting_config();
struct engine *__wrap_read_exporting_config();
struct engine *__mock_read_exporting_config();

int __real_init_connectors(struct engine *engine);
int __wrap_init_connectors(struct engine *engine);

int __real_mark_scheduled_instances(struct engine *engine);
int __wrap_mark_scheduled_instances(struct engine *engine);

calculated_number __real_exporting_calculate_value_from_stored_data(
    struct instance *instance,
    RRDDIM *rd,
    time_t *last_timestamp);
calculated_number __wrap_exporting_calculate_value_from_stored_data(
    struct instance *instance,
    RRDDIM *rd,
    time_t *last_timestamp);

int __real_prepare_buffers(struct engine *engine);
int __wrap_prepare_buffers(struct engine *engine);

void __real_create_main_rusage_chart(RRDSET **st_rusage, RRDDIM **rd_user, RRDDIM **rd_system);
void __wrap_create_main_rusage_chart(RRDSET **st_rusage, RRDDIM **rd_user, RRDDIM **rd_system);

void __real_send_main_rusage(RRDSET *st_rusage, RRDDIM *rd_user, RRDDIM *rd_system);
void __wrap_send_main_rusage(RRDSET *st_rusage, RRDDIM *rd_user, RRDDIM *rd_system);

int __real_send_internal_metrics(struct instance *instance);
int __wrap_send_internal_metrics(struct instance *instance);

int __real_rrdhost_is_exportable(struct instance *instance, RRDHOST *host);
int __wrap_rrdhost_is_exportable(struct instance *instance, RRDHOST *host);

int __real_rrdset_is_exportable(struct instance *instance, RRDSET *st);
int __wrap_rrdset_is_exportable(struct instance *instance, RRDSET *st);

int __mock_start_batch_formatting(struct instance *instance);
int __mock_start_host_formatting(struct instance *instance, RRDHOST *host);
int __mock_start_chart_formatting(struct instance *instance, RRDSET *st);
int __mock_metric_formatting(struct instance *instance, RRDDIM *rd);
int __mock_end_chart_formatting(struct instance *instance, RRDSET *st);
int __mock_end_host_formatting(struct instance *instance, RRDHOST *host);
int __mock_end_batch_formatting(struct instance *instance);

int __wrap_simple_connector_end_batch(struct instance *instance);

#if ENABLE_PROMETHEUS_REMOTE_WRITE
void *__real_init_write_request();
void *__wrap_init_write_request();

void __real_add_host_info(
    void *write_request_p,
    const char *name, const char *instance, const char *application, const char *version, const int64_t timestamp);
void __wrap_add_host_info(
    void *write_request_p,
    const char *name, const char *instance, const char *application, const char *version, const int64_t timestamp);

void __real_add_label(void *write_request_p, char *key, char *value);
void __wrap_add_label(void *write_request_p, char *key, char *value);

void __real_add_metric(
    void *write_request_p,
    const char *name, const char *chart, const char *family, const char *dimension,
    const char *instance, const double value, const int64_t timestamp);
void __wrap_add_metric(
    void *write_request_p,
    const char *name, const char *chart, const char *family, const char *dimension,
    const char *instance, const double value, const int64_t timestamp);
#endif /* ENABLE_PROMETHEUS_REMOTE_WRITE */

#if HAVE_KINESIS
void __wrap_aws_sdk_init();
void __wrap_kinesis_init(
    void *kinesis_specific_data_p, const char *region, const char *access_key_id, const char *secret_key,
    const long timeout);
void __wrap_kinesis_put_record(
    void *kinesis_specific_data_p, const char *stream_name, const char *partition_key, const char *data,
    size_t data_len);
int __wrap_kinesis_get_result(void *request_outcomes_p, char *error_message, size_t *sent_bytes, size_t *lost_bytes);
#endif /* HAVE_KINESIS */

#if ENABLE_EXPORTING_PUBSUB
int __wrap_pubsub_init(
    void *pubsub_specific_data_p, char *error_message, const char *destination, const char *credentials_file,
    const char *project_id, const char *topic_id);
int __wrap_pubsub_add_message(void *pubsub_specific_data_p, char *data);
int __wrap_pubsub_publish(
    void *pubsub_specific_data_p, char *error_message, size_t buffered_metrics, size_t buffered_bytes);
int __wrap_pubsub_get_result(
    void *pubsub_specific_data_p, char *error_message,
    size_t *sent_metrics, size_t *sent_bytes, size_t *lost_metrics, size_t *lost_bytes);
#endif /* ENABLE_EXPORTING_PUBSUB */

#if HAVE_MONGOC
void __wrap_mongoc_init();
mongoc_uri_t *__wrap_mongoc_uri_new_with_error(const char *uri_string, bson_error_t *error);
int32_t __wrap_mongoc_uri_get_option_as_int32(const mongoc_uri_t *uri, const char *option, int32_t fallback);
bool __wrap_mongoc_uri_set_option_as_int32(const mongoc_uri_t *uri, const char *option, int32_t value);
mongoc_client_t *__wrap_mongoc_client_new_from_uri(const mongoc_uri_t *uri);
bool __wrap_mongoc_client_set_appname(mongoc_client_t *client, const char *appname);
mongoc_collection_t *
__wrap_mongoc_client_get_collection(mongoc_client_t *client, const char *db, const char *collection);
mongoc_collection_t *
__real_mongoc_client_get_collection(mongoc_client_t *client, const char *db, const char *collection);
void __wrap_mongoc_uri_destroy(mongoc_uri_t *uri);
bool __wrap_mongoc_collection_insert_many(
    mongoc_collection_t *collection,
    const bson_t **documents,
    size_t n_documents,
    const bson_t *opts,
    bson_t *reply,
    bson_error_t *error);
#endif /* HAVE_MONGOC */

// -----------------------------------------------------------------------
// fixtures

int setup_configured_engine(void **state);
int teardown_configured_engine(void **state);
int setup_rrdhost();
int teardown_rrdhost();
int setup_initialized_engine(void **state);
int teardown_initialized_engine(void **state);
int setup_prometheus(void **state);
int teardown_prometheus(void **state);

void init_connectors_in_tests(struct engine *engine);

#endif /* TEST_EXPORTING_ENGINE_H */
