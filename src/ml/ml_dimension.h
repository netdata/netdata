#ifndef ML_LOOKUP_H
#define ML_LOOKUP_H

#include "ml_string_wrapper.h"
#include "ml_enums.h"
#include "ml_kmeans.h"
#include "ml_chart.h"

#include <array>

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
    size_t cns_head;

    std::vector<calculated_number_t> cns;

    std::vector<ml_kmeans_inlined_t> km_contexts;
    DSample feature;
};

bool
ml_dimension_predict(ml_dimension_t *dim, calculated_number_t value, bool exists);

// host is the RRDHOST the streaming connection that delivered this payload is authenticated for.
// The payload's machine-guid must name that same host; see the implementation.
bool ml_dimension_deserialize_kmeans(RRDHOST *host, const char *json_str);

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
        // bounded copy: the source is a caller-supplied C string and is not guaranteed to be
        // GUID_LEN + 1 bytes long. Copying this->MachineGuid.size() unconditionally reads past a
        // shorter source - reachable from the streaming ML_MODEL payload.
        strncpyz(this->MachineGuid.data(), MachineGuid, GUID_LEN);
    }

    DimensionLookupInfo(const char *MachineGuid, const char *ChartId, const char *DimensionId)
        : ChartId(ChartId), DimensionId(DimensionId)
    {
        // bounded copy, for the reason given in the ctor above
        strncpyz(this->MachineGuid.data(), MachineGuid, GUID_LEN);
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
    explicit AcquiredDimension(const DimensionLookupInfo &DLI)
    {
        rrd_rdlock();

        AcqRH = rrdhost_find_and_acquire(DLI.machineGuid());
        if (AcqRH) {
            RRDHOST *RH = rrdhost_acquired_to_rrdhost(AcqRH);
            if (RH && !rrdhost_flag_check(RH, RRDHOST_FLAG_ORPHAN | RRDHOST_FLAG_ARCHIVED)) {
                // obsolete charts are found too, so they are told apart from deleted ones below; their access
                // time is left alone, or the retries of a queued dimension would keep the chart from being freed
                AcqRS = rrdset_find_and_acquire_obsolete_untouched(RH, DLI.chartId());
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
                    else {
                        acquire_failure_reason = "chart is obsolete";
                        temporarily_unavailable = true;
                    }
                }
                else
                    acquire_failure_reason = "can't find chart";
            }
            else {
                acquire_failure_reason = "host is orphan or archived";
                temporarily_unavailable = true;
            }
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

    // not acquired, but the dimension still exists and can come back (a child reconnects, a chart is revived)
    bool unavailable_temporarily() const {
        return temporarily_unavailable;
    }

    AcquiredMLHost host() const {
        assert(acquired());
        return AcquiredMLHost(rrdhost_acquired_to_rrdhost(AcqRH));
    }

    ml_dimension_t *dimension() const {
        assert(acquired());
        return Dim;
    }

    // the acquired RRDDIM itself, valid whether or not ML state exists for it
    RRDDIM *rrddim() const {
        assert(acquired());
        return rrddim_acquired_to_rrddim(AcqRD);
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
    bool temporarily_unavailable = false;
    RRDHOST_ACQUIRED *AcqRH = nullptr;
    RRDSET_ACQUIRED *AcqRS = nullptr;
    RRDDIM_ACQUIRED *AcqRD = nullptr;
    ml_dimension_t *Dim = nullptr;
};

#endif /* ML_LOOKUP_H */
