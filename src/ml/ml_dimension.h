#ifndef ML_LOOKUP_H
#define ML_LOOKUP_H

#include "ml_string_wrapper.h"
#include "ml_enums.h"
#include "ml_kmeans.h"
#include "ml_chart.h"

#include <array>

// Upper bound on the prediction history ring:
//   diff_n (0..1) + smoothing window (1..5) + lag_n (1..5)
// per the clamps in ml_config.cc:173-175. ml_dimension_predict() asserts the
// effective n against this.
#define ML_DIMENSION_MAX_CNS 11

struct ml_dimension_t {
    RRDDIM *rd;

    enum ml_metric_type mt;
    enum ml_training_status ts;
    enum ml_machine_learning_status mls;
    SPINLOCK slock;
    uint32_t suppression_window_counter;
    uint32_t suppression_anomaly_counter;
    uint32_t reset_generation;
    bool training_in_progress;
    bool has_received_downstream_model;
    bool create_new_model_queued;

    // Prediction history, an inline ring instead of a heap vector: at most 11
    // doubles, so a vector cost 24 bytes of header plus a 128-byte allocation
    // to hold 88 bytes of payload.
    //
    // cns_count is the LOGICAL size and carries the warmup progress that
    // cns.size() used to carry. Every site that used to cns.clear() MUST zero
    // both cns_count and cns_head; zeroing cns_head alone would leave stale
    // history readable.
    uint8_t cns_head;
    uint8_t cns_count;

    std::vector<ml_kmeans_inlined_t> km_contexts;

    calculated_number_t cns[ML_DIMENSION_MAX_CNS];
};

bool
ml_dimension_predict(ml_dimension_t *dim, calculated_number_t value, bool exists);

bool ml_dimension_deserialize_kmeans(const char *json_str);

// Set dim's post-training state (mt/ts/suppression counters). Caller must hold
// dim->slock. Used by both the successful-training path and the undersampled
// early-return so the post-cycle state machine stays in sync.
void ml_dimension_finalize_constant_state(ml_dimension_t *dim);

class DimensionLookupInfo {
public:
    DimensionLookupInfo()
    {
        memset(MachineGuid.data(), 0, MachineGuid.size());
    }

    DimensionLookupInfo(const char *MachineGuid, STRING *ChartId, STRING *DimensionId)
        : ChartId(ChartId), DimensionId(DimensionId)
    {
        memcpy(this->MachineGuid.data(), MachineGuid, this->MachineGuid.size());
    }

    DimensionLookupInfo(const char *MachineGuid, const char *ChartId, const char *DimensionId)
        : ChartId(ChartId), DimensionId(DimensionId)
    {
        memcpy(this->MachineGuid.data(), MachineGuid, this->MachineGuid.size());
    }

    const char *machineGuid() const
    {
        return MachineGuid.data();
    }

    const char *chartId() const
    {
        return ChartId;
    }

    const char *dimensionId() const
    {
        return DimensionId;
    }

private:
    std::array<char, GUID_LEN + 1> MachineGuid;
    StringWrapper ChartId;
    StringWrapper DimensionId;
};

class AcquiredDimension {
public:
    AcquiredDimension(const DimensionLookupInfo &DLI) : AcqRH(nullptr), AcqRS(nullptr), AcqRD(nullptr), Dim(nullptr)
    {
        rrd_rdlock();

        AcqRH = rrdhost_find_and_acquire(DLI.machineGuid());
        if (AcqRH) {
            RRDHOST *RH = rrdhost_acquired_to_rrdhost(AcqRH);
            if (RH && !rrdhost_flag_check(RH, RRDHOST_FLAG_ORPHAN | RRDHOST_FLAG_ARCHIVED)) {
                AcqRS = rrdset_find_and_acquire(RH, DLI.chartId(), false);
                if (AcqRS) {
                    RRDSET *RS = rrdset_acquired_to_rrdset(AcqRS);
                    if (RS && !rrdset_flag_check(RS, RRDSET_FLAG_OBSOLETE)) {
                        AcqRD = rrddim_find_and_acquire(RS, DLI.dimensionId(), false);
                        if (AcqRD) {
                            RRDDIM *RD = rrddim_acquired_to_rrddim(AcqRD);
                            if (RD) {
                                Dim = reinterpret_cast<ml_dimension_t *>(RD->ml_dimension);
                                acquire_failure_reason = "ok";
                            }
                            else
                                acquire_failure_reason = "no dimension";
                        }
                        else
                            acquire_failure_reason = "can't find dimension";
                    }
                    else
                        acquire_failure_reason = "chart is obsolete";
                }
                else
                    acquire_failure_reason = "can't find chart";
            }
            else
                acquire_failure_reason = "host is orphan or archived";
        }
        else
            acquire_failure_reason = "can't find host";

        rrd_rdunlock();
    }

    AcquiredDimension(const AcquiredDimension &) = delete;
    AcquiredDimension operator=(const AcquiredDimension &) = delete;

    AcquiredDimension(AcquiredDimension &&) = default;
    AcquiredDimension &operator=(AcquiredDimension &&) = default;

    bool acquired() const {
        return AcqRD != nullptr;
    }

    const char *acquire_failure() const {
        return acquire_failure_reason;
    }

    AcquiredMLHost host() const {
        assert(acquired());
        return AcquiredMLHost(rrdhost_acquired_to_rrdhost(AcqRH));
    }

    ml_dimension_t *dimension() const {
        assert(acquired());
        return Dim;
    }

    ~AcquiredDimension()
    {
        if (AcqRD)
            rrddim_acquired_release(AcqRD);

        if (AcqRS)
            rrdset_acquired_release(AcqRS);

        if (AcqRH)
            rrdhost_acquired_release(AcqRH);
    }

private:
    const char *acquire_failure_reason;
    RRDHOST_ACQUIRED *AcqRH;
    RRDSET_ACQUIRED *AcqRS;
    RRDDIM_ACQUIRED *AcqRD;
    ml_dimension_t *Dim;
};

#endif /* ML_LOOKUP_H */
