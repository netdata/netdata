use super::{apps_lookup_dispatch, cgroups_lookup_dispatch, ClientState, DispatchError, RawClient};
use crate::protocol::{
    self, AppsLookupBuilder, CgroupsLookupBuilder, APPS_CGROUP_KNOWN, APPS_LOOKUP_KEY_SIZE,
    APPS_LOOKUP_REQ_HDR_SIZE, APPS_LOOKUP_RESP_HDR_SIZE, CGROUPS_LOOKUP_REQ_HDR_SIZE,
    CGROUPS_LOOKUP_RESP_HDR_SIZE, CGROUP_LOOKUP_KNOWN, LOOKUP_DIR_ENTRY_SIZE, NIPC_UID_UNSET,
    ORCHESTRATOR_DOCKER, ORCHESTRATOR_K8S, PID_LOOKUP_KNOWN,
};
#[cfg(unix)]
use crate::transport::posix::ClientConfig;
#[cfg(windows)]
use crate::transport::windows::ClientConfig;
use std::sync::Arc;

fn apps_request(pids: &[u32]) -> Vec<u8> {
    let mut request = vec![
        0u8;
        APPS_LOOKUP_REQ_HDR_SIZE
            + pids.len() * (LOOKUP_DIR_ENTRY_SIZE + APPS_LOOKUP_KEY_SIZE)
    ];
    let len = protocol::encode_apps_lookup_request(pids, &mut request).unwrap();
    request.truncate(len);
    request
}

fn cgroups_request(paths: &[&[u8]]) -> Vec<u8> {
    let mut request = vec![0u8; CGROUPS_LOOKUP_REQ_HDR_SIZE + paths.len() * 64];
    let len = protocol::encode_cgroups_lookup_request(paths, &mut request).unwrap();
    request.truncate(len);
    request
}

#[test]
fn apps_dispatch_preserves_builder_overflow_on_handler_failure() {
    let request = apps_request(&[1234]);
    let mut response = vec![0u8; APPS_LOOKUP_RESP_HDR_SIZE + LOOKUP_DIR_ENTRY_SIZE];
    let dispatch =
        apps_lookup_dispatch(Arc::new(|request, builder: &mut AppsLookupBuilder<'_>| {
            let pid = request.item(0).unwrap();
            assert_eq!(
                builder.add(
                    PID_LOOKUP_KNOWN,
                    APPS_CGROUP_KNOWN,
                    ORCHESTRATOR_DOCKER,
                    pid,
                    1,
                    NIPC_UID_UNSET,
                    42,
                    b"proc",
                    b"/docker/abc",
                    b"container-a",
                    &[(b"role".as_slice(), b"web".as_slice())],
                ),
                Err(protocol::NipcError::Overflow)
            );
            false
        }));

    assert_eq!(
        dispatch(&request, &mut response),
        Err(DispatchError::Overflow)
    );
}

#[test]
fn apps_dispatch_reports_handler_failure_without_builder_error() {
    let request = apps_request(&[1234]);
    let mut response = vec![0u8; APPS_LOOKUP_RESP_HDR_SIZE + LOOKUP_DIR_ENTRY_SIZE];
    let dispatch = apps_lookup_dispatch(Arc::new(|_, _| false));

    assert_eq!(
        dispatch(&request, &mut response),
        Err(DispatchError::HandlerFailed)
    );
}

#[test]
fn cgroups_dispatch_preserves_builder_overflow_on_handler_failure() {
    let request = cgroups_request(&[b"/docker/abc"]);
    let mut response = vec![0u8; CGROUPS_LOOKUP_RESP_HDR_SIZE + LOOKUP_DIR_ENTRY_SIZE];
    let dispatch = cgroups_lookup_dispatch(Arc::new(
        |request, builder: &mut CgroupsLookupBuilder<'_>| {
            let path = request.item(0).unwrap();
            assert_eq!(
                builder.add(
                    CGROUP_LOOKUP_KNOWN,
                    ORCHESTRATOR_K8S,
                    path.as_bytes(),
                    b"container-a",
                    &[(b"role".as_slice(), b"web".as_slice())],
                ),
                Err(protocol::NipcError::Overflow)
            );
            false
        },
    ));

    assert_eq!(
        dispatch(&request, &mut response),
        Err(DispatchError::Overflow)
    );
}

#[test]
fn cgroups_dispatch_reports_handler_failure_without_builder_error() {
    let request = cgroups_request(&[b"/docker/abc"]);
    let mut response = vec![0u8; CGROUPS_LOOKUP_RESP_HDR_SIZE + LOOKUP_DIR_ENTRY_SIZE];
    let dispatch = cgroups_lookup_dispatch(Arc::new(|_, _| false));

    assert_eq!(
        dispatch(&request, &mut response),
        Err(DispatchError::HandlerFailed)
    );
}

#[test]
fn request_capacity_growth_honors_abort_before_reconnect() {
    let mut config = ClientConfig::default();
    config.max_request_payload_bytes = protocol::MAX_PAYLOAD_CAP;
    let mut client = RawClient::new_apps_lookup("/tmp", "missing", config);

    client.state = ClientState::Ready;
    client.abort();

    assert_eq!(
        client.ensure_lookup_request_capacity(protocol::MAX_PAYLOAD_DEFAULT as usize + 1),
        Err(protocol::NipcError::Aborted)
    );
    assert_eq!(client.state, ClientState::Broken);
    assert_eq!(client.reconnect_count, 0);
    assert_eq!(client.error_count, 1);
}
