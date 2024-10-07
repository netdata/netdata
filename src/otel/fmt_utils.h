#ifndef ND_FMT_UTILS_H
#define ND_FMT_UTILS_H

#include "otel_utils.h"

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

#endif /* ND_FMT_UTILS_H */
