#include "otel_ingestor.h"
#include "otel_utils.h"

using namespace ingestor;
  
template<typename T>
void BufferManager<T>::fill(const uv_buf_t &buf)
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

template<typename T>
uint32_t BufferManager<T>::messageLength() const {
    if (!haveAtLeastXBytes(sizeof(uint32_t)))
        return 0;

    uint32_t MessageBytes = 0;
    memcpy(&MessageBytes, &Data[Pos], sizeof(uint32_t));
    MessageBytes = ntohl(MessageBytes);

    if (!haveAtLeastXBytes(sizeof(uint32_t) + MessageBytes))
        return 0;

    return MessageBytes;
}

template<typename T>
absl::StatusOr<T> BufferManager<T>::readMessage(uint32_t MessageLength)
{
    uv_buf_t Dst = {.base = nullptr, .len = 0};

    Pos += sizeof(uint32_t);
    Dst.base = &Data[Pos];
    Dst.len = MessageLength;

    T Msg;
    if (!Msg.ParseFromArray(Dst.base, Dst.len))
        return absl::InternalError("failed to parse protobuf message");

    Pos += MessageLength;
    return Msg;
}

template class ingestor::BufferManager<pb::MetricsData>;
