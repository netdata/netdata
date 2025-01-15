// SPDX-License-Identifier: GPL-3.0-or-later

#include "test_cgroups_plugin.h"

void rrdset_is_obsolete___safe_from_collector_thread(RRDSET *st)
{
    UNUSED(st);
}

void rrdset_isnot_obsolete___safe_from_collector_thread(RRDSET *st)
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

RRDSET *rrdset_create_custom(
    RRDHOST *host, const char *type, const char *id, const char *name, const char *family, const char *context,
    const char *title, const char *units, const char *plugin, const char *module, long priority, int update_every,
    RRDSET_TYPE chart_type, RRD_DB_MODE memory_mode, long history_entries)
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
    RRD_ALGORITHM algorithm, RRD_DB_MODE memory_mode)
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

const RRDVAR_ACQUIRED *rrdvar_chart_variable_add_and_acquire(RRDSET *st, const char *name)
{
    UNUSED(st);
    UNUSED(name);

    return NULL;
}

void rrdvar_chart_variable_set(RRDSET *st, const RRDVAR_ACQUIRED *rsa, NETDATA_DOUBLE value)
{
    UNUSED(st);
    UNUSED(rsa);
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
    const char *host_device, const char *container_device, const char *container_name, DICTIONARY *labels, const char *ctx_prefix)
{
    UNUSED(host_device);
    UNUSED(container_device);
    UNUSED(container_name);
    UNUSED(labels);
    UNUSED(ctx_prefix);
}

void netdev_rename_device_del(const char *host_device)
{
    UNUSED(host_device);
}

void rrdcalc_update_rrdlabels(RRDSET *st) {
    (void)st;
}

void db_execute(const char *cmd)
{
    UNUSED(cmd);
}
