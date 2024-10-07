#ifndef ND_FMT_UTILS_H
#define ND_FMT_UTILS_H

#include "otel_utils.h"
#include "fmt/chrono.h"
#include <chrono>

template <> struct fmt::formatter<std::chrono::nanoseconds> : fmt::formatter<std::chrono::system_clock::time_point> {
    template <typename FormatContext> auto format(const std::chrono::nanoseconds &NS, FormatContext &Ctx) const
    {
        auto TP = std::chrono::system_clock::time_point() + NS;
        return fmt::formatter<std::chrono::system_clock::time_point>::format(TP, Ctx);
    }
};

template <> struct fmt::formatter<pb::AnyValue> {
    constexpr auto parse(format_parse_context &Ctx) -> decltype(Ctx.begin())
    {
        return Ctx.end();
    }

    template <typename FormatContext>
    auto format(const pb::AnyValue &AV, FormatContext &Ctx) const -> decltype(Ctx.out())
    {
        switch (AV.value_case()) {
            case pb::AnyValue::kStringValue:
                return fmt::format_to(Ctx.out(), "{}", AV.string_value());
            case pb::AnyValue::kBoolValue:
                return fmt::format_to(Ctx.out(), "{}", AV.bool_value());
            case pb::AnyValue::kIntValue:
                return fmt::format_to(Ctx.out(), "{}", AV.int_value());
            case pb::AnyValue::kDoubleValue:
                return fmt::format_to(Ctx.out(), "{}", AV.double_value());
            case pb::AnyValue::kArrayValue:
                return fmt::format_to(Ctx.out(), "{}", AV.array_value());
            case pb::AnyValue::kKvlistValue:
                return fmt::format_to(Ctx.out(), "{}", AV.kvlist_value());
            case pb::AnyValue::kBytesValue:
                return fmt::format_to(Ctx.out(), "<bytes-value>");
            default:
                return fmt::format_to(Ctx.out(), "<unknown>");
        }
    }
};

template <> struct fmt::formatter<pb::KeyValue> {
    constexpr auto parse(format_parse_context &Ctx) -> decltype(Ctx.begin())
    {
        return Ctx.end();
    }

    template <typename FormatContext>
    auto format(const pb::KeyValue &KV, FormatContext &Ctx) const -> decltype(Ctx.out())
    {
        return fmt::format_to(Ctx.out(), "{}: {}", KV.key(), KV.value());
    }
};

template <> struct fmt::formatter<pb::RepeatedPtrField<pb::KeyValue> > {
    constexpr auto parse(format_parse_context &Ctx) -> decltype(Ctx.begin())
    {
        return Ctx.end();
    }

    template <typename FormatContext>
    auto format(const pb::RepeatedPtrField<pb::KeyValue> &KVs, FormatContext &Ctx) const -> decltype(Ctx.out())
    {
        fmt::format_to(Ctx.out(), "{{");
        bool First = true;

        for (const auto &KV : KVs) {
            if (!First) {
                fmt::format_to(Ctx.out(), ", ");
            }
            fmt::format_to(Ctx.out(), "{}", KV);
            First = false;
        }

        return fmt::format_to(Ctx.out(), "}}");
    }
};

template <> struct fmt::formatter<pb::ArrayValue> {
    constexpr auto parse(format_parse_context &Ctx) -> decltype(Ctx.begin())
    {
        return Ctx.end();
    }

    template <typename FormatContext>
    auto format(const pb::ArrayValue &AV, FormatContext &Ctx) const -> decltype(Ctx.out())
    {
        return fmt::format_to(Ctx.out(), "{}", AV.values());
    }
};

template <> struct fmt::formatter<pb::KeyValueList> {
    constexpr auto parse(format_parse_context &Ctx) -> decltype(Ctx.begin())
    {
        return Ctx.end();
    }

    template <typename FormatContext>
    auto format(const pb::KeyValueList &KVL, FormatContext &Ctx) const -> decltype(Ctx.out())
    {
        fmt::format_to(Ctx.out(), "{{");

        bool First = true;
        for (const auto &KV : KVL.values()) {
            if (!First) {
                fmt::format_to(Ctx.out(), ", ");
            }
            fmt::format_to(Ctx.out(), "{}", KV);
            First = false;
        }

        return fmt::format_to(Ctx.out(), "}}");
    }
};

