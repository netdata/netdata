// SPDX-License-Identifier: GPL-3.0-or-later

#include "ml_private.h"

#include "database/sqlite/sqlite_db_migration.h"

#include <random>

#define ML_METADATA_VERSION 2

static void ml_host_clear_context_anomaly_rate(ml_host_t *host)
{
    spinlock_lock(&host->context_anomaly_rate_spinlock);

    for (auto &entry : host->context_anomaly_rate)
        string_freez(entry.first);

    host->context_anomaly_rate.clear();

    spinlock_unlock(&host->context_anomaly_rate_spinlock);
}

static void ml_dimension_enqueue_create_model(RRDHOST *rh, RRDDIM *rd)
{
    ml_host_t *host = (ml_host_t *) __atomic_load_n(&rh->ml_host, __ATOMIC_ACQUIRE);
    if (!host)
        return;

    ml_dimension_t *dim = (ml_dimension_t *) rd->ml_dimension;
    if (!dim)
        return;

    spinlock_lock(&dim->slock);
    bool should_enqueue = !dim->create_new_model_queued &&
                          dim->ts == TRAINING_STATUS_UNTRAINED &&
                          (!dim->has_received_downstream_model || dim->km_contexts.empty());
    if (should_enqueue)
        dim->create_new_model_queued = true;
    spinlock_unlock(&dim->slock);

    if (!should_enqueue)
        return;

    ml_queue_item_t item;
    item.type = ML_QUEUE_ITEM_TYPE_CREATE_NEW_MODEL;
    item.create_new_model.DLI = DimensionLookupInfo(
        &rh->machine_guid[0],
        rd->rrdset->id,
        rd->id
    );

    ml_queue_push(host->queue, item);
}

bool ml_capable()
{
    return true;
}

bool ml_enabled(RRDHOST *rh)
{
    if (!rh)
        return false;

    if (!Cfg.enable_anomaly_detection)
        return false;

    if (simple_pattern_matches(Cfg.sp_host_to_skip, rrdhost_hostname(rh)))
        return false;

    return true;
}

bool ml_streaming_enabled()
{
    return Cfg.stream_anomaly_detection_charts;
}

void ml_host_new(RRDHOST *rh)
{
    if (!ml_enabled(rh))
        return;

    ml_host_t *host = new ml_host_t();

    host->rh = rh;
    host->mls = ml_machine_learning_stats_t();
    host->host_anomaly_rate = 0.0;
    host->anomaly_rate_rs = NULL;

    static std::atomic<size_t> times_called(0);
    host->queue = Cfg.workers[times_called++ % Cfg.num_worker_threads].queue;

    netdata_mutex_init(&host->mutex);
    netdata_mutex_init(&host->start_stop_mutex);
    spinlock_init(&host->context_anomaly_rate_spinlock);

    host->ml_running = false;
    host->ml_stop_generation = 0;

    // Publish with release semantics so readers that load rh->ml_host with
    // acquire semantics observe the host's `rh`, `ml_running`, `mutex`,
    // `queue`, etc. as fully initialized. Without this, the C++ compiler
    // may reorder field stores after the publish store of rh->ml_host, and
    // a concurrent reader would see host != NULL with partially-initialized
    // fields, producing SIGSEGV faults inside ml_dimension_is_anomalous and
    // similar readers.
    __atomic_store_n(&rh->ml_host, (rrd_ml_host_t *)host, __ATOMIC_RELEASE);
}

void ml_host_delete(RRDHOST *rh)
{
    // Atomically detach `rh->ml_host` and obtain the previous pointer in a
    // single RMW. Using exchange (rather than separate load + store) keeps
    // the unpublish and the freeing on this thread strictly ordered: no
    // store/operation that follows can be reordered before the unpublish,
    // so concurrent readers observe either the live host or NULL -- never
    // the freed host memory.
    ml_host_t *host = (ml_host_t *) __atomic_exchange_n(&rh->ml_host, (rrd_ml_host_t *)NULL, __ATOMIC_ACQ_REL);
    if (!host)
        return;

    ml_host_clear_context_anomaly_rate(host);
    netdata_mutex_destroy(&host->mutex);
    netdata_mutex_destroy(&host->start_stop_mutex);

    delete host;
}

