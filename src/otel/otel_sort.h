// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ND_OTEL_SORT_H
#define ND_OTEL_SORT_H

#include "otel_utils.h"

namespace pb
{

int compareArrayValue(const pb::ArrayValue &LHS, const pb::ArrayValue &RHS);
int compareKeyValueList(const pb::KeyValueList &LHS, const pb::KeyValueList &RHS);
int compareAnyValue(const pb::AnyValue &LHS, const pb::AnyValue &RHS);
int compareKeyValue(const pb::KeyValue &LHS, const pb::KeyValue &RHS);
int compareNumberDataPoint(const pb::NumberDataPoint &LHS, const pb::NumberDataPoint &RHS);
int compareMetric(const pb::Metric &LHS, const pb::Metric &RHS);
int compareScopeMetrics(const pb::ScopeMetrics &LHS, const pb::ScopeMetrics &RHS);
int compareResourceMetrics(const pb::ResourceMetrics &LHS, const pb::ResourceMetrics &RHS);

void sortAttributes(pb::RepeatedPtrField<pb::KeyValue> *Attrs);
void sortDataPoints(pb::Metric &M);
void sortMetrics(pb::RepeatedPtrField<pb::Metric> *Arr);
void sortScopeMetrics(pb::RepeatedPtrField<pb::ScopeMetrics> *Arr);
void sortResourceMetrics(pb::RepeatedPtrField<pb::ResourceMetrics> *Arr);
void sortMetricsData(pb::MetricsData &MD);

} // namespace pb

#endif /* ND_OTEL_SORT_H */
