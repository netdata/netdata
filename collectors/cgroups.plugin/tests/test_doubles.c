// SPDX-License-Identifier: GPL-3.0-or-later

#include "test_cgroups_plugin.h"

void rrdset_is_obsolete(RRDSET *st)
{
    UNUSED(st);
}

void rrdset_isnot_obsolete(RRDSET *st)
{
    UNUSED(st);
}

struct mountinfo *mountinfo_read(int do_statvfs)
{
    UNUSED(do_statvfs);

    return NULL;
}

struct mountinfo *
mountinfo_find_by_filesystem_mount_source(struct mountinfo *root, const char *filesystem, const char *mount_source)
{
    UNUSED(root);
    UNUSED(filesystem);
    UNUSED(mount_source);

    return NULL;
}

struct mountinfo *
mountinfo_find_by_filesystem_super_option(struct mountinfo *root, const char *filesystem, const char *super_options)
{
    UNUSED(root);
    UNUSED(filesystem);
    UNUSED(super_options);

    return NULL;
}

void mountinfo_free_all(struct mountinfo *mi)
{
    UNUSED(mi);
}

struct label *__wrap_add_label_to_list(struct label *l, char *key, char *value, LABEL_SOURCE label_source)
{
    function_called();
    check_expected_ptr(l);
    check_expected_ptr(key);
    check_expected_ptr(value);
    check_expected(label_source);
    return l;
}

void rrdset_update_labels(RRDSET *st, struct label *labels)
{
    UNUSED(st);
    UNUSED(labels);
}

RRDSET *rrdset_create_custom(
    RRDHOST *host, const char *type, const char *id, const char *name, const char *family, const char *context,
    const char *title, const char *units, const char *plugin, const char *module, long priority, int update_every,
    RRDSET_TYPE chart_type, RRD_MEMORY_MODE memory_mode, long history_entries)
{
    UNUSED(host);
    UNUSED(type);
    UNUSED(id);
    UNUSED(name);
    UNUSED(family);
    UNUSED(context);
    UNUSED(title);
    UNUSED(units);
    UNUSED(plugin);
    UNUSED(module);
    UNUSED(priority);
    UNUSED(update_every);
    UNUSED(chart_type);
    UNUSED(memory_mode);
    UNUSED(history_entries);

    return NULL;
}

RRDDIM *rrddim_add_custom(
    RRDSET *st, const char *id, const char *name, collected_number multiplier, collected_number divisor,
    RRD_ALGORITHM algorithm, RRD_MEMORY_MODE memory_mode)
{
    UNUSED(st);
    UNUSED(id);
    UNUSED(name);
    UNUSED(multiplier);
    UNUSED(divisor);
    UNUSED(algorithm);
    UNUSED(memory_mode);

    return NULL;
}

collected_number rrddim_set(RRDSET *st, const char *id, collected_number value)
{
    UNUSED(st);
    UNUSED(id);
    UNUSED(value);

    return 0;
}

collected_number rrddim_set_by_pointer(RRDSET *st, RRDDIM *rd, collected_number value)
{
    UNUSED(st);
    UNUSED(rd);
    UNUSED(value);

    return 0;
}

RRDSETVAR *rrdsetvar_custom_chart_variable_create(RRDSET *st, const char *name)
{
    UNUSED(st);
    UNUSED(name);

    return NULL;
}

void rrdsetvar_custom_chart_variable_set(RRDSETVAR *rs, calculated_number value)
{
    UNUSED(rs);
    UNUSED(value);
}

void rrdset_next_usec(RRDSET *st, usec_t microseconds)
{
    UNUSED(st);
    UNUSED(microseconds);
}

void rrdset_done(RRDSET *st)
{
    UNUSED(st);
}

void update_pressure_charts(struct pressure_charts *charts)
{
    UNUSED(charts);
}

void netdev_rename_device_add(
    const char *host_device, const char *container_device, const char *container_name, struct label *labels)
{
    UNUSED(host_device);
    UNUSED(container_device);
    UNUSED(container_name);
    UNUSED(labels);
}

void netdev_rename_device_del(const char *host_device)
{
    UNUSED(host_device);
}