void ml_host_start(RRDHOST *rh) {
    ml_host_t *host = (ml_host_t *) __atomic_load_n(&rh->ml_host, __ATOMIC_ACQUIRE);
    if (!host)
        return;

    // Serialize against ml_host_stop(): we must not re-enable ml_running
    // while a stop is still resetting chart/dim state (see ml_host_stop),
    // and concurrent ml_host_start() calls must not run the sweep twice.
    netdata_mutex_lock(&host->start_stop_mutex);

    if (host->ml_running) {
        netdata_mutex_unlock(&host->start_stop_mutex);
        return;
    }

    // Run the sweep under host->mutex so the visibility window of the flag
    // flip is bounded by the same critical section that performs the sweep.
    netdata_mutex_lock(&host->mutex);

    host->ml_running = true;

    void *rsp = NULL;
    rrdset_foreach_read(rsp, host->rh) {
        RRDSET *rs = static_cast<RRDSET *>(rsp);

        void *rdp = NULL;
        rrddim_foreach_read(rdp, rs) {
            RRDDIM *rd = static_cast<RRDDIM *>(rdp);
            ml_dimension_enqueue_create_model(rh, rd);
        }
        rrddim_foreach_done(rdp);
    }
    rrdset_foreach_done(rsp);

    netdata_mutex_unlock(&host->mutex);
    netdata_mutex_unlock(&host->start_stop_mutex);
}

void ml_host_stop(RRDHOST *rh) {
    ml_host_t *host = (ml_host_t *) __atomic_load_n(&rh->ml_host, __ATOMIC_ACQUIRE);
    if (!host)
        return;

    // Serialize with ml_host_start() for the WHOLE stop sequence, including
    // the unlocked chart/dim reset walk and the final generation bump. If a
    // racing start could flip ml_running back to true mid-reset, a concurrent
    // ml_host_detect_once would observe ml_running==true with an unchanged
    // stop generation and publish a snapshot torn by our in-flight resets.
    netdata_mutex_lock(&host->start_stop_mutex);

    if (!host->ml_running) {
        netdata_mutex_unlock(&host->start_stop_mutex);
        return;
    }

    // Prevent new ML activity from publishing while we reset host/dimension
    // state. The ml_running flag gates collectors and the detect loop; the
    // stop generation is bumped at the end of the function so a concurrent
    // ml_host_detect_once that observes the new generation is guaranteed to
    // also see all of our chart->mls / dim resets via seq_cst ordering.
    host->ml_running = false;

    netdata_mutex_lock(&host->mutex);

    // reset host stats
    host->mls = ml_machine_learning_stats_t();
    ml_host_clear_context_anomaly_rate(host);

    // Chart deletion can hold the dictionary writer across lengthy cleanup.
    // Do not carry host->mutex into the traversal below.
    netdata_mutex_unlock(&host->mutex);

    // reset charts/dims
    void *rsp = NULL;
    rrdset_foreach_read(rsp, host->rh) {
        RRDSET *rs = static_cast<RRDSET *>(rsp);

        ml_chart_t *chart = (ml_chart_t *) __atomic_load_n(&rs->ml_chart, __ATOMIC_ACQUIRE);
        if (!chart)
            continue;

        // reset chart
        chart->mls = ml_machine_learning_stats_t();

        void *rdp = NULL;
        rrddim_foreach_read(rdp, rs) {
            RRDDIM *rd = static_cast<RRDDIM *>(rdp);

            ml_dimension_t *dim = (ml_dimension_t *) rd->ml_dimension;
            if (!dim)
                continue;

            spinlock_lock(&dim->slock);

            dim->mt = METRIC_TYPE_CONSTANT;
            dim->ts = TRAINING_STATUS_UNTRAINED;

            dim->suppression_anomaly_counter = 0;
            dim->suppression_window_counter = 0;
            dim->cns.clear();
            dim->cns_head = 0;
            dim->km_contexts.clear();
            dim->has_received_downstream_model = false;
            // create_new_model_queued not reset here: stop does not drain the
            // worker queue, so pending CREATE_NEW_MODEL items remain valid.
            dim->reset_generation++;

            spinlock_unlock(&dim->slock);
        }
        rrddim_foreach_done(rdp);
    }
    rrdset_foreach_done(rsp);

    // Publish the stop only after every chart->mls / dim reset is committed.
    // ml_host_detect_once treats a generation change as "discard the snapshot",
    // so bumping here guarantees that if detect saw stale chart->mls it will
    // either also observe the new generation or have already published before
    // any of our resets started.
    host->ml_stop_generation.fetch_add(1);

    netdata_mutex_unlock(&host->start_stop_mutex);
}

