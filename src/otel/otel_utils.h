#ifndef OTEL_UTILS_HPP
#define OTEL_UTILS_HPP

#include <iostream>
#include "opentelemetry/proto/metrics/v1/metrics.pb.h"

namespace pb
{

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

template <typename Element>
using ConstFieldIterator = typename ::google::protobuf::RepeatedPtrField<Element>::const_iterator;

template <typename Element>
using FieldIterator = typename ::google::protobuf::RepeatedPtrField<Element>::const_iterator;

void printAnyValue(std::ostream &OS, const AnyValue &Value);
void printArrayValue(std::ostream &OS, const ArrayValue &Value);
void printKeyValueList(std::ostream &OS, const KeyValueList &Value);
void printInstrumentationScope(std::ostream &OS, const InstrumentationScope &IS);

void printResource(std::ostream &OS, const Resource &Res);
void printExemplar(std::ostream &OS, const Exemplar &Ex);
void printNumberDataPoint(std::ostream &OS, const NumberDataPoint &NDP);
void printSummaryDataPoint(std::ostream &OS, const SummaryDataPoint &SDP);
void printHistogramDataPoint(std::ostream &OS, const HistogramDataPoint &HDP);
void printExponentialHistogramDataPoint(std::ostream &OS, const ExponentialHistogramDataPoint &EHDP);

void printGauge(std::ostream &OS, const Gauge &G);
void printSum(std::ostream &OS, const Sum &S);
void printHistogram(std::ostream &OS, const Histogram &H);
void printExponentialHistogram(std::ostream &OS, const ExponentialHistogram &EH);
void printSummary(std::ostream &OS, const Summary &S);

void printMetric(std::ostream &OS, const Metric &M);
void printScopeMetrics(std::ostream &OS, const ScopeMetrics &SM);
void printResourceMetrics(std::ostream &OS, const ResourceMetrics &RM);
void printMetricsData(std::ostream &OS, const MetricsData &MD);

} // namespace pb

#endif /* OTEL_UTILS_HPP */
