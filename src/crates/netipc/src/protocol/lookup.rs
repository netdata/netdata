//! Lookup service-kind codecs.

mod apps_lookup;
mod cgroups_lookup;
mod common;

pub use apps_lookup::*;
pub use cgroups_lookup::*;
pub use common::{
    LookupLabelView, LOOKUP_DIR_ENTRY_SIZE, LOOKUP_LABEL_ENTRY_SIZE, ORCHESTRATOR_DOCKER,
    ORCHESTRATOR_K8S, ORCHESTRATOR_KVM, ORCHESTRATOR_LXC, ORCHESTRATOR_NSPAWN, ORCHESTRATOR_PODMAN,
    ORCHESTRATOR_SYSTEMD, ORCHESTRATOR_UNKNOWN,
};