void ml_host_get_info(RRDHOST *rh, BUFFER *wb)
{
    ml_host_t *host = (ml_host_t *) __atomic_load_n(&rh->ml_host, __ATOMIC_ACQUIRE);
    if (!host) {
        buffer_json_member_add_boolean(wb, "enabled", false);
        return;
    }

    buffer_json_member_add_uint64(wb, "version", 1);

    buffer_json_member_add_boolean(wb, "enabled", Cfg.enable_anomaly_detection);

    buffer_json_member_add_uint64(wb, "training-window", Cfg.training_window);
    buffer_json_member_add_uint64(wb, "min-training-window", Cfg.min_training_window);
    buffer_json_member_add_uint64(wb, "max-training-vectors", Cfg.max_training_vectors);
    buffer_json_member_add_uint64(wb, "max-samples-to-smooth", Cfg.max_samples_to_smooth);
    buffer_json_member_add_uint64(wb, "train-every", Cfg.train_every);

    buffer_json_member_add_uint64(wb, "diff-n", Cfg.diff_n);
    buffer_json_member_add_uint64(wb, "lag-n", Cfg.lag_n);

    buffer_json_member_add_uint64(wb, "max-kmeans-iters", Cfg.max_kmeans_iters);

    buffer_json_member_add_double(wb, "dimension-anomaly-score-threshold", Cfg.dimension_anomaly_score_threshold);

    buffer_json_member_add_string(wb, "anomaly-detection-grouping-method", time_grouping_id2txt(Cfg.anomaly_detection_grouping_method));

    buffer_json_member_add_int64(wb, "anomaly-detection-query-duration", Cfg.anomaly_detection_query_duration);

    buffer_json_member_add_string(wb, "hosts-to-skip", Cfg.hosts_to_skip.c_str());
    buffer_json_member_add_string(wb, "charts-to-skip", Cfg.charts_to_skip.c_str());
}

void ml_host_get_detection_info(RRDHOST *rh, BUFFER *wb)
{
    ml_host_t *host = (ml_host_t *) __atomic_load_n(&rh->ml_host, __ATOMIC_ACQUIRE);
    if (!host)
        return;

    netdata_mutex_lock(&host->mutex);

    buffer_json_member_add_uint64(wb, "version", 2);
    buffer_json_member_add_uint64(wb, "ml-running", host->ml_running);
    buffer_json_member_add_uint64(wb, "anomalous-dimensions", host->mls.num_anomalous_dimensions);
    buffer_json_member_add_uint64(wb, "normal-dimensions", host->mls.num_normal_dimensions);
    buffer_json_member_add_uint64(wb, "total-dimensions", host->mls.num_anomalous_dimensions +
                                                          host->mls.num_normal_dimensions);
    buffer_json_member_add_uint64(wb, "trained-dimensions", host->mls.num_training_status_trained +
                                                            host->mls.num_training_status_pending_with_model);
    netdata_mutex_unlock(&host->mutex);
}

