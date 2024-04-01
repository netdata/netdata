#include "otel_utils.h"

using Buckets = opentelemetry::proto::metrics::v1::ExponentialHistogramDataPoint::Buckets;

static void printBuckets(std::ostream &OS, const Buckets &B)
{
    OS << "Offset: " << B.offset() << "\n";

    OS << "Bucket Counts: ";
    for (const auto &Count : B.bucket_counts())
    {
        OS << Count << " ";
    }

    OS << "\n";
}

void pb::printAnyValue(std::ostream &OS, const pb::AnyValue &Value)
{
    switch (Value.value_case())
    {
        case pb::AnyValue::kStringValue:
            OS << Value.string_value();
            break;
        case pb::AnyValue::kBoolValue:
            OS << std::boolalpha << Value.bool_value();
            break;
        case pb::AnyValue::kIntValue:
            OS << Value.int_value();
            break;
        case pb::AnyValue::kDoubleValue:
            OS << Value.double_value();
            break;
        case pb::AnyValue::kArrayValue:
            pb::printArrayValue(OS, Value.array_value());
            break;
        case pb::AnyValue::kKvlistValue:
            pb::printKeyValueList(OS, Value.kvlist_value());
            break;
        case pb::AnyValue::kBytesValue:
            OS << "[bytes]"; // Placeholder, handling bytes can be more complex
            break;
        default:
            OS << "Empty or unknown type";
    }
}

void pb::printArrayValue(std::ostream &OS, const pb::ArrayValue &AV)
{
    OS << "[";
    for (int i = 0; i < AV.values_size(); ++i)
    {
        printAnyValue(OS, AV.values(i));
        if (i < AV.values_size() - 1)
        {
            OS << ", ";
        }
    }
    OS << "]";
}

void pb::printKeyValueList(std::ostream &OS, const pb::KeyValueList &KVL)
{
    OS << "{";

    for (int i = 0; i < KVL.values_size(); ++i)
    {
        const pb::KeyValue &KV = KVL.values(i);

        OS << KV.key() << ": ";
        printAnyValue(OS, KV.value());

        if (i < KVL.values_size() - 1)
            OS << ", ";
    }

    OS << "}";
}

void pb::printInstrumentationScope(std::ostream &OS, const pb::InstrumentationScope &IS)
{
    OS << "Instrumentation Scope:\n";
    OS << "Name: " << IS.name() << "\n";
    OS << "Version: " << IS.version() << "\n";
    OS << "Attributes:\n";

    for (const auto &KV : IS.attributes())
    {
        OS << KV.key() << ": ";
        printAnyValue(OS, KV.value());
        OS << "\n";
    }
    OS << "Dropped Attributes Count: " << IS.dropped_attributes_count() << "\n";
}

void pb::printResource(std::ostream& OS, const pb::Resource& Res)
{
    OS << "Resource Attributes:\n";

    for (const auto &KV : Res.attributes())
    {
        OS << KV.key() << ": ";
        printAnyValue(OS, KV.value());
        OS << "\n";
    }

    OS << "Dropped Attributes Count: " << Res.dropped_attributes_count() << "\n";
}

void pb::printExemplar(std::ostream &OS, const pb::Exemplar &Ex)
{
    OS << "Exemplar:\n";
    OS << "Filtered Attributes:\n";

    for (const auto &KV : Ex.filtered_attributes())
    {
        OS << KV.key() << ": ";
        printAnyValue(OS, KV.value());
        OS << "\n";
    }

    OS << "Time Unix Nano: " << Ex.time_unix_nano() << "\n";
    OS << "Value: ";
    switch (Ex.value_case())
    {
        case Exemplar::kAsDouble:
            OS << Ex.as_double();
            break;
        case Exemplar::kAsInt:
            OS << Ex.as_int();
            break;
        default:
            OS << "Invalid or unknown value type";
    }
    OS << "\n";

    if (!Ex.span_id().empty())
    {
        OS << "Span ID: " << std::hex;
        for (unsigned char c : Ex.span_id()) OS << static_cast<int>(c);
        OS << std::dec << "\n";
    }

    if (!Ex.trace_id().empty())
    {
        OS << "Trace ID: " << std::hex;
        for (unsigned char c : Ex.trace_id()) OS << static_cast<int>(c);
        OS << std::dec << "\n";
    }
}

