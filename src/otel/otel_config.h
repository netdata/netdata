// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ND_OTEL_CONFIG_H
#define ND_OTEL_CONFIG_H

#include "libnetdata/libnetdata.h"

#include "yaml-cpp/exceptions.h"
#include <yaml-cpp/yaml.h>

#include "absl/status/status.h"
#include "absl/status/statusor.h"

namespace otel
{
class MetricConfig {
public:
    static absl::StatusOr<MetricConfig> load(const YAML::Node &Node)
    {
        std::string DimensionsAttribute;
        std::vector<std::string> InstanceAttributes;

        try {
            if (Node["dimensions_attribute"]) {
                DimensionsAttribute = Node["dimensions_attribute"].as<std::string>();
            }

            if (Node["instance_attributes"]) {
                InstanceAttributes = Node["instance_attributes"].as<std::vector<std::string> >();
            }

            return MetricConfig(DimensionsAttribute, InstanceAttributes);
        } catch (YAML::Exception &E) {
            std::stringstream SS;

            SS << "Failed to parse \"metrics\" node";
            if (!E.mark.is_null()) {
                SS << ":" << E.mark.line << ":" << E.mark.column;
            }
            if (!E.msg.empty()) {
                SS << " " << E.msg;
            }

            return absl::FailedPreconditionError(SS.str());
        }
    }

private:
    MetricConfig() = default;

    MetricConfig(std::string DimensionsAttribute, std::vector<std::string> InstanceAttributes)
        : DimensionsAttribute(DimensionsAttribute), InstanceAttributes(InstanceAttributes)
    {
    }

public:
    const std::string *getDimensionsAttribute() const
    {
        return &DimensionsAttribute;
    }

    const std::vector<std::string> *getInstanceAttributes() const
    {
        return &InstanceAttributes;
    }

private:
    std::string DimensionsAttribute;
    std::vector<std::string> InstanceAttributes;
};

class ScopeConfig {
public:
    static absl::StatusOr<ScopeConfig> load(const YAML::Node &Node)
    {
        std::unordered_map<std::string, MetricConfig> Metrics;

        try {
            for (const auto &M : Node["metrics"]) {
                auto MetricCfg = MetricConfig::load(M.second);
                Metrics.emplace(M.first.as<std::string>(), *MetricCfg);
            }

            return ScopeConfig(Metrics);
        } catch (YAML::Exception &E) {
            std::stringstream SS;

            SS << "Failed to parse \"metrics\" key";
            if (!E.mark.is_null()) {
                SS << ":" << E.mark.line << ":" << E.mark.column;
            }
            if (!E.msg.empty()) {
                SS << " " << E.msg;
            }

            return absl::FailedPreconditionError(SS.str());
        }
    }

private:
    ScopeConfig() = default;

    ScopeConfig(std::unordered_map<std::string, MetricConfig> Metrics) : Metrics(Metrics)
    {
    }

public:
    const MetricConfig *getMetric(const std::string &Name) const
    {
        auto It = Metrics.find(Name);
        return (It != Metrics.end()) ? &(It->second) : nullptr;
    }

private:
    std::unordered_map<std::string, MetricConfig> Metrics;
};

class Config {
public:
    static absl::StatusOr<Config *> load(const std::string &Path)
    {
        std::unordered_map<SIMPLE_PATTERN *, ScopeConfig> Patterns;
        std::unordered_map<std::string, ScopeConfig> Scopes;

        try {
            YAML::Node Node = YAML::LoadFile(Path);

            for (const auto &ScopeNode : Node["scopes"]) {
                const std::string &Key = ScopeNode.first.as<std::string>();
                SIMPLE_PATTERN *SP = simple_pattern_create(Key.c_str(), NULL, SIMPLE_PATTERN_EXACT, true);

                auto ScopeCfg = ScopeConfig::load(ScopeNode.second);
                if (!ScopeCfg.ok()) {
                    return ScopeCfg.status();
                }

                Patterns.emplace(SP, *ScopeCfg);
                Scopes.emplace(Key, *ScopeCfg);
            }

            return new Config(Path, Patterns, Scopes);
        } catch (YAML::Exception &E) {
            std::stringstream SS;

            SS << "Failed to load " << Path;
            if (!E.mark.is_null()) {
                SS << ":" << E.mark.line << ":" << E.mark.column;
            }
            if (!E.msg.empty()) {
                SS << " " << E.msg;
            }

            return absl::FailedPreconditionError(SS.str());
        }
    }

private:
    Config() = default;

    Config(
        const std::string &Path,
        std::unordered_map<SIMPLE_PATTERN *, ScopeConfig> Patterns,
        std::unordered_map<std::string, ScopeConfig> Scopes)
        : Path(Path), Patterns(Patterns), Scopes(Scopes)
    {
    }

public:
    const ScopeConfig *getScope(const std::string &Name) const
    {
        auto It = Scopes.find(Name);
        if (It != Scopes.end())
            return &It->second;

        return getScopeFromPatterns(Name);
    }

    const MetricConfig *getMetric(const std::string &ScopeName, const std::string &MetricName) const
    {
        const ScopeConfig *S = getScope(ScopeName);
        if (!S)
            return nullptr;

        return S->getMetric(MetricName);
    }

    const std::string *getDimensionsAttribute(const std::string &ScopeName, const std::string &MetricName) const
    {
        const MetricConfig *M = getMetric(ScopeName, MetricName);
        if (!M)
            return nullptr;

        return M->getDimensionsAttribute();
    }

    const std::vector<std::string> *
    getInstanceAttribute(const std::string &ScopeName, const std::string &MetricName) const
    {
        const MetricConfig *M = getMetric(ScopeName, MetricName);
        if (!M)
            return nullptr;

        return M->getInstanceAttributes();
    }

    void release()
    {
        for (auto &P : Patterns)
            simple_pattern_free(P.first);
    }

private:
    const ScopeConfig *getScopeFromPatterns(const std::string &Name) const
    {
        for (const auto &P : Patterns) {
            SIMPLE_PATTERN *SP = P.first;

            if (simple_pattern_matches(SP, Name.c_str())) {
                Scopes.emplace(Name.c_str(), P.second);
                return &P.second;
            }
        }

        return nullptr;
    }

private:
    std::string Path;
    std::unordered_map<SIMPLE_PATTERN *, ScopeConfig> Patterns;
    mutable std::unordered_map<std::string, ScopeConfig> Scopes;
};

} // namespace otel

#endif /* ND_OTEL_CONFIG_H */
