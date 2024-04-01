#ifndef NETDATA_OTEL_INGESTOR_H
#define NETDATA_OTEL_INGESTOR_H

#include "database/rrd.h"
#include "metadata.h"
#include "otel_iterator.h"
#include "otel_utils.h"

#include "libnetdata/libnetdata.h"
#include <absl/status/statusor.h>
#include <absl/types/optional.h>
#include <absl/strings/str_cat.h>
#include <vector>
#include <fstream>

static inline struct tm *convertTimevalToTm(const struct timeval &tv) {
    time_t rawtime = tv.tv_sec;
    struct tm *timeinfo = localtime(&rawtime);
    return timeinfo;
}

static inline std::string getHourAndMinutes(const struct timeval &tv) {
    struct tm *timeinfo = convertTimevalToTm(tv);

    // Use strftime to format the time as HH:MM
    char buffer[128];
    strftime(buffer, sizeof(buffer), "%H:%M:%S", timeinfo);
    return buffer;
}

namespace ingestor {
  
template <typename T>
class BufferManager
{
public:
    void fill(const uv_buf_t &Buf);

    uint32_t messageLength() const;

    absl::StatusOr<T> readMessage(uint32_t MessageLength);

private:
    inline size_t remainingBytes() const {
        return Data.size() - Pos;
    }

    inline bool haveAtLeastXBytes(uint32_t Bytes) const {
        return remainingBytes() >= Bytes;
    }

private:
    std::vector<char> Data;
    size_t Pos = {0};
};

class MetricInstance {
public:
    MetricInstance() : RS(nullptr), Dimensions(), OEV(), LastCollectionTime(0) {}

    inline void addOtelElement(const OtelElement &OE) {
        OEV.push_back(OE);
    }

    void flush(const std::string &Name) {
        if (OEV.empty()) {
            LastCollectionTime = 0;
            return;
        }

        if (!LastCollectionTime) {
            LastCollectionTime = OEV[0].time();
            OEV.clear();
            return;
        }

        if (!RS)
            setupChart(Name);

        std::ofstream OS("/tmp/output.txt", std::ios::app);
        if (!OS)
            fatal("Failed to oppen out file");

        for (const auto &OE : OEV)
        {
            RRDDIM *RD = getOrCreateRD(OE);

            struct timeval PIT;
            PIT.tv_sec = OEV[0].time() / NSEC_PER_SEC;
            PIT.tv_usec = 0;

            OS << "RS: " << string2str(RS->id) << ", RD: " << string2str(RD->id) << ", PIT: " << getHourAndMinutes(PIT) << ", V: " << OE.value(1000) << std::endl;
            rrddim_timed_set_by_pointer(RS, RD, PIT, OE.value(1000));
        }
        rrdset_done(RS);

        OEV.clear();
    }

private:
    bool homogeneousCollectionTime() const {
        std::pair<usec_t, usec_t> P = {
            std::numeric_limits<usec_t>::max(),
            std::numeric_limits<usec_t>::min(),
        };

        for (const auto &OE: OEV) {
            if (P.first > OE.time())
                P.first = OE.time();

            if (P.second < OE.time())
                P.second = OE.time();
        }

        return P.first == P.second;
    }

    void setupChart(const std::string &Id)
    {
        const pb::ScopeMetrics *SM = OEV[0].SM;
        const pb::Metric *M = OEV[0].M;

        usec_t UpdateEvery = (OEV[0].time() - LastCollectionTime) / NSEC_PER_SEC;

        RS = rrdset_create_localhost(
            SM->scope().name().c_str(),
            Id.c_str() + (SM->scope().name().length() + 1),
            NULL,
            M->name().c_str(),
            M->name().c_str(),
            M->description().c_str(),
            M->unit().c_str(),
            "otel.plugin",
            "otel.module",
            666666,
            UpdateEvery,
            RRDSET_TYPE_LINE);
    }

    RRDDIM *getOrCreateRD(const OtelElement &OE) {
        const auto &DimensionName = OE.dimensionName();
        if (!DimensionName.ok())
            fatal("Failed to get dimension name");

        auto It = Dimensions.find(*DimensionName);
        if (It == Dimensions.end()) {
            auto Algorithm = OE.monotonic() ? RRD_ALGORITHM_INCREMENTAL : RRD_ALGORITHM_ABSOLUTE;
            RRDDIM *RD = rrddim_add(RS, DimensionName->c_str(), NULL, 1, 1000, Algorithm);
            Dimensions[*DimensionName] = RD;
            return RD;
        }

        return It->second;
    }

private:
    RRDSET *RS;
    std::unordered_map<std::string, RRDDIM *> Dimensions;
    std::vector<OtelElement> OEV;

    uint64_t LastCollectionTime;
};

class Otel
{
public:
    static Otel* get(const std::string &Path)
    {
        otel::config::Config *Cfg = new otel::config::Config(Path);
        return new Otel(Cfg);
    }

    bool processMessages(const uv_buf_t &Buf)
    {
        BM.fill(Buf);

        uint32_t MessageLength = BM.messageLength();
        if (MessageLength == 0)
            return true;

        auto MD = BM.readMessage(MessageLength);
        if (!MD.ok())
            return true;

        processMetricsData(*MD);
        return true;
    }

private:
    void processMetricsData(const pb::MetricsData &MD) {
        std::vector<pb::MetricsData> V = { MD };

        auto OD = OtelData(Cfg, V);
        for (const auto &OE: OD) {
            const auto &InstanceName = OE.instanceName();
            if (!InstanceName.ok()) {
                fatal("instance name not found");
                continue; // :trollface
            }
            std::ofstream OS("/tmp/output.txt", std::ios::app);
            if (!OS)
                fatal("Failed to oppen out file");

            OS << "Handling instance name " << *InstanceName << std::endl;

            MetricInstances[*InstanceName].addOtelElement(OE);
        }

        for (auto &P : MetricInstances) {
            P.second.flush(P.first);
        }
    }

private:
    Otel(const otel::config::Config *Cfg) : Cfg(Cfg) { }

private:
    const otel::config::Config *Cfg;
    ingestor::BufferManager<pb::MetricsData> BM;
    std::unordered_map<std::string, MetricInstance> MetricInstances;
};

} // namespace ingestor

#endif /* NETDATA_OTEL_INGESTOR_H */