void pb::printNumberDataPoint(std::ostream &OS, const pb::NumberDataPoint &DP)
{
    OS << "NumberDataPoint:\n";
    OS << "Attributes:\n";
    for (const auto &KV : DP.attributes())
    {
        OS << KV.key() << ": ";
        printAnyValue(OS, KV.value());
        OS << "\n";
    }

    OS << "Start Time Unix Nano: " << DP.start_time_unix_nano() << "\n";
    OS << "Time Unix Nano: " << DP.time_unix_nano() << "\n";

    OS << "Value: ";
    switch (DP.value_case())
    {
        case NumberDataPoint::kAsDouble:
            OS << DP.as_double();
            break;
        case NumberDataPoint::kAsInt:
            OS << DP.as_int();
            break;
        default:
            OS << "Invalid or unknown value type";
    }
    OS << "\n";

    OS << "Exemplars:\n";
    for (const auto &Ex : DP.exemplars())
    {
        printExemplar(OS, Ex);
        OS << "\n";
    }

    OS << "Flags: " << DP.flags();
    if (DP.flags() & DataPointFlags::DATA_POINT_FLAGS_NO_RECORDED_VALUE_MASK)
    {
        OS << " (No Recorded Value)";
    }

    OS << "\n";
}

void pb::printSummaryDataPoint(std::ostream &OS, const pb::SummaryDataPoint &DP)
{
    OS << "SummaryDataPoint:\n";

    OS << "Attributes:\n";
    for (const auto& KV : DP.attributes())
    {
        OS << KV.key() << ": ";
        printAnyValue(OS, KV.value());
        OS << "\n";
    }

    OS << "Start Time Unix Nano: " << DP.start_time_unix_nano() << "\n";
    OS << "Time Unix Nano: " << DP.time_unix_nano() << "\n";
    OS << "Count: " << DP.count() << "\n";
    OS << "Sum: " << DP.sum() << "\n";

    OS << "Quantile Values:\n";
    for (const auto& quantileValue : DP.quantile_values())
    {
        OS << "Quantile: " << quantileValue.quantile() << ", Value: " << quantileValue.value() << "\n";
    }

    OS << "Flags: " << DP.flags();
    if (DP.flags() & DataPointFlags::DATA_POINT_FLAGS_NO_RECORDED_VALUE_MASK)
    {
        OS << " (No Recorded Value)";
    }
    OS << "\n";
}

void pb::printHistogramDataPoint(std::ostream &OS, const pb::HistogramDataPoint &DP)
{
    OS << "HistogramDataPoint:\n";

    OS << "Attributes:\n";
    for (const auto& attribute : DP.attributes())
    {
        OS << attribute.key() << ": ";
        printAnyValue(OS, attribute.value());
        OS << "\n";
    }

    OS << "Start Time Unix Nano: " << DP.start_time_unix_nano() << "\n";
    OS << "Time Unix Nano: " << DP.time_unix_nano() << "\n";
    OS << "Count: " << DP.count() << "\n";
    if (DP.has_sum())
    {
        OS << "Sum: " << DP.sum() << "\n";
    }

    OS << "Bucket Counts:\n";
    for (const auto& count : DP.bucket_counts())
    {
        OS << count << "\n";
    }

    OS << "Explicit Bounds:\n";
    for (const auto& bound : DP.explicit_bounds())
    {
        OS << bound << "\n";
    }

    OS << "Exemplars:\n";
    for (const auto& exemplar : DP.exemplars())
    {
        printExemplar(OS, exemplar);
        OS << "\n";
    }

    OS << "Flags: " << DP.flags();
    if (DP.flags() & DataPointFlags::DATA_POINT_FLAGS_NO_RECORDED_VALUE_MASK)
    {
        OS << " (No Recorded Value)";
    }
    OS << "\n";

    if (DP.has_min()) {
        OS << "Min: " << DP.min() << "\n";
    }

    if (DP.has_max()) {
        OS << "Max: " << DP.max() << "\n";
    }
}


