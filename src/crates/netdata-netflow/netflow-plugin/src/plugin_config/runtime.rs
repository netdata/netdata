use super::*;

impl PluginConfig {
    pub(crate) fn new() -> Result<Self> {
        let netdata_env = NetdataEnv::from_environment();

        let mut cfg = if netdata_env.running_under_netdata() {
            Self::load_from_netdata_config(&netdata_env)?
        } else {
            Self::parse()
        };

        cfg._netdata_env = netdata_env.clone();
        cfg.journal.journal_dir =
            resolve_relative_path(&cfg.journal.journal_dir, netdata_env.cache_dir.as_deref());
        cfg.auto_detect_geoip_databases();
        cfg.ensure_storage_layout()?;

        cfg.validate()?;
        Ok(cfg)
    }

    pub(super) fn auto_detect_geoip_databases(&mut self) {
        let intel_dirs = [
            self.inferred_cache_dir().join(TOPOLOGY_IP_INTEL_DIR),
            self.inferred_stock_data_dir().join(TOPOLOGY_IP_INTEL_DIR),
        ];
        let geoip = &mut self.enrichment.geoip;
        if !geoip.asn_database.is_empty() || !geoip.geo_database.is_empty() {
            return;
        }

        let mut detected = false;
        for intel_dir in intel_dirs {
            if geoip.asn_database.is_empty() {
                let asn_db = intel_dir.join(TOPOLOGY_IP_ASN_MMDB);
                if asn_db.is_file() {
                    geoip
                        .asn_database
                        .push(asn_db.to_string_lossy().to_string());
                    detected = true;
                }
            }

            if geoip.geo_database.is_empty() {
                let geo_db = intel_dir.join(TOPOLOGY_IP_GEO_MMDB);
                if geo_db.is_file() {
                    geoip
                        .geo_database
                        .push(geo_db.to_string_lossy().to_string());
                    detected = true;
                }
            }

            if !geoip.asn_database.is_empty() && !geoip.geo_database.is_empty() {
                break;
            }
        }

        if detected {
            // Auto-detected databases should not prevent startup if they disappear.
            geoip.optional = true;
        }
    }

    fn inferred_cache_dir(&self) -> PathBuf {
        let base_dir = self.journal.base_dir();
        if base_dir.is_absolute()
            && let Some(parent) = base_dir.parent()
            && !parent.as_os_str().is_empty()
        {
            return parent.to_path_buf();
        }

        self._netdata_env
            .cache_dir
            .clone()
            .unwrap_or_else(|| PathBuf::from(DEFAULT_NETDATA_CACHE_DIR))
    }

    fn inferred_stock_data_dir(&self) -> PathBuf {
        self._netdata_env
            .stock_data_dir
            .clone()
            .unwrap_or_else(|| PathBuf::from(DEFAULT_NETDATA_STOCK_DATA_DIR))
    }

    fn load_from_netdata_config(netdata_env: &NetdataEnv) -> Result<Self> {
        let candidates = [
            netdata_env
                .user_config_dir
                .as_ref()
                .map(|p| p.join("netflow.yaml")),
            netdata_env
                .stock_config_dir
                .as_ref()
                .map(|p| p.join("netflow.yaml")),
        ];

        for path in candidates.into_iter().flatten() {
            if path.is_file() {
                return Self::from_yaml_file(&path).with_context(|| {
                    format!("failed to load netflow config from {}", path.display())
                });
            }
        }

        Ok(Self::default())
    }

    fn from_yaml_file(path: &Path) -> Result<Self> {
        let content = fs::read_to_string(path)
            .with_context(|| format!("failed to read {}", path.display()))?;
        let cfg = serde_yaml::from_str::<Self>(&content)
            .with_context(|| format!("failed to parse {}", path.display()))?;
        Ok(cfg)
    }

    fn ensure_storage_layout(&self) -> Result<()> {
        for dir in self.journal.all_tier_dirs() {
            fs::create_dir_all(&dir)
                .with_context(|| format!("failed to create tier directory {}", dir.display()))?;
        }
        Ok(())
    }
}

fn resolve_relative_path(path: &str, base_dir: Option<&Path>) -> String {
    let p = Path::new(path);
    if p.is_absolute() {
        return p.to_string_lossy().to_string();
    }

    if let Some(base) = base_dir {
        return PathBuf::from(base).join(p).to_string_lossy().to_string();
    }

    p.to_string_lossy().to_string()
}
