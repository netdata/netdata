use super::*;

pub(crate) fn load_geoip_readers(
    paths: &[String],
    field_name: &str,
    optional: bool,
) -> Result<Vec<Reader<Vec<u8>>>> {
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

    let modified = metadata.modified().unwrap_or(SystemTime::UNIX_EPOCH);
    let modified_usec = modified
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_micros() as u64;
    Ok(Some(GeoIpFileSignature {
        modified_usec,
        size: metadata.len(),
    }))
}
