use super::*;

impl SamplingState {
    pub(crate) fn clear_namespace(&mut self, exporter_ip: IpAddr, observation_domain_id: u32) {
        if let Some(rates) = self.by_exporter.get_mut(&exporter_ip) {
            rates.retain(|key, _| key.observation_domain_id != observation_domain_id);
            if rates.is_empty() {
                self.by_exporter.remove(&exporter_ip);
            }
        }
    }

    pub(crate) fn set(
        &mut self,
        exporter_ip: IpAddr,
        version: u16,
        observation_domain_id: u32,
        sampler_id: u64,
        sampling_rate: u64,
    ) {
        if sampling_rate == 0 {
            return;
        }
        let key = SamplingKey {
            version,
            observation_domain_id,
            sampler_id,
        };
        self.by_exporter
            .entry(exporter_ip)
            .or_default()
            .insert(key, sampling_rate);
    }

    pub(crate) fn get(
        &self,
        exporter_ip: IpAddr,
        version: u16,
        observation_domain_id: u32,
        sampler_id: u64,
    ) -> Option<u64> {
        let map = self.by_exporter.get(&exporter_ip)?;
        map.get(&SamplingKey {
            version,
            observation_domain_id,
            sampler_id,
        })
        .copied()
    }

    pub(crate) fn apply_decoder_state_namespace(
        &mut self,
        key: &DecoderStateNamespaceKey,
        namespace: &DecoderStateNamespace,
    ) {
        let Ok(exporter_ip) = key.exporter_ip.parse::<IpAddr>() else {
            return;
        };

        self.clear_namespace(exporter_ip, key.observation_domain_id);

        for row in namespace.sampling_rates.values() {
            self.set(
                exporter_ip,
                row.version,
                key.observation_domain_id,
                row.sampler_id,
                row.sampling_rate,
            );
        }
    }
}
