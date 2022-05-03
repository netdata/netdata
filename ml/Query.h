#ifndef QUERY_H
#define QUERY_H

#include "ml-private.h"

namespace ml {

class Query {
public:
    Query(RRDDIM *RD) : RD(RD) {
        Ops = &RD->state->query_ops;
    }

    time_t latestTime() {
        return Ops->latest_time(RD);
    }

    time_t oldestTime() {
        return Ops->oldest_time(RD);
    }

    void init(time_t AfterT, time_t BeforeT) {
        Ops->init(RD, &Handle, AfterT, BeforeT);
    }

    bool isFinished() {
        return Ops->is_finished(&Handle);
    }

    std::pair<time_t, storage_number> nextMetric() {
        time_t CurrT;
        storage_number SN = Ops->next_metric(&Handle, &CurrT);
        return { CurrT, SN };
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
