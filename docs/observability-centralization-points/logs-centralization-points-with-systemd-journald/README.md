# Logs Centralization Points with systemd-journald

```mermaid
stateDiagram-v2
    classDef userFeature fill:#f00,color:white,font-weight:bold,stroke-width:2px,stroke:orange
    classDef usedByNC fill:#090,color:white,font-weight:bold,stroke-width:2px,stroke:orange
    classDef log2journal fill:#040,color:white,font-weight:bold,stroke-width:2px,stroke:orange

    logsQuery:::usedByNC
    logsDashboard:::userFeature
    journald:::thirdparty
    journalRemote:::thirdparty
    journalUpload:::thirdparty
    journalFiles:::thirdparty
    logSources:::thirdparty
    textFiles:::thirdparty
    log2journal:::log2journal
     
    journalRemote: systemd-journal-remote
    journalUpload: systemd-journal-upload
    journalFiles: Store
    journalFiles: systemd-journal files
    journald: systemd-journald
    logSources: Local Logs Sources
    log2journal:log2journal
    log2journal:Convert text, json, logfmt files
    log2journal:to structured journal entries.
    logsDashboard: Netdata Dashboards
    logsQuery: Query Journal Files
    textFiles: Text Log Files

    logSources --> journald: journald API
    logSources --> textFiles: write to log files
    textFiles --> log2journal: tail log files
    log2journal --> journald: journald API
    journald --> journalFiles

    journalFiles --> Netdata
    journalFiles --> journalUpload

    journalRemote -->journalFiles
    journalUpload --> [*]: to a remote journald
    [*] --> journalRemote: from a remote journald

    state Netdata {
        [*]-->logsQuery
        logsQuery --> logsDashboard
    }
```

Logs centralization points can be built using the `systemd-journald` methodologies, by configuring `systemd-journal-remote` (on the centralization point) and `systemd-journal-upload` (on the production system).

The logs centralization points and the metrics centralization points do not need to be the same. For clarity and simplicity, however, when not otherwise required for operational or regulatory reasons, we recommend to have unified centralization points for both metrics and logs.

A Netdata running at the logs centralization point will automatically detect and present the logs of all servers aggregated to it in a unified way (i.e., logs from all servers multiplexed in the same view). This Netdata may or may not be a Netdata Parent for metrics.
