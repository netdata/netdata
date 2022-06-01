#include "replication-private.h"
#include <google/protobuf/io/zero_copy_stream_impl.h>

using namespace replication;

std::vector<TimeRange> replication::splitTimeRange(const TimeRange &TR, size_t Epoch) {
    size_t Duration = TR.second - TR.first + 1;
    size_t NumEpochs = (Duration / Epoch) + (Duration % Epoch != 0);

    std::vector<TimeRange> TRs;
    TRs.reserve(NumEpochs);

    for (time_t Offset = TR.first; TRs.size() != NumEpochs; Offset += Epoch)
        TRs.emplace_back(Offset, Offset + (Epoch - 1));
    TRs.back().second = TR.second;

    return TRs;
}

std::vector<TimeRange> replication::coalesceTimeRanges(std::vector<TimeRange> &TRs) {
    std::sort(TRs.rbegin(), TRs.rend());

    for (size_t Idx = 0; Idx != TRs.size(); Idx++) {
        const TimeRange &TR = TRs[Idx];
        const size_t Len = 1024;

        auto AfterTM = *std::localtime(&TR.first);
        char AfterBuf[Len];
        strftime(AfterBuf, Len, "%H:%M:%S", &AfterTM);

        auto BeforeTM = *std::localtime(&TR.second);
        char BeforeBuf[Len];
        strftime(BeforeBuf, Len, "%H:%M:%S", &BeforeTM);

        time_t Duration = TR.second - TR.first +1;

        error("GVD: TR[%zu/%zu] = [%s, %s] (Duration=%ld)",
              Idx + 1, TRs.size(), AfterBuf, BeforeBuf, Duration);
    }

    {
        while (TRs.size() > Cfg.MaxNumGapsToReplicate)
            TRs.pop_back();

        std::reverse(TRs.begin(), TRs.end());
    }

    std::vector<TimeRange> RetTRs;

    if (TRs.size() == 0)
        return RetTRs;

    // Pick the most recent connection time when the latest db time is the same
    RetTRs.push_back(TRs[0]);
    for (size_t Idx = 1; Idx != TRs.size(); Idx++) {
        if (RetTRs.back().first == TRs[Idx].first)
            RetTRs.back() = TRs[Idx];
        else
            RetTRs.push_back(TRs[Idx]);
    }

    /*
     * TODO: should we coalesce gaps that are than 1024 seconds apart???
     *       we would end up with more data transferred but fewer new DB
     *       engine pages created.
     */

    for (size_t Idx = 0; Idx != RetTRs.size(); Idx++) {
        const TimeRange &TR = RetTRs[Idx];
        const size_t Len = 1024;

        auto AfterTM = *std::localtime(&TR.first);
        char AfterBuf[Len];
        strftime(AfterBuf, Len, "%H:%M:%S", &AfterTM);

        auto BeforeTM = *std::localtime(&TR.second);
        char BeforeBuf[Len];
        strftime(BeforeBuf, Len, "%H:%M:%S", &BeforeTM);

        time_t Duration = TR.second - TR.first +1;

        error("GVD: RetTR[%zu/%zu] = [%s, %s] (Duration=%ld)",
              Idx + 1, RetTRs.size(), AfterBuf, BeforeBuf, Duration);
    }

    return RetTRs;
}

std::string replication::serializeTimeRangesToString(const std::vector<TimeRange> &TRs) {
    protocol::RequestFillGaps RFGs;
    for (const TimeRange &TR : TRs) {
        protocol::TimeRange *ProtoTR = RFGs.add_timeranges();
        ProtoTR->set_after(TR.first);
        ProtoTR->set_before(TR.second);
    }

    std::string Buf;
    RFGs.SerializeToString(&Buf);
    return Buf;
}

std::vector<TimeRange> replication::deserializeTimeRangesFromArray(const char *Buf, size_t Len) {
    std::vector<TimeRange> TRs;

    protocol::RequestFillGaps RFGs;
    if (!RFGs.ParseFromArray(Buf, Len))
        return TRs;

    size_t N = RFGs.timeranges_size();
    if (!N)
        return TRs;

    TRs.reserve(N);
    for (uint32_t Idx = 0; Idx != N; Idx++) {;
        const protocol::TimeRange &ProtoTR = RFGs.timeranges(Idx);
        TRs.push_back({ ProtoTR.after(), ProtoTR.before() });
    }

    return TRs;
}
