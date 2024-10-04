#ifndef OTEL_NETDATA_H
#define OTEL_NETDATA_H

#include "absl/strings/string_view.h"
#include "fmt/chrono.h"
#include "fmt/core.h"
#include "fmt/format.h"
#include "fmt/ranges.h"
#include <chrono>

namespace netdata
{

enum class Instruction {
    Begin,
    Chart,
    Dimension,
    Disable,
    End,
    Flush,
    Label,
    Set,
    Variable,
};

enum class ChartType { Line, Area, Stacked };

enum class Algorithm { Absolute, Incremental, PercentageOfAbsoluteRow, PercentageOfIncrementalRow };

enum class ChartOption { Detail, Hidden, Obsolete, StoreFirst };

enum class DimensionOption { Hidden, Obsolete };

enum class Scope { Chart, Global, Host, Local };

struct Dimension {
    std::string Id;
    std::string Name;
    Algorithm Alg;
    int Multiplier;
    int Divisor;
    std::vector<DimensionOption> Options;
};

struct Chart {
    absl::string_view Type;
    absl::string_view Id;
    absl::string_view Name;
    absl::string_view Title;
    absl::string_view Units;
    absl::string_view Family;
    absl::string_view Context;
    ChartType CT;
    int Priority;
    std::chrono::seconds UpdateEvery;
    std::vector<absl::string_view> Options;
    absl::string_view Plugin;
    absl::string_view Module;
};

struct Variable {
    Scope Scope_;
    std::string Name;
    double Value;
};

struct BeginInstruction {
    std::string TypeId;
    std::chrono::microseconds Microseconds;
};

struct SetInstruction {
    std::string Id;
    uint32_t Value;
};

inline absl::string_view instructionToString(Instruction Instr)
{
    switch (Instr) {
        case Instruction::Begin:
            return "BEGIN";
        case Instruction::Chart:
            return "CHART";
        case Instruction::Dimension:
            return "DIMENSION";
        case Instruction::Disable:
            return "DISABLE";
        case Instruction::End:
            return "END";
        case Instruction::Flush:
            return "FLUSH";
        case Instruction::Label:
            return "LABEL";
        case Instruction::Set:
            return "SET";
        case Instruction::Variable:
            return "Variable";
    }
}

inline absl::string_view chartTypeToString(ChartType CT)
{
    switch (CT) {
        case ChartType::Line:
            return "line";
        case ChartType::Area:
            return "area";
        case ChartType::Stacked:
            return "stacked";
    }
}

inline absl::string_view algorithmToString(Algorithm Alg)
{
    switch (Alg) {
        case Algorithm::Absolute:
            return "absolute";
        case Algorithm::Incremental:
            return "incremental";
        case Algorithm::PercentageOfAbsoluteRow:
            return "percentage-of-absolute-row";
        case Algorithm::PercentageOfIncrementalRow:
            return "percentage-of-incremental-row";
    }
}

inline absl::string_view scopeToString(Scope S)
{
    switch (S) {
        case Scope::Chart:
            return "Chart";
        case Scope::Global:
            return "global";
        case Scope::Host:
            return "Host";
        case Scope::Local:
            return "local";
    }
}

inline absl::string_view dimensionToString(DimensionOption DO)
{
    switch (DO) {
        case DimensionOption::Hidden:
            return "hidden";
        case DimensionOption::Obsolete:
            return "obsolete";
    }
}

inline absl::string_view chartToString(ChartOption CO)
{
    switch (CO) {
        case ChartOption::Detail:
            return "detail";
        case ChartOption::Hidden:
            return "hidden";
        case ChartOption::Obsolete:
            return "obsolete";
        case ChartOption::StoreFirst:
            return "store_first";
    }
}

} // namespace netdata

template <> struct fmt::formatter<netdata::Instruction> : fmt::formatter<absl::string_view> {
    template <typename FormatContext> auto format(netdata::Instruction Instr, FormatContext &Ctx) const
    {
        return formatter<string_view>::format(instructionToString(Instr), Ctx);
    }
};

template <> struct fmt::formatter<netdata::ChartType> : formatter<absl::string_view> {
    template <typename FormatContext> auto format(netdata::ChartType CT, FormatContext &Ctx) const
    {
        return formatter<string_view>::format(chartTypeToString(CT), Ctx);
    }
};

template <> struct fmt::formatter<netdata::Algorithm> : fmt::formatter<absl::string_view> {
    template <typename FormatContext> auto format(netdata::Algorithm Alg, FormatContext &Ctx) const
    {
        return formatter<string_view>::format(algorithmToString(Alg), Ctx);
    }
};

