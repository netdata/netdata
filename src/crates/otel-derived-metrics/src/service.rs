//! What a service is, for every per-service metric set.
//!
//! A service is identified by the OTel resource attributes `service.namespace`
//! and `service.name`, kept apart by tenant and deployment environment
//! (`deployment.environment.name`, or the deprecated `deployment.environment`).
//! The per-service metric sets label their entities with [`service_labels`]
//! and share [`service_settings`], so a service's request charts and its log
//! chart carry the same labels and the same chart id suffix.

use netdata_plugin_metrics::{CardinalityLimit, Labels, Settings};
use opentelemetry_proto::tonic::common::v1::KeyValue;
use opentelemetry_proto::tonic::common::v1::any_value::Value;
use opentelemetry_proto::tonic::resource::v1::Resource;
use std::time::Duration;

/// How long after its end a window waits before it is emitted, so spans and
/// log records that arrive late still count in it. SDKs and collectors export
/// them in batches every few seconds (5 s by default for spans), so emitting a
/// second as soon as it ends would show each batch as one spike.
const ALLOWED_LATENESS: Duration = Duration::from_secs(10);

/// Value of `service.name` when a resource has none (the SDK default prefix).
const UNKNOWN_SERVICE: &str = "unknown_service";

/// The settings of a per-service metric set: late spans and log records still
/// count in their second, and services over `max_services` share the
/// overflow entity.
pub(crate) fn service_settings(max_services: usize) -> Settings {
    let mut overflow_labels = Labels::new();
    overflow_labels.insert("otel.metric.overflow", "true");
    Settings {
        allowed_lateness: ALLOWED_LATENESS,
        cardinality_limit: Some(CardinalityLimit {
            max_entities: max_services,
            overflow_labels,
        }),
        ..Settings::default()
    }
}

/// The labels of the service a resource belongs to.
pub(crate) fn service_labels(tenant: &str, resource: Option<&Resource>) -> Labels {
    let attributes: &[KeyValue] = resource.map_or(&[], |resource| &resource.attributes);
    let name = string_attribute(attributes, "service.name")
        .filter(|name| !is_blank(name))
        .unwrap_or(UNKNOWN_SERVICE);
    let namespace = string_attribute(attributes, "service.namespace").unwrap_or_default();
    let environment = string_attribute(attributes, "deployment.environment.name")
        .filter(|environment| !is_blank(environment))
        .or_else(|| string_attribute(attributes, "deployment.environment"))
        .unwrap_or_default();

    let mut labels = Labels::new();
    labels.insert("resource.attributes.service.name", name);
    labels.insert("resource.attributes.service.namespace", namespace);
    labels.insert(
        "resource.attributes.deployment.environment.name",
        environment,
    );
    labels.insert("tenant", tenant);
    labels
}

/// The string value of the last attribute `key` that has one (a sender
/// should not repeat a key). `None` when no attribute `key` has a string
/// value.
fn string_attribute<'a>(attributes: &'a [KeyValue], key: &str) -> Option<&'a str> {
    for attribute in attributes.iter().rev() {
        if attribute.key != key {
            continue;
        }
        let value = attribute.value.as_ref().and_then(|any| any.value.as_ref());
        if let Some(Value::StringValue(string)) = value {
            return Some(string);
        }
    }
    None
}

/// Blank as the agent sees label values: whitespace and control characters
/// only (it would store `[none]`).
fn is_blank(value: &str) -> bool {
    value.chars().all(|c| c.is_whitespace() || c.is_control())
}
