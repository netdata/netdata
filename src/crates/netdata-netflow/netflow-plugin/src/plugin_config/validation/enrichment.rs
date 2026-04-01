use crate::plugin_config::{NetworkAttributesValue, PluginConfig, RemoteNetworkSourceTlsConfig};
use anyhow::{Context, Result};
use jaq_interpret::ParseCtx;
use std::net::SocketAddr;
use std::time::Duration;

pub(super) fn validate_enrichment(cfg: &PluginConfig) -> Result<()> {
    validate_dynamic_routing(cfg)?;

    if cfg.enrichment.classifier_cache_duration < Duration::from_secs(1) {
        anyhow::bail!("enrichment.classifier_cache_duration must be >= 1s");
    }

    for (source_name, source_cfg) in &cfg.enrichment.network_sources {
        if source_cfg.url.trim().is_empty() {
            anyhow::bail!("enrichment.network_sources.{source_name}.url must be non-empty");
        }
        let method = source_cfg.method.trim().to_ascii_uppercase();
        if method != "GET" && method != "POST" {
            anyhow::bail!("enrichment.network_sources.{source_name}.method must be GET or POST");
        }
        if source_cfg.timeout.is_zero() {
            anyhow::bail!("enrichment.network_sources.{source_name}.timeout must be > 0");
        }
        if source_cfg.interval.is_zero() {
            anyhow::bail!("enrichment.network_sources.{source_name}.interval must be > 0");
        }
        validate_network_source_tls(source_name, &source_cfg.tls)?;
        validate_network_source_transform(source_name, &source_cfg.transform)?;
    }

    for (prefix, value) in &cfg.enrichment.networks {
        validate_static_network_attributes(prefix, value)?;
    }

    Ok(())
}

fn validate_static_network_attributes(prefix: &str, value: &NetworkAttributesValue) -> Result<()> {
    let NetworkAttributesValue::Attributes(attrs) = value else {
        return Ok(());
    };

    match (attrs.latitude, attrs.longitude) {
        (Some(_), None) | (None, Some(_)) => {
            anyhow::bail!(
                "enrichment.networks.{prefix} must set both latitude and longitude when either coordinate is configured"
            );
        }
        (Some(latitude), Some(longitude)) => {
            if !latitude.is_finite() || !(-90.0..=90.0).contains(&latitude) {
                anyhow::bail!(
                    "enrichment.networks.{prefix}.latitude must be a finite value between -90 and 90"
                );
            }
            if !longitude.is_finite() || !(-180.0..=180.0).contains(&longitude) {
                anyhow::bail!(
                    "enrichment.networks.{prefix}.longitude must be a finite value between -180 and 180"
                );
            }
        }
        (None, None) => {}
    }

    Ok(())
}

