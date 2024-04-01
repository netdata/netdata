#ifndef NETDATA_OTEL_INGEST_HPP
#define NETDATA_OTEL_INGEST_HPP

#include "otel_config.hpp"
#include "otel_sort.hpp"
#include "otel_transform.hpp"
#include "otel_process.hpp"

#include <absl/status/statusor.h>
#include <absl/types/optional.h>
#include <absl/strings/str_cat.h>
#include <fstream>

namespace otel
{
class BufferManager {
public:
    void fill(const uv_buf_t &Buf);

    uint32_t messageLength() const;

    absl::StatusOr<pb::MetricsData *> readMetricData(pb::Arena *A, uint32_t N);

private:
    inline size_t remainingBytes() const
    {
        return Data.size() - Pos;
    }

    inline bool haveAtLeastXBytes(uint32_t Bytes) const
    {
        return remainingBytes() >= Bytes;
    }

private:
    std::vector<char> Data;
    size_t Pos = {0};
};

class Otel {
public:
    static absl::StatusOr<Otel *> get(const std::string &Path)
    {
        auto Cfg = Config::load(Path);
        if (!Cfg.ok()) {
            return Cfg.status();
        }

        return new Otel(*Cfg);
    }

    bool processMessages(const uv_buf_t &Buf)
    {
        BM.fill(Buf);

        uint32_t MessageLength = BM.messageLength();
        if (MessageLength == 0)
            return true;

        auto Result = BM.readMetricData(&A, MessageLength);
        if (!Result.ok())
            return true;

        auto *MD = Result.value();

        {
            pb::transformMetricData(Ctx.config(), *MD);
            pb::sortMetricsData(*MD);

            MetricsDataProcessor MDP(Ctx);
            Data D(*MD, MDP);
            for (Element E : D) {
                UNUSED(E);
            }

            A.Reset();
        }

        return true;
    }

private:
    template <typename T> void dump(const std::string &Path, const T *PB)
    {
        std::ofstream OS(Path, std::ios_base::app);
        if (OS.is_open()) {
            OS << PB->Utf8DebugString() << std::endl;
            OS.close();
        } else {
            std::cerr << "Unable to open /tmp/foo.txt for appending" << std::endl;
        }
    }

private:
    Otel(const Config *Cfg) : Ctx(Cfg)
    {
    }

private:
    ProcessorContext Ctx;

    pb::Arena A;
    BufferManager BM;
};

} // namespace otel

#endif /* NETDATA_OTEL_INGEST_HPP */