template <> struct fmt::formatter<pb::InstrumentationScope> {
    constexpr auto parse(format_parse_context &Ctx) -> decltype(Ctx.begin())
    {
        return Ctx.end();
    }

    template <typename FormatContext>
    auto format(const pb::InstrumentationScope &IS, FormatContext &Ctx) const -> decltype(Ctx.out())
    {
        fmt::format_to(Ctx.out(), "InstrumentationScope{{name: {}, version: {}", IS.name(), IS.version());

        if (IS.attributes_size() > 0) {
            fmt::format_to(Ctx.out(), ", attributes: {}", IS.attributes());
        }

        if (IS.dropped_attributes_count() > 0) {
            fmt::format_to(Ctx.out(), ", dropped_attributes_count: {}", IS.dropped_attributes_count());
        }

        return fmt::format_to(Ctx.out(), "}}");
    }
};

template <> struct fmt::formatter<pb::Resource> {
    constexpr auto parse(format_parse_context &Ctx) -> decltype(Ctx.begin())
    {
        return Ctx.end();
    }

    template <typename FormatContext>
    auto format(const pb::Resource &Res, FormatContext &Ctx) const -> decltype(Ctx.out())
    {
        fmt::format_to(Ctx.out(), "Resource{{");

        if (Res.attributes_size() > 0) {
            fmt::format_to(Ctx.out(), "attributes: {}", Res.attributes());
        }

        if (Res.dropped_attributes_count() > 0) {
            if (Res.attributes_size() > 0) {
                fmt::format_to(Ctx.out(), ", ");
            }
            fmt::format_to(Ctx.out(), "dropped_attributes_count: {}", Res.dropped_attributes_count());
        }

        return fmt::format_to(Ctx.out(), "}}");
    }
};

template <> struct fmt::formatter<pb::Exemplar> {
    constexpr auto parse(format_parse_context &Ctx) -> decltype(Ctx.begin())
    {
        return Ctx.end();
    }

    template <typename FormatContext>
    auto format(const pb::Exemplar &E, FormatContext &Ctx) const -> decltype(Ctx.out())
    {
        fmt::format_to(Ctx.out(), "Exemplar{{time_unix_nano: {}", E.time_unix_nano());

        if (E.has_as_double()) {
            fmt::format_to(Ctx.out(), ", value: {}", E.as_double());
        } else if (E.has_as_int()) {
            fmt::format_to(Ctx.out(), ", value: {}", E.as_int());
        }

        if (E.filtered_attributes_size() > 0) {
            fmt::format_to(Ctx.out(), ", filtered_attributes: {}", E.filtered_attributes());
        }

        if (!E.span_id().empty()) {
            fmt::format_to(Ctx.out(), ", span_id: {:02x}", fmt::join(E.span_id(), ""));
        }

        if (!E.trace_id().empty()) {
            fmt::format_to(Ctx.out(), ", trace_id: {:02x}", fmt::join(E.trace_id(), ""));
        }

        return fmt::format_to(Ctx.out(), "}}");
    }
};

template <> struct fmt::formatter<pb::NumberDataPoint> {
    constexpr auto parse(format_parse_context &Ctx) -> decltype(Ctx.begin())
    {
        return Ctx.end();
    }

    template <typename FormatContext>
    auto format(const pb::NumberDataPoint &NDP, FormatContext &Ctx) const -> decltype(Ctx.out())
    {
        fmt::format_to(Ctx.out(), "NumberDataPoint{{");

        if (NDP.attributes_size() > 0) {
            fmt::format_to(Ctx.out(), "attributes: {}, ", NDP.attributes());
        }

        if (NDP.start_time_unix_nano() != 0) {
            fmt::format_to(
                Ctx.out(), "start_time: {:%H:%M:%S}, ", std::chrono::nanoseconds(NDP.start_time_unix_nano()));
        }

        fmt::format_to(Ctx.out(), "time: {:%H:%M:%S}, ", std::chrono::nanoseconds(NDP.time_unix_nano()));

        if (NDP.value_case() == opentelemetry::proto::metrics::v1::NumberDataPoint::kAsDouble) {
            fmt::format_to(Ctx.out(), "value: {}", NDP.as_double());
        } else if (NDP.value_case() == opentelemetry::proto::metrics::v1::NumberDataPoint::kAsInt) {
            fmt::format_to(Ctx.out(), "value: {}", NDP.as_int());
        } else {
            fmt::format_to(Ctx.out(), "value: <unset>");
        }

        if (NDP.exemplars_size() > 0) {
            fmt::format_to(Ctx.out(), ", exemplars: [");
            bool first = true;
            for (const auto &exemplar : NDP.exemplars()) {
                if (!first) {
                    fmt::format_to(Ctx.out(), ", ");
                }
                fmt::format_to(Ctx.out(), "{}", exemplar);
                first = false;
            }
            fmt::format_to(Ctx.out(), "]");
        }

        if (NDP.flags() != 0) {
            fmt::format_to(Ctx.out(), ", flags: {}", NDP.flags());
        }

        return fmt::format_to(Ctx.out(), "}}");
    }
};

