#ifndef NETDATA_OTEL_ITERATOR_H
#define NETDATA_OTEL_ITERATOR_H

#include "otel_utils.h"

#include "absl/status/status.h"
#include "absl/status/statusor.h"

using KeyValueArray = google::protobuf::RepeatedPtrField<pb::KeyValue>;

static inline absl::StatusOr<std::string> anyValueToString(const pb::AnyValue &V)
{
    if (V.has_string_value())
        return V.string_value();

    if (V.has_bool_value())
        return V.bool_value() ? "true" : "false";

    if (V.has_int_value())
        return std::to_string(V.int_value());

    if (V.has_double_value())
        return std::to_string(V.double_value());

    return absl::InvalidArgumentError("unknown conversion type");
}

enum class DataPointKind {
    Number,
    Sum,
    Summary,
    Histogram,
    Exponential,
    NotAvailable,
};

class DataPoint {
public:
    static const std::string DefaultDimensionName;

public:
    DataPoint() : DpKind(DataPointKind::NotAvailable)
    {
    }

    DataPoint(const pb::NumberDataPoint *NDP) : DpKind(DataPointKind::Number), NDP(NDP)
    {
    }

    DataPoint(const pb::SummaryDataPoint *SDP) : DpKind(DataPointKind::Summary), SDP(SDP)
    {
    }

    DataPoint(const pb::HistogramDataPoint *HDP) : DpKind(DataPointKind::Histogram), HDP(HDP)
    {
    }

    DataPoint(const pb::ExponentialHistogramDataPoint *EHDP) : DpKind(DataPointKind::Exponential), EHDP(EHDP)
    {
    }

    inline absl::StatusOr<const pb::AnyValue *> attribute(const std::string *Key) const
    {
        auto KVA = getAttrs();
        if (!KVA.ok()) {
            return KVA.status();
        }

        for (const auto &KV : *KVA.value()) {
            if (KV.key() != *Key)
                continue;

            if (!KV.has_value())
                return absl::NotFoundError("Datapoint key has no value");

            return &KV.value();
        }

        std::stringstream SS;
        SS << "Datapoint key not found: '" << Key->c_str() << "'";
        return absl::NotFoundError(SS.str());
    }

    inline absl::StatusOr<std::string> dimensionName(const std::string *Key) const
    {
        const auto &Result = attribute(Key);
        if (!Result.ok())
            return Result.status();

        return anyValueToString(*Result.value());
    }

    inline absl::StatusOr<std::string> instanceName(const std::set<std::string> *Keys) const
    {
        std::string Name;

        bool Underscore = false;
        for (const auto &Key : *Keys) {
            const auto &Result = attribute(&Key);
            if (!Result.ok())
                return Result.status();

            if (Underscore)
                Name += "_";

            auto Part = anyValueToString(*Result.value());
            if (!Part.ok()) {
                return Part.status();
            }

            Name += *Part;
            Underscore = true;
        }

        if (Keys->size() && Name.empty())
            fatal("Could not create instance name...");

        return Name;
    }

    uint64_t time() const
    {
        switch (DpKind) {
            case DataPointKind::Number:
            case DataPointKind::Sum:
                return NDP->time_unix_nano();
            case DataPointKind::Summary:
                return SDP->time_unix_nano();
            case DataPointKind::Histogram:
                return HDP->time_unix_nano();
            case DataPointKind::Exponential:
                return EHDP->time_unix_nano();
            case DataPointKind::NotAvailable:
                return 0;
        }

        return 0;
    }

    uint64_t value(uint64_t multiplier) const
    {
        switch (DpKind) {
            case DataPointKind::Number:
            case DataPointKind::Sum:
                if (NDP->has_as_double())
                    return NDP->as_double() * multiplier;
                else
                    return NDP->as_int() * multiplier;
            case DataPointKind::Summary:
            case DataPointKind::Histogram:
            case DataPointKind::Exponential:
            case DataPointKind::NotAvailable:
                return 0;
        }
    }

