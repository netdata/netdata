#ifndef QUERY_H
#define QUERY_H

#include "ml-private.h"

namespace ml {

class Query {
public:
    Query(RRDDIM *RD) : RD(RD) {
        Ops = &RD->tiers[0]->query_ops;
    }

    time_t latestTime() {
        return Ops->latest_time(RD->tiers[0]->db_metric_handle);
    }

    time_t oldestTime() {
        return Ops->oldest_time(RD->tiers[0]->db_metric_handle);
    }

    void init(time_t AfterT, time_t BeforeT) {
        Ops->init(RD->tiers[0]->db_metric_handle, &Handle, AfterT, BeforeT, TIER_QUERY_FETCH_SUM);
    }

    bool isFinished() {
        if (!Ops->is_finished(&Handle))
            return false;

        Ops->finalize(&Handle);
        return true;
    }

    std::pair<time_t, CalculatedNumber> nextMetric() {
        STORAGE_POINT sp = Ops->next_metric(&Handle);
        return { sp.start_time, sp.sum / sp.count };
    }

private:
    RRDDIM *RD;

    struct rrddim_query_ops *Ops;
    struct rrddim_query_handle Handle;
};

} // namespace ml

#endif /* QUERY_H */