template <> struct fmt::formatter<netdata::DimensionOption> : fmt::formatter<absl::string_view> {
    template <typename FormatContext> auto format(netdata::DimensionOption DO, FormatContext &Ctx) const
    {
        return formatter<string_view>::format(dimensionToString(DO), Ctx);
    }
};

template <> struct fmt::formatter<netdata::ChartOption> : fmt::formatter<absl::string_view> {
    template <typename FormatContext> auto format(netdata::ChartOption CO, FormatContext &Ctx) const
    {
        return formatter<string_view>::format(chartToString(CO), Ctx);
    }
};

template <> struct fmt::formatter<netdata::Scope> : fmt::formatter<absl::string_view> {
    template <typename FormatContext> auto format(netdata::Scope S, FormatContext &Ctx) const
    {
        return formatter<string_view>::format(scopeToString(S), Ctx);
    }
};

template <> struct fmt::formatter<netdata::Dimension> {
    char Presentation = 'f';

    constexpr auto parse(format_parse_context &Ctx) -> decltype(Ctx.begin())
    {
        auto It = Ctx.begin(), End = Ctx.end();
        if (It != End && (*It == 'f' || *It == 's')) {
            Presentation = *It++;
        }

        if (It != End && *It != '}')
            throw format_error("invalid format");

        return It;
    }

    template <typename FormatContext> auto format(const netdata::Dimension &D, FormatContext &Ctx) const
    {
        if (Presentation == 's') {
            return fmt::format_to(Ctx.out(), "{}:{}", D.Id, D.Name);
        }

        return fmt::format_to(
            Ctx.out(),
            "{} {} {} {} {} {} {}",
            netdata::Instruction::Dimension,
            D.Id,
            D.Name,
            D.Alg,
            D.Multiplier,
            D.Divisor,
            D.Options);
    }
};

template <> struct fmt::formatter<netdata::Chart> {
    char Presentation = 'f';

    constexpr auto parse(format_parse_context &Ctx) -> decltype(Ctx.begin())
    {
        auto It = Ctx.begin(), End = Ctx.end();
        if (It != End && (*It == 'f' || *It == 's')) {
            Presentation = *It++;
        }

        if (It != End && *It != '}')
            throw format_error("invalid format");

        return It;
    }

    template <typename FormatContext> auto format(const netdata::Chart &C, FormatContext &Ctx) const
    {
        if (Presentation == 's') {
            return fmt::format_to(Ctx.out(), "{} {}:{}", C.Type, C.Id, C.Name);
        }

        return fmt::format_to(
            Ctx.out(),
            "{} \"{}.{}\" \"{}\" \"{}\" \"{}\" \"{}\" \"{}\" \"{}\" \"{}\" \"{}\" \"{}\" \"{}\" \"{}\"",
            netdata::Instruction::Chart,
            C.Type,
            C.Id,
            C.Name,
            C.Title,
            C.Units,
            C.Family,
            C.Context,
            C.CT,
            C.Priority,
            C.UpdateEvery.count(),
            fmt::join(C.Options,"|"),
            C.Plugin,
            C.Module);
    }
};

template <> struct fmt::formatter<netdata::Variable> {
    char Presentation = 'f';

    constexpr auto parse(format_parse_context &Ctx) -> decltype(Ctx.begin())
    {
        auto It = Ctx.begin(), End = Ctx.end();
        if (It != End && (*It == 'f' || *It == 's')) {
            Presentation = *It++;
        }

        if (It != End && *It != '}')
            throw format_error("invalid format");

        return It;
    }

    template <typename FormatContext> auto format(const netdata::Variable &Var, FormatContext &Ctx) const
    {
        if (Presentation == 's') {
            return fmt::format_to(Ctx.out(), "{}:{}", Var.Name, Var.Scope_);
        }

        return fmt::format_to(Ctx.out(), "{} {} {} = {}", netdata::Instruction::Variable, Var.Scope_, Var.Name, Var.Value);
    }
};

template <> struct fmt::formatter<netdata::BeginInstruction> {
    template <typename FormatContext> auto format(const netdata::BeginInstruction &Instr, FormatContext &Ctx) const
    {
        return fmt::format_to(Ctx.out(), "{} {} {}", netdata::Instruction::Begin, Instr.TypeId, Instr.Microseconds);
    }
};

template <> struct fmt::formatter<netdata::SetInstruction> {
    template <typename FormatContext> auto format(const netdata::SetInstruction &Instr, FormatContext &Ctx) const
    {
        return fmt::format_to(Ctx.out(), "{} {} {}", netdata::Instruction::Set, Instr.Id, Instr.Value);
    }
};

#endif /* OTEL_NETDATA_H */