    friend inline bool operator==(const DataPoint &LHS, const DataPoint &RHS)
    {
        if (LHS.DpKind == RHS.DpKind) {
            switch (LHS.DpKind) {
                case DataPointKind::Number:
                case DataPointKind::Sum:
                    return LHS.NDP == RHS.NDP;
                case DataPointKind::Summary:
                    return LHS.SDP == RHS.SDP;
                case DataPointKind::Histogram:
                    return LHS.HDP == RHS.HDP;
                case DataPointKind::Exponential:
                    return LHS.EHDP == RHS.EHDP;
                default:
                    return false;
            }
        }

        return false;
    }

private:
    const absl::StatusOr<const KeyValueArray *> getAttrs() const
    {
        switch (DpKind) {
            case DataPointKind::Number:
            case DataPointKind::Sum:
                return &NDP->attributes();
            case DataPointKind::Summary:
                return &SDP->attributes();
            case DataPointKind::Histogram:
                return &HDP->attributes();
            case DataPointKind::Exponential:
                return &EHDP->attributes();
            default:
                return absl::InvalidArgumentError("Uknown data point kind");
        }
    }

private:
    DataPointKind DpKind;

    union {
        const pb::NumberDataPoint *NDP;
        const pb::SummaryDataPoint *SDP;
        const pb::HistogramDataPoint *HDP;
        const pb::ExponentialHistogramDataPoint *EHDP;
    };
};

struct OtelElement {
    const pb::ResourceMetrics *RM;
    const pb::ScopeMetrics *SM;
    const pb::Metric *M;
    DataPoint DP;

    OtelElement() : RM(nullptr), SM(nullptr), M(nullptr), DP()
    {
    }

public:
    inline uint64_t time() const
    {
        return DP.time();
    }

    inline uint64_t value(uint64_t multiplier) const
    {
        return DP.value(multiplier);
    }

    inline bool monotonic() const {
        if (!M->has_sum())
            return false;

        return M->sum().is_monotonic();
    }

    friend inline bool operator==(const OtelElement &LHS, const OtelElement &RHS)
    {
        return LHS.DP == RHS.DP;
    }
};

class OtelData {
    class Iterator {
    public:
        using ResourceMetricsIterator = typename pb::ConstFieldIterator<pb::ResourceMetrics>;
        using ScopeMetricsIterator = typename pb::ConstFieldIterator<pb::ScopeMetrics>;
        using MetricsIterator = typename pb::ConstFieldIterator<pb::Metric>;

        using NumberDataPointIterator = typename pb::ConstFieldIterator<pb::NumberDataPoint>;
        using SummaryDataPointIterator = typename pb::ConstFieldIterator<pb::SummaryDataPoint>;
        using HistogramDataPointIterator = typename pb::ConstFieldIterator<pb::HistogramDataPoint>;
        using ExponentialHistogramDataPointIterator =
            typename pb::ConstFieldIterator<pb::ExponentialHistogramDataPoint>;

        union DataPointIterator {
            NumberDataPointIterator NDPIt;
            SummaryDataPointIterator SDPIt;
            HistogramDataPointIterator HDPIt;
            ExponentialHistogramDataPointIterator EHDPIt;

            DataPointIterator()
            {
            }

            ~DataPointIterator()
            {
            }
        };

    public:
        using iterator_category = std::input_iterator_tag;
        using value_type = OtelElement;
        using difference_type = std::ptrdiff_t;
        using pointer = OtelElement *;
        using reference = OtelElement &;

