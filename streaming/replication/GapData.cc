#include "replication-private.h"

#include <iomanip>
#include <ctime>

using namespace replication;

void GapData::print(RRDHOST *RH) const {
    error("GD host: %s", rrdhost_hostname(RH));
    error("GD chart: %s", Chart.c_str());
    error("GD dimension: %s", Dimension.c_str());
    error("GD entries: %zu", StorageNumbers.size());

    size_t N = StorageNumbers.size();
    for (size_t Idx = 0; Idx != N; Idx++) {
        error("GD[%zu]: <time=%ld, value=%u>", Idx, Timestamps[Idx], StorageNumbers[Idx]);
    }
}

bool GapData::push(struct sender_state *Sender) {
    /*
     * Parent's dbengine functions will cause a crash if we send
     * a GapData with 0 entries.
     */
    if (StorageNumbers.size() == 0)
        return true;

    netdata_mutex_lock(&Sender->mutex);
    double MaxBufferCapacity = Sender->buffer->max_size;
    double RemainingBufferCapacity = cbuffer_remaining_capacity(Sender->buffer, false);
    double RemainingRatio = RemainingBufferCapacity / MaxBufferCapacity;
    netdata_mutex_unlock(&Sender->mutex);

    // Close enough but not 100% correct because we release the lock
    if (RemainingRatio < 0.25)
        return false;

    sender_start(Sender);
    buffer_sprintf(Sender->build, "FILLGAP \"%s\"\n", toBase64().c_str());
    sender_commit(Sender);

    return true;
}

protocol::ResponseFillGap GapData::toProto() {
    protocol::ResponseFillGap PM;

    PM.set_chart(Chart);
    PM.set_dimension(Dimension);

    std::adjacent_difference(Timestamps.begin(), Timestamps.end(), Timestamps.begin());
    PM.mutable_deltaencodedtimestamps()->Add(
        Timestamps.begin(),
        Timestamps.end()
    );
    std::partial_sum(Timestamps.begin(), Timestamps.end(), Timestamps.begin());

    PM.mutable_storagenumbers()->Add(
        StorageNumbers.begin(),
        StorageNumbers.end()
    );

    return PM;
}

GapData GapData::fromProto(const protocol::ResponseFillGap &PM) {
    GapData GD(PM.chart(), PM.dimension());

    GD.Timestamps.reserve(PM.deltaencodedtimestamps_size());
    for (int Idx = 0; Idx != PM.deltaencodedtimestamps_size(); Idx++)
        GD.Timestamps.push_back(PM.deltaencodedtimestamps(Idx));
    std::partial_sum(GD.Timestamps.begin(), GD.Timestamps.end(), GD.Timestamps.begin());

    GD.StorageNumbers.reserve(PM.storagenumbers_size());
    for (int Idx = 0; Idx != PM.storagenumbers_size(); Idx++)
        GD.StorageNumbers.push_back(PM.storagenumbers(Idx));

    return GD;
}

std::string GapData::toBase64() {
    protocol::ResponseFillGap PM = toProto();
    std::string PBS = PM.SerializeAsString();
    return base64_encode(PBS);
}

GapData GapData::fromBase64(const std::string &EncodedData) {
    protocol::ResponseFillGap PM;

    std::string DecodedData = base64_decode(EncodedData);
    if (!PM.ParseFromString(DecodedData))
        error("Could not decode protobuf message for GapData");

    return fromProto(PM);
}

#ifdef ENABLE_DBENGINE
bool GapData::flushToDBEngine(RRDHOST *RH) const {
    if (StorageNumbers.size() == 0) {
        error("[%s] No storage numbers to flush to DBEngine for %s.%s",
              rrdhost_hostname(RH), Chart.c_str(), Dimension.c_str());
        return false;
    }

    if (RH->rrd_memory_mode != RRD_MEMORY_MODE_DBENGINE) {
        error("[%s] host memory mode is not dbengine (dropping gap data for %s.%s.",
              rrdhost_hostname(RH), Chart.c_str(), Dimension.c_str());
        return false;
    }

    constexpr time_t MaxEntriesPerPage = RRDENG_BLOCK_SIZE / sizeof(storage_number);
    storage_number Page[MaxEntriesPerPage] = { 0 };

    RRDDIM_PAST_DATA DPD;
    memset(&DPD, 0, sizeof(DPD));

    DPD.host = RH;
    DPD.page = Page;

    rrdhost_rdlock(RH);
    DPD.st = rrdset_find(RH, Chart.c_str());
    if (!DPD.st) {
        error("[%s] Can not find chart %s", rrdhost_hostname(RH), Chart.c_str());
        rrdhost_unlock(RH);
        return false;
    }

    if (DPD.st->rrd_memory_mode != RRD_MEMORY_MODE_DBENGINE) {
        error("[%s] Can not fill gap data because chart %s is not using dbengine", rrdhost_hostname(RH), Chart.c_str());
        rrdhost_unlock(RH);
        return false;
    }

    rrdset_rdlock(DPD.st);
    DPD.rd = rrddim_find(DPD.st, Dimension.c_str());
    if (!DPD.rd) {
        error("[%s] Can not find dimension %s.%s", rrdhost_hostname(RH), Chart.c_str(), Dimension.c_str());
        rrdset_unlock(DPD.st);
        rrdhost_unlock(RH);
        return false;
    }

    if (!DPD.rd->update_every) {
        error("[%s] dimension %s.%s has update_every 0",
              rrdhost_hostname(RH), Chart.c_str(), Dimension.c_str());
        rrdset_unlock(DPD.st);
        rrdhost_unlock(RH);
        return false;
    }

    time_t PageIdx = 0;
    for (size_t Idx = 0; Idx != Timestamps.size(); Idx++) {
        if (Timestamps[Idx] % DPD.rd->update_every) {
            error("[%s] Unaligned replication data %s.%s (timestamp: %ld, update_every: %d)",
                  rrdhost_hostname(RH), Chart.c_str(), Dimension.c_str(),
                  Timestamps[Idx], DPD.rd->update_every);

            rrdset_unlock(DPD.st);
            rrdhost_unlock(RH);
            return false;
        }

        PageIdx = (Timestamps[Idx] - Timestamps[0]) / DPD.rd->update_every;
        if (PageIdx >= MaxEntriesPerPage) {
            error("[%s] Dropping %zu items for %s.%s because they don't fit in a single dbengine page",
                  rrdhost_hostname(RH), PageIdx - MaxEntriesPerPage + 1, Chart.c_str(), Dimension.c_str());

            PageIdx -= DPD.rd->update_every;
            break;
        }

        Page[PageIdx] = StorageNumbers[Idx];
    }

    DPD.start_time = Timestamps[0] * USEC_PER_SEC;
    DPD.end_time = (Timestamps[0] + (PageIdx * DPD.rd->update_every)) * USEC_PER_SEC;
    DPD.page_length = (PageIdx + 1) * sizeof(storage_number);

    rrdeng_store_past_metrics_realtime(DPD.rd, &DPD);

    rrdset_unlock(DPD.st);
    rrdhost_unlock(RH);
    return true;
}
#else
bool GapData::flushToDBEngine(RRDHOST *RH) const {
    error("[%s] Can not fill gap data for %s.%s because the agent does not support DBEngine",
          rrdhost_hostname(RH), Chart.c_str(), Dimension.c_str());
    return false;
}
#endif