bool ml_host_get_host_status(RRDHOST *rh, struct ml_metrics_statistics *mlm) {
    ml_host_t *host = (ml_host_t *) __atomic_load_n(&rh->ml_host, __ATOMIC_ACQUIRE);
    if (!host) {
        memset(mlm, 0, sizeof(*mlm));
        return false;
    }

    netdata_mutex_lock(&host->mutex);

    mlm->anomalous = host->mls.num_anomalous_dimensions;
    mlm->normal = host->mls.num_normal_dimensions;
    mlm->trained = host->mls.num_training_status_trained + host->mls.num_training_status_pending_with_model;
    mlm->pending = host->mls.num_training_status_untrained + host->mls.num_training_status_pending_without_model;
    mlm->silenced = host->mls.num_training_status_silenced;

    netdata_mutex_unlock(&host->mutex);

    return true;
}

bool ml_host_running(RRDHOST *rh) {
    ml_host_t *host = (ml_host_t *) __atomic_load_n(&rh->ml_host, __ATOMIC_ACQUIRE);
    if(!host)
        return false;

    return host->ml_running;
}

void ml_host_get_models(RRDHOST *rh, BUFFER *wb)
{
    UNUSED(rh);
    UNUSED(wb);

    // TODO: To be implemented
    netdata_log_error("Fetching KMeans models is not supported yet");
}

void ml_chart_new(RRDSET *rs)
{
    ml_host_t *host = (ml_host_t *) __atomic_load_n(&rs->rrdhost->ml_host, __ATOMIC_ACQUIRE);
    if (!host)
        return;

    ml_chart_t *chart = new ml_chart_t();

    chart->rs = rs;
    chart->mls = ml_machine_learning_stats_t();

    // Publish with release semantics so readers that load rs->ml_chart with
    // acquire semantics observe the chart's `rs` and `mls` fields as fully
    // initialized. Without this, the C++ compiler may reorder the plain
    // `chart->rs = rs` store after the publish store of rs->ml_chart, and a
    // concurrent reader would see chart != NULL with chart->rs still NULL
    // (from value-init in `new ml_chart_t()`), producing the SIGSEGV /
    // MAPERR / 0x80 fault inside ml_chart_is_available_for_ml.
    __atomic_store_n(&rs->ml_chart, (rrd_ml_chart_t *)chart, __ATOMIC_RELEASE);
}

void ml_chart_delete(RRDSET *rs)
{
    ml_host_t *host = (ml_host_t *) __atomic_load_n(&rs->rrdhost->ml_host, __ATOMIC_ACQUIRE);
    if (!host)
        return;

    // Atomically detach `rs->ml_chart` and obtain the previous pointer in a
    // single RMW. Using exchange (rather than separate load + store) keeps
    // the unpublish and the freeing on this thread strictly ordered: no
    // store/operation that follows can be reordered before the unpublish,
    // so concurrent readers observe either the live chart (with chart->rs
    // set) or NULL -- never the freed chart memory.
    ml_chart_t *chart = (ml_chart_t *) __atomic_exchange_n(&rs->ml_chart, (rrd_ml_chart_t *)NULL, __ATOMIC_ACQ_REL);
    delete chart;
}

ALWAYS_INLINE_ONLY bool ml_chart_update_begin(RRDSET *rs)
{
    ml_chart_t *chart = (ml_chart_t *) __atomic_load_n(&rs->ml_chart, __ATOMIC_ACQUIRE);
    if (!chart)
        return false;

    chart->mls = {};
    return true;
}

void ml_chart_update_end(RRDSET *rs)
{
    ml_chart_t *chart = (ml_chart_t *) __atomic_load_n(&rs->ml_chart, __ATOMIC_ACQUIRE);
    if (!chart)
        return;
}

