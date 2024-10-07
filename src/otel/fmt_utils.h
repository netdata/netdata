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
                return fmt::format_to(Ctx.out(), "{}", AV.array_value().values());
            case pb::AnyValue::kKvlistValue:
                return format_kvlist(AV.kvlist_value(), Ctx);
            case pb::AnyValue::kBytesValue:
                return fmt::format_to(Ctx.out(), "<bytes-value>");
            default:
                return fmt::format_to(Ctx.out(), "<unknown>");
        }
    }

private:
    template <typename FormatContext>
    auto format_kvlist(const pb::KeyValueList &KVL, FormatContext &Ctx) const -> decltype(Ctx.out())
    {
        fmt::format_to(Ctx.out(), "{{");

        bool First = true;
        for (const auto &KV : KVL.values()) {
            if (!First) {
                fmt::format_to(Ctx.out(), ", ");
            }
            fmt::format_to(Ctx.out(), "{}: {}", KV.key(), KV.value());
            First = false;
        }
        return fmt::format_to(Ctx.out(), "}}");
    }
};

#endif /* ND_FMT_UTILS_H */
