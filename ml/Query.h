#ifndef QUERY_H
#define QUERY_H

#include "ml-private.h"

namespace ml {

class Query {
public:
    Query(RRDDIM *RD) : RD(RD), Initialized(false) {}

    time_t latestTime() {
        return se_metric_latest_time(RD->tiers[0]->mode, RD->tiers[0]->db_metric_handle);
    }

    time_t oldestTime() {
        return se_metric_oldest_time(RD->tiers[0]->mode, RD->tiers[0]->db_metric_handle);
    }

    void init(time_t AfterT, time_t BeforeT) {
        se_query_init(RD->tiers[0]->mode, RD->tiers[0]->db_metric_handle,
                      &Handle, AfterT, BeforeT);
        Initialized = true;
        points_read = 0;
    }

    bool isFinished() {
        return se_query_is_finished(RD->tiers[0]->mode, &Handle);
    }

    ~Query() {
        if (Initialized) {
            se_query_finalize(RD->tiers[0]->mode, &Handle);
            global_statistics_ml_query_completed(points_read);
            points_read = 0;
        }
    }

    std::pair<time_t, CalculatedNumber> nextMetric() {
        points_read++;
        STORAGE_POINT sp = se_query_next_metric(RD->tiers[0]->mode, (&Handle));
        return { sp.start_time, sp.sum / sp.count };
    }

private:
    RRDDIM *RD;
    bool Initialized;
    size_t points_read;

    struct storage_engine_query_handle Handle;
};

} // namespace ml

#endif /* QUERY_H */
