use super::*;
use lru::LruCache;
use netflow_parser::variable_versions::DEFAULT_MAX_TEMPLATE_CACHE_SIZE;
use std::num::NonZeroUsize;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum PersistedTemplateKind {
    V9Data,
    V9Options,
    IpfixData,
    IpfixOptions,
    IpfixV9Data,
    IpfixV9Options,
}

#[derive(Debug, Default)]
struct LazyTemplateLru(Option<LruCache<u16, ()>>);

impl LazyTemplateLru {
    fn insert(&mut self, template_id: u16) -> Option<u16> {
        let cache = self.0.get_or_insert_with(|| {
            LruCache::new(
                NonZeroUsize::new(DEFAULT_MAX_TEMPLATE_CACHE_SIZE)
                    .expect("parser template cache limit must be positive"),
            )
        });
        if cache.contains(&template_id) {
            cache.get(&template_id);
            return None;
        }
        cache.push(template_id, ()).map(|(id, ())| id)
    }

    fn touch(&mut self, template_id: u16) {
        if let Some(cache) = &mut self.0 {
            cache.get(&template_id);
        }
    }

    fn remove(&mut self, template_id: u16) {
        let Some(cache) = &mut self.0 else {
            return;
        };
        cache.pop(&template_id);
        if cache.is_empty() {
            self.0 = None;
        }
    }
}

#[derive(Debug, Default)]
struct NamespaceTemplateState {
    v9_data: LazyTemplateLru,
    v9_options: LazyTemplateLru,
    ipfix_data: LazyTemplateLru,
    ipfix_options: LazyTemplateLru,
    ipfix_v9_data: LazyTemplateLru,
    ipfix_v9_options: LazyTemplateLru,
}

impl NamespaceTemplateState {
    fn cache_mut(&mut self, kind: PersistedTemplateKind) -> &mut LazyTemplateLru {
        match kind {
            PersistedTemplateKind::V9Data => &mut self.v9_data,
            PersistedTemplateKind::V9Options => &mut self.v9_options,
            PersistedTemplateKind::IpfixData => &mut self.ipfix_data,
            PersistedTemplateKind::IpfixOptions => &mut self.ipfix_options,
            PersistedTemplateKind::IpfixV9Data => &mut self.ipfix_v9_data,
            PersistedTemplateKind::IpfixV9Options => &mut self.ipfix_v9_options,
        }
    }

    fn remove_other_kinds(&mut self, kind: PersistedTemplateKind, template_id: u16) {
        let kinds: &[PersistedTemplateKind] = match kind {
            PersistedTemplateKind::V9Data | PersistedTemplateKind::V9Options => &[
                PersistedTemplateKind::V9Data,
                PersistedTemplateKind::V9Options,
            ],
            _ => &[
                PersistedTemplateKind::IpfixData,
                PersistedTemplateKind::IpfixOptions,
                PersistedTemplateKind::IpfixV9Data,
                PersistedTemplateKind::IpfixV9Options,
            ],
        };
        for other in kinds {
            if *other != kind {
                self.cache_mut(*other).remove(template_id);
            }
        }
    }
}

#[derive(Debug, Default)]
pub(crate) struct TemplateState {
    by_namespace: HashMap<DecoderStateNamespaceKey, NamespaceTemplateState>,
}

impl TemplateState {
    pub(crate) fn install(
        &mut self,
        key: &DecoderStateNamespaceKey,
        kind: PersistedTemplateKind,
        template_id: u16,
        namespace: &mut DecoderStateNamespace,
    ) {
        let state = self.by_namespace.entry(key.clone()).or_default();
        state.remove_other_kinds(kind, template_id);
        if let Some(evicted_id) = state.cache_mut(kind).insert(template_id) {
            match key.protocol {
                DecoderStateProtocol::V9 => {
                    if namespace
                        .v9_templates
                        .get(&evicted_id)
                        .is_some_and(|template| v9_kind(template) == kind)
                    {
                        namespace.v9_templates.remove(&evicted_id);
                    }
                }
                DecoderStateProtocol::Ipfix => {
                    if namespace
                        .ipfix_templates
                        .get(&evicted_id)
                        .is_some_and(|template| ipfix_kind(template) == kind)
                    {
                        namespace.ipfix_templates.remove(&evicted_id);
                    }
                }
            }
        }
    }

