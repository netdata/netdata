#include "replication.h"

#include <vector>
#include <utility>
#include <limits>

class Query {
public:
    Query(RRDDIM *RD, time_t After, time_t Before) {
        assert(After <= Before && "Invalid time range: After > Before");

        Ops = RD->tiers[0]->query_ops;
        Ops->init(RD->tiers[0]->db_metric_handle, &Handle, After, Before);
    }

    bool isFinished() {
        return Ops->is_finished(&Handle);
    }

    STORAGE_POINT nextMetric() {
        assert(!isFinished() && "Tried to get next metric from finished query");
        return Ops->next_metric(&Handle);
    }

    ~Query() {
        assert(isFinished() && "Tried to finalize unfinished query");
        Ops->finalize(&Handle);
    }

private:
    struct storage_engine_query_ops *Ops;
    struct storage_engine_query_handle Handle;
};

enum class StoragePointStatus : uint8_t {
    Uninitialized = 0,
    Unused,
    Consumed,
    Finished,
};

struct ReplicationStoragePoint {
    StoragePointStatus Status;
    STORAGE_POINT SP;
};

class ChartQuery {
public:
    static std::pair<time_t, time_t> get(BUFFER *WB, RRDSET *RS, time_t After, time_t Before) {
        ChartQuery CQ(RS, After, Before);
        return CQ.fill(WB);
    }

private:
    ChartQuery(RRDSET *RS, time_t After, time_t Before) : RS(RS)
    {
        this->After = After;
        this->Before = Before;
        assert(After <= Before && "Invalid query time range: After > Before");
        assert(After >= rrdset_first_entry_t(RS) && "After < rrdset's first entry");
        assert(Before <= rrdset_last_entry_t(RS) && "Before > rrdset's last entry");

        size_t N = rrdset_number_of_dimensions(RS);
        assert(N != 0 && "Asked query chart with 0 dimensions");

        // Init queries
        {
            Queries.reserve(N);

            void *RDP = nullptr;
            rrddim_foreach_read(RDP, RS) {
                RRDDIM *RD = static_cast<RRDDIM *>(RDP);
                Queries.emplace_back(RD, After, Before);
            }
            rrddim_foreach_done(RDP);

            assert(Queries.size() == N && "Unexpected number of queries");
        }

        // Init storage points
        {
            ReplSPs.reserve(N);

            for (size_t Idx = 0; Idx != N; Idx++)
                ReplSPs.push_back({ StoragePointStatus::Uninitialized, STORAGE_POINT() });
        }
    }

    std::pair<time_t, time_t> fill(BUFFER *WB) {
        fillHeader(WB, RS);

        time_t FirstStartTime = std::numeric_limits<time_t>::max();
        time_t LastEndTime = std::numeric_limits<time_t>::min();

        time_t StartTime, EndTime;
        while (advance(&StartTime, &EndTime)) {
            if (FirstStartTime > StartTime)
                FirstStartTime = StartTime;

            if (LastEndTime < EndTime)
                LastEndTime = EndTime;

            fillLine(WB, StartTime, EndTime);
        }

        return { FirstStartTime, LastEndTime };
    }

    void fillHeader(BUFFER *WB, RRDSET *RS) {
        buffer_sprintf(WB, "REPLAY_RRDSET_HEADER start_time end_time");

        void *RDP;
        rrddim_foreach_read(RDP, RS) {
            RRDDIM *RD = static_cast<RRDDIM *>(RDP);
            buffer_sprintf(WB, " \"%s\"", rrddim_id(RD));
        }
        rrddim_foreach_done(RDP);

        buffer_strcat(WB, "\n");
    }

    void fillLine(BUFFER *WB, time_t StartTime, time_t EndTime) {
        buffer_sprintf(WB, "REPLAY_RRDSET_DONE %ld %ld", StartTime, EndTime);

        bool UsedSP = false;
        for (ReplicationStoragePoint &RSP : ReplSPs) {
            switch (RSP.Status) {
                case StoragePointStatus::Uninitialized:
                    assert(false && "Found uninitialized replication storage point");
                    continue;
                case StoragePointStatus::Consumed:
                    assert(false && "Found consumed replication storage point");
                    continue;
                case StoragePointStatus::Unused: {
                    STORAGE_POINT &SP = RSP.SP;

                    if (SP.start_time != StartTime) {
                        buffer_sprintf(WB, " NULL 0");
                    } else {
                        buffer_sprintf(WB, " %lf %u", SP.sum, SP.flags);
                        RSP.Status = StoragePointStatus::Consumed;
                        UsedSP = true;
                    }
                    continue;
                }
                case StoragePointStatus::Finished:
                    buffer_sprintf(WB, " NULL NULL");
                    continue;
            }
        }
        assert(UsedSP && "Line produced without consuming a storage point");

        buffer_strcat(WB, "\n");
    }

