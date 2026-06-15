use super::*;
use bumpalo::Bump;

fn idx<'a>(arena: &'a Bump) -> RowIndex<'a> {
    RowIndex::new(arena, 100)
}

#[test]
fn returns_pair_when_one_namespace_and_one_name() {
    let arena = Bump::new();
    let mut w = idx(&arena);
    w.kv_interner.intern("service.namespace=prod");
    w.kv_interner.intern("service.name=api");
    let s = w.service_stream().unwrap();
    assert_eq!(s.namespace, "prod");
    assert_eq!(s.name, "api");
}

#[test]
fn name_only_returns_empty_namespace() {
    let arena = Bump::new();
    let mut w = idx(&arena);
    w.kv_interner.intern("service.name=api");
    let s = w.service_stream().unwrap();
    assert_eq!(s.namespace, "");
    assert_eq!(s.name, "api");
}

#[test]
fn namespace_only_returns_empty_name() {
    let arena = Bump::new();
    let mut w = idx(&arena);
    w.kv_interner.intern("service.namespace=prod");
    let s = w.service_stream().unwrap();
    assert_eq!(s.namespace, "prod");
    assert_eq!(s.name, "");
}

#[test]
fn neither_returns_empty_pair() {
    let arena = Bump::new();
    let mut w = idx(&arena);
    // Some unrelated kv pairs in the interner — shouldn't affect the result.
    w.kv_interner.intern("host.name=foo");
    w.kv_interner.intern("k8s.pod.uid=bar");
    let s = w.service_stream().unwrap();
    assert_eq!(s.namespace, "");
    assert_eq!(s.name, "");
}

#[test]
fn multiple_names_yield_error() {
    let arena = Bump::new();
    let mut w = idx(&arena);
    w.kv_interner.intern("service.namespace=prod");
    w.kv_interner.intern("service.name=api");
    w.kv_interner.intern("service.name=worker");
    let IndexError::MultipleStreams { namespaces, names } = w.service_stream().unwrap_err() else {
        panic!("expected MultipleStreams");
    };
    assert_eq!(namespaces, vec!["prod"]);
    assert_eq!(names.len(), 2);
    assert!(names.contains(&"api".to_string()));
    assert!(names.contains(&"worker".to_string()));
}

#[test]
fn multiple_namespaces_yield_error() {
    let arena = Bump::new();
    let mut w = idx(&arena);
    w.kv_interner.intern("service.namespace=prod");
    w.kv_interner.intern("service.namespace=staging");
    w.kv_interner.intern("service.name=api");
    let IndexError::MultipleStreams { namespaces, names } = w.service_stream().unwrap_err() else {
        panic!("expected MultipleStreams");
    };
    assert_eq!(names, vec!["api"]);
    assert_eq!(namespaces.len(), 2);
    assert!(namespaces.contains(&"prod".to_string()));
    assert!(namespaces.contains(&"staging".to_string()));
}

#[test]
fn prefix_matching_does_not_pick_up_subkeys() {
    let arena = Bump::new();
    let mut w = idx(&arena);
    // Keys that share the prefix without the trailing `=` must not match.
    w.kv_interner.intern("service.name_extra=foo");
    w.kv_interner.intern("service.namespace_extra=bar");
    let s = w.service_stream().unwrap();
    assert_eq!(s.namespace, "");
    assert_eq!(s.name, "");
}
