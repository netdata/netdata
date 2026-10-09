use super::super::*;

#[cfg(test)]
mod endpoints;
#[cfg(test)]
mod exporter;
#[cfg(test)]
mod headers;
#[cfg(test)]
mod helpers;
#[cfg(test)]
mod interfaces;

#[cfg(test)]
use endpoints::*;
#[cfg(test)]
use exporter::*;
#[cfg(test)]
use headers::*;
#[cfg(test)]
use interfaces::*;

impl FlowRecord {
    /// Convert to FlowFields (BTreeMap) for backward compatibility.
    /// Used during the transition period while tiering/encode still expect FlowFields.
    #[cfg(test)]
    pub(crate) fn to_fields(&self) -> FlowFields {
        let mut fields = FlowFields::new();
        insert_exporter_fields(self, &mut fields);
        insert_endpoint_fields(self, &mut fields);
        insert_interface_fields(self, &mut fields);
        insert_header_fields(self, &mut fields);
        fields
    }
}