    bool advance(time_t *StartTime, time_t *EndTime) {
        *StartTime = std::numeric_limits<time_t>::max();
        *EndTime = std::numeric_limits<time_t>::min();

        for (size_t Idx = 0; Idx != ReplSPs.size(); Idx++) {
            ReplicationStoragePoint &RSP = ReplSPs[Idx];
            Query &Q = Queries[Idx];

            switch (RSP.Status) {
            case StoragePointStatus::Uninitialized:
                /* fall through */
            case StoragePointStatus::Consumed:
                if (Q.isFinished())
                    RSP.Status = StoragePointStatus::Finished;
                else {
                    RSP.SP = Q.nextMetric();
                    if (RSP.SP.start_time < *StartTime) {
                        *StartTime = RSP.SP.start_time;
                        *EndTime = RSP.SP.end_time;
                    }
                    RSP.Status = StoragePointStatus::Unused;
                }
                break;
            case StoragePointStatus::Unused:
                if (RSP.SP.start_time < *StartTime) {
                    *StartTime = RSP.SP.start_time;
                    *EndTime = RSP.SP.end_time;
                }
                break;
            case StoragePointStatus::Finished:
                break;
            }
        }

#ifdef NETDATA_INTERNAL_CHECKS
        // check that all SPs that have the same start time and the same end time
        for (size_t Idx = 0; Idx != ReplSPs.size(); Idx++) {
            ReplicationStoragePoint &RSP = ReplSPs[Idx];

            switch (RSP.Status) {
            case StoragePointStatus::Unused:
                if (*StartTime != RSP.SP.start_time)
                    continue;

                assert(RSP.SP.end_time == *EndTime &&
                       "Storage points have same start time but different end time");
                break;
            default:
                continue;
            }
        }
#endif

        return *StartTime != std::numeric_limits<time_t>::max();
    }

private:
    RRDSET *RS;
    time_t After;
    time_t Before;

    std::vector<Query> Queries;
    std::vector<ReplicationStoragePoint> ReplSPs;
};

bool replicate_chart_response(RRDHOST *host, RRDSET *st,
                              bool start_streaming, time_t after, time_t before)
{
    time_t query_after = after;
    time_t query_before = before;

    // only happens when the parent received nonsensical timestamps from
    // us, in which case we want to skip replication and start streaming.
    // (or when replication is disabled)
    if (start_streaming && after == 0 && before == 0)
        return true;

    // find the first entry we have
    time_t first_entry_local = rrdset_first_entry_t(st);

    if (query_after < first_entry_local)
        query_after = first_entry_local;

    // find the latest entry we have
    time_t last_entry_local = rrdset_last_entry_t(st);

    if (query_before > last_entry_local)
        query_before = last_entry_local;

    // if the parent asked us to start streaming, then fill the rest of the
    // data that we have
    if (start_streaming)
        query_before = last_entry_local;

    // should never happen, but nevertheless enable streaming
    if (query_after > query_before)
        return true;

    // we might want to optimize this by filling a temporary buffer
    // and copying the result to the host's buffer in order to avoid
    // holding the host's buffer lock for too long
    BUFFER *wb = sender_start(host->sender);
    {
        // pass the original after/before so that the parent knows about
        // which time range we responded
        buffer_sprintf(wb, "REPLAY_RRDSET_BEGIN \"%s\"\n", rrdset_id(st));

        // fill the data table
        (void) ChartQuery::get(wb, st, after, before);

        // end with first/last entries we have, and the first start time and
        // last end time of the data we sent
        buffer_sprintf(wb, "REPLAY_RRDSET_END %ld %ld %ld %s %ld %ld\n",
                       (time_t) st->update_every, first_entry_local, last_entry_local,
                       start_streaming ? "true" : "false", after, before);
    }
    sender_commit(host->sender, wb);

    return start_streaming;
}

static bool send_replay_chart_cmd(FILE *outfp, const char *chart,
                                  bool start_streaming, time_t after, time_t before)
{

    fprintf(stdout, "REPLAY_CHART \"%s\" \"%s\" %ld %ld\n",
                    chart, start_streaming ? "true" : "false",
                    after, before);
    fflush(stdout);

    int ret = fprintf(outfp, "REPLAY_CHART \"%s\" \"%s\" %ld %ld\n",
                      chart, start_streaming ? "true" : "false",
                      after, before);
    if (ret < 0) {
        error("failed to send replay request to child (ret=%d)", ret);
        return false;
    }

    fflush(outfp);
    return true;
}

