use super::fetch::{build_client, fetch_source_once};
use super::transform::compile_transform;
use super::types::SourceRecordState;
use super::*;

pub(crate) async fn run_network_sources_refresher(
    sources: BTreeMap<String, RemoteNetworkSourceConfig>,
    runtime: NetworkSourcesRuntime,
    shutdown: CancellationToken,
) -> Result<()> {
    if sources.is_empty() {
        return Ok(());
    }

    let state = SourceRecordState {
        by_source: Arc::new(RwLock::new(BTreeMap::new())),
    };

    let mut tasks = JoinSet::new();
    for (name, source) in sources {
        let runtime = runtime.clone();
        let state = state.clone();
        let shutdown = shutdown.clone();
        tasks.spawn(async move {
            run_source_loop(name, source, runtime, state, shutdown).await;
        });
    }

    shutdown.cancelled().await;
    tasks.abort_all();
    while let Some(result) = tasks.join_next().await {
        if let Err(err) = result
            && !err.is_cancelled()
        {
            tracing::warn!("network-sources task join error: {}", err);
        }
    }

    Ok(())
}

async fn run_source_loop(
    name: String,
    source: RemoteNetworkSourceConfig,
    runtime: NetworkSourcesRuntime,
    state: SourceRecordState,
    shutdown: CancellationToken,
) {
    let transform = match compile_transform(&source.transform) {
        Ok(compiled) => compiled,
        Err(err) => {
            tracing::warn!(
                "network-sources source '{}' disabled: invalid transform: {:#}",
                name,
                err
            );
            return;
        }
    };

    let client = match build_client(source.proxy, &source.tls) {
        Ok(client) => client,
        Err(err) => {
            tracing::warn!(
                "network-sources source '{}' disabled: cannot initialize HTTP client: {:#}",
                name,
                err
            );
            return;
        }
    };

    let regular_interval = source.interval.max(Duration::from_secs(60));
    let mut retry_interval = regular_interval / 10;
    if retry_interval < Duration::from_secs(1) {
        retry_interval = Duration::from_secs(1);
    }

    let mut next_wait = Duration::ZERO;
    let mut ticker = tokio::time::interval(regular_interval);
    ticker.set_missed_tick_behavior(MissedTickBehavior::Skip);

    loop {
        if next_wait.is_zero() {
            match fetch_source_once(&client, &source, &transform).await {
                Ok(records) => {
                    publish_source_records(&name, records, &runtime, &state);
                    next_wait = regular_interval;
                    retry_interval = (regular_interval / 10).max(Duration::from_secs(1));
                }
                Err(err) => {
                    tracing::warn!(
                        "network-sources source '{}' refresh failed: {:#}",
                        name,
                        err
                    );
                    next_wait = retry_interval;
                    retry_interval = std::cmp::min(retry_interval * 2, regular_interval);
                }
            }
        }

        tokio::select! {
            _ = shutdown.cancelled() => return,
            _ = tokio::time::sleep(next_wait) => {
                next_wait = Duration::ZERO;
            }
            _ = ticker.tick() => {
                if next_wait > regular_interval {
                    next_wait = regular_interval;
                }
            }
        }
    }
}

fn publish_source_records(
    source_name: &str,
    records: Vec<NetworkSourceRecord>,
    runtime: &NetworkSourcesRuntime,
    state: &SourceRecordState,
) {
    match state.by_source.write() {
        Ok(mut guard) => {
            guard.insert(source_name.to_string(), records);
            let total_len: usize = guard.values().map(|items| items.len()).sum();
            let mut merged = Vec::with_capacity(total_len);
            for items in guard.values() {
                merged.extend(items.iter().cloned());
            }
            runtime.replace_records(merged);
        }
        Err(err) => {
            tracing::warn!(
                "network-sources source '{}' failed to publish records due to poisoned lock: {}",
                source_name,
                err
            );
        }
    }
}
