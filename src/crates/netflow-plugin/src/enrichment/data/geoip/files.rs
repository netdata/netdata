use super::*;

pub(crate) fn load_geoip_readers(
    paths: &[String],
    field_name: &str,
    optional: bool,
) -> Result<Vec<GeoIpDatabaseReader>> {
    let mut readers = Vec::new();
    for path in paths {
        match Reader::open_readfile(path) {
            Ok(reader) => readers.push(reader),
            Err(err) if optional => {
                tracing::warn!(
                    "{}: failed to load optional database '{}': {}",
                    field_name,
                    path,
                    err
                );
            }
            Err(err) => {
                return Err(anyhow::anyhow!(
                    "{}: failed to load database '{}': {}",
                    field_name,
                    path,
                    err
                ));
            }
        }
    }
    Ok(readers)
}

pub(crate) fn build_geoip_signature(
    asn_paths: &[String],
    geo_paths: &[String],
    optional: bool,
) -> Result<GeoIpDatabasesSignature> {
    Ok(GeoIpDatabasesSignature {
        asn: asn_paths
            .iter()
            .map(|path| read_geoip_file_signature(path, optional))
            .collect::<Result<Vec<_>>>()?,
        geo: geo_paths
            .iter()
            .map(|path| read_geoip_file_signature(path, optional))
            .collect::<Result<Vec<_>>>()?,
    })
}

pub(crate) fn read_geoip_file_signature(
    path: &str,
    optional: bool,
) -> Result<Option<GeoIpFileSignature>> {
    let metadata = match fs::metadata(path) {
        Ok(metadata) => metadata,
        Err(_) if optional => {
            return Ok(None);
        }
        Err(err) => {
            return Err(anyhow::anyhow!(
                "geoip: failed to stat database '{}': {}",
                path,
                err
            ));
        }
    };

    let modified = match metadata.modified() {
        Ok(time) => time,
        Err(err) if optional => {
            tracing::warn!(
                "geoip: failed to read modification time for optional database '{}': {}",
                path,
                err
            );
            return Ok(None);
        }
        Err(err) => {
            return Err(anyhow::anyhow!(
                "geoip: failed to read modification time for database '{}': {}",
                path,
                err
            ));
        }
    };
    let modified_duration = match modified.duration_since(UNIX_EPOCH) {
        Ok(duration) => duration,
        Err(err) if optional => {
            tracing::warn!(
                "geoip: modification time is before UNIX_EPOCH for optional database '{}': {:?}",
                path,
                err
            );
            return Ok(None);
        }
        Err(err) => {
            return Err(anyhow::anyhow!(
                "geoip: modification time is before UNIX_EPOCH for database '{}': {:?}",
                path,
                err
            ));
        }
    };
    let modified_micros = modified_duration.as_micros();
    let modified_usec = match u64::try_from(modified_micros) {
        Ok(value) => value,
        Err(_) if optional => {
            tracing::warn!(
                "geoip: modification time for optional database '{}' exceeds u64 microsecond range: {}",
                path,
                modified_micros
            );
            return Ok(None);
        }
        Err(_) => {
            return Err(anyhow::anyhow!(
                "geoip: modification time for database '{}' exceeds u64 microsecond range: {}",
                path,
                modified_micros
            ));
        }
    };
    Ok(Some(GeoIpFileSignature {
        modified_usec,
        size: metadata.len(),
    }))
}
