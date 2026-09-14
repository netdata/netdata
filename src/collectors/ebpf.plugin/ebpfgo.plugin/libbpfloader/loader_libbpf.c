//go:build netdata_ebpf_libbpf
// +build netdata_ebpf_libbpf

#include <stdbool.h>
#include <errno.h>
#include <stdlib.h>
#include <string.h>

#include <bpf/libbpf.h>

#if defined(LIBBPF_MAJOR_VERSION) && (LIBBPF_MAJOR_VERSION >= 1) && defined(__has_include) && __has_include(<linux/btf.h>)
#include <bpf/btf.h>
#define NETDATA_LIBBPF_CORE_SUPPORTED 1
#endif

struct bpf_object *netdata_ebpf_open_file(const char *path)
{
    return bpf_object__open_file(path, NULL);
}

int netdata_ebpf_load_object(struct bpf_object *obj)
{
    return bpf_object__load(obj);
}

void netdata_ebpf_close_object(struct bpf_object *obj)
{
    if (obj != NULL) {
        bpf_object__close(obj);
    }
}

#ifdef NETDATA_LIBBPF_CORE_SUPPORTED
struct btf *netdata_ebpf_parse_btf_file(const char *filename)
{
    struct btf *bf = btf__parse(filename, NULL);
    if (libbpf_get_error(bf)) {
        btf__free(bf);
        return NULL;
    }

    return bf;
}

void netdata_ebpf_free_btf(struct btf *file)
{
    if (file != NULL) {
        btf__free(file);
    }
}

static inline const struct btf_type *netdata_ebpf_find_btf_attach_type(struct btf *file)
{
    int id = btf__find_by_name_kind(file, "bpf_attach_type", BTF_KIND_ENUM);
    if (id < 0) {
        return NULL;
    }

    return btf__type_by_id(file, id);
}

int netdata_ebpf_is_function_inside_btf(struct btf *file, const char *function)
{
    const struct btf_type *type = netdata_ebpf_find_btf_attach_type(file);
    if (!type) {
        return -1;
    }

    const struct btf_enum *e = btf_enum(type);
    int i, id;
    for (id = -1, i = 0; i < btf_vlen(type); i++, e++) {
        if (!strcmp(btf__name_by_offset(file, e->name_off), "BPF_TRACE_FENTRY")) {
            id = btf__find_by_name_kind(file, function, BTF_KIND_FUNC);
            break;
        }
    }

    return (id > 0) ? 1 : 0;
}
#else
struct btf *netdata_ebpf_parse_btf_file(const char *filename)
{
    (void)filename;
    errno = ENOTSUP;
    return NULL;
}

void netdata_ebpf_free_btf(struct btf *file)
{
    (void)file;
}

int netdata_ebpf_is_function_inside_btf(struct btf *file, const char *function)
{
    (void)file;
    (void)function;
    errno = ENOTSUP;
    return -1;
}
#endif