fn validate_dynamic_routing(cfg: &PluginConfig) -> Result<()> {
    if cfg.enrichment.routing_dynamic.bmp.enabled {
        cfg.enrichment
            .routing_dynamic
            .bmp
            .listen
            .parse::<SocketAddr>()
            .with_context(|| {
                format!(
                    "invalid enrichment.routing_dynamic.bmp.listen address: {}",
                    cfg.enrichment.routing_dynamic.bmp.listen
                )
            })?;
        if cfg.enrichment.routing_dynamic.bmp.keep.is_zero() {
            anyhow::bail!("enrichment.routing_dynamic.bmp.keep must be greater than 0");
        }
        if cfg
            .enrichment
            .routing_dynamic
            .bmp
            .max_consecutive_decode_errors
            == 0
        {
            anyhow::bail!(
                "enrichment.routing_dynamic.bmp.max_consecutive_decode_errors must be greater than 0"
            );
        }
    }

    if cfg.enrichment.routing_dynamic.bioris.enabled {
        if cfg
            .enrichment
            .routing_dynamic
            .bioris
            .ris_instances
            .is_empty()
        {
            anyhow::bail!(
                "enrichment.routing_dynamic.bioris.ris_instances must contain at least one instance when bioris is enabled"
            );
        }
        if cfg.enrichment.routing_dynamic.bioris.timeout.is_zero() {
            anyhow::bail!("enrichment.routing_dynamic.bioris.timeout must be greater than 0");
        }
        if cfg.enrichment.routing_dynamic.bioris.refresh.is_zero() {
            anyhow::bail!("enrichment.routing_dynamic.bioris.refresh must be greater than 0");
        }
        if cfg
            .enrichment
            .routing_dynamic
            .bioris
            .refresh_timeout
            .is_zero()
        {
            anyhow::bail!(
                "enrichment.routing_dynamic.bioris.refresh_timeout must be greater than 0"
            );
        }
        for (idx, instance) in cfg
            .enrichment
            .routing_dynamic
            .bioris
            .ris_instances
            .iter()
            .enumerate()
        {
            let addr = instance.grpc_addr.trim();
            if addr.is_empty() {
                anyhow::bail!(
                    "enrichment.routing_dynamic.bioris.ris_instances[{idx}].grpc_addr must be non-empty"
                );
            }
            if !addr.contains("://") && !addr.contains(':') {
                anyhow::bail!(
                    "enrichment.routing_dynamic.bioris.ris_instances[{idx}].grpc_addr must include host:port or an explicit URI scheme"
                );
            }
        }
    }

    Ok(())
}

fn validate_network_source_transform(source_name: &str, transform: &str) -> Result<()> {
    let normalized = if transform.trim().is_empty() {
        "."
    } else {
        transform.trim()
    };
    let tokens = jaq_syn::Lexer::new(normalized).lex().map_err(|errs| {
        anyhow::anyhow!(
            "enrichment.network_sources.{source_name}.transform lex error: {:?}",
            errs
        )
    })?;
    let main = jaq_syn::Parser::new(&tokens)
        .parse(|parser| parser.module(|module| module.term()))
        .map_err(|errs| {
            anyhow::anyhow!(
                "enrichment.network_sources.{source_name}.transform parse error: {:?}",
                errs
            )
        })?
        .conv(normalized);
    let mut ctx = ParseCtx::new(Vec::new());
    ctx.insert_natives(jaq_core::core());
    ctx.insert_defs(jaq_std::std());
    let _compiled = ctx.compile(main);
    if !ctx.errs.is_empty() {
        let errors = ctx
            .errs
            .into_iter()
            .map(|err| err.0.to_string())
            .collect::<Vec<_>>()
            .join("; ");
        anyhow::bail!(
            "enrichment.network_sources.{source_name}.transform compile error: {}",
            errors
        );
    }
    Ok(())
}

fn validate_network_source_tls(
    source_name: &str,
    tls: &RemoteNetworkSourceTlsConfig,
) -> Result<()> {
    let has_tls_fields = !tls.ca_file.trim().is_empty()
        || !tls.cert_file.trim().is_empty()
        || !tls.key_file.trim().is_empty();
    if has_tls_fields && !tls.enable {
        anyhow::bail!(
            "enrichment.network_sources.{source_name}.tls.enable must be true when tls.ca_file/tls.cert_file/tls.key_file are set"
        );
    }
    if !tls.key_file.trim().is_empty() && tls.cert_file.trim().is_empty() {
        anyhow::bail!(
            "enrichment.network_sources.{source_name}.tls.cert_file must be set when tls.key_file is set"
        );
    }
    if !tls.verify {
        anyhow::bail!(
            "enrichment.network_sources.{source_name}.tls.verify must remain true; disabling TLS certificate verification is not supported for remote network sources"
        );
    }
    if tls.skip_verify {
        anyhow::bail!(
            "enrichment.network_sources.{source_name}.tls.skip_verify is not supported; use tls.ca_file to trust custom certificate authorities"
        );
    }
    Ok(())
}
