#ifndef REPLICATION_UTILS_H
#define REPLICATION_UTILS_H

#include "replication-private.h"

namespace replication {

/*
 * Wraps a standard std::mutex and enables/disables cancelability
 * in lock/unlock operations.
 */
class Mutex {
public:
    void lock() {
        netdata_thread_disable_cancelability();
        M.lock();
    }

    void unlock() {
        M.unlock();
        netdata_thread_enable_cancelability();
    }

    bool try_lock() {
        netdata_thread_disable_cancelability();
        if (M.try_lock())
            return true;

        netdata_thread_enable_cancelability();
        return false;
    }

private:
    std::mutex M;
};

/*
 * Wraps the query ops under a single class. It exposes just a single
 * static function "getSNs()" which we use to get the data of a dimension
 * for the time range we want.
 */
class Query {
public:
    /*
     * TODO: align <after, before>
     */
    static std::pair<std::vector<time_t>, std::vector<storage_number>>
    getSNs(RRDDIM *RD, time_t After, time_t Before)
    {
        std::vector<time_t> Timestamps;
        std::vector<storage_number> StorageNumbers;

        if (After > Before) {
            error("[%s] Tried to query %s.%s with <After=%ld GT Before=%ld>",
                  rrdhost_hostname(RD->rrdset->rrdhost), rrdset_id(RD->rrdset), rrddim_id(RD), After, Before);
            return { Timestamps, StorageNumbers };
        }

        Query Q(RD);

        After = std::max(After, Q.oldestTime());
        Before = std::min(Before + 1, Q.latestTime());

        if (After > Before) {
            return { Timestamps, StorageNumbers };
        }

        size_t N = Before - After + 1;
        Timestamps.reserve(N);
        StorageNumbers.reserve(N);

        Q.init(After, Before);
        while (!Q.isFinished()) {
            auto P = Q.nextMetric();
            if (P.first < After || P.first > Before)
                continue;

            Timestamps.push_back(P.first);
            StorageNumbers.push_back(P.second);
        }

        return { Timestamps, StorageNumbers };
    }

private:
    Query(RRDDIM *RD) : RD(RD), Initialized(false) {
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
        Initialized = true;
    }

    bool isFinished() {
        return Ops->is_finished(&Handle);
    }

    std::pair<time_t, storage_number> nextMetric() {
        STORAGE_POINT SP = Ops->next_metric(&Handle);
        storage_number SN = pack_storage_number(SP.sum / SP.count, SP.flags);
        return { SP.end_time, SN };
    }

    ~Query() {
        if (Initialized)
            Ops->finalize(&Handle);
    }

private:
    RRDDIM *RD;
    struct rrddim_query_ops *Ops;

    bool Initialized;
    struct rrddim_query_handle Handle;
};

/*
 * A rate-limiter class that is initialized with the number of requests
 * we want to execute in the specified time window. The replication
 * thread uses this to limit the total amount of queries we perform
 * to collect history data of dimensions.
 */
class RateLimiter {

using SystemClock = std::chrono::system_clock;
using TimePoint = std::chrono::time_point<SystemClock>;

public:
    RateLimiter(size_t NumRequests, std::chrono::milliseconds Window)
        : NumRequests(NumRequests), Window(Window),
          Q(NumRequests, TimePoint()), Index(0) {}

    void request() {
        auto CurrT = SystemClock::now();
        bool BelowLimit = (CurrT - Q[Index]) >= Window;

        if (!BelowLimit) {
            std::this_thread::sleep_for(Window * 0.25);
            CurrT = SystemClock::now();
        }

        addTimePoint(CurrT);
    }

private:
    void addTimePoint(TimePoint TP) {
        Q[Index] = TP;
        Index = (Index + 1) % NumRequests;
    }

private:
    size_t NumRequests;
    std::chrono::milliseconds Window;

    std::vector<TimePoint> Q;
    size_t Index = 0;
};

} // namespace replication

#endif /* REPLICATION_UTILS_H */