void ml_dimension_new(RRDDIM *rd)
{
    ml_chart_t *chart = (ml_chart_t *) __atomic_load_n(&rd->rrdset->ml_chart, __ATOMIC_ACQUIRE);
    if (!chart)
        return;

    ml_dimension_t *dim = new ml_dimension_t();

    dim->rd = rd;

    dim->mt = METRIC_TYPE_CONSTANT;
    dim->ts = TRAINING_STATUS_UNTRAINED;
    dim->suppression_anomaly_counter = 0;
    dim->suppression_window_counter = 0;
    dim->training_in_progress = false;
    dim->has_received_downstream_model = false;
    dim->create_new_model_queued = false;
    dim->reset_generation = 0;
    dim->cns_head = 0;

    ml_kmeans_init(&dim->kmeans);

    if (simple_pattern_matches(Cfg.sp_charts_to_skip, rrdset_name(rd->rrdset)))
        dim->mls = MACHINE_LEARNING_STATUS_DISABLED_DUE_TO_EXCLUDED_CHART;
    else
        dim->mls = MACHINE_LEARNING_STATUS_ENABLED;

    spinlock_init(&dim->slock);

    dim->km_contexts.reserve(Cfg.num_models_to_use);

    rd->ml_dimension = (rrd_ml_dimension_t *) dim;

    metaqueue_ml_load_models(rd);

    // Only enqueue once ml is running for this host. Otherwise, ml_host_start()
    // will sweep all untrained dimensions and enqueue them when it runs.
    // This avoids double-enqueueing the same dim from both paths.
    RRDHOST *rh = rd->rrdset->rrdhost;
    ml_host_t *host = (ml_host_t *) __atomic_load_n(&rh->ml_host, __ATOMIC_ACQUIRE);
    if (host && host->ml_running)
        ml_dimension_enqueue_create_model(rh, rd);
}

void ml_dimension_delete(RRDDIM *rd)
{
    ml_dimension_t *dim = (ml_dimension_t *) rd->ml_dimension;
    if (!dim)
        return;

    // Wait for any in-progress training to complete before deleting
    // This prevents use-after-free crashes when training thread accesses dim->rd
    size_t wait_iterations = 0;
    const size_t max_wait_iterations = 3000; // 30 seconds max (3000 * 10ms)

    spinlock_lock(&dim->slock);
    while (dim->training_in_progress && wait_iterations < max_wait_iterations) {
        spinlock_unlock(&dim->slock);
        sleep_usec(10000); // Wait 10ms
        wait_iterations++;
        spinlock_lock(&dim->slock);
    }

    if (dim->training_in_progress) {
        // Training is stuck, but we can't wait forever
        // Log the issue but proceed with deletion
        netdata_log_error("ML: Dimension '%s' of chart '%s' is being deleted while training is in progress after waiting %zu ms",
                          rrddim_id(rd), rrdset_id(rd->rrdset), wait_iterations * 10);
    }

    spinlock_unlock(&dim->slock);

    delete dim;
    rd->ml_dimension = NULL;
}

ALWAYS_INLINE_ONLY void ml_dimension_received_anomaly(RRDDIM *rd, bool is_anomalous) {
    ml_dimension_t *dim = (ml_dimension_t *) rd->ml_dimension;
    if (!dim)
        return;

    ml_host_t *host = (ml_host_t *) __atomic_load_n(&rd->rrdset->rrdhost->ml_host, __ATOMIC_ACQUIRE);
    if (!host || !host->ml_running)
        return;

    ml_chart_t *chart = (ml_chart_t *) __atomic_load_n(&rd->rrdset->ml_chart, __ATOMIC_ACQUIRE);
    if (!chart)
        return;

    ml_chart_update_dimension(chart, dim, is_anomalous);
}

bool ml_dimension_is_anomalous(RRDDIM *rd, time_t curr_time, double value, bool exists)
{
    UNUSED(curr_time);

    ml_dimension_t *dim = (ml_dimension_t *) rd->ml_dimension;
    if (!dim)
        return false;

    ml_host_t *host = (ml_host_t *) __atomic_load_n(&rd->rrdset->rrdhost->ml_host, __ATOMIC_ACQUIRE);
    if (!host || !host->ml_running)
        return false;

    ml_chart_t *chart = (ml_chart_t *) __atomic_load_n(&rd->rrdset->ml_chart, __ATOMIC_ACQUIRE);
    if (!chart)
        return false;

    bool is_anomalous = ml_dimension_predict(dim, value, exists);
    ml_chart_update_dimension(chart, dim, is_anomalous);

    return is_anomalous;
}