    public:
        explicit Iterator(const pb::RepeatedPtrField<pb::ResourceMetrics> *RPF)
            : DPKind(DataPointKind::NotAvailable), End(true)
        {
            if (RPF) {
                RMIt = RPF->begin();
                RMEnd = RPF->end();

                if (RMIt != RMEnd) {
                    SMIt = RMIt->scope_metrics().begin();
                    SMEnd = RMIt->scope_metrics().end();

                    if (SMIt != SMEnd) {
                        MIt = SMIt->metrics().begin();
                        MEnd = SMIt->metrics().end();

                        if (MIt != MEnd) {
                            const pb::Metric &M = *MIt;
                            initializeDataPointIterator(M);
                            End = false;
                        }
                    }
                }
            }

            if (!End) {
                CurrOE = next();
            }
        }

        ~Iterator()
        {
            destroyCurrentIterator();
        }

        inline reference operator*()
        {
            return CurrOE;
        }

        inline pointer operator->()
        {
            return &CurrOE;
        }

        // Pre-increment operator
        Iterator &operator++()
        {
            if (hasNext()) {
                CurrOE = next();
            } else {
                End = true;
            }
            return *this;
        }

        // Post-increment operator
        inline Iterator operator++(int)
        {
            Iterator Tmp = *this;
            ++(*this);
            return Tmp;
        }

        inline bool operator==(const Iterator &Other) const
        {
            if (!End && !Other.End) {
                return CurrOE == Other.CurrOE;
            }

            return End == Other.End;
        }

        inline bool operator!=(const Iterator &Other) const
        {
            return !(*this == Other);
        }

    private:
        inline DataPointKind dpKind(const pb::Metric &M) const
        {
            if (M.has_gauge())
                return DataPointKind::Number;
            if (M.has_sum())
                return DataPointKind::Sum;
            if (M.has_summary())
                return DataPointKind::Summary;
            else if (M.has_histogram())
                return DataPointKind::Histogram;
            else if (M.has_exponential_histogram())
                return DataPointKind::Exponential;

            throw std::out_of_range("Unknown data point kind");
        }

        inline bool hasNext() const
        {
            if (RMIt == RMEnd)
                return false;

            if (SMIt == SMEnd)
                return false;

            if (MIt == MEnd)
                return false;

            switch (DPKind) {
                case DataPointKind::Number:
                case DataPointKind::Sum:
                    return DPIt.NDPIt != DPEnd.NDPIt;
                case DataPointKind::Summary:
                    return DPIt.SDPIt != DPEnd.SDPIt;
                case DataPointKind::Histogram:
                    return DPIt.HDPIt != DPEnd.HDPIt;
                case DataPointKind::Exponential:
                    return DPIt.EHDPIt != DPEnd.EHDPIt;
                case DataPointKind::NotAvailable:
                    return false;
                default:
                    throw std::out_of_range("WTF?");
            }
        }

        OtelElement next()
        {
            if (!hasNext())
                throw std::out_of_range("No more elements");

            // Fill element
            OtelElement OE;
            {
                OE.RM = &*RMIt;
                OE.SM = &*SMIt;
                OE.M = &*MIt;

                switch (DPKind) {
                    case DataPointKind::Number:
                    case DataPointKind::Sum:
                        OE.DP = DataPoint(&*DPIt.NDPIt);
                        break;
                    case DataPointKind::Summary:
                        OE.DP = DataPoint(&*DPIt.SDPIt);
                        break;
                    case DataPointKind::Histogram:
                        OE.DP = DataPoint(&*DPIt.HDPIt);
                        break;
                    case DataPointKind::Exponential:
                        OE.DP = DataPoint(&*DPIt.EHDPIt);
                        break;
                    case DataPointKind::NotAvailable:
                    default:
                        throw std::out_of_range("No more elements");
                }
            }

            advanceIterators();
            return OE;
        }

