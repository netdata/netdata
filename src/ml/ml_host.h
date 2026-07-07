// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_ML_HOST_H
#define NETDATA_ML_HOST_H

#include "ml_calculated_number.h"

#include "database/rrd.h"

#include <atomic>
#include <unordered_map>

struct ml_queue_t;

typedef struct machine_learning_stats_t {
    uint32_t num_machine_learning_status_enabled;
    uint32_t num_machine_learning_status_disabled_sp;

    uint32_t num_metric_type_constant;
    uint32_t num_metric_type_variable;

    uint32_t num_training_status_untrained;
    uint32_t num_training_status_pending_without_model;
    uint32_t num_training_status_trained;
    uint32_t num_training_status_pending_with_model;
    uint32_t num_training_status_silenced;

    uint32_t num_anomalous_dimensions;
    uint32_t num_normal_dimensions;
} ml_machine_learning_stats_t;

typedef struct {
    RRDDIM *rd;
    uint32_t normal_dimensions;
    uint32_t anomalous_dimensions;
} ml_context_anomaly_rate_t;

typedef struct {
    RRDHOST *rh;

    // Incremented at the END of every ml_host_stop, after all chart/dim resets
    // have committed. ml_host_detect_once samples this before and after its
    // chart walk; a change means a stop completed during the walk or a
    // stop+start cycle happened around the walk. In either case the
    // accumulated snapshot must be discarded even if ml_running is back to true
    // at the re-check.
    std::atomic<uint64_t> ml_stop_generation;

    ml_machine_learning_stats_t mls;

    calculated_number_t host_anomaly_rate;

    netdata_mutex_t mutex;

    // Serializes ml_host_start() against ml_host_stop(). Stop holds it across
    // its full chart/dim reset walk and the final stop-generation bump (it
    // cannot carry host->mutex into that walk), so a racing start cannot
    // re-enable ml_running while stop is mid-reset. Without it, a detect walk
    // could observe ml_running==true with an unchanged stop generation and
    // publish a snapshot spanning stop's in-flight resets.
    netdata_mutex_t start_stop_mutex;

    ml_queue_t *queue;

    /*
     * bookkeeping for anomaly detection charts
    */

    RRDSET *ml_running_rs;
    RRDDIM *ml_running_rd;

    RRDSET *machine_learning_status_rs;
    RRDDIM *machine_learning_status_enabled_rd;
    RRDDIM *machine_learning_status_disabled_sp_rd;

    RRDSET *metric_type_rs;
    RRDDIM *metric_type_constant_rd;
    RRDDIM *metric_type_variable_rd;

    RRDSET *training_status_rs;
    RRDDIM *training_status_untrained_rd;
    RRDDIM *training_status_pending_without_model_rd;
    RRDDIM *training_status_trained_rd;
    RRDDIM *training_status_pending_with_model_rd;
    RRDDIM *training_status_silenced_rd;

    RRDSET *dimensions_rs;
    RRDDIM *dimensions_anomalous_rd;
    RRDDIM *dimensions_normal_rd;

    RRDSET *anomaly_rate_rs;
    RRDDIM *anomaly_rate_rd;

    RRDSET *detector_events_rs;
    RRDDIM *detector_events_above_threshold_rd;
    RRDDIM *detector_events_new_anomaly_event_rd;

    RRDSET *context_anomaly_rate_rs;
    SPINLOCK context_anomaly_rate_spinlock;
    std::unordered_map<STRING *, ml_context_anomaly_rate_t> context_anomaly_rate;

    bool reset_pointers;
} ml_host_t;

static ALWAYS_INLINE bool ml_running_load(const RRDHOST *rh)
{
    return __atomic_load_n(&rh->ml_running, __ATOMIC_RELAXED);
}

static ALWAYS_INLINE void ml_running_store(RRDHOST *rh, bool running)
{
    __atomic_store_n(&rh->ml_running, running, __ATOMIC_RELAXED);
}

#ifdef __cplusplus
class AcquiredMLHost {
public:
    explicit AcquiredMLHost(RRDHOST *rh) : RH(rh), Host(nullptr)
    {
        if (!RH)
            return;

        rw_spinlock_read_lock(&RH->ml_host_rwlock);
        Host = reinterpret_cast<ml_host_t *>(__atomic_load_n(&RH->ml_host, __ATOMIC_ACQUIRE));
        if (!Host)
            release();
    }

    AcquiredMLHost(const AcquiredMLHost &) = delete;
    AcquiredMLHost &operator=(const AcquiredMLHost &) = delete;

    AcquiredMLHost(AcquiredMLHost &&other) noexcept : RH(other.RH), Host(other.Host)
    {
        other.RH = nullptr;
        other.Host = nullptr;
    }

    AcquiredMLHost &operator=(AcquiredMLHost &&other) noexcept
    {
        if (this != &other) {
            release();
            RH = other.RH;
            Host = other.Host;
            other.RH = nullptr;
            other.Host = nullptr;
        }

        return *this;
    }

    ~AcquiredMLHost()
    {
        release();
    }

    ml_host_t *get() const
    {
        return Host;
    }

    explicit operator bool() const
    {
        return Host != nullptr;
    }

    void release()
    {
        if (RH) {
            rw_spinlock_read_unlock(&RH->ml_host_rwlock);
            RH = nullptr;
            Host = nullptr;
        }
    }

private:
    RRDHOST *RH;
    ml_host_t *Host;
};
#endif

#endif /* NETDATA_ML_HOST_H */
