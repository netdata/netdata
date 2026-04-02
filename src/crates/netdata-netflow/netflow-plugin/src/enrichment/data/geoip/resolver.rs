use super::*;

impl GeoIpResolver {
    pub(crate) fn from_config(config: &GeoIpConfig) -> Result<Option<Self>> {
        if config.asn_database.is_empty() && config.geo_database.is_empty() {
            return Ok(None);
        }

        let asn_databases = load_geoip_readers(
            &config.asn_database,
            "enrichment.geoip.asn_database",
            config.optional,
        )?;
        let geo_databases = load_geoip_readers(
            &config.geo_database,
            "enrichment.geoip.geo_database",
            config.optional,
        )?;

        let signature =
            build_geoip_signature(&config.asn_database, &config.geo_database, config.optional)?;

        Ok(Some(Self {
            asn_paths: config.asn_database.clone(),
            geo_paths: config.geo_database.clone(),
            optional: config.optional,
            asn_databases,
            geo_databases,
            signature,
            last_reload_check: Instant::now(),
        }))
    }

    pub(crate) fn refresh_if_needed(&mut self) {
        if self.last_reload_check.elapsed() < GEOIP_RELOAD_CHECK_INTERVAL {
            return;
        }

        let next_signature =
            match build_geoip_signature(&self.asn_paths, &self.geo_paths, self.optional) {
                Ok(signature) => signature,
                Err(err) => {
                    tracing::warn!("geoip: failed to check database signatures: {}", err);
                    return;
                }
            };
        self.last_reload_check = Instant::now();

        if next_signature == self.signature {
            return;
        }

        match (
            load_geoip_readers(
                &self.asn_paths,
                "enrichment.geoip.asn_database",
                self.optional,
            ),
            load_geoip_readers(
                &self.geo_paths,
                "enrichment.geoip.geo_database",
                self.optional,
            ),
        ) {
            (Ok(asn_databases), Ok(geo_databases)) => {
                self.asn_databases = asn_databases;
                self.geo_databases = geo_databases;
                self.signature = next_signature;
                tracing::info!(
                    "geoip: reloaded databases (asn={}, geo={})",
                    self.asn_databases.len(),
                    self.geo_databases.len()
                );
            }
            (Err(err), _) | (_, Err(err)) => {
                tracing::warn!("geoip: reload skipped, keeping previous databases: {}", err);
            }
        }
    }

    pub(crate) fn lookup(&self, address: IpAddr) -> Option<NetworkAttributes> {
        let mut out = NetworkAttributes::default();

        for db in &self.asn_databases {
            if let Ok(record) = db.lookup::<AsnLookupRecord>(address) {
                if let Some(asn) = decode_asn_record(&record) {
                    out.asn = asn;
                }
                if let Some(asn_name) = decode_asn_name(&record) {
                    out.asn_name = asn_name;
                }
                if let Some(ip_class) = decode_ip_class(&record) {
                    out.ip_class = ip_class;
                }
            }
        }

        for db in &self.geo_databases {
            if let Ok(record) = db.lookup::<GeoLookupRecord>(address) {
                apply_geo_record(&mut out, &record);
            }
        }

        if out.is_empty() { None } else { Some(out) }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn refresh_if_needed_keeps_retry_window_open_after_signature_error() {
        let old_check = Instant::now() - GEOIP_RELOAD_CHECK_INTERVAL - Duration::from_secs(1);
        let mut resolver = GeoIpResolver {
            asn_paths: vec!["/path/that/does/not/exist/asn.mmdb".to_string()],
            geo_paths: Vec::new(),
            optional: false,
            asn_databases: Vec::new(),
            geo_databases: Vec::new(),
            signature: GeoIpDatabasesSignature {
                asn: Vec::new(),
                geo: Vec::new(),
            },
            last_reload_check: old_check,
        };

        resolver.refresh_if_needed();

        assert_eq!(resolver.last_reload_check, old_check);
    }
}
