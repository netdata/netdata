#ifndef QUERY_H
#define QUERY_H

#include "ml-private.h"

namespace ml {

class Query {
public:
    Query(RRDDIM *RD) : RD(RD), Initialized(false) {
        Ops = RD->tiers[0]->query_ops;
    }

    time_t latestTime() {
        return Ops->latest_time(RD->tiers[0]->db_metric_handle);
    }

    time_t oldestTime() {
        return Ops->oldest_time(RD->tiers[0]->db_metric_handle);
    }

    void init(time_t AfterT, time_t BeforeT) {
        Ops->init(RD->tiers[0]->db_metric_handle, &Handle, AfterT, BeforeT);
        Initialized = true;
        points_read = 0;
    }

    bool isFinished() {
        return Ops->is_finished(&Handle);
    }

    ~Query() {
        if (Initialized) {
            Ops->finalize(&Handle);
            global_statistics_ml_query_completed(points_read);
            points_read = 0;
        }
    }

    std::pair<time_t, CalculatedNumber> nextMetric() {
        points_read++;
        STORAGE_POINT sp = Ops->next_metric(&Handle);
        return { sp.start_time, sp.sum / sp.count };
    }

private:
    RRDDIM *RD;
    bool Initialized;
    size_t points_read;

    struct storage_engine_query_ops *Ops;
    struct storage_engine_query_handle Handle;
};

} // namespace ml

#endif /* QUERY_H */