        inline void destroyCurrentIterator()
        {
            switch (DPKind) {
                case DataPointKind::Number:
                case DataPointKind::Sum:
                    DPIt.NDPIt.~NumberDataPointIterator();
                    DPEnd.NDPIt.~NumberDataPointIterator();
                    break;
                case DataPointKind::Summary:
                    DPIt.SDPIt.~SummaryDataPointIterator();
                    DPEnd.SDPIt.~SummaryDataPointIterator();
                    break;
                case DataPointKind::Histogram:
                    DPIt.HDPIt.~HistogramDataPointIterator();
                    DPEnd.HDPIt.~HistogramDataPointIterator();
                    break;
                case DataPointKind::Exponential:
                    DPIt.EHDPIt.~ExponentialHistogramDataPointIterator();
                    DPEnd.EHDPIt.~ExponentialHistogramDataPointIterator();
                    break;
                case DataPointKind::NotAvailable:
                    break;
                default:
                    throw std::out_of_range("Unknown data point kind");
            }
        }

        void initializeDataPointIterator(const pb::Metric &M)
        {
            DPKind = dpKind(M);

            switch (DPKind) {
                case DataPointKind::Number:
                    DPIt.NDPIt = M.gauge().data_points().begin();
                    DPEnd.NDPIt = M.gauge().data_points().end();
                    break;
                case DataPointKind::Sum:
                    DPIt.NDPIt = M.sum().data_points().begin();
                    DPEnd.NDPIt = M.sum().data_points().end();
                    break;
                case DataPointKind::Summary:
                    DPIt.SDPIt = M.summary().data_points().begin();
                    DPEnd.SDPIt = M.summary().data_points().end();
                    break;
                case DataPointKind::Histogram:
                    DPIt.HDPIt = M.histogram().data_points().begin();
                    DPEnd.HDPIt = M.histogram().data_points().end();
                    break;
                case DataPointKind::Exponential:
                    DPIt.EHDPIt = M.exponential_histogram().data_points().begin();
                    DPEnd.EHDPIt = M.exponential_histogram().data_points().end();
                    break;
                default:
                    throw std::out_of_range("WTF?");
            }
        }

        void advanceIterators()
        {
            switch (DPKind) {
                case DataPointKind::Number:
                case DataPointKind::Sum:
                    if (++DPIt.NDPIt != DPEnd.NDPIt)
                        return;
                    break;
                case DataPointKind::Summary:
                    if (++DPIt.SDPIt != DPEnd.SDPIt)
                        return;
                    break;
                case DataPointKind::Histogram:
                    if (++DPIt.HDPIt != DPEnd.HDPIt)
                        return;
                    break;
                case DataPointKind::Exponential:
                    if (++DPIt.EHDPIt != DPEnd.EHDPIt)
                        return;
                    break;
                case DataPointKind::NotAvailable:
                    // FIXME: What should we do here?
                    break;
            }

            if (++MIt != MEnd) {
                initializeDataPointIterator(*MIt);
                return;
            }

            if (++SMIt != SMEnd) {
                MIt = SMIt->metrics().begin();
                MEnd = SMIt->metrics().end();

                if (MIt != MEnd)
                    initializeDataPointIterator(*MIt);

                return;
            }

            if (++RMIt != RMEnd) {
                SMIt = RMIt->scope_metrics().begin();
                SMEnd = RMIt->scope_metrics().end();

                if (SMIt != SMEnd) {
                    MIt = SMIt->metrics().begin();
                    MEnd = SMIt->metrics().end();

                    if (MIt != MEnd)
                        initializeDataPointIterator(*MIt);
                }

                return;
            }
        }

    private:
        ResourceMetricsIterator RMIt, RMEnd;
        ScopeMetricsIterator SMIt, SMEnd;
        MetricsIterator MIt, MEnd;

        DataPointIterator DPIt, DPEnd;
        DataPointKind DPKind;

        OtelElement CurrOE;
        bool End;
    };

public:
    OtelData(const pb::RepeatedPtrField<pb::ResourceMetrics> *RPF) : RPF(RPF)
    {
    }

    inline Iterator begin()
    {
        return Iterator(RPF);
    }

    inline Iterator end() const
    {
        return Iterator(nullptr);
    }

private:
    const pb::RepeatedPtrField<pb::ResourceMetrics> *RPF;
};

#endif /* NETDATA_OTEL_ITERATOR_H */