    pub(crate) fn touch(
        &mut self,
        key: &DecoderStateNamespaceKey,
        kind: PersistedTemplateKind,
        template_id: u16,
    ) {
        if let Some(state) = self.by_namespace.get_mut(key) {
            state.cache_mut(kind).touch(template_id);
        }
    }

    pub(crate) fn remove_template(
        &mut self,
        key: &DecoderStateNamespaceKey,
        kind: PersistedTemplateKind,
        template_id: u16,
    ) {
        if let Some(state) = self.by_namespace.get_mut(key) {
            state.cache_mut(kind).remove(template_id);
        }
    }

    pub(crate) fn remove_namespace(&mut self, key: &DecoderStateNamespaceKey) {
        self.by_namespace.remove(key);
    }

    pub(crate) fn load_namespace(
        &mut self,
        key: &DecoderStateNamespaceKey,
        namespace: &mut DecoderStateNamespace,
    ) {
        self.remove_namespace(key);
        match key.protocol {
            DecoderStateProtocol::V9 => {
                let entries = namespace
                    .v9_templates
                    .iter()
                    .map(|(id, template)| (*id, v9_kind(template)))
                    .collect::<Vec<_>>();
                for (id, kind) in entries {
                    self.install(key, kind, id, namespace);
                }
            }
            DecoderStateProtocol::Ipfix => {
                let entries = namespace
                    .ipfix_templates
                    .iter()
                    .map(|(id, template)| (*id, ipfix_kind(template)))
                    .collect::<Vec<_>>();
                for (id, kind) in entries {
                    self.install(key, kind, id, namespace);
                }
            }
        }
    }
}

pub(crate) fn v9_kind(template: &PersistedV9Template) -> PersistedTemplateKind {
    match &template.definition {
        PersistedV9TemplateDefinition::Data { .. } => PersistedTemplateKind::V9Data,
        PersistedV9TemplateDefinition::Options { .. } => PersistedTemplateKind::V9Options,
    }
}

pub(crate) fn ipfix_kind(template: &PersistedIPFixTemplate) -> PersistedTemplateKind {
    match &template.definition {
        PersistedIPFixTemplateDefinition::Data { .. } => PersistedTemplateKind::IpfixData,
        PersistedIPFixTemplateDefinition::Options { .. } => PersistedTemplateKind::IpfixOptions,
        PersistedIPFixTemplateDefinition::V9Data { .. } => PersistedTemplateKind::IpfixV9Data,
        PersistedIPFixTemplateDefinition::V9Options { .. } => PersistedTemplateKind::IpfixV9Options,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn key() -> DecoderStateNamespaceKey {
        DecoderStateNamespaceKey {
            protocol: DecoderStateProtocol::V9,
            exporter_ip: "192.0.2.1".to_string(),
            source_port: 2055,
            observation_domain_id: 1,
        }
    }

    #[test]
    fn mirror_removes_the_same_v9_data_owner_as_the_parser_capacity() {
        let key = key();
        let mut state = TemplateState::default();
        let mut namespace = DecoderStateNamespace::default();

        for id in 256..=(256 + DEFAULT_MAX_TEMPLATE_CACHE_SIZE as u16) {
            namespace.set_v9_template(id, Vec::new(), 1, false);
            state.install(&key, PersistedTemplateKind::V9Data, id, &mut namespace);
        }

        assert_eq!(
            namespace.v9_templates.len(),
            DEFAULT_MAX_TEMPLATE_CACHE_SIZE
        );
        assert!(!namespace.v9_templates.contains_key(&256));
    }

    #[test]
    fn eviction_from_one_physical_kind_cannot_remove_the_current_opposite_kind_owner() {
        let key = key();
        let mut state = TemplateState::default();
        let mut namespace = DecoderStateNamespace::default();
        namespace.set_v9_template(256, Vec::new(), 1, false);
        state.install(&key, PersistedTemplateKind::V9Data, 256, &mut namespace);
        namespace.set_v9_options_template(256, Vec::new(), Vec::new(), 2);
        state.install(&key, PersistedTemplateKind::V9Options, 256, &mut namespace);

        for id in 300..(300 + DEFAULT_MAX_TEMPLATE_CACHE_SIZE as u16 + 1) {
            namespace.set_v9_template(id, Vec::new(), 3, false);
            state.install(&key, PersistedTemplateKind::V9Data, id, &mut namespace);
        }

        assert!(matches!(
            namespace
                .v9_templates
                .get(&256)
                .map(|template| &template.definition),
            Some(PersistedV9TemplateDefinition::Options { .. })
        ));
    }
}
