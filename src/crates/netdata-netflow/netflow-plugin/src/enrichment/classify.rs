use super::*;

fn maybe_prune_classifier_cache<K, V>(
    cache: &mut TimedClassifierCache<K, V>,
    ttl: Duration,
    now: Instant,
) where
    K: Eq + std::hash::Hash,
{
    if now.duration_since(cache.last_prune) < ttl.min(CLASSIFIER_CACHE_PRUNE_INTERVAL) {
        return;
    }

    cache
        .entries
        .retain(|_, entry| now.duration_since(entry.last_access) <= ttl);
    cache.last_prune = now;
}

impl FlowEnricher {
    pub(super) fn get_cached_exporter_classification(
        &self,
        exporter: &ExporterInfo,
    ) -> Option<ExporterClassification> {
        let Ok(mut cache) = self.exporter_classifier_cache.lock() else {
            return None;
        };
        let now = Instant::now();
        maybe_prune_classifier_cache(&mut cache, self.classifier_cache_duration, now);
        if let Some(entry) = cache.entries.get_mut(exporter) {
            if now.duration_since(entry.last_access) <= self.classifier_cache_duration {
                entry.last_access = now;
                return Some(entry.value.clone());
            }
        }
        cache.entries.remove(exporter);
        None
    }

    pub(super) fn set_cached_exporter_classification(
        &self,
        exporter: &ExporterInfo,
        classification: &ExporterClassification,
    ) {
        let Ok(mut cache) = self.exporter_classifier_cache.lock() else {
            return;
        };
        let now = Instant::now();
        maybe_prune_classifier_cache(&mut cache, self.classifier_cache_duration, now);
        cache.entries.insert(
            exporter.clone(),
            TimedClassifierEntry {
                value: classification.clone(),
                last_access: now,
            },
        );
    }

    pub(super) fn get_cached_interface_classification(
        &self,
        exporter: &ExporterInfo,
        interface: &InterfaceInfo,
        exporter_classification: &ExporterClassification,
    ) -> Option<InterfaceClassification> {
        let key = ExporterAndInterfaceInfo {
            exporter: exporter.clone(),
            interface: interface.clone(),
            exporter_classification: exporter_classification.clone(),
        };
        let Ok(mut cache) = self.interface_classifier_cache.lock() else {
            return None;
        };
        let now = Instant::now();
        maybe_prune_classifier_cache(&mut cache, self.classifier_cache_duration, now);
        if let Some(entry) = cache.entries.get_mut(&key) {
            if now.duration_since(entry.last_access) <= self.classifier_cache_duration {
                entry.last_access = now;
                return Some(entry.value.clone());
            }
        }
        cache.entries.remove(&key);
        None
    }

    pub(super) fn set_cached_interface_classification(
        &self,
        exporter: &ExporterInfo,
        interface: &InterfaceInfo,
        exporter_classification: &ExporterClassification,
        classification: &InterfaceClassification,
    ) {
        let key = ExporterAndInterfaceInfo {
            exporter: exporter.clone(),
            interface: interface.clone(),
            exporter_classification: exporter_classification.clone(),
        };
        let Ok(mut cache) = self.interface_classifier_cache.lock() else {
            return;
        };
        let now = Instant::now();
        maybe_prune_classifier_cache(&mut cache, self.classifier_cache_duration, now);
        cache.entries.insert(
            key,
            TimedClassifierEntry {
                value: classification.clone(),
                last_access: now,
            },
        );
    }

    pub(super) fn classify_exporter(
        &self,
        exporter: &ExporterInfo,
        classification: &mut ExporterClassification,
    ) -> bool {
        // Akvorado parity: metadata-provided classification has priority.
        if !classification.is_empty() {
            return !classification.reject;
        }
        if self.exporter_classifiers.is_empty() {
            return true;
        }

        if let Some(cached) = self.get_cached_exporter_classification(exporter) {
            *classification = cached;
            return !classification.reject;
        }

        for rule in &self.exporter_classifiers {
            if rule.evaluate_exporter(exporter, classification).is_err() {
                break;
            }
            if classification.is_complete() {
                break;
            }
        }

        self.set_cached_exporter_classification(exporter, classification);
        !classification.reject
    }

    pub(super) fn classify_interface(
        &self,
        exporter: &ExporterInfo,
        interface: &InterfaceInfo,
        exporter_classification: &ExporterClassification,
        classification: &mut InterfaceClassification,
    ) -> bool {
        // Akvorado parity: metadata-provided classification has priority.
        if !classification.is_empty() {
            classification.name = interface.name.clone();
            classification.description = interface.description.clone();
            return !classification.reject;
        }

        if self.interface_classifiers.is_empty() {
            classification.name = interface.name.clone();
            classification.description = interface.description.clone();
            return true;
        }

        if let Some(cached) =
            self.get_cached_interface_classification(exporter, interface, exporter_classification)
        {
            *classification = cached;
            return !classification.reject;
        }

        for rule in &self.interface_classifiers {
            if rule
                .evaluate_interface(exporter, interface, exporter_classification, classification)
                .is_err()
            {
                break;
            }
            if !classification.connectivity.is_empty()
                && !classification.provider.is_empty()
                && classification.boundary != 0
            {
                break;
            }
        }

        if classification.name.is_empty() {
            classification.name = interface.name.clone();
        }
        if classification.description.is_empty() {
            classification.description = interface.description.clone();
        }

        self.set_cached_interface_classification(
            exporter,
            interface,
            exporter_classification,
            classification,
        );
        !classification.reject
    }
}
