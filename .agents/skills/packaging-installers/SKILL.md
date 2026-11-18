---
name: packaging-installers
description: Change, review or troubleshoot Netdata kickstart, source installer, updater, Docker deployment helpers, or shared service and logrotate integration. Covers install/update/removal, persistent state and real lifecycle validation. Native package metadata and scriptlets use packaging-native-packages; static builds use packaging-static-installer; publication uses packaging-component-releases.
---

# Installer and deployment lifecycle

Use this skill for developer work on installation owners and their callers. Start with the requested route and
operation, then trace the shared helpers it uses. Read-only review uses source and existing evidence; it does not
run the installation procedures in linked operator docs. Action boundaries remain in `AGENTS.md#skill-selection`.

## Choose the affected contract

| Task | Owners to inspect |
|---|---|
| Source installation or kickstart fallback | `netdata-installer.sh`, `packaging/installer/kickstart.sh`, `packaging/installer/functions.sh` |
| Scheduled/manual Agent updates | `packaging/installer/netdata-updater.sh` and the installer or package manager it invokes |
| Optional DEM pairing, offline inputs or repair | `packaging/installer/netdata-dem-pair.sh`, `packaging/installer/DEM-RUNTIME.md#paired-agent-updates` |
| Docker entrypoint, host setup or image replacement | `packaging/docker/run.sh`, `packaging/docker/README.md#update-your-netdata-docker-container`, `packaging/installer/DEM-RUNTIME.md#docker-pair-switching` |
| Shared service, IPC or log rotation | `system/systemd/`, `system/logrotate/`, `system/install-service.sh.in`, `docs/netdata-agent/start-stop-restart.md#unix` |
| DEB/RPM dependencies or package hooks | `.agents/skills/packaging-native-packages/SKILL.md`; this skill covers their shared installer/service callers |
| Static build, ELF checks or makeself artifact production | `.agents/skills/packaging-static-installer/SKILL.md`; this skill covers installation/update callers |
| Release identity, signing or public transport | `.agents/skills/packaging-component-releases/SKILL.md`; this skill covers installed lifecycle |

## Trace ownership before changing behavior

1. Map the complete path from entrypoint to installed helper and service. Record which account runs each step,
   which paths it can traverse/execute, how arguments/environment survive privilege elevation, and how callers
   interpret exit statuses. A root-only helper test does not prove access by the service account; a correct child
   failure can still trigger the wrong caller cleanup or fallback.
2. Separate shipped content from retained configuration, data and runtime ownership records. Inspect the existing
   cleanup and permission owners before changing them. Preserve preexisting prefixes on failed updates, including
   symlinked prefixes; distinguish them from newly created partial installations. For static/source state boundaries,
   use `packaging/makeself/README.md#state-permissions-during-updates` and its linked regression tests.
3. Keep companion coordination around the normal installation owner. Determine the required failure/retry behavior
   before adding rollback machinery; pairing alone does not require a second Agent installer or executable backup
   framework. Use the documented DEM transaction and execution boundaries from the paired-update owner above.
4. Trace service changes beyond the component hook. Changes to PID or IPC paths also affect logrotate,
   installer/uninstaller and legacy service readers. Test custom accounts and IPC configuration when affected.
   Directory ownership checks: `.agents/skills/packaging-native-packages/SKILL.md#validate-at-the-relevant-layer`.
   For logrotate, verify a fresh record reaches the new inode without restarting the Agent or signalling
   unrelated child processes.
5. Treat Docker image, persistent volume and host-policy lifetimes separately. Inspect the target image's helper,
   entrypoint and mounts before inferring update behavior from image replacement. Use
   `packaging/installer/DEM-RUNTIME.md#docker-host-preparation` for supported enrollment/host setup and its paired
   switching section for updates. Preserve the service identity and machine-ID mapping used by stored history.

## Validate the affected path

- Start with focused fixtures that execute shipped helpers and callers. Select by the behavior changed:
  `packaging/installer/tests/test_static_install_callers.py`, `packaging/installer/tests/test_dem_pair.py`,
  `packaging/installer/tests/test_updater_dem.py`, `packaging/installer/tests/test_updater_artifacts.py`, or
  `packaging/installer/tests/test_dem_docker_pair.py`. Do not recreate the installer in a test or claim a fixture
  establishes a full package transaction or real service restart.
- For an authorized lifecycle test, use a disposable target with the relevant kernel/service policy. Record exact
  artifact identities and distinguish public artifacts from locally modified candidates. Pin Docker image digests
  when tags may move. Follow the user's normal installation/update entrypoint and retain its prerequisites.
- Cover the affected transitions: fresh versus enrolled, running versus deliberately stopped, update/retry, and
  removal/reinstallation. Check configuration/history and ordinary monitoring as well as the optional component.
  A successful installer exit or active service is not sufficient: run a fresh workload through the installed
  service account, and inspect errors during the observation window instead of waiting only for eventual success.
- Preserve the target's security policy while diagnosing failures. Record actual process credentials, confinement
  and sandbox evidence when making those claims; evidence distinctions are owned by
  `.agents/skills/packaging-component-releases/sre-handoff.md#packaging-review-and-first-publication`.
  Correlate service logs and policy denials before assigning a shared startup failure to a component.
- Reuse valid evidence for unchanged paths. Scope architecture/platform checks to affected binaries, ABIs, scripts
  and host policies; report untested routes. Repeat a full matrix only when changed contracts or evidence gaps
  warrant it.

Capture reusable discoveries under `AGENTS.md#knowledge-capture`. Keep task evidence under
`<repo>/.local/audits/installers/`; linked operator docs remain the owners of installation commands and behavior.
