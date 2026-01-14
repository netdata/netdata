// SPDX-License-Identifier: GPL-3.0-or-later

#include "ml_private.h"

#include <array>

#include "ad_charts.h"
#include "database/sqlite/vendored/sqlite3.h"
#include "streaming/stream-control.h"

#define WORKER_TRAIN_QUEUE_POP         0
#define WORKER_TRAIN_ACQUIRE_DIMENSION 1
#define WORKER_TRAIN_QUERY             2
#define WORKER_TRAIN_KMEANS            3
#define WORKER_TRAIN_UPDATE_MODELS     4
#define WORKER_TRAIN_RELEASE_DIMENSION 5
#define WORKER_TRAIN_UPDATE_HOST       6
#define WORKER_TRAIN_FLUSH_MODELS      7

sqlite3 *ml_db = NULL;
static netdata_mutex_t db_mutex;

static void __attribute__((constructor)) init_mutex(void) {
    netdata_mutex_init(&db_mutex);
}

static void __attribute__((destructor)) destroy_mutex(void) {
    netdata_mutex_destroy(&db_mutex);
}

typedef struct {
    // First/last entry of the dimension in DB when generating the response
    time_t first_entry_on_response;
    time_t last_entry_on_response;

    // After/Before timestamps of our DB query
    time_t query_after_t;
    time_t query_before_t;

    // Actual after/before returned by the DB query ops
    time_t db_after_t;
    time_t db_before_t;

    // Number of doubles returned by the DB query
    size_t collected_values;

    // Number of values we return to the caller
    size_t total_values;
} ml_training_response_t;

static std::pair<enum ml_worker_result, ml_training_response_t>
ml_dimension_calculated_numbers(ml_worker_t *worker, ml_dimension_t *dim)
{
    ml_training_response_t training_response = {};

    training_response.first_entry_on_response = rrddim_first_entry_s_of_tier(dim->rd, 0);
    training_response.last_entry_on_response = rrddim_last_entry_s_of_tier(dim->rd, 0);

    unsigned chart_update_every = dim->rd->rrdset->update_every;
    size_t smoothing_window = (chart_update_every > nd_profile.update_every) ? 1 : Cfg.max_samples_to_smooth;
    size_t min_required_samples = Cfg.diff_n + smoothing_window + Cfg.lag_n;

    auto round_up_div = [](time_t window, unsigned step) -> size_t {
        if (window <= 0 || step == 0)
            return 0;
        return static_cast<size_t>((window + step - 1) / step);
    };

    size_t min_n = round_up_div(Cfg.min_training_window, chart_update_every);
    size_t max_n = round_up_div(Cfg.training_window, chart_update_every);

    if (min_n < min_required_samples)
        min_n = min_required_samples;
    if (max_n < min_required_samples)
        max_n = min_required_samples;

    // Figure out what our time window should be.
    training_response.query_before_t = training_response.last_entry_on_response;
    training_response.query_after_t = std::max(
        training_response.query_before_t - Cfg.training_window,  // Fixed time window
        training_response.first_entry_on_response
    );

    if (training_response.query_after_t >= training_response.query_before_t) {
        return { ML_WORKER_RESULT_INVALID_QUERY_TIME_RANGE, training_response };
    }

    if (rrdset_is_replicating(dim->rd->rrdset)) {
        return { ML_WORKER_RESULT_CHART_UNDER_REPLICATION, training_response };
    }

    /*
     * Execute the query
    */
    struct storage_engine_query_handle handle;

    storage_engine_query_init(dim->rd->tiers[0].seb, dim->rd->tiers[0].smh, &handle,
              training_response.query_after_t, training_response.query_before_t,
              STORAGE_PRIORITY_SYNCHRONOUS);

    size_t idx = 0;
    memset(worker->training_cns, 0, sizeof(calculated_number_t) * max_n * (Cfg.lag_n + 1));
    calculated_number_t last_value = std::numeric_limits<calculated_number_t>::quiet_NaN();

    while (!storage_engine_query_is_finished(&handle)) {
        if (idx == max_n)
            break;

        STORAGE_POINT sp = storage_engine_query_next_metric(&handle);

        time_t timestamp = sp.end_time_s;
        calculated_number_t value = sp.sum / sp.count;

        if (netdata_double_isnumber(value)) {
            if (!training_response.db_after_t)
                training_response.db_after_t = timestamp;
            training_response.db_before_t = timestamp;

            worker->training_cns[idx] = value;
            last_value = worker->training_cns[idx];
            training_response.collected_values++;
        } else
            worker->training_cns[idx] = last_value;

        idx++;
    }
    storage_engine_query_finalize(&handle);

    pulse_queries_ml_query_completed(/* points_read */ idx);

    training_response.total_values = idx;
    if (training_response.collected_values < min_n) {
        return { ML_WORKER_RESULT_NOT_ENOUGH_COLLECTED_VALUES, training_response };
    }

    // Find first non-NaN value.
    for (idx = 0; std::isnan(worker->training_cns[idx]); idx++, training_response.total_values--) { }

    // Overwrite NaN values.
    if (idx != 0)
        memmove(worker->training_cns, &worker->training_cns[idx], sizeof(calculated_number_t) * training_response.total_values);

    if (training_response.total_values < min_required_samples)
        return { ML_WORKER_RESULT_NOT_ENOUGH_COLLECTED_VALUES, training_response };

    return { ML_WORKER_RESULT_OK, training_response };
}

const char *db_models_create_table =
    "CREATE TABLE IF NOT EXISTS models("
    "    dim_id BLOB, after INT, before INT,"
    "    min_dist REAL, max_dist REAL,"
    "    c00 REAL, c01 REAL, c02 REAL, c03 REAL, c04 REAL, c05 REAL,"
    "    c10 REAL, c11 REAL, c12 REAL, c13 REAL, c14 REAL, c15 REAL,"
    "    PRIMARY KEY(dim_id, after)"
    ");";

