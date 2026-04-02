use super::*;

impl FlowEnricher {
    pub(crate) fn from_config(config: &EnrichmentConfig) -> Result<Option<Self>> {
        let default_sampling_rate = build_sampling_map(
            config.default_sampling_rate.as_ref(),
            "enrichment.default_sampling_rate",
        )?;
        let override_sampling_rate = build_sampling_map(
            config.override_sampling_rate.as_ref(),
            "enrichment.override_sampling_rate",
        )?;
        let static_metadata = StaticMetadata::from_config(config)?;
        let networks = build_network_attributes_map(&config.networks)?;
        let geoip = GeoIpResolver::from_config(&config.geoip)?;
        let network_sources_runtime = if config.network_sources.is_empty() {
            None
        } else {
            Some(NetworkSourcesRuntime::default())
        };

        let exporter_classifiers = config
            .exporter_classifiers
            .iter()
            .enumerate()
            .map(|(idx, rule)| {
                ClassifierRule::parse(rule).with_context(|| {
                    format!("invalid enrichment.exporter_classifiers[{idx}] rule: {rule}")
                })
            })
            .collect::<Result<Vec<_>>>()?;
        let interface_classifiers = config
            .interface_classifiers
            .iter()
            .enumerate()
            .map(|(idx, rule)| {
                ClassifierRule::parse(rule).with_context(|| {
                    format!("invalid enrichment.interface_classifiers[{idx}] rule: {rule}")
                })
            })
            .collect::<Result<Vec<_>>>()?;
        let static_routing = StaticRouting::from_config(&config.routing_static)?;
        let dynamic_routing =
            if config.routing_dynamic.bmp.enabled || config.routing_dynamic.bioris.enabled {
                Some(DynamicRoutingRuntime::default())
            } else {
                None
            };

        if default_sampling_rate.is_empty()
            && override_sampling_rate.is_empty()
            && static_metadata.is_empty()
            && networks.is_empty()
            && geoip.is_none()
            && network_sources_runtime.is_none()
            && exporter_classifiers.is_empty()
            && interface_classifiers.is_empty()
            && static_routing.is_empty()
            && dynamic_routing.is_none()
        {
            return Ok(None);
        }

        Ok(Some(Self {
            default_sampling_rate,
            override_sampling_rate,
            static_metadata,
            networks,
            geoip,
            network_sources_runtime,
            exporter_classifiers,
            interface_classifiers,
            classifier_cache_duration: config.classifier_cache_duration,
            exporter_classifier_cache: Arc::new(Mutex::new(ExporterClassifierCache::default())),
            interface_classifier_cache: Arc::new(Mutex::new(InterfaceClassifierCache::default())),
            asn_providers: config.asn_providers.clone(),
            net_providers: config.net_providers.clone(),
            static_routing,
            dynamic_routing,
        }))
    }

    pub(crate) fn dynamic_routing_runtime(&self) -> Option<DynamicRoutingRuntime> {
        self.dynamic_routing.clone()
    }

    pub(crate) fn network_sources_runtime(&self) -> Option<NetworkSourcesRuntime> {
        self.network_sources_runtime.clone()
    }

    pub(crate) fn refresh_runtime_state(&mut self) {
        if let Some(geoip) = &mut self.geoip {
            geoip.refresh_if_needed();
        }
    }
}
