#ifndef REPLICATION_GAPDATA_H
#define REPLICATION_GAPDATA_H

#include "replication-private.h"

namespace replication {

class GapData {
public:
    static GapData fromBase64(const std::string &EncodedData);

    GapData(std::string Chart, std::string Dimension)
        : Chart(Chart), Dimension(Dimension) {}

public:
    std::string getChart() const {
        return Chart;
    }

    void setChart(std::string Name) {
        Chart = Name;
    }

    std::string getDimension() const {
        return Dimension;
    }

    void setDimension(std::string Name) {
        Dimension = Name;
    }

    void setPayload(std::vector<time_t> Timestamps, std::vector<storage_number> StorageNumbers) {
        if (!Timestamps.size())
            return;

        this->Timestamps = Timestamps;
        this->StorageNumbers = StorageNumbers;
    }

    std::pair<size_t, TimeRange> getTimeRangeSpan() const {
        if (!StorageNumbers.size()) {
            return { 0, TimeRange(0, 0) };
        }

        return { StorageNumbers.size(), TimeRange(Timestamps[0], Timestamps.back()) };
    }

    void print(RRDHOST *RH) const;

    bool push(struct sender_state *sender);

    std::string toBase64();

    bool flushToDBEngine(RRDHOST *RH) const;

private:
    protocol::ResponseFillGap toProto();
    static GapData fromProto(const protocol::ResponseFillGap &PM);

private:
    std::string Chart;
    std::string Dimension;

    std::vector<time_t> Timestamps;
    std::vector<storage_number> StorageNumbers;
};

} // namespace replication

#endif /* REPLICATION_GAPDATA_H */