void pb::printExponentialHistogramDataPoint(std::ostream &OS, const pb::ExponentialHistogramDataPoint &DP)
{
    OS << "ExponentialHistogramDataPoint:\n";

    OS << "Attributes:\n";
    for (const auto& attribute : DP.attributes())
    {
        OS << attribute.key() << ": ";
        printAnyValue(OS, attribute.value());
        OS << "\n";
    }

    OS << "Start Time Unix Nano: " << DP.start_time_unix_nano() << "\n";
    OS << "Time Unix Nano: " << DP.time_unix_nano() << "\n";
    OS << "Count: " << DP.count() << "\n";

    if (DP.has_sum()) {
        OS << "Sum: " << DP.sum() << "\n";
    }

    OS << "Scale: " << DP.scale() << "\n";
    OS << "Zero Count: " << DP.zero_count() << "\n";

    OS << "Positive Buckets:\n";
    printBuckets(OS, DP.positive());

    OS << "Negative Buckets:\n";
    printBuckets(OS, DP.negative());

    OS << "Flags: " << DP.flags();
    if (DP.flags() & DataPointFlags::DATA_POINT_FLAGS_NO_RECORDED_VALUE_MASK)
    {
        OS << " (No Recorded Value)";
    }
    OS << "\n";

    OS << "Exemplars:\n";
    for (const auto& exemplar : DP.exemplars())
    {
        printExemplar(OS, exemplar);
        OS << "\n";
    }

    if (DP.has_min())
    {
        OS << "Min: " << DP.min() << "\n";
    }

    if (DP.has_max())
    {
        OS << "Max: " << DP.max() << "\n";
    }

    if (DP.zero_threshold() != 0)
    {
        OS << "Zero Threshold: " << DP.zero_threshold() << "\n";
    }
}


void pb::printGauge(std::ostream &OS, const pb::Gauge& G) {
    OS << "Gauge:\n";
    for (const auto& DP : G.data_points())
    {
        printNumberDataPoint(OS, DP);
        OS << "\n";
    }
}

void pb::printSum(std::ostream &OS, const pb::Sum &S)
{
    OS << "Sum:\n";
    OS << "Aggregation Temporality: " << S.aggregation_temporality() << "\n";
    OS << "Is Monotonic: " << std::boolalpha << S.is_monotonic() << "\n";

    for (const auto& DP : S.data_points())
    {
        printNumberDataPoint(OS, DP);
        OS << "\n";
    }
}

void pb::printHistogram(std::ostream &OS, const pb::Histogram &H)
{
    OS << "Histogram:\n";
    OS << "Aggregation Temporality: " << H.aggregation_temporality() << "\n";

    for (const auto& DP : H.data_points())
    {
        printHistogramDataPoint(OS, DP);
        OS << "\n";
    }
}

void pb::printExponentialHistogram(std::ostream &OS, const pb::ExponentialHistogram &H)
{
    OS << "ExponentialHistogram:\n";
    OS << "Aggregation Temporality: " << H.aggregation_temporality() << "\n";

    for (const auto& DP : H.data_points())
    {
        printExponentialHistogramDataPoint(OS, DP);
        OS << "\n";
    }
}

void pb::printSummary(std::ostream &OS, const pb::Summary &S)
{
    OS << "Summary:\n";
    for (const auto& DP : S.data_points())
    {
        printSummaryDataPoint(OS, DP);
        OS << "\n";
    }
}

void pb::printMetric(std::ostream &OS, const pb::Metric &M)
{
    OS << "Metric Name: " << M.name() << "\n";
    OS << "Description: " << M.description() << "\n";
    OS << "Unit: " << M.unit() << "\n";

    if (M.has_gauge())
    {
        printGauge(OS, M.gauge());
    }
    else if (M.has_sum())
    {
        printSum(OS, M.sum());
    }
    else if (M.has_histogram())
    {
        printHistogram(OS, M.histogram());
    }
    else if (M.has_exponential_histogram())
    {
        printExponentialHistogram(OS, M.exponential_histogram());
    }
    else if (M.has_summary())
    {
        printSummary(OS, M.summary());
    }

    OS << "Metadata:\n";
    for (const auto& attribute : M.metadata())
    {
        OS << attribute.key() << ": ";
        printAnyValue(OS, attribute.value());
        OS << "\n";
    }
}

void pb::printScopeMetrics(std::ostream &OS, const ScopeMetrics &SM)
{
    OS << "ScopeMetrics:\n";

    OS << "Scope:\n";
    printInstrumentationScope(OS, SM.scope());

    OS << "Metrics:\n";
    for (const auto &M : SM.metrics())
    {
        printMetric(OS, M);
        OS << "\n";
    }

    OS << "Schema URL: " << SM.schema_url() << "\n";
}

void pb::printResourceMetrics(std::ostream &OS, const pb::ResourceMetrics &RM)
{
    OS << "ResourceMetrics:\n";

    OS << "Resource:\n";
    printResource(OS, RM.resource());

    OS << "Schema URL: " << RM.schema_url() << "\n";

    for (const auto &SM : RM.scope_metrics())
    {
        printScopeMetrics(OS, SM);
        OS << "\n";
    }
}

void pb::printMetricsData(std::ostream &OS, const pb::MetricsData &MD)
{
    OS << "MetricsData:\n";
    for (const auto& resourceMetrics : MD.resource_metrics()) {
        printResourceMetrics(OS, resourceMetrics);
        OS << "\n";
    }
}