void ml_init()
{
    // Read config values
    ml_config_load(&Cfg);

    if (!Cfg.enable_anomaly_detection)
        return;

    // Generate random numbers to efficiently sample the features we need
    // for KMeans clustering.
    std::random_device RD;
    std::mt19937 Gen(RD());

    Cfg.random_nums.reserve(Cfg.max_training_vectors);
    for (size_t Idx = 0; Idx != Cfg.max_training_vectors; Idx++)
        Cfg.random_nums.push_back(Gen());

    // init training thread-specific data
    Cfg.workers.resize(Cfg.num_worker_threads);
    for (size_t idx = 0; idx != Cfg.num_worker_threads; idx++) {
        ml_worker_t *worker = &Cfg.workers[idx];

        // Calculate max elements needed based on the highest frequency metrics
        // For 1-second metrics: training_window samples
        // We allocate for worst case (1-second update frequency)
        size_t max_elements_needed_for_training = (size_t) Cfg.training_window * (size_t) (Cfg.lag_n + 1);
        worker->training_cns = new calculated_number_t[max_elements_needed_for_training]();
        worker->scratch_training_cns = new calculated_number_t[max_elements_needed_for_training]();

        worker->id = idx;
        worker->queue = ml_queue_init();
        worker->pending_model_info.reserve(Cfg.flush_models_batch_size);
        netdata_mutex_init(&worker->nd_mutex);

        // Initialize reusable buffers for streaming kmeans models
        worker->stream_payload_buffer = buffer_create(0, NULL);
        worker->stream_wb_buffer = buffer_create(0, NULL);
    }

    // open sqlite db
    char path[FILENAME_MAX];
    snprintfz(path, sizeof(path), "%s/%s", netdata_configured_cache_dir, "ml.db");

    // Consume the quarantine sentinel dropped by ml_db_mark_corrupt() in a
    // prior session: rename the corrupt ml.db to a timestamped ml.db.bad.*
    // and drop the WAL siblings so sqlite3_open recreates a fresh DB.
    // No .recover variant -- ml.db only holds k-means models and is cheap
    // to rebuild.
    //
    // Use unlink() itself as the existence check (no separate stat/access
    // probe) to avoid a TOCTOU race between check and use. If the
    // quarantine rename then fails, restore the sentinel so the next start
    // retries. Always use a timestamped destination so rename() never
    // collides with a pre-existing ml.db.bad on Windows (which refuses to
    // overwrite) and never silently overwrites one on POSIX.
    char sentinel[FILENAME_MAX + 1];
    snprintfz(sentinel, sizeof(sentinel), "%s/.ml.db.delete", netdata_configured_cache_dir);

    // Attempt to consume the sentinel via unlink(). Three outcomes:
    //  - unlink() == 0:    sentinel existed, removed -> proceed with quarantine.
    //  - errno == ENOENT:  no sentinel -> normal startup, no quarantine.
    //  - any other errno:  sentinel exists but couldn't be removed (EACCES,
    //                      EROFS, EISDIR, ...). DO NOT proceed -- if we
    //                      quarantine without consuming the sentinel, the next
    //                      startup would re-quarantine the freshly created
    //                      ml.db, looping indefinitely. Log loudly and skip;
    //                      operator must remove the sentinel manually.
    int unlink_rc = unlink(sentinel);
    if (int unlink_err = errno; unlink_rc == -1 && unlink_err != ENOENT) {
        // The sentinel from a prior session is on disk but we can't remove
        // it (EACCES, EROFS, EISDIR, ...). The DB was previously flagged
        // corrupt; falling through to open it would just retrigger CORRUPT
        // errors all session long. Mark the DB unusable in-memory and skip
        // the open entirely -- ML runs without persistence until the
        // operator removes the sentinel manually.
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "ML: sentinel %s exists but unlink() failed (errno=%d). "
               "Marking ml.db unusable for this session and skipping the open. "
               "ML will run without persistence; operator must remove %s manually.",
               sentinel, unlink_err, sentinel);
        ml_db_force_unusable();
        return;
    }
    if (unlink_rc == 0) {
        char bad_path[FILENAME_MAX + 1];
        // Microsecond resolution so back-to-back restarts within the same
        // wall-clock second don't collide: a second-resolution suffix would
        // either silently overwrite the prior .bad on POSIX (forensic loss)
        // or fail rename() with EEXIST on Windows (sentinel kept restoring).
        snprintfz(bad_path, sizeof(bad_path), "%s/ml.db.bad.%llu",
                  netdata_configured_cache_dir, (unsigned long long) now_realtime_usec());

        int rename_rc = rename(path, bad_path);
        if (rename_rc == 0 || errno == ENOENT) {
            // Quarantine succeeded, or there was nothing to quarantine
            // (ml.db didn't exist). Either way, clean up the WAL/SHM
            // siblings so the fresh sqlite3_open() starts clean.
            char wal_path[FILENAME_MAX + 1];
            snprintfz(wal_path, sizeof(wal_path), "%s-wal", path);
            (void) unlink(wal_path);
            snprintfz(wal_path, sizeof(wal_path), "%s-shm", path);
            (void) unlink(wal_path);
            if (rename_rc == 0) {
                nd_log(NDLS_DAEMON, NDLP_WARNING,
                       "ML: quarantined corrupt %s as %s; a fresh ml.db will be created. "
                       "Inspect or remove the .bad file manually.",
                       path, bad_path);
            }
        } else {
            // Quarantine failed for a reason other than "source doesn't
            // exist". Restore the sentinel so we retry on next start
            // instead of silently re-opening the same corrupt file.
            //
            // Use O_CREAT|O_EXCL (atomic create-or-fail) rather than
            // O_CREAT|O_TRUNC so a symlink swap between our earlier
            // unlink() and this open() cannot redirect the write -- if
            // anything (legitimate or hostile) re-created the path in
            // that window, open() fails with EEXIST.
            int rerr = errno;
            int fd = open(sentinel, O_WRONLY | O_CREAT | O_EXCL | O_CLOEXEC, 0600);
            int oerr = errno;
            if (fd >= 0) {
                close(fd);
                nd_log(NDLS_DAEMON, NDLP_ERR,
                       "ML: failed to quarantine %s to %s (errno=%d); sentinel %s restored, will retry on next start.",
                       path, bad_path, rerr, sentinel);
            } else if (oerr == EEXIST) {
                // Something occupies the path already (race or stale file).
                // Next start's unlink() will remove it and trigger quarantine
                // regardless of what it is, so retry is still effective.
                nd_log(NDLS_DAEMON, NDLP_ERR,
                       "ML: failed to quarantine %s to %s (errno=%d); sentinel path %s already occupied, will retry on next start.",
                       path, bad_path, rerr, sentinel);
            } else {
                // Sentinel restore actually failed (permission, ENOSPC,
                // read-only FS, etc.). Retry on next start is NOT guaranteed
                // -- be honest with the operator.
                nd_log(NDLS_DAEMON, NDLP_ERR,
                       "ML: failed to quarantine %s to %s (rename errno=%d) AND failed to restore sentinel %s (open errno=%d). "
                       "Retry will NOT happen on next start; remove ml.db or recreate the sentinel manually.",
                       path, bad_path, rerr, sentinel, oerr);
            }
        }
    }

    int rc = sqlite3_open(path, &ml_db);
    if (rc != SQLITE_OK) {
        error_report("Failed to initialize database at %s, due to \"%s\"", path, sqlite3_errstr(rc));
        // Drop the sentinel now so the next start quarantines the file;
        // otherwise a corrupt header here leaves ml.db on disk and we'd
        // hit the same open() failure forever.
        ml_db_mark_if_corrupt(rc);
        sqlite3_close(ml_db);
        ml_db = NULL;
    }

    // create table
    if (ml_db) {
        int target_version = perform_ml_database_migration(ml_db, ML_METADATA_VERSION);
        if (configure_sqlite_database(ml_db, target_version, "ml_config")) {
            // configure_sqlite_database() returns 0/1 and loses the SQLite rc;
            // recover the last error from the connection before close() so
            // corruption surfaced by PRAGMA/migration drops the sentinel too.
            int errc = sqlite3_extended_errcode(ml_db);
            error_report("Failed to setup ML database (errcode=%d)", errc);
            ml_db_mark_if_corrupt(errc);
            sqlite3_close(ml_db);
            ml_db = NULL;
        }
        else {
            char *err = NULL;
            int rc = sqlite3_exec(ml_db, db_models_create_table, NULL, NULL, &err);
            if (rc != SQLITE_OK) {
                error_report("Failed to create models table (%s, %s)", sqlite3_errstr(rc), err ? err : "");
                ml_db_mark_if_corrupt(rc);
                sqlite3_close(ml_db);
                sqlite3_free(err);
                ml_db = NULL;
            }
        }
    }
}

