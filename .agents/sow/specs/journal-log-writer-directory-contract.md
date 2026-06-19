# journal-log-writer directory contract

## Scope

Durable contract for the `journal-log-writer` crate (`Log::new`) regarding the
journal directory it is given. Applies to every consumer that constructs a
journal `Log`: the OTEL logs plugin (`otel-plugin`) and the NetFlow plugin
(`netflow-plugin`).

## Contract

- **The journal directory passed to `Log::new` MUST be an absolute path.**
  A non-absolute path is rejected at construction with
  `WriterError::InvalidPath`, naming the offending value. The rejection happens
  **before** any filesystem mutation (no directory is created).
- Rationale: journal file paths are parsed by
  `journal_registry::repository::File::from_path`, which only accepts absolute
  paths. A relative directory therefore could never produce a writable journal —
  previously it failed late, at first write, with a cause-less
  `failed to create journal file in <path>` error. The early rejection makes the
  misconfiguration diagnosable at startup.

## Consumer responsibilities

- **Resolve relative configuration before calling `Log::new`.** A consumer that
  accepts a possibly-relative directory from its own config SHOULD resolve it to
  an absolute path first, against a stable base directory it controls — never
  rely on the process working directory. The `Log::new` check is the safety net,
  not the primary resolution mechanism.
  - NetFlow resolves `journal_dir` against the netdata cache directory **when one
    is available**, via `resolve_relative_path`
    (`netflow-plugin/src/plugin_config/runtime.rs`). Its default is relative
    (`flows`), so under netdata (where `NETDATA_CACHE_DIR` is set) it resolves to
    an absolute path. Standalone with no cache dir, the value stays relative and
    is rejected by the `Log::new` check at startup (fail-fast, not a regression —
    that configuration never produced a writable journal).
  - The OTEL logs plugin does not resolve `journal_dir`; it requires an absolute
    value. Its stock config renders one (`<logdir>/otel/v1` via `@logdir_POST@`);
    a relative override (`NETDATA_OTEL_LOGS_JOURNAL_DIR` env or a hand-templated
    config) is treated as operator error and rejected at startup.

## Notes

- `Log::new` still calls `canonicalize()` on the (now guaranteed absolute) path
  to verify accessibility; its result is intentionally not used to rewrite the
  stored path (no symlink normalization), to keep behavior minimal.
