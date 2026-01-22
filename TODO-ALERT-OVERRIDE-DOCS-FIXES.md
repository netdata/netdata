# TODO: Alert Override Docs Fixes

## TL;DR
- Fix remaining doc accuracy issues: disk space chart ID example, disable‑trick explanation, and `edit-config` usage wording.
- Ensure new docs are tracked and mapped under Alerts & Notifications in Learn.

## Analysis (facts from code/docs)
- Stock health config path defaults to `/usr/lib/netdata/conf.d/health.d`. Evidence: `system/edit-config:69-92`, `src/health/README.md:81`, `src/health/health.c:135-145`.
- File shadowing exists: user `health.d` file/subdir with same name prevents stock file/subdir from loading. Evidence: `src/libnetdata/paths/paths.c:219-312`.
- Alert application order: alarms before templates; only one alert per (chart,name) is created because RRDCALC key is `{alert,chart}` and conflicts are rejected. Evidence: `src/health/health_prototypes.c:591-616`, `src/health/rrdcalc.c:177-183`, `src/health/rrdcalc.c:349-352`, `src/health/rrdcalc.c:409-426`.
- Disk space chart IDs are `disk_space.<sanitized_mount>`; context is `disk.space`. Evidence: `src/collectors/diskspace.plugin/plugin_diskspace.c:224-235`, `src/collectors/proc.plugin/proc_self_mountinfo.c:293-295`, `src/database/rrdset-index-id.c:444-445`.
- The `!*` disable shortcut is handled by the health config parser (sets `ap->match.enabled = false`). Evidence: `src/health/health_config.c:506-516`.
- Style guide discourages hardcoding `/etc/netdata/edit-config` in docs; recommends running `edit-config` from the config dir. Evidence: `docs/developer-and-contributor-corner/style-guide.md:303-305`.

## Decisions (confirmed by Costa)
1) **Paths must be stated by name and path**  
   - Use the *config directory name* (e.g., “stock health config directory”) **and** the concrete path.  
   - Also mention this can vary by package/prefix (e.g., `/opt/...`) and point to `edit-config` / `netdata.conf` for discovery.

2) **Keep and document the `host labels: _hostname=!*` trick**  
   - This pattern exists in stock configs today; document it clearly (with caution) rather than remove it.

3) **Reload is reliable; do not imply otherwise**  
   - Use `netdatacli reload-health` (and SIGUSR2 if needed) as the canonical method.  
   - Avoid “restart required” language.

4) **Docs are in repo; keep them and map them**  
   - Add/track the new docs and keep the `REFERENCE.md` link.  
   - Update `docs/.map/map.csv` so the new pages appear in Learn.

5) **Learn path**  
   - Place both new pages under `Alerts & Notifications` so they appear at `https://learn.netdata.cloud/docs/alerts-&-notifications/`.
   - Evidence: existing Alerts & Notifications entries are in `docs/.map/map.csv:159-169`.

## Plan
- Fix disk space chart ID example and update ID discovery snippet.
- Clarify disable‑trick explanation (`!*` parser shortcut).
- Align `edit-config` usage with style guide while keeping path names/paths.
- Add new docs to git and keep map entries under Alerts & Notifications.
- Re-run a focused doc scan to confirm no remaining contradictions.

## Implied Decisions (if you approve recommendations)
- Use flexible stock path wording with a default example.
- Keep the disable trick and explain it as a parser shortcut.
- Prefer reload-health; restart only as a last resort.
- Add new docs to repo.

## Testing Requirements
- Not applicable (documentation-only).

## Documentation Updates Required
- `src/health/alert-configuration-ordering.md`
- `src/health/overriding-stock-alerts.md`
- `src/health/REFERENCE.md` (if link/path wording changes)