uint64_t sqlite_get_ml_space(void)
{
    return sqlite_get_db_space(ml_db);
}

void ml_fini() {
    if (!Cfg.enable_anomaly_detection || !ml_db)
        return;

    sql_close_database(ml_db, "ML");
    ml_db = NULL;
}

void ml_start_threads() {
    if (!Cfg.enable_anomaly_detection)
        return;

    // start detection & training threads
    Cfg.detection_stop = false;
    Cfg.training_stop = false;

    char tag[NETDATA_THREAD_TAG_MAX + 1];

    snprintfz(tag, NETDATA_THREAD_TAG_MAX, "%s", "PREDICT");
    Cfg.detection_thread = nd_thread_create(tag, NETDATA_THREAD_OPTION_DEFAULT, ml_detect_main, NULL);

    for (size_t idx = 0; idx != Cfg.num_worker_threads; idx++) {
        ml_worker_t *worker = &Cfg.workers[idx];
        snprintfz(tag, NETDATA_THREAD_TAG_MAX, "TRAIN[%zu]", worker->id);
        worker->nd_thread = nd_thread_create(tag, NETDATA_THREAD_OPTION_DEFAULT, ml_train_main, worker);
    }
}

void ml_stop_threads()
{
    if (!Cfg.enable_anomaly_detection)
        return;

    Cfg.detection_stop = true;
    Cfg.training_stop = true;

    if (!Cfg.detection_thread)
        return;

    nd_thread_join(Cfg.detection_thread);
    Cfg.detection_thread = 0;

    // signal the worker queue of each thread
    for (size_t idx = 0; idx != Cfg.num_worker_threads; idx++) {
        ml_worker_t *worker = &Cfg.workers[idx];
        ml_queue_signal(worker->queue);
    }

    // join worker threads
    for (size_t idx = 0; idx != Cfg.num_worker_threads; idx++) {
        ml_worker_t *worker = &Cfg.workers[idx];

        nd_thread_join(worker->nd_thread);
    }

    // clear worker thread data
    for (size_t idx = 0; idx != Cfg.num_worker_threads; idx++) {
        ml_worker_t *worker = &Cfg.workers[idx];

        delete[] worker->training_cns;
        delete[] worker->scratch_training_cns;
        ml_queue_destroy(worker->queue);
        netdata_mutex_destroy(&worker->nd_mutex);

        // Free reusable buffers
        buffer_free(worker->stream_payload_buffer);
        buffer_free(worker->stream_wb_buffer);
    }
}

bool ml_model_received_from_child(RRDHOST *host, const char *json)
{
    UNUSED(host);

    bool ok = ml_dimension_deserialize_kmeans(json);
    if (!ok) {
        global_statistics_ml_models_deserialization_failures();
    }

    return ok;
}

void ml_host_disconnected(RRDHOST *rh) {
    ml_host_t *host = (ml_host_t *) __atomic_load_n(&rh->ml_host, __ATOMIC_ACQUIRE);
    if (!host)
        return;

    __atomic_store_n(&host->reset_pointers, true, __ATOMIC_RELAXED);
}
