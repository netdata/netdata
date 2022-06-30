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
        Ops->init(RD->tiers[0]->db_metric_handle, &Handle, AfterT, BeforeT);
    }

    bool isFinished() {
        return Ops->is_finished(&Handle);
    }

    std::pair<time_t, CalculatedNumber> nextMetric() {
        time_t CurrT, EndT;
        SN_FLAGS Flags;
        auto Value = (CalculatedNumber)Ops->next_metric(&Handle, &CurrT, &EndT, &Flags, NULL);
        return { CurrT, Value };
    }

    ~Query() {
        Ops->finalize(&Handle);
    }

private:
    RRDDIM *RD;

    struct rrddim_query_ops *Ops;
    struct rrddim_query_handle Handle;
};

} // namespace ml

#endif /* QUERY_H */