bool replicate_chart_request(FILE *outfp, RRDHOST *host, RRDSET *st,
                             time_t first_entry_child, time_t last_entry_child,
                             time_t prev_first_entry_wanted, time_t prev_last_entry_wanted)
{
    // if replication is disabled, send an empty replication request
    // asking no data
    if (!host->rrdpush_enable_replication) {
        error("Skipping replication request because it's disabled on host %s", rrdhost_hostname(host));
        rrdset_flag_set(st, RRDSET_FLAG_RECEIVER_REPLICATION_FINISHED);
        return send_replay_chart_cmd(outfp, rrdset_id(st), true, 0, 0);
    }

    // Child has no stored data
    if (!last_entry_child) {
        error("Skipping replication request for %s.%s because child has no stored data",
              rrdhost_hostname(host), rrdset_id(st));
        rrdset_flag_set(st, RRDSET_FLAG_RECEIVER_REPLICATION_FINISHED);
        return send_replay_chart_cmd(outfp, rrdset_id(st), true, 0, 0);
    }

    // Nothing to get if the chart has not dimensions
    if (!rrdset_number_of_dimensions(st)) {
        error("Skipping replication request for %s.%s because it has no dimensions",
              rrdhost_hostname(host), rrdset_id(st));
        rrdset_flag_set(st, RRDSET_FLAG_RECEIVER_REPLICATION_FINISHED);
        return send_replay_chart_cmd(outfp, rrdset_id(st), true, 0, 0);
    }

    // if the child's first/last entries are nonsensical, resume streaming
    // without asking for any data
    if (first_entry_child <= 0) {
        error("Skipping replication for %s.%s because FEC is %ld",
              rrdhost_hostname(host), rrdset_id(st), first_entry_child);
        rrdset_flag_set(st, RRDSET_FLAG_RECEIVER_REPLICATION_FINISHED);
        return send_replay_chart_cmd(outfp, rrdset_id(st), true, 0, 0);
    }
    if (first_entry_child > last_entry_child) {
        error("Skipping replication for %s.%s because FEC > LEC (%ld > %ld)",
              rrdhost_hostname(host), rrdset_id(st), first_entry_child, last_entry_child);
        rrdset_flag_set(st, RRDSET_FLAG_RECEIVER_REPLICATION_FINISHED);
        return send_replay_chart_cmd(outfp, rrdset_id(st), true, 0, 0);
    }

    time_t last_entry_local = rrdset_last_entry_t(st);

    // should never happen but it if does, start streaming without asking
    // for any data
    if (last_entry_local > last_entry_child) {
        error("Skipping replication for %s.%s because LEP > LEC (%ld > %ld)",
              rrdhost_hostname(host), rrdset_id(st), last_entry_local, last_entry_child);
        rrdset_flag_set(st, RRDSET_FLAG_RECEIVER_REPLICATION_FINISHED);
        return send_replay_chart_cmd(outfp, rrdset_id(st), true, 0, 0);
    }

    // ask for the next timestamp
    time_t first_entry_wanted = last_entry_local + 1;

    // make sure the 1st entry is GE than the child's
    if (first_entry_wanted < first_entry_child)
        first_entry_wanted = first_entry_child;

    // don't ask for more than `rrdpush_seconds_to_replicate`
    time_t now = now_realtime_sec();
    if ((now - first_entry_wanted) > host->rrdpush_seconds_to_replicate)
        first_entry_wanted = now - host->rrdpush_seconds_to_replicate;

    // ask the next X points
    time_t last_entry_wanted = first_entry_wanted + host->rrdpush_replication_step;

    // make sure we don't ask more than the child has
    if (last_entry_wanted > last_entry_child)
        last_entry_wanted = last_entry_child;

    // sanity check to make sure our time range is well formed
    if (first_entry_wanted > last_entry_wanted)
        first_entry_wanted = last_entry_wanted;

    // on subsequent calls we can use just the end time of the last entry
    // received
    if (prev_first_entry_wanted && prev_last_entry_wanted) {
        first_entry_wanted = prev_last_entry_wanted + st->update_every;
        last_entry_wanted = std::min(first_entry_wanted + host->rrdpush_replication_step,
                                     last_entry_child);
    }

    bool start_streaming = (last_entry_wanted == last_entry_child);

    return send_replay_chart_cmd(outfp, rrdset_id(st), start_streaming,
                                                       first_entry_wanted,
                                                       last_entry_wanted);
}
