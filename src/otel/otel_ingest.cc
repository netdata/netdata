#include "otel_ingest.hpp"

void otel::BufferManager::fill(const uv_buf_t &buf)
{
    if (Pos > Data.size())
        fatal("invalid position");
    else if (Pos == Data.size()) {
        Data.clear();
    } else if (Pos != 0) {
        Data.erase(Data.begin(), Data.begin() + Pos);
    }

    Pos = 0;
    Data.insert(Data.end(), buf.base, buf.base + buf.len);
}

uint32_t otel::BufferManager::messageLength() const
{
    if (!haveAtLeastXBytes(sizeof(uint32_t)))
        return 0;

    uint32_t MessageBytes = 0;
    memcpy(&MessageBytes, &Data[Pos], sizeof(uint32_t));
    MessageBytes = ntohl(MessageBytes);

    if (!haveAtLeastXBytes(sizeof(uint32_t) + MessageBytes))
        return 0;

    return MessageBytes;
}

absl::StatusOr<pb::MetricsData *> otel::BufferManager::readMetricData(pb::Arena *A, uint32_t N)
{
    uv_buf_t Dst = {.base = nullptr, .len = 0};

    Pos += sizeof(uint32_t);
    Dst.base = &Data[Pos];
    Dst.len = N;

    pb::MetricsData *MD = pb::Arena::CreateMessage<pb::MetricsData>(A);
    if (!MD->ParseFromArray(Dst.base, Dst.len))
        return absl::InternalError("failed to parse protobuf message");

    Pos += N;
    return MD;
}