template <> struct fmt::formatter<pb::RepeatedPtrField<pb::NumberDataPoint> > {
    constexpr auto parse(format_parse_context &Ctx) -> decltype(Ctx.begin())
    {
        return Ctx.end();
    }

    template <typename FormatContext>
    auto format(const google::protobuf::RepeatedPtrField<pb::NumberDataPoint> &NDPs, FormatContext &Ctx) const
        -> decltype(Ctx.out())
    {
        if (NDPs.empty()) {
            return fmt::format_to(Ctx.out(), "[]");
        }

        fmt::format_to(Ctx.out(), "[");
        bool first = true;
        for (const auto &NDP : NDPs) {
            if (!first) {
                fmt::format_to(Ctx.out(), ", ");
            }
            fmt::format_to(Ctx.out(), "{}", NDP);
            first = false;
        }
        return fmt::format_to(Ctx.out(), "]");
    }
};

template <> struct fmt::formatter<pb::Gauge> {
    constexpr auto parse(format_parse_context &Ctx) -> decltype(Ctx.begin())
    {
        return Ctx.end();
    }

    template <typename FormatContext> auto format(const pb::Gauge &G, FormatContext &Ctx) const -> decltype(Ctx.out())
    {
        return fmt::format_to(Ctx.out(), "Gauge{{data_points: {}}}", G.data_points());
    }
};

template <> struct fmt::formatter<pb::Sum> {
    constexpr auto parse(format_parse_context &Ctx) -> decltype(Ctx.begin())
    {
        return Ctx.end();
    }

    template <typename FormatContext> auto format(const pb::Sum &S, FormatContext &Ctx) const -> decltype(Ctx.out())
    {
        fmt::format_to(Ctx.out(), "Sum{{data_points: {}", S.data_points());

        if (S.aggregation_temporality() != pb::AggregationTemporality::AGGREGATION_TEMPORALITY_UNSPECIFIED) {
            fmt::format_to(Ctx.out(), ", aggregation_temporality: {}", S.aggregation_temporality());
        }

        if (S.is_monotonic()) {
            fmt::format_to(Ctx.out(), ", is_monotonic: {}", S.is_monotonic());
        }

        return fmt::format_to(Ctx.out(), "}}");
    }
};

template <> struct fmt::formatter<pb::AggregationTemporality> {
    constexpr auto parse(format_parse_context &Ctx) -> decltype(Ctx.begin())
    {
        return Ctx.end();
    }

    template <typename FormatContext>
    auto format(const pb::AggregationTemporality &AT, FormatContext &Ctx) const -> decltype(Ctx.out())
    {
        const char *S = nullptr;

        switch (AT) {
            case pb::AggregationTemporality::AGGREGATION_TEMPORALITY_CUMULATIVE:
                S = "cumulative";
                break;
            case pb::AggregationTemporality::AGGREGATION_TEMPORALITY_DELTA:
                S = "delta";
                break;
            case pb::AggregationTemporality::AGGREGATION_TEMPORALITY_UNSPECIFIED:
            default:
                return Ctx.out();
        }

        return fmt::format_to(Ctx.out(), S);
    }
};

template <> struct fmt::formatter<pb::Metric> {
    constexpr auto parse(format_parse_context &Ctx) -> decltype(Ctx.begin())
    {
        return Ctx.end();
    }

    template <typename FormatContext> auto format(const pb::Metric &M, FormatContext &Ctx) const -> decltype(Ctx.out())
    {
        fmt::format_to(Ctx.out(), "Metric{{name: {}, description: {}, unit: {}", M.name(), M.description(), M.unit());

        switch (M.data_case()) {
            case opentelemetry::proto::metrics::v1::Metric::kGauge:
                fmt::format_to(Ctx.out(), ", gauge: {}", M.gauge());
                break;
            case opentelemetry::proto::metrics::v1::Metric::kSum:
                fmt::format_to(Ctx.out(), ", sum: {}", M.sum());
                break;
            case opentelemetry::proto::metrics::v1::Metric::kHistogram:
                fmt::format_to(Ctx.out(), ", histogram: <not supported>");
                break;
            case opentelemetry::proto::metrics::v1::Metric::kExponentialHistogram:
                fmt::format_to(Ctx.out(), ", exponential_histogram: <not supported>");
                break;
            case opentelemetry::proto::metrics::v1::Metric::kSummary:
                fmt::format_to(Ctx.out(), ", summary: <not supported>");
                break;
            default:
                fmt::format_to(Ctx.out(), ", data: <unset>");
                break;
        }

        if (M.metadata().size() > 0) {
            fmt::format_to(Ctx.out(), ", metadata: {}", M.metadata());
        }

        return fmt::format_to(Ctx.out(), "}}");
    }
};

#endif /* ND_FMT_UTILS_H */