const char *db_models_add_model =
    "INSERT OR REPLACE INTO models("
    "    dim_id, after, before,"
    "    min_dist, max_dist,"
    "    c00, c01, c02, c03, c04, c05,"
    "    c10, c11, c12, c13, c14, c15)"
    "VALUES("
    "    @dim_id, @after, @before,"
    "    @min_dist, @max_dist,"
    "    @c00, @c01, @c02, @c03, @c04, @c05,"
    "    @c10, @c11, @c12, @c13, @c14, @c15);";

const char *db_models_load =
    "SELECT * FROM models "
    "WHERE dim_id = @dim_id AND after >= @after ORDER BY before ASC;";

const char *db_models_delete =
    "DELETE FROM models "
    "WHERE dim_id = @dim_id AND before < @before;";

const char *db_models_prune =
    "DELETE FROM models "
    "WHERE after < @after LIMIT @n;";

static int
ml_dimension_add_model(const nd_uuid_t *metric_uuid, const ml_kmeans_inlined_t *inlined_km)
{
    static __thread sqlite3_stmt *res = NULL;
    int param = 0;
    int rc = 0;

    if (unlikely(!ml_db)) {
        nd_log_limit_static_global_var(erl, 1, 0);
        nd_log_limit(&erl, NDLS_DAEMON, NDLP_ERR, "ML: Database has not been initialized to add ML models");
        return 1;
    }

    if (unlikely(!res)) {
        rc = prepare_statement(ml_db, db_models_add_model, &res);
        if (unlikely(rc != SQLITE_OK)) {
            error_report("Failed to prepare statement to store model, rc = %d", rc);
            return 1;
        }
    }

    rc = sqlite3_bind_blob(res, ++param, metric_uuid, sizeof(*metric_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_int(res, ++param, (int) inlined_km->after);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_int(res, ++param, (int) inlined_km->before);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_double(res, ++param, inlined_km->min_dist);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_double(res, ++param, inlined_km->max_dist);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    for (const DSample &ds : inlined_km->cluster_centers) {
        if (ds.size() != 6)
            fatal("Expected dsample with 6 dimensions, got %ld", ds.size());

        for (long idx = 0; idx != ds.size(); idx++) {
            calculated_number_t cn = ds(idx);
            int rc = sqlite3_bind_double(res, ++param, cn);
            if (unlikely(rc != SQLITE_OK))
                goto bind_fail;
        }
    }

    rc = execute_insert(res);
    if (unlikely(rc != SQLITE_DONE)) {
        error_report("Failed to store model, rc = %d", rc);
        return rc;
    }

    rc = sqlite3_reset(res);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to reset statement when storing model, rc = %d", rc);
        return rc;
    }

    return 0;

bind_fail:
    error_report("Failed to bind parameter %d to store model, rc = %d", param, rc);
    rc = sqlite3_reset(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement to store model, rc = %d", rc);
    return rc;
}

static int
ml_dimension_delete_models(const nd_uuid_t *metric_uuid, time_t before)
{
    static __thread sqlite3_stmt *res = NULL;
    int rc = 0;
    int param = 0;

    if (unlikely(!ml_db)) {
        nd_log_limit_static_global_var(erl, 1, 0);
        nd_log_limit(&erl, NDLS_DAEMON, NDLP_ERR, "ML: Database has not been initialized to delete ML models");
        return 1;
    }

    if (unlikely(!res)) {
        rc = prepare_statement(ml_db, db_models_delete, &res);
        if (unlikely(rc != SQLITE_OK)) {
            error_report("Failed to prepare statement to delete models, rc = %d", rc);
            return rc;
        }
    }

    rc = sqlite3_bind_blob(res, ++param, metric_uuid, sizeof(*metric_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_int(res, ++param, (int) before);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = execute_insert(res);
    if (unlikely(rc != SQLITE_DONE)) {
        error_report("Failed to delete models, rc = %d", rc);
        return rc;
    }

    rc = sqlite3_reset(res);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to reset statement when deleting models, rc = %d", rc);
        return rc;
    }

    return 0;

bind_fail:
    error_report("Failed to bind parameter %d to delete models, rc = %d", param, rc);
    rc = sqlite3_reset(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement to delete models, rc = %d", rc);
    return rc;
}

static int
ml_prune_old_models(size_t num_models_to_prune)
{
    static __thread sqlite3_stmt *res = NULL;
    int rc = 0;
    int param = 0;

    if (unlikely(!ml_db)) {
        nd_log_limit_static_global_var(erl, 1, 0);
        nd_log_limit(&erl, NDLS_DAEMON, NDLP_ERR, "ML: Database has not been initialized to prune old ML models");
        return 1;
    }

    if (unlikely(!res)) {
        rc = prepare_statement(ml_db, db_models_prune, &res);
        if (unlikely(rc != SQLITE_OK)) {
            error_report("Failed to prepare statement to prune models, rc = %d", rc);
            return rc;
        }
    }

    int after = (int) (now_realtime_sec() - Cfg.delete_models_older_than);

    rc = sqlite3_bind_int(res, ++param, after);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_int(res, ++param, num_models_to_prune);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = execute_insert(res);
    if (unlikely(rc != SQLITE_DONE)) {
        error_report("Failed to prune old models, rc = %d", rc);
        return rc;
    }

    rc = sqlite3_reset(res);
    if (unlikely(rc != SQLITE_OK)) {
        error_report("Failed to reset statement when pruning old models, rc = %d", rc);
        return rc;
    }

    return 0;

bind_fail:
    error_report("Failed to bind parameter %d to prune old models, rc = %d", param, rc);
    rc = sqlite3_reset(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement to prune old models, rc = %d", rc);
    return rc;
}

int ml_dimension_load_models(RRDDIM *rd, sqlite3_stmt **active_stmt) {
    ml_dimension_t *dim = (ml_dimension_t *) rd->ml_dimension;
    if (!dim)
        return 0;

    spinlock_lock(&dim->slock);
    bool is_empty = dim->km_contexts.empty();
    spinlock_unlock(&dim->slock);

    if (!is_empty)
        return 0;

    std::vector<ml_kmeans_t> V;

    sqlite3_stmt *res = active_stmt ? *active_stmt : NULL;
    int rc = 0;
    int param = 0;

    if (unlikely(!ml_db)) {
        nd_log_limit_static_global_var(erl, 1, 0);
        nd_log_limit(&erl, NDLS_DAEMON, NDLP_ERR, "ML: Database has not been initialized to load ML models");
        return 1;
    }

    if (unlikely(!res)) {
        rc = sqlite3_prepare_v2(ml_db, db_models_load, -1, &res, NULL);
        if (unlikely(rc != SQLITE_OK)) {
            error_report("Failed to prepare statement to load models, rc = %d", rc);
            return 1;
        }
        if (active_stmt)
            *active_stmt = res;
    }

    nd_uuid_t *rd_uuid = uuidmap_uuid_ptr(dim->rd->uuid);
    rc = sqlite3_bind_blob(res, ++param, rd_uuid, sizeof(*rd_uuid), SQLITE_STATIC);
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    rc = sqlite3_bind_int64(res, ++param, now_realtime_sec() - (Cfg.num_models_to_use * Cfg.train_every));
    if (unlikely(rc != SQLITE_OK))
        goto bind_fail;

    spinlock_lock(&dim->slock);

    dim->km_contexts.reserve(Cfg.num_models_to_use);
    while ((rc = sqlite3_step_monitored(res)) == SQLITE_ROW) {
        ml_kmeans_t km;

        km.after = sqlite3_column_int(res, 2);
        km.before = sqlite3_column_int(res, 3);

        km.min_dist = sqlite3_column_int(res, 4);
        km.max_dist = sqlite3_column_int(res, 5);

        km.cluster_centers.resize(2);

        km.cluster_centers[0].set_size(Cfg.lag_n + 1);
        km.cluster_centers[0](0) = sqlite3_column_double(res, 6);
        km.cluster_centers[0](1) = sqlite3_column_double(res, 7);
        km.cluster_centers[0](2) = sqlite3_column_double(res, 8);
        km.cluster_centers[0](3) = sqlite3_column_double(res, 9);
        km.cluster_centers[0](4) = sqlite3_column_double(res, 10);
        km.cluster_centers[0](5) = sqlite3_column_double(res, 11);

        km.cluster_centers[1].set_size(Cfg.lag_n + 1);
        km.cluster_centers[1](0) = sqlite3_column_double(res, 12);
        km.cluster_centers[1](1) = sqlite3_column_double(res, 13);
        km.cluster_centers[1](2) = sqlite3_column_double(res, 14);
        km.cluster_centers[1](3) = sqlite3_column_double(res, 15);
        km.cluster_centers[1](4) = sqlite3_column_double(res, 16);
        km.cluster_centers[1](5) = sqlite3_column_double(res, 17);

        dim->km_contexts.emplace_back(km);
    }

    if (!dim->km_contexts.empty()) {
        dim->ts = TRAINING_STATUS_TRAINED;
    }

    spinlock_unlock(&dim->slock);

    if (unlikely(rc != SQLITE_DONE))
        error_report("Failed to load models, rc = %d", rc);

    if (active_stmt)
        rc = sqlite3_reset(res);
    else
        rc = sqlite3_finalize(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to %s statement when loading models, rc = %d", active_stmt ? "reset" : "finalize", rc);

    return 0;

bind_fail:
    error_report("Failed to bind parameter %d to load models, rc = %d", param, rc);
    rc = sqlite3_reset(res);
    if (unlikely(rc != SQLITE_OK))
        error_report("Failed to reset statement to load models, rc = %d", rc);
    return 1;
}

static void ml_dimension_serialize_kmeans(const ml_dimension_t *dim, BUFFER *wb)
{
    RRDDIM *rd = dim->rd;

    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);
    buffer_json_member_add_string(wb, "version", "1");
    buffer_json_member_add_string(wb, "machine-guid", rd->rrdset->rrdhost->machine_guid);
    buffer_json_member_add_string(wb, "chart", rrdset_id(rd->rrdset));
    buffer_json_member_add_string(wb, "dimension", rrddim_id(rd));

    buffer_json_member_add_object(wb, "model");
    ml_kmeans_serialize(&dim->km_contexts.back(), wb);
    buffer_json_object_close(wb);

    buffer_json_finalize(wb);
}

bool
ml_dimension_deserialize_kmeans(const char *json_str)
{
    if (!json_str) {
        netdata_log_error("Failed to deserialize kmeans: json string is null");
        return false;
    }

    struct json_object *root = json_tokener_parse(json_str);
    if (!root) {
        netdata_log_error("Failed to deserialize kmeans: json parsing failed");
        return false;
    }

    // Check the version
    {
        struct json_object *tmp_obj;
        if (!json_object_object_get_ex(root, "version", &tmp_obj)) {
            netdata_log_error("Failed to deserialize kmeans: missing key 'version'");
            json_object_put(root);
            return false;
        }
        if (!json_object_is_type(tmp_obj, json_type_string)) {
            netdata_log_error("Failed to deserialize kmeans: failed to parse string for 'version'");
            json_object_put(root);
            return false;
        }
        const char *version = json_object_get_string(tmp_obj);

        if (strcmp(version, "1")) {
            netdata_log_error("Failed to deserialize kmeans: expected version 1");
            json_object_put(root);
            return false;
        }
    }

    // Get the value of each key
    std::array<const char *, 3> values;
    {
        std::array<const char *, 3> keys = {
            "machine-guid",
            "chart",
            "dimension",
        };

        struct json_object *tmp_obj;
        for (size_t i = 0; i != keys.size(); i++) {
            if (!json_object_object_get_ex(root, keys[i], &tmp_obj)) {
                netdata_log_error("Failed to deserialize kmeans: missing key '%s'", keys[i]);
                json_object_put(root);
                return false;
            }
            if (!json_object_is_type(tmp_obj, json_type_string)) {
                netdata_log_error("Failed to deserialize kmeans: missing string value for key '%s'", keys[i]);
                json_object_put(root);
                return false;
            }
            values[i] = json_object_get_string(tmp_obj);
        }
    }

    DimensionLookupInfo DLI(values[0], values[1], values[2]);

    // Parse the kmeans model
    ml_kmeans_inlined_t inlined_km;
    {
        struct json_object *kmeans_obj;
        if (!json_object_object_get_ex(root, "model", &kmeans_obj)) {
            netdata_log_error("Failed to deserialize kmeans: missing key 'model'");
            json_object_put(root);
            return false;
        }
        if (!json_object_is_type(kmeans_obj, json_type_object)) {
            netdata_log_error("Failed to deserialize kmeans: failed to parse object for 'model'");
            json_object_put(root);
            return false;
        }

        if (!ml_kmeans_deserialize(&inlined_km, kmeans_obj)) {
            json_object_put(root);
            return false;
        }
    }

    AcquiredDimension AcqDim(DLI);
    if (!AcqDim.acquired()) {
        json_object_put(root);
        return false;
    }

    ml_dimension_t *Dim = reinterpret_cast<ml_dimension_t *>(AcqDim.dimension());
    if (!Dim) {
        pulse_ml_models_ignored();
        json_object_put(root);
        return true;
    }

    ml_queue_item_t item;
    item.type = ML_QUEUE_ITEM_TYPE_ADD_EXISTING_MODEL;
    item.add_existing_model = {
        DLI, inlined_km
    };
    ml_queue_push(AcqDim.queue(), item);

    json_object_put(root);
    return true;
}

static void ml_dimension_stream_kmeans(ml_worker_t *worker, const ml_dimension_t *dim)
{
    struct sender_state *s = dim->rd->rrdset->rrdhost->sender;
    if (!s)
        return;

    if(!stream_sender_has_capabilities(dim->rd->rrdset->rrdhost, STREAM_CAP_ML_MODELS) ||
        !rrdset_check_upstream_exposed(dim->rd->rrdset) ||
        !rrddim_check_upstream_exposed(dim->rd))
        return;

    // Reuse worker's buffers instead of allocating new ones
    BUFFER *payload = worker->stream_payload_buffer;
    buffer_flush(payload);
    ml_dimension_serialize_kmeans(dim, payload);

    BUFFER *wb = worker->stream_wb_buffer;
    buffer_flush(wb);

    buffer_sprintf(
        wb, PLUGINSD_KEYWORD_JSON " " PLUGINSD_KEYWORD_JSON_CMD_ML_MODEL "\n%s\n" PLUGINSD_KEYWORD_JSON_END "\n",
        buffer_tostring(payload));

    sender_commit_clean_buffer(s, wb, STREAM_TRAFFIC_TYPE_METADATA);
    pulse_ml_models_sent();
}

static void ml_dimension_update_models(ml_worker_t *worker, ml_dimension_t *dim)
{
    worker_is_busy(WORKER_TRAIN_UPDATE_MODELS);

    spinlock_lock(&dim->slock);

    if (dim->km_contexts.size() < Cfg.num_models_to_use) {
        dim->km_contexts.emplace_back(dim->kmeans);
    } else {
        bool can_drop_middle_km = false;

        if (Cfg.num_models_to_use > 2) {
            const ml_kmeans_inlined_t *old_km = &dim->km_contexts[dim->km_contexts.size() - 1];
            const ml_kmeans_inlined_t *middle_km = &dim->km_contexts[dim->km_contexts.size() - 2];
            const ml_kmeans_t *new_km = &dim->kmeans;

            can_drop_middle_km = (middle_km->after < old_km->before) &&
                                 (middle_km->before > new_km->after);
        }

        if (can_drop_middle_km) {
            dim->km_contexts.back() = dim->kmeans;
        } else {
            std::rotate(std::begin(dim->km_contexts), std::begin(dim->km_contexts) + 1, std::end(dim->km_contexts));
            dim->km_contexts[dim->km_contexts.size() - 1] = dim->kmeans;
        }
    }

    dim->mt = METRIC_TYPE_CONSTANT;
    dim->ts = TRAINING_STATUS_TRAINED;

    dim->suppression_anomaly_counter = 0;
    dim->suppression_window_counter = 0;

    // Add the newly generated model to the list of pending models to flush
    ml_model_info_t model_info;
    nd_uuid_t *rd_uuid = uuidmap_uuid_ptr(dim->rd->uuid);
    uuid_copy(model_info.metric_uuid, *rd_uuid);
    model_info.inlined_kmeans = dim->km_contexts.back();
    worker->pending_model_info.push_back(model_info);

    ml_dimension_stream_kmeans(worker, dim);

    // Clear the training in progress flag
    dim->training_in_progress = false;

    spinlock_unlock(&dim->slock);
}

static enum ml_worker_result
ml_dimension_train_model(ml_worker_t *worker, ml_dimension_t *dim)
{
    worker_is_busy(WORKER_TRAIN_QUERY);

    spinlock_lock(&dim->slock);
    if (dim->mt == METRIC_TYPE_CONSTANT) {
        spinlock_unlock(&dim->slock);
        return ML_WORKER_RESULT_OK;
    }

    // Check if training is already in progress for this dimension
    // If so, skip this training request to prevent concurrent access to dim->kmeans
    if (dim->training_in_progress) {
        spinlock_unlock(&dim->slock);
        return ML_WORKER_RESULT_OK;
    }

    // Mark training as in progress
    dim->training_in_progress = true;
    spinlock_unlock(&dim->slock);

    auto P = ml_dimension_calculated_numbers(worker, dim);
    ml_worker_result worker_result = P.first;
    ml_training_response_t training_response = P.second;

    if (worker_result != ML_WORKER_RESULT_OK) {
        spinlock_lock(&dim->slock);

        dim->mt = METRIC_TYPE_CONSTANT;
        dim->suppression_anomaly_counter = 0;
        dim->suppression_window_counter = 0;
        dim->training_in_progress = false;

        spinlock_unlock(&dim->slock);

        return worker_result;
    }

    // compute kmeans
    worker_is_busy(WORKER_TRAIN_KMEANS);
    {
        memcpy(worker->scratch_training_cns, worker->training_cns,
               training_response.total_values * sizeof(calculated_number_t));

        size_t smoothing_window = (dim->rd->rrdset->update_every > nd_profile.update_every) ? 1 : Cfg.max_samples_to_smooth;

        ml_features_t features = {
            Cfg.diff_n, smoothing_window, Cfg.lag_n,
            worker->scratch_training_cns, training_response.total_values,
            worker->training_cns, training_response.total_values,
            worker->training_samples
        };
        
        // Calculate dynamic sampling ratio based on expected output size
        // After diff and smooth, we'll have approximately this many vectors
        size_t expected_vectors = training_response.total_values;
        if (Cfg.diff_n > 0) expected_vectors--;
        if (smoothing_window > 1) expected_vectors = expected_vectors - smoothing_window + 1;
        expected_vectors = expected_vectors - Cfg.lag_n;
        
        double sampling_ratio = 1.0;
        if (expected_vectors > Cfg.max_training_vectors) {
            sampling_ratio = (double)Cfg.max_training_vectors / expected_vectors;
        }

        // Apply sampling during lag feature extraction
        ml_features_preprocess(&features, sampling_ratio);

        ml_kmeans_init(&dim->kmeans);
        ml_kmeans_train(&dim->kmeans, &features,  Cfg.max_kmeans_iters, training_response.query_after_t, training_response.query_before_t);
    }

    // update models
    ml_dimension_update_models(worker, dim);

    return worker_result;
}

bool
ml_dimension_predict(ml_dimension_t *dim, calculated_number_t value, bool exists)
{
    // Nothing to do if ML is disabled for this dimension
    if (dim->mls != MACHINE_LEARNING_STATUS_ENABLED)
        return false;

    // Acquire lock to protect dim->cns from concurrent access by ml_host_stop()
    if (spinlock_trylock(&dim->slock) == 0)
        return false;

    // Don't treat values that don't exist as anomalous
    if (!exists) {
        dim->cns.clear();
        spinlock_unlock(&dim->slock);
        return false;
    }

    // Save the value and return if we don't have enough values for a sample
    unsigned n = Cfg.diff_n + Cfg.max_samples_to_smooth + Cfg.lag_n;
    if (dim->cns.size() < n) {
        dim->cns.push_back(value);
        spinlock_unlock(&dim->slock);
        return false;
    }

    // Push the value and check if it's different from the last one
    bool same_value = true;
    std::rotate(std::begin(dim->cns), std::begin(dim->cns) + 1, std::end(dim->cns));
    if (dim->cns[n - 1] != value)
        same_value = false;
    dim->cns[n - 1] = value;

    // Create the sample
    assert((n * (Cfg.lag_n + 1) <= 128) &&
           "Static buffers too small to perform prediction. "
           "This should not be possible with the default clamping of feature extraction options");
    calculated_number_t src_cns[128];
    calculated_number_t dst_cns[128];

    memset(src_cns, 0, n * (Cfg.lag_n + 1) * sizeof(calculated_number_t));
    memcpy(src_cns, dim->cns.data(), n * sizeof(calculated_number_t));
    memcpy(dst_cns, dim->cns.data(), n * sizeof(calculated_number_t));

    ml_features_t features = {
        Cfg.diff_n, Cfg.max_samples_to_smooth, Cfg.lag_n,
        dst_cns, n, src_cns, n,
        dim->feature
    };
    ml_features_preprocess(&features, 1.0);

    // Mark the metric time as variable if we received different values
    if (!same_value)
        dim->mt = METRIC_TYPE_VARIABLE;

    // Ignore silenced dimensions
    if (dim->ts == TRAINING_STATUS_SILENCED) {
        spinlock_unlock(&dim->slock);
        return false;
    }

    dim->suppression_window_counter++;

    /*
     * Use the KMeans models to check if the value is anomalous
    */

    size_t sum = 0;
    size_t models_consulted = 0;

    for (const auto &km_ctx : dim->km_contexts) {
        models_consulted++;

        calculated_number_t anomaly_score = ml_kmeans_anomaly_score(&km_ctx, features.preprocessed_features[0]);
        if (std::isnan(anomaly_score))
            continue;

        if (anomaly_score < (100 * Cfg.dimension_anomaly_score_threshold)) {
            spinlock_unlock(&dim->slock);
            pulse_ml_models_consulted(models_consulted);
            return false;
        }

        sum += 1;
    }

    dim->suppression_anomaly_counter += sum ? 1 : 0;

    if ((dim->suppression_anomaly_counter >= Cfg.suppression_threshold) &&
        (dim->suppression_window_counter >= Cfg.suppression_window)) {
        dim->ts = TRAINING_STATUS_SILENCED;
    }

    spinlock_unlock(&dim->slock);

    pulse_ml_models_consulted(models_consulted);
    return sum;
}

/*
 * Chart
*/

static bool
ml_chart_is_available_for_ml(ml_chart_t *chart)
{
    return rrdset_is_available_for_exporting_and_alarms(chart->rs);
}

void
ml_chart_update_dimension(ml_chart_t *chart, ml_dimension_t *dim, bool is_anomalous)
{
    switch (dim->mls) {
        case MACHINE_LEARNING_STATUS_DISABLED_DUE_TO_EXCLUDED_CHART:
            chart->mls.num_machine_learning_status_disabled_sp++;
            return;
        case MACHINE_LEARNING_STATUS_ENABLED: {
            chart->mls.num_machine_learning_status_enabled++;

            switch (dim->mt) {
                case METRIC_TYPE_CONSTANT:
                    chart->mls.num_metric_type_constant++;
                    chart->mls.num_training_status_trained++;
                    chart->mls.num_normal_dimensions++;
                    return;
                case METRIC_TYPE_VARIABLE:
                    chart->mls.num_metric_type_variable++;
                    break;
            }

            switch (dim->ts) {
                case TRAINING_STATUS_UNTRAINED:
                    chart->mls.num_training_status_untrained++;
                    return;
                case TRAINING_STATUS_TRAINED:
                    chart->mls.num_training_status_trained++;

                    chart->mls.num_anomalous_dimensions += is_anomalous;
                    chart->mls.num_normal_dimensions += !is_anomalous;
                    return;
                case TRAINING_STATUS_SILENCED:
                    chart->mls.num_training_status_silenced++;
                    chart->mls.num_training_status_trained++;

                    chart->mls.num_anomalous_dimensions += is_anomalous;
                    chart->mls.num_normal_dimensions += !is_anomalous;
                    return;
            }

            return;
        }
    }
}

/*
 * Host detection & training functions
*/

#define WORKER_JOB_DETECTION_COLLECT_STATS 0
#define WORKER_JOB_DETECTION_DIM_CHART 1
#define WORKER_JOB_DETECTION_HOST_CHART 2
#define WORKER_JOB_DETECTION_STATS 3

static void
ml_host_detect_once(ml_host_t *host)
{
    worker_is_busy(WORKER_JOB_DETECTION_COLLECT_STATS);

    host->mls = {};
    ml_machine_learning_stats_t mls_copy = {};

    if (host->ml_running) {
        netdata_mutex_lock(&host->mutex);

        /*
         * prediction/detection stats
        */
        void *rsp = NULL;
        rrdset_foreach_read(rsp, host->rh) {
            RRDSET *rs = static_cast<RRDSET *>(rsp);

            ml_chart_t *chart = (ml_chart_t *) rs->ml_chart;
            if (!chart)
                continue;

            if (!ml_chart_is_available_for_ml(chart))
                continue;

            ml_machine_learning_stats_t chart_mls = chart->mls;

            host->mls.num_machine_learning_status_enabled += chart_mls.num_machine_learning_status_enabled;
            host->mls.num_machine_learning_status_disabled_sp += chart_mls.num_machine_learning_status_disabled_sp;

            host->mls.num_metric_type_constant += chart_mls.num_metric_type_constant;
            host->mls.num_metric_type_variable += chart_mls.num_metric_type_variable;

            host->mls.num_training_status_untrained += chart_mls.num_training_status_untrained;
            host->mls.num_training_status_pending_without_model += chart_mls.num_training_status_pending_without_model;
            host->mls.num_training_status_trained += chart_mls.num_training_status_trained;
            host->mls.num_training_status_pending_with_model += chart_mls.num_training_status_pending_with_model;
            host->mls.num_training_status_silenced += chart_mls.num_training_status_silenced;

            host->mls.num_anomalous_dimensions += chart_mls.num_anomalous_dimensions;
            host->mls.num_normal_dimensions += chart_mls.num_normal_dimensions;

            if (spinlock_trylock(&host->context_anomaly_rate_spinlock))
            {
                STRING *key = rs->context;
                auto &um = host->context_anomaly_rate;
                auto it = um.find(key);
                if (it == um.end()) {
                    um[key] = ml_context_anomaly_rate_t {
                        .rd = NULL,
                        .normal_dimensions = 0,
                        .anomalous_dimensions = 0
                    };
                    it = um.find(key);
                }

                it->second.anomalous_dimensions += chart_mls.num_anomalous_dimensions;
                it->second.normal_dimensions += chart_mls.num_normal_dimensions;
                spinlock_unlock(&host->context_anomaly_rate_spinlock);
            }
        }
        rrdset_foreach_done(rsp);

        host->host_anomaly_rate = 0.0;
        size_t NumActiveDimensions = host->mls.num_anomalous_dimensions + host->mls.num_normal_dimensions;
        if (NumActiveDimensions)
              host->host_anomaly_rate = static_cast<double>(host->mls.num_anomalous_dimensions) / NumActiveDimensions;

        mls_copy = host->mls;

        netdata_mutex_unlock(&host->mutex);

        worker_is_busy(WORKER_JOB_DETECTION_DIM_CHART);
        ml_update_dimensions_chart(host, mls_copy);

        worker_is_busy(WORKER_JOB_DETECTION_HOST_CHART);
        ml_update_host_and_detection_rate_charts(host, host->host_anomaly_rate * 10000.0);
    } else {
        host->host_anomaly_rate = 0.0;

        auto &um = host->context_anomaly_rate;
        for (auto &entry: um) {
            entry.second = ml_context_anomaly_rate_t {
                .rd = NULL,
                .normal_dimensions = 0,
                .anomalous_dimensions = 0
            };
        }
    }
}

void ml_detect_main(void *arg)
{
    UNUSED(arg);

    worker_register("MLDETECT");
    worker_register_job_name(WORKER_JOB_DETECTION_COLLECT_STATS, "collect stats");
    worker_register_job_name(WORKER_JOB_DETECTION_DIM_CHART, "dim chart");
    worker_register_job_name(WORKER_JOB_DETECTION_HOST_CHART, "host chart");
    worker_register_job_name(WORKER_JOB_DETECTION_STATS, "training stats");

    heartbeat_t hb;
    heartbeat_init(&hb, USEC_PER_SEC);

    while (!Cfg.detection_stop && service_running(SERVICE_COLLECTORS)) {
        worker_is_idle();
        heartbeat_next(&hb);

        RRDHOST *rh;
        rrd_rdlock();
        rrdhost_foreach_read(rh) {
            if (!rh->ml_host)
                continue;

            if (!service_running(SERVICE_COLLECTORS))
                break;

            ml_host_detect_once((ml_host_t *) rh->ml_host);
        }
        rrd_rdunlock();

        if (Cfg.enable_statistics_charts) {
            // collect and update training thread stats
            for (size_t idx = 0; idx != Cfg.num_worker_threads; idx++) {
                ml_worker_t *worker = &Cfg.workers[idx];

                netdata_mutex_lock(&worker->nd_mutex);
                ml_queue_stats_t queue_stats = worker->queue_stats;
                netdata_mutex_unlock(&worker->nd_mutex);

                ml_update_training_statistics_chart(worker, queue_stats);
            }
        }
    }
    Cfg.training_stop = true;
    finalize_self_prepared_sql_statements();
}

static void ml_flush_pending_models(ml_worker_t *worker) {
    static time_t next_vacuum_run = 0;
    int op_no = 1;

    // begin transaction
    int rc = db_execute(ml_db, "BEGIN TRANSACTION;", NULL);

    // add/delete models
    if (!rc) {
        op_no++;

        for (const auto &pending_model: worker->pending_model_info) {
            if (!rc)
                rc = ml_dimension_add_model(&pending_model.metric_uuid, &pending_model.inlined_kmeans);

            if (!rc)
                rc = ml_dimension_delete_models(&pending_model.metric_uuid, pending_model.inlined_kmeans.before - (Cfg.num_models_to_use * Cfg.train_every));
        }
    }

    // prune old models
    if (!rc) {
        if ((worker->num_db_transactions % 64) == 0) {
            rc = ml_prune_old_models(worker->num_models_to_prune);
            if (!rc)
                worker->num_models_to_prune = 0;
        }
    }

    // commit transaction
    if (!rc) {
        op_no++;
        rc = db_execute(ml_db, "COMMIT TRANSACTION;", NULL);
    }

    // rollback transaction on failure
    if (rc) {
        netdata_log_error("Trying to rollback ML transaction because it failed with rc=%d, op_no=%d", rc, op_no);
        op_no++;
        rc = db_execute(ml_db, "ROLLBACK;", NULL);
        if (rc)
            netdata_log_error("ML transaction rollback failed with rc=%d", rc);
    }

    if (!rc) {
        worker->num_db_transactions++;
        worker->num_models_to_prune += worker->pending_model_info.size();
    }

    vacuum_database(ml_db, "ML", 0, 0, &next_vacuum_run);

    worker->pending_model_info.clear();
}

static enum ml_worker_result ml_worker_create_new_model(ml_worker_t *worker, ml_request_create_new_model_t req) {
    AcquiredDimension AcqDim(req.DLI);

    if (!AcqDim.acquired()) {
        return ML_WORKER_RESULT_NULL_ACQUIRED_DIMENSION;
    }

    ml_dimension_t *Dim = reinterpret_cast<ml_dimension_t *>(AcqDim.dimension());
    return ml_dimension_train_model(worker, Dim);
}

static enum ml_worker_result ml_worker_add_existing_model(ml_worker_t *worker, ml_request_add_existing_model_t req) {
    UNUSED(worker);
    UNUSED(req);

    AcquiredDimension AcqDim(req.DLI);

    if (!AcqDim.acquired()) {
        return ML_WORKER_RESULT_NULL_ACQUIRED_DIMENSION;
    }

    ml_dimension_t *Dim = reinterpret_cast<ml_dimension_t *>(AcqDim.dimension());
    if (!Dim) {
        pulse_ml_models_ignored();
        return ML_WORKER_RESULT_OK;
    }

    // Check if training is in progress and skip if so to avoid race condition
    spinlock_lock(&Dim->slock);
    if (Dim->training_in_progress) {
        spinlock_unlock(&Dim->slock);
        pulse_ml_models_ignored();
        return ML_WORKER_RESULT_OK;
    }
    spinlock_unlock(&Dim->slock);

    Dim->kmeans = req.inlined_km;
    ml_dimension_update_models(worker, Dim);
    pulse_ml_models_received();
    return ML_WORKER_RESULT_OK;
}

void ml_train_main(void *arg) {
    ml_worker_t *worker = (ml_worker_t *) arg;

    char worker_name[1024];
    snprintfz(worker_name, 1024, "ml_worker_%zu", worker->id);
    worker_register("MLTRAIN");

    worker_register_job_name(WORKER_TRAIN_QUEUE_POP, "pop queue");
    worker_register_job_name(WORKER_TRAIN_ACQUIRE_DIMENSION, "acquire");
    worker_register_job_name(WORKER_TRAIN_QUERY, "query");
    worker_register_job_name(WORKER_TRAIN_KMEANS, "kmeans");
    worker_register_job_name(WORKER_TRAIN_UPDATE_MODELS, "update models");
    worker_register_job_name(WORKER_TRAIN_RELEASE_DIMENSION, "release");
    worker_register_job_name(WORKER_TRAIN_UPDATE_HOST, "update host");
    worker_register_job_name(WORKER_TRAIN_FLUSH_MODELS, "flush models");

    while (!Cfg.training_stop) {
        if(!stream_control_ml_should_be_running()) {
            worker_is_idle();
            stream_control_throttle();
            continue;
        }

        worker_is_busy(WORKER_TRAIN_QUEUE_POP);

        ml_queue_stats_t loop_stats{};

        ml_queue_item_t item = ml_queue_pop(worker->queue);
        if (item.type == ML_QUEUE_ITEM_STOP_REQUEST) {
            break;
        }

        ml_queue_size_t queue_size = ml_queue_size(worker->queue);

        usec_t allotted_ut = (Cfg.train_every * USEC_PER_SEC) / (queue_size.create_new_model + 1);
        if (allotted_ut > USEC_PER_SEC)
            allotted_ut = USEC_PER_SEC;

        usec_t start_ut = now_monotonic_usec();

        enum ml_worker_result worker_res;

        switch (item.type) {
            case ML_QUEUE_ITEM_TYPE_CREATE_NEW_MODEL: {
                worker_res = ml_worker_create_new_model(worker, item.create_new_model);
                if (worker_res != ML_WORKER_RESULT_NULL_ACQUIRED_DIMENSION) {
                    ml_queue_push(worker->queue, item);
                }
                break;
            }
            case ML_QUEUE_ITEM_TYPE_ADD_EXISTING_MODEL: {
                worker_res = ml_worker_add_existing_model(worker, item.add_existing_model);
                break;
            }
            default: {
                fatal("Unknown queue item type");
            }
        }

        usec_t consumed_ut = now_monotonic_usec() - start_ut;

        usec_t remaining_ut = 0;
        if (consumed_ut < allotted_ut)
            remaining_ut = allotted_ut - consumed_ut;

        if (Cfg.enable_statistics_charts) {
            worker_is_busy(WORKER_TRAIN_UPDATE_HOST);

            ml_queue_stats_t queue_stats = ml_queue_stats(worker->queue);

            loop_stats.total_add_existing_model_requests_pushed = queue_stats.total_add_existing_model_requests_pushed;
            loop_stats.total_add_existing_model_requests_popped = queue_stats.total_add_existing_model_requests_popped;
            loop_stats.total_create_new_model_requests_pushed = queue_stats.total_create_new_model_requests_pushed;
            loop_stats.total_create_new_model_requests_popped = queue_stats.total_create_new_model_requests_popped;

            loop_stats.allotted_ut = allotted_ut;
            loop_stats.consumed_ut = consumed_ut;
            loop_stats.remaining_ut = remaining_ut;

            switch (worker_res) {
                case ML_WORKER_RESULT_OK:
                    loop_stats.item_result_ok = 1;
                    break;
                case ML_WORKER_RESULT_INVALID_QUERY_TIME_RANGE:
                    loop_stats.item_result_invalid_query_time_range = 1;
                    break;
                case ML_WORKER_RESULT_NOT_ENOUGH_COLLECTED_VALUES:
                    loop_stats.item_result_not_enough_collected_values = 1;
                    break;
                case ML_WORKER_RESULT_NULL_ACQUIRED_DIMENSION:
                    loop_stats.item_result_null_acquired_dimension = 1;
                    break;
                case ML_WORKER_RESULT_CHART_UNDER_REPLICATION:
                    loop_stats.item_result_chart_under_replication = 1;
                    break;
            }

            netdata_mutex_lock(&worker->nd_mutex);

            worker->queue_stats.total_add_existing_model_requests_pushed = loop_stats.total_add_existing_model_requests_pushed;
            worker->queue_stats.total_add_existing_model_requests_popped = loop_stats.total_add_existing_model_requests_popped;

            worker->queue_stats.total_create_new_model_requests_pushed = loop_stats.total_create_new_model_requests_pushed;
            worker->queue_stats.total_create_new_model_requests_popped = loop_stats.total_create_new_model_requests_popped;

            worker->queue_stats.allotted_ut += loop_stats.allotted_ut;
            worker->queue_stats.consumed_ut += loop_stats.consumed_ut;
            worker->queue_stats.remaining_ut += loop_stats.remaining_ut;

            worker->queue_stats.item_result_ok += loop_stats.item_result_ok;
            worker->queue_stats.item_result_invalid_query_time_range += loop_stats.item_result_invalid_query_time_range;
            worker->queue_stats.item_result_not_enough_collected_values += loop_stats.item_result_not_enough_collected_values;
            worker->queue_stats.item_result_null_acquired_dimension += loop_stats.item_result_null_acquired_dimension;
            worker->queue_stats.item_result_chart_under_replication += loop_stats.item_result_chart_under_replication;

            netdata_mutex_unlock(&worker->nd_mutex);
        }

        bool should_sleep = true;

        if (worker->pending_model_info.size() >= Cfg.flush_models_batch_size) {
            worker_is_busy(WORKER_TRAIN_FLUSH_MODELS);
            netdata_mutex_lock(&db_mutex);
            ml_flush_pending_models(worker);
            netdata_mutex_unlock(&db_mutex);
            should_sleep = false;
        }

        if (item.type == ML_QUEUE_ITEM_TYPE_ADD_EXISTING_MODEL) {
           should_sleep = false;
        }

        if (!should_sleep)
            continue;

        worker_is_idle();
        std::this_thread::sleep_for(std::chrono::microseconds{remaining_ut});
    }
    finalize_self_prepared_sql_statements();
}
