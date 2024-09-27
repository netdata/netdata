// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ND_OTEL_UTILS_H
#define ND_OTEL_UTILS_H

#include "opentelemetry/proto/metrics/v1/metrics.pb.h"

namespace pb
{
using Arena = google::protobuf::Arena;
template <typename Element> using RepeatedPtrField = google::protobuf::RepeatedPtrField<Element>;
template <typename Element> using ConstFieldIterator = typename RepeatedPtrField<Element>::const_iterator;
template <typename Element> using FieldIterator = typename RepeatedPtrField<Element>::const_iterator;

void dumpArenaStats(const std::string &Path, const std::string &Label, const pb::Arena &A);

using AnyValue = opentelemetry::proto::common::v1::AnyValue;
using ArrayValue = opentelemetry::proto::common::v1::ArrayValue;
using KeyValue = opentelemetry::proto::common::v1::KeyValue;
using KeyValueList = opentelemetry::proto::common::v1::KeyValueList;
using InstrumentationScope = opentelemetry::proto::common::v1::InstrumentationScope;

using Resource = opentelemetry::proto::resource::v1::Resource;

using MetricsData = opentelemetry::proto::metrics::v1::MetricsData;
using ResourceMetrics = opentelemetry::proto::metrics::v1::ResourceMetrics;
using ScopeMetrics = opentelemetry::proto::metrics::v1::ScopeMetrics;
using Metric = opentelemetry::proto::metrics::v1::Metric;
using NumberDataPoint = opentelemetry::proto::metrics::v1::NumberDataPoint;
using DataPointFlags = opentelemetry::proto::metrics::v1::DataPointFlags;
using SummaryDataPoint = opentelemetry::proto::metrics::v1::SummaryDataPoint;
using Exemplar = opentelemetry::proto::metrics::v1::Exemplar;
using HistogramDataPoint = opentelemetry::proto::metrics::v1::HistogramDataPoint;
using ExponentialHistogramDataPoint = opentelemetry::proto::metrics::v1::ExponentialHistogramDataPoint;

using Gauge = opentelemetry::proto::metrics::v1::Gauge;
using Sum = opentelemetry::proto::metrics::v1::Sum;
using Histogram = opentelemetry::proto::metrics::v1::Histogram;
using ExponentialHistogram = opentelemetry::proto::metrics::v1::ExponentialHistogram;
using Summary = opentelemetry::proto::metrics::v1::Summary;

std::string anyValueToString(const AnyValue &AV);
uint64_t findOldestCollectionTime(const pb::Metric &M);

template <typename DataPoint> uint64_t collectionTime(const DataPoint &DP);

} // namespace pb

#endif /* ND_OTEL_UTILS_H */
