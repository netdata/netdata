# Vendored protobuf files

The `.rs` files in this directory are generated from the `.proto` definitions
under `netflow-plugin/proto/` (bio-routing/bio-rd RIS API). They are committed
rather than generated at build time so that builds do not require `protoc`.

## Regenerating

After updating any `.proto` file, run:

```
cargo test -p netflow-plugin --test grpc_build
```

This compiles the proto files, compares the output against the committed files,
and overwrites them if stale. Commit the updated `.rs` files.

CI runs the same test and will fail if the vendored files are out of date.
