// SPDX-License-Identifier: GPL-3.0-or-later

#include "cgroup-internals.h"

void update_cpu_utilization_chart(struct cgroup *cg) {
    RRDSET *chart = cg->st_cpu;

    if (unlikely(!cg->st_cpu)) {
        char *title;
        char *context;
        int prio;

        if (is_cgroup_systemd_service(cg)) {
            title = "Systemd Services CPU utilization (100%% = 1 core)";
            context = "systemd.service.cpu.utilization";
            prio = NETDATA_CHART_PRIO_CGROUPS_SYSTEMD;
        } else {
            title = k8s_is_kubepod(cg) ? "CPU Usage (100%% = 1000 mCPU)" : "CPU Usage (100%% = 1 core)";
            context = k8s_is_kubepod(cg) ? "k8s.cgroup.cpu" : "cgroup.cpu";
            prio = NETDATA_CHART_PRIO_CGROUPS_CONTAINERS;
        }

        char buff[RRD_ID_LENGTH_MAX + 1];
        chart = cg->st_cpu = rrdset_create_localhost(
            cgroup_chart_type(buff, cg),
            "cpu",
            NULL,
            "cpu",
            context,
            title,
            "percentage",
            PLUGIN_CGROUPS_NAME,
            is_cgroup_systemd_service(cg) ? PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME : PLUGIN_CGROUPS_MODULE_CGROUPS_NAME,
            prio,
            cgroup_update_every,
            RRDSET_TYPE_STACKED);

        rrdset_update_rrdlabels(chart, cg->chart_labels);

        if (!(cg->options & CGROUP_OPTIONS_IS_UNIFIED)) {
            cg->st_cpu_rd_user = rrddim_add(chart, "user", NULL, 100, system_hz, RRD_ALGORITHM_INCREMENTAL);
            cg->st_cpu_rd_system = rrddim_add(chart, "system", NULL, 100, system_hz, RRD_ALGORITHM_INCREMENTAL);
        } else {
            cg->st_cpu_rd_user = rrddim_add(chart, "user", NULL, 100, 1000000, RRD_ALGORITHM_INCREMENTAL);
            cg->st_cpu_rd_system = rrddim_add(chart, "system", NULL, 100, 1000000, RRD_ALGORITHM_INCREMENTAL);
        }
    }

    rrddim_set_by_pointer(chart, cg->st_cpu_rd_user, (collected_number)cg->cpuacct_stat.user);
    rrddim_set_by_pointer(chart, cg->st_cpu_rd_system, (collected_number)cg->cpuacct_stat.system);
    rrdset_done(chart);
}

void update_cpu_utilization_limit_chart(struct cgroup *cg, NETDATA_DOUBLE cpu_limit) {
    if (is_cgroup_systemd_service(cg))
        return;

    RRDSET *chart = cg->st_cpu_limit;

    if (unlikely(!cg->st_cpu_limit)) {
        char *title = "CPU Usage within the limits";
        char *context = k8s_is_kubepod(cg) ? "k8s.cgroup.cpu_limit" : "cgroup.cpu_limit";
        int prio = NETDATA_CHART_PRIO_CGROUPS_CONTAINERS - 1;

        char buff[RRD_ID_LENGTH_MAX + 1];
        chart = cg->st_cpu_limit = rrdset_create_localhost(
            cgroup_chart_type(buff, cg),
            "cpu_limit",
            NULL,
            "cpu",
            context,
            title,
            "percentage",
            PLUGIN_CGROUPS_NAME,
            is_cgroup_systemd_service(cg) ? PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME : PLUGIN_CGROUPS_MODULE_CGROUPS_NAME,
            prio,
            cgroup_update_every,
            RRDSET_TYPE_LINE);

        rrdset_update_rrdlabels(chart, cg->chart_labels);

        if (!(cg->options & CGROUP_OPTIONS_IS_UNIFIED))
            rrddim_add(chart, "used", NULL, 1, system_hz, RRD_ALGORITHM_ABSOLUTE);
        else
            rrddim_add(chart, "used", NULL, 1, 1000000, RRD_ALGORITHM_ABSOLUTE);
        cg->prev_cpu_usage = (NETDATA_DOUBLE)(cg->cpuacct_stat.user + cg->cpuacct_stat.system) * 100;
    }

    NETDATA_DOUBLE cpu_usage = 0;
    cpu_usage = (NETDATA_DOUBLE)(cg->cpuacct_stat.user + cg->cpuacct_stat.system) * 100;
    NETDATA_DOUBLE cpu_used = 100 * (cpu_usage - cg->prev_cpu_usage) / (cpu_limit * cgroup_update_every);

    rrdset_isnot_obsolete___safe_from_collector_thread(chart);

    rrddim_set(chart, "used", (cpu_used > 0) ? (collected_number)cpu_used : 0);

    cg->prev_cpu_usage = cpu_usage;

    rrdvar_chart_variable_set(cg->st_cpu, cg->chart_var_cpu_limit, cpu_limit);
    rrdset_done(chart);
}

void update_cpu_throttled_chart(struct cgroup *cg) {
    if (is_cgroup_systemd_service(cg))
        return;

    RRDSET *chart = cg->st_cpu_nr_throttled;

    if (unlikely(!cg->st_cpu_nr_throttled)) {
        char *title = "CPU Throttled Runnable Periods";
        char *context = k8s_is_kubepod(cg) ? "k8s.cgroup.throttled" : "cgroup.throttled";
        int prio = NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 10;

        char buff[RRD_ID_LENGTH_MAX + 1];
        chart = cg->st_cpu_nr_throttled = rrdset_create_localhost(
            cgroup_chart_type(buff, cg),
            "throttled",
            NULL,
            "cpu",
            context,
            title,
            "percentage",
            PLUGIN_CGROUPS_NAME,
            is_cgroup_systemd_service(cg) ? PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME : PLUGIN_CGROUPS_MODULE_CGROUPS_NAME,
            prio,
            cgroup_update_every,
            RRDSET_TYPE_LINE);

        rrdset_update_rrdlabels(chart, cg->chart_labels);
        rrddim_add(chart, "throttled", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set(chart, "throttled", (collected_number)cg->cpuacct_cpu_throttling.nr_throttled_perc);
    rrdset_done(chart);
}

void update_cpu_throttled_duration_chart(struct cgroup *cg) {
    if (is_cgroup_systemd_service(cg))
        return;

    RRDSET *chart = cg->st_cpu_throttled_time;

    if (unlikely(!cg->st_cpu_throttled_time)) {
        char *title = "CPU Throttled Time Duration";
        char *context = k8s_is_kubepod(cg) ? "k8s.cgroup.throttled_duration" : "cgroup.throttled_duration";
        int prio = NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 15;

        char buff[RRD_ID_LENGTH_MAX + 1];
        chart = cg->st_cpu_throttled_time = rrdset_create_localhost(
            cgroup_chart_type(buff, cg),
            "throttled_duration",
            NULL,
            "cpu",
            context,
            title,
            "ms",
            PLUGIN_CGROUPS_NAME,
            is_cgroup_systemd_service(cg) ? PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME : PLUGIN_CGROUPS_MODULE_CGROUPS_NAME,
            prio,
            cgroup_update_every,
            RRDSET_TYPE_LINE);

        rrdset_update_rrdlabels(chart, cg->chart_labels);
        rrddim_add(chart, "duration", NULL, 1, 1000000, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set(chart, "duration", (collected_number)cg->cpuacct_cpu_throttling.throttled_time);
    rrdset_done(chart);
}

void update_cpu_shares_chart(struct cgroup *cg) {
    if (is_cgroup_systemd_service(cg))
        return;

    RRDSET *chart = cg->st_cpu_shares;

    if (unlikely(!cg->st_cpu_shares)) {
        char *title = "CPU Time Relative Share";
        char *context = k8s_is_kubepod(cg) ? "k8s.cgroup.cpu_shares" : "cgroup.cpu_shares";
        int prio = NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 20;

        char buff[RRD_ID_LENGTH_MAX + 1];
        chart = cg->st_cpu_shares = rrdset_create_localhost(
            cgroup_chart_type(buff, cg),
            "cpu_shares",
            NULL,
            "cpu",
            context,
            title,
            "shares",
            PLUGIN_CGROUPS_NAME,
            is_cgroup_systemd_service(cg) ? PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME : PLUGIN_CGROUPS_MODULE_CGROUPS_NAME,
            prio,
            cgroup_update_every,
            RRDSET_TYPE_LINE);

        rrdset_update_rrdlabels(chart, cg->chart_labels);
        rrddim_add(chart, "shares", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set(chart, "shares", (collected_number)cg->cpuacct_cpu_shares.shares);
    rrdset_done(chart);
}

void update_cpu_per_core_usage_chart(struct cgroup *cg) {
    if (is_cgroup_systemd_service(cg))
        return;

    char id[RRD_ID_LENGTH_MAX + 1];
    unsigned int i;

    if (unlikely(!cg->st_cpu_per_core)) {
        char *title = k8s_is_kubepod(cg) ? "CPU Usage (100%% = 1000 mCPU) Per Core" : "CPU Usage (100%% = 1 core) Per Core";
        char *context = k8s_is_kubepod(cg) ? "k8s.cgroup.cpu_per_core" : "cgroup.cpu_per_core";
        int prio = NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 100;

        char buff[RRD_ID_LENGTH_MAX + 1];
        cg->st_cpu_per_core = rrdset_create_localhost(
            cgroup_chart_type(buff, cg),
            "cpu_per_core",
            NULL,
            "cpu",
            context,
            title,
            "percentage",
            PLUGIN_CGROUPS_NAME,
            is_cgroup_systemd_service(cg) ? PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME : PLUGIN_CGROUPS_MODULE_CGROUPS_NAME,
            prio,
            cgroup_update_every,
            RRDSET_TYPE_STACKED);

        rrdset_update_rrdlabels(cg->st_cpu_per_core, cg->chart_labels);

        for (i = 0; i < cg->cpuacct_usage.cpus; i++) {
            snprintfz(id, RRD_ID_LENGTH_MAX, "cpu%u", i);
            rrddim_add(cg->st_cpu_per_core, id, NULL, 100, 1000000000, RRD_ALGORITHM_INCREMENTAL);
        }
    }

    for (i = 0; i < cg->cpuacct_usage.cpus; i++) {
        snprintfz(id, RRD_ID_LENGTH_MAX, "cpu%u", i);
        rrddim_set(cg->st_cpu_per_core, id, (collected_number)cg->cpuacct_usage.cpu_percpu[i]);
    }
    rrdset_done(cg->st_cpu_per_core);
}

void update_mem_usage_detailed_chart(struct cgroup *cg) {
    RRDSET *chart = cg->st_mem;

    if (unlikely(!cg->st_mem)) {
        char *title;
        char *context;
        int prio;
        if (is_cgroup_systemd_service(cg)) {
            title = "Systemd Services Memory";
            context = "systemd.service.memory.ram.usage";
            prio = NETDATA_CHART_PRIO_CGROUPS_SYSTEMD + 15;
        } else {
            title = "Memory Usage";
            context = k8s_is_kubepod(cg) ? "k8s.cgroup.mem" : "cgroup.mem";
            prio = NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 220;
        }

        char buff[RRD_ID_LENGTH_MAX + 1];

        chart = cg->st_mem = rrdset_create_localhost(
            cgroup_chart_type(buff, cg),
            "mem",
            NULL,
            "mem",
            context,
            title,
            "MiB",
            PLUGIN_CGROUPS_NAME,
            is_cgroup_systemd_service(cg) ? PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME : PLUGIN_CGROUPS_MODULE_CGROUPS_NAME,
            prio,
            cgroup_update_every,
            RRDSET_TYPE_STACKED);

        rrdset_update_rrdlabels(chart, cg->chart_labels);

        if (!(cg->options & CGROUP_OPTIONS_IS_UNIFIED)) {
            rrddim_add(chart, "cache", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
            rrddim_add(chart, "rss", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);

            if (cg->memory.detailed_has_swap)
                rrddim_add(chart, "swap", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);

            rrddim_add(chart, "rss_huge", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
            rrddim_add(chart, "mapped_file", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
        } else {
            rrddim_add(chart, "anon", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
            rrddim_add(chart, "kernel_stack", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
            rrddim_add(chart, "slab", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
            rrddim_add(chart, "sock", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
            rrddim_add(chart, "anon_thp", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
            rrddim_add(chart, "file", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
        }
    }

    if (!(cg->options & CGROUP_OPTIONS_IS_UNIFIED)) {
        rrddim_set(chart, "cache", (collected_number)cg->memory.total_cache);
        collected_number rss = (collected_number)(cg->memory.total_rss - cg->memory.total_rss_huge);
        if (rss < 0)
            rss = 0;
        rrddim_set(chart, "rss", rss);
        if (cg->memory.detailed_has_swap)
            rrddim_set(chart, "swap", (collected_number)cg->memory.total_swap);
        rrddim_set(chart, "rss_huge", (collected_number)cg->memory.total_rss_huge);
        rrddim_set(chart, "mapped_file", (collected_number)cg->memory.total_mapped_file);
    } else {
        rrddim_set(chart, "anon", (collected_number)cg->memory.anon);
        rrddim_set(chart, "kernel_stack", (collected_number)cg->memory.kernel_stack);
        rrddim_set(chart, "slab", (collected_number)cg->memory.slab);
        rrddim_set(chart, "sock", (collected_number)cg->memory.sock);
        rrddim_set(chart, "anon_thp", (collected_number)cg->memory.anon_thp);
        rrddim_set(chart, "file", (collected_number)cg->memory.total_mapped_file);
    }
    rrdset_done(chart);
}

void update_mem_writeback_chart(struct cgroup *cg) {
    RRDSET *chart = cg->st_writeback;

    if (unlikely(!cg->st_writeback)) {
        char *title;
        char *context;
        int prio;
        if (is_cgroup_systemd_service(cg)) {
            title = "Systemd Services Writeback Memory";
            context = "systemd.service.memory.writeback";
            prio = NETDATA_CHART_PRIO_CGROUPS_SYSTEMD + 20;
        } else {
            title = "Writeback Memory";
            context = k8s_is_kubepod(cg) ? "k8s.cgroup.writeback" : "cgroup.writeback";
            prio = NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 300;
        }

        char buff[RRD_ID_LENGTH_MAX + 1];
        chart = cg->st_writeback = rrdset_create_localhost(
            cgroup_chart_type(buff, cg),
            "writeback",
            NULL,
            "mem",
            context,
            title,
            "MiB",
            PLUGIN_CGROUPS_NAME,
            is_cgroup_systemd_service(cg) ? PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME : PLUGIN_CGROUPS_MODULE_CGROUPS_NAME,
            prio,
            cgroup_update_every,
            RRDSET_TYPE_AREA);

        rrdset_update_rrdlabels(chart, cg->chart_labels);
        if (cg->memory.detailed_has_dirty)
            rrddim_add(chart, "dirty", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
        rrddim_add(chart, "writeback", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
    }

    if (cg->memory.detailed_has_dirty)
        rrddim_set(chart, "dirty", (collected_number)cg->memory.total_dirty);
    rrddim_set(chart, "writeback", (collected_number)cg->memory.total_writeback);
    rrdset_done(chart);
}

void update_mem_activity_chart(struct cgroup *cg) {
    RRDSET *chart = cg->st_mem_activity;

    if (unlikely(!cg->st_mem_activity)) {
        char *title;
        char *context;
        int prio;
        if (is_cgroup_systemd_service(cg)) {
            title = "Systemd Services Memory Paging IO";
            context = "systemd.service.memory.paging.io";
            prio = NETDATA_CHART_PRIO_CGROUPS_SYSTEMD + 30;
        } else {
            title = "Memory Activity";
            context = k8s_is_kubepod(cg) ? "k8s.cgroup.mem_activity" : "cgroup.mem_activity";
            prio = NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 400;
        }

        char buff[RRD_ID_LENGTH_MAX + 1];
        chart = cg->st_mem_activity = rrdset_create_localhost(
            cgroup_chart_type(buff, cg),
            "mem_activity",
            NULL,
            "mem",
            context,
            title,
            "MiB/s",
            PLUGIN_CGROUPS_NAME,
            is_cgroup_systemd_service(cg) ? PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME : PLUGIN_CGROUPS_MODULE_CGROUPS_NAME,
            prio,
            cgroup_update_every,
            RRDSET_TYPE_LINE);

        rrdset_update_rrdlabels(chart, cg->chart_labels);
        // FIXME: systemd just in, out 
        rrddim_add(chart, "pgpgin", "in", system_page_size, 1024 * 1024, RRD_ALGORITHM_INCREMENTAL);
        rrddim_add(chart, "pgpgout", "out", -system_page_size, 1024 * 1024, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set(chart, "pgpgin", (collected_number)cg->memory.total_pgpgin);
    rrddim_set(chart, "pgpgout", (collected_number)cg->memory.total_pgpgout);
    rrdset_done(chart);
}

void update_mem_pgfaults_chart(struct cgroup *cg) {
    RRDSET *chart = cg->st_pgfaults;

    if (unlikely(!cg->st_pgfaults)) {
        char *title;
        char *context;
        int prio;
        if (is_cgroup_systemd_service(cg)) {
            title = "Systemd Services Memory Page Faults";
            context = "systemd.service.memory.paging.faults";
            prio = NETDATA_CHART_PRIO_CGROUPS_SYSTEMD + 25;
        } else {
            title = "Memory Page Faults";
            context = k8s_is_kubepod(cg) ? "k8s.cgroup.pgfaults" : "cgroup.pgfaults";
            prio = NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 500;
        }

        char buff[RRD_ID_LENGTH_MAX + 1];
        chart = cg->st_pgfaults = rrdset_create_localhost(
            cgroup_chart_type(buff, cg),
            "pgfaults",
            NULL,
            "mem",
            context,
            title,
            "MiB/s",
            PLUGIN_CGROUPS_NAME,
            is_cgroup_systemd_service(cg) ? PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME : PLUGIN_CGROUPS_MODULE_CGROUPS_NAME,
            prio,
            cgroup_update_every,
            RRDSET_TYPE_LINE);

        rrdset_update_rrdlabels(chart, cg->chart_labels);
        rrddim_add(chart, "pgfault", NULL, system_page_size, 1024 * 1024, RRD_ALGORITHM_INCREMENTAL);
        rrddim_add(chart, "pgmajfault", "swap", -system_page_size, 1024 * 1024, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set(chart, "pgfault", (collected_number)cg->memory.total_pgfault);
    rrddim_set(chart, "pgmajfault", (collected_number)cg->memory.total_pgmajfault);
    rrdset_done(chart);
}

void update_mem_usage_limit_chart(struct cgroup *cg, unsigned long long memory_limit) {
    if (is_cgroup_systemd_service(cg) || !memory_limit)
        return;

    RRDSET *chart = cg->st_mem_usage_limit;

    if (unlikely(!cg->st_mem_usage_limit)) {
        char *title = "Used RAM within the limits";
        char *context = k8s_is_kubepod(cg) ? "k8s.cgroup.mem_usage_limit" : "cgroup.mem_usage_limit";
        int prio = NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 200;

        char buff[RRD_ID_LENGTH_MAX + 1];
        chart = cg->st_mem_usage_limit = rrdset_create_localhost(
            cgroup_chart_type(buff, cg),
            "mem_usage_limit",
            NULL,
            "mem",
            context,
            title,
            "MiB",
            PLUGIN_CGROUPS_NAME,
            is_cgroup_systemd_service(cg) ? PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME : PLUGIN_CGROUPS_MODULE_CGROUPS_NAME,
            prio,
            cgroup_update_every,
            RRDSET_TYPE_STACKED);

        rrdset_update_rrdlabels(chart, cg->chart_labels);

        rrddim_add(chart, "available", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
        rrddim_add(chart, "used", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
    }

    rrdset_isnot_obsolete___safe_from_collector_thread(chart);

    rrddim_set(chart, "available", (collected_number)(memory_limit - cg->memory.usage_in_bytes));
    rrddim_set(chart, "used", (collected_number)cg->memory.usage_in_bytes);
    rrdset_done(chart);
}

void update_mem_utilization_chart(struct cgroup *cg, unsigned long long memory_limit) {
    if (is_cgroup_systemd_service(cg) || !memory_limit)
        return;

    RRDSET *chart = cg->st_mem_utilization;

    if (unlikely(!cg->st_mem_utilization)) {
        char *title = "Memory Utilization";
        char *context = k8s_is_kubepod(cg) ? "k8s.cgroup.mem_utilization" : "cgroup.mem_utilization";
        int prio = NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 199;

        char buff[RRD_ID_LENGTH_MAX + 1];
        chart = cg->st_mem_utilization = rrdset_create_localhost(
            cgroup_chart_type(buff, cg),
            "mem_utilization",
            NULL,
            "mem",
            context,
            title,
            "percentage",
            PLUGIN_CGROUPS_NAME,
            is_cgroup_systemd_service(cg) ? PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME : PLUGIN_CGROUPS_MODULE_CGROUPS_NAME,
            prio,
            cgroup_update_every,
            RRDSET_TYPE_AREA);

        rrdset_update_rrdlabels(chart, cg->chart_labels);

        rrddim_add(chart, "utilization", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrdset_isnot_obsolete___safe_from_collector_thread(chart);
    collected_number util = (collected_number)(cg->memory.usage_in_bytes * 100 / memory_limit);
    rrddim_set(chart, "utilization", util);
    rrdset_done(chart);
}

void update_mem_failcnt_chart(struct cgroup *cg) {
    RRDSET *chart = cg->st_mem_failcnt;

    if (unlikely(!cg->st_mem_failcnt)) {
        char *title;
        char *context;
        int prio;
        if (is_cgroup_systemd_service(cg)) {
            title = "Systemd Services Memory Limit Failures";
            context = "systemd.service.memory.failcnt";
            prio = NETDATA_CHART_PRIO_CGROUPS_SYSTEMD + 10;
        } else {
            title = "Memory Limit Failures";
            context = k8s_is_kubepod(cg) ? "k8s.cgroup.mem_failcnt" : "cgroup.mem_failcnt";
            prio = NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 250;
        }

        char buff[RRD_ID_LENGTH_MAX + 1];
        chart = cg->st_mem_failcnt = rrdset_create_localhost(
            cgroup_chart_type(buff, cg),
            "mem_failcnt",
            NULL,
            "mem",
            context,
            title,
            "count",
            PLUGIN_CGROUPS_NAME,
            is_cgroup_systemd_service(cg) ? PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME : PLUGIN_CGROUPS_MODULE_CGROUPS_NAME,
            prio,
            cgroup_update_every,
            RRDSET_TYPE_LINE);

        rrdset_update_rrdlabels(chart, cg->chart_labels);
        rrddim_add(chart, "failures", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set(chart, "failures", (collected_number)cg->memory.failcnt);
    rrdset_done(chart);
}

void update_mem_usage_chart(struct cgroup *cg) {
    RRDSET *chart = cg->st_mem_usage;

    if (unlikely(!cg->st_mem_usage)) {
        char *title;
        char *context;
        int prio;
        if (is_cgroup_systemd_service(cg)) {
            title = "Systemd Services Used Memory";
            context = "systemd.service.memory.usage";
            prio = NETDATA_CHART_PRIO_CGROUPS_SYSTEMD + 5;
        } else {
            title = "Used Memory";
            context = k8s_is_kubepod(cg) ? "k8s.cgroup.mem_usage" : "cgroup.mem_usage";
            prio = NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 210;
        }

        char buff[RRD_ID_LENGTH_MAX + 1];
        chart = cg->st_mem_usage = rrdset_create_localhost(
            cgroup_chart_type(buff, cg),
            "mem_usage",
            NULL,
            "mem",
            context,
            title,
            "MiB",
            PLUGIN_CGROUPS_NAME,
            is_cgroup_systemd_service(cg) ? PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME : PLUGIN_CGROUPS_MODULE_CGROUPS_NAME,
            prio,
            cgroup_update_every,
            RRDSET_TYPE_STACKED);

        rrdset_update_rrdlabels(chart, cg->chart_labels);

        cg->st_mem_rd_ram = rrddim_add(chart, "ram", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
        cg->st_mem_rd_swap = rrddim_add(chart, "swap", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(chart, cg->st_mem_rd_ram, (collected_number)cg->memory.usage_in_bytes);

    if (!(cg->options & CGROUP_OPTIONS_IS_UNIFIED)) {
        rrddim_set_by_pointer(
            chart,
            cg->st_mem_rd_swap,
            cg->memory.msw_usage_in_bytes > (cg->memory.usage_in_bytes + cg->memory.total_inactive_file) ?
                (collected_number)(cg->memory.msw_usage_in_bytes -
                                   (cg->memory.usage_in_bytes + cg->memory.total_inactive_file)) :
                0);
    } else {
        rrddim_set_by_pointer(chart, cg->st_mem_rd_swap, (collected_number)cg->memory.msw_usage_in_bytes);
    }

    rrdset_done(chart);
}

void update_io_serviced_bytes_chart(struct cgroup *cg) {
    RRDSET *chart = cg->st_io;

    if (unlikely(!cg->st_io)) {
        char *title;
        char *context;
        int prio;
        if (is_cgroup_systemd_service(cg)) {
            title = "Systemd Services Disk Read/Write Bandwidth";
            context = "systemd.service.disk.io";
            prio = NETDATA_CHART_PRIO_CGROUPS_SYSTEMD + 35;
        } else {
            title = "I/O Bandwidth (all disks)";
            context = k8s_is_kubepod(cg) ? "k8s.cgroup.io" : "cgroup.io";
            prio = NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 1200;
        }

        char buff[RRD_ID_LENGTH_MAX + 1];
        chart = cg->st_io = rrdset_create_localhost(
            cgroup_chart_type(buff, cg),
            "io",
            NULL,
            "disk",
            context,
            title,
            "KiB/s",
            PLUGIN_CGROUPS_NAME,
            is_cgroup_systemd_service(cg) ? PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME : PLUGIN_CGROUPS_MODULE_CGROUPS_NAME,
            prio,
            cgroup_update_every,
            RRDSET_TYPE_AREA);

        rrdset_update_rrdlabels(chart, cg->chart_labels);
        cg->st_io_rd_read = rrddim_add(chart, "read", NULL, 1, 1024, RRD_ALGORITHM_INCREMENTAL);
        cg->st_io_rd_written = rrddim_add(cg->st_io, "write", NULL, -1, 1024, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(chart, cg->st_io_rd_read, (collected_number)cg->io_service_bytes.Read);
    rrddim_set_by_pointer(chart, cg->st_io_rd_written, (collected_number)cg->io_service_bytes.Write);
    rrdset_done(chart);
}

void update_io_serviced_ops_chart(struct cgroup *cg) {
    RRDSET *chart = cg->st_serviced_ops;

    if (unlikely(!cg->st_serviced_ops)) {
        char *title;
        char *context;
        int prio;
        if (is_cgroup_systemd_service(cg)) {
            title = "Systemd Services Disk Read/Write Operations";
            context = "systemd.service.disk.iops";
            prio = NETDATA_CHART_PRIO_CGROUPS_SYSTEMD + 40;
        } else {
            title = "Serviced I/O Operations (all disks)";
            context = k8s_is_kubepod(cg) ? "k8s.cgroup.serviced_ops" : "cgroup.serviced_ops";
            prio = NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 1200;
        }

        char buff[RRD_ID_LENGTH_MAX + 1];
        chart = cg->st_serviced_ops = rrdset_create_localhost(
            cgroup_chart_type(buff, cg),
            "serviced_ops",
            NULL,
            "disk",
            context,
            title,
            "operations/s",
            PLUGIN_CGROUPS_NAME,
            is_cgroup_systemd_service(cg) ? PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME : PLUGIN_CGROUPS_MODULE_CGROUPS_NAME,
            prio,
            cgroup_update_every,
            RRDSET_TYPE_LINE);

        rrdset_update_rrdlabels(chart, cg->chart_labels);
        rrddim_add(chart, "read", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        rrddim_add(chart, "write", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set(chart, "read", (collected_number)cg->io_serviced.Read);
    rrddim_set(chart, "write", (collected_number)cg->io_serviced.Write);
    rrdset_done(chart);
}

void update_throttle_io_serviced_bytes_chart(struct cgroup *cg) {
    RRDSET *chart = cg->st_throttle_io;

    if (unlikely(!cg->st_throttle_io)) {
        char *title;
        char *context;
        int prio;
        if (is_cgroup_systemd_service(cg)) {
            title = "Systemd Services Throttle Disk Read/Write Bandwidth";
            context = "systemd.service.disk.throttle.io";
            prio = NETDATA_CHART_PRIO_CGROUPS_SYSTEMD + 45;
        } else {
            title = "Throttle I/O Bandwidth (all disks)";
            context = k8s_is_kubepod(cg) ? "k8s.cgroup.throttle_io" : "cgroup.throttle_io";
            prio = NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 1200;
        }

        char buff[RRD_ID_LENGTH_MAX + 1];
        chart = cg->st_throttle_io = rrdset_create_localhost(
            cgroup_chart_type(buff, cg),
            "throttle_io",
            NULL,
            "disk",
            context,
            title,
            "KiB/s",
            PLUGIN_CGROUPS_NAME,
            is_cgroup_systemd_service(cg) ? PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME : PLUGIN_CGROUPS_MODULE_CGROUPS_NAME,
            prio,
            cgroup_update_every,
            RRDSET_TYPE_AREA);

        rrdset_update_rrdlabels(chart, cg->chart_labels);

        cg->st_throttle_io_rd_read = rrddim_add(chart, "read", NULL, 1, 1024, RRD_ALGORITHM_INCREMENTAL);
        cg->st_throttle_io_rd_written = rrddim_add(chart, "write", NULL, -1, 1024, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(chart, cg->st_throttle_io_rd_read, (collected_number)cg->throttle_io_service_bytes.Read);
    rrddim_set_by_pointer(chart, cg->st_throttle_io_rd_written, (collected_number)cg->throttle_io_service_bytes.Write);
    rrdset_done(chart);
}

void update_throttle_io_serviced_ops_chart(struct cgroup *cg) {
    RRDSET *chart = cg->st_throttle_serviced_ops;

    if (unlikely(!cg->st_throttle_serviced_ops)) {
        char *title;
        char *context;
        int prio;
        if (is_cgroup_systemd_service(cg)) {
            title = "Systemd Services Throttle Disk Read/Write Operations";
            context = "systemd.service.disk.throttle.iops";
            prio = NETDATA_CHART_PRIO_CGROUPS_SYSTEMD + 50;
        } else {
            title = "Throttle Serviced I/O Operations (all disks)";
            context = k8s_is_kubepod(cg) ? "k8s.cgroup.throttle_serviced_ops" : "cgroup.throttle_serviced_ops";
            prio = NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 1200;
        }

        char buff[RRD_ID_LENGTH_MAX + 1];
        chart = cg->st_throttle_serviced_ops = rrdset_create_localhost(
            cgroup_chart_type(buff, cg),
            "throttle_serviced_ops",
            NULL,
            "disk",
            context,
            title,
            "operations/s",
            PLUGIN_CGROUPS_NAME,
            is_cgroup_systemd_service(cg) ? PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME : PLUGIN_CGROUPS_MODULE_CGROUPS_NAME,
            prio,
            cgroup_update_every,
            RRDSET_TYPE_LINE);

        rrdset_update_rrdlabels(chart, cg->chart_labels);
        rrddim_add(chart, "read", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        rrddim_add(chart, "write", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set(chart, "read", (collected_number)cg->throttle_io_serviced.Read);
    rrddim_set(chart, "write", (collected_number)cg->throttle_io_serviced.Write);
    rrdset_done(chart);
}

void update_io_queued_ops_chart(struct cgroup *cg) {
    RRDSET *chart = cg->st_queued_ops;

    if (unlikely(!cg->st_queued_ops)) {
        char *title;
        char *context;
        int prio;
        if (is_cgroup_systemd_service(cg)) {
            title = "Systemd Services Queued Disk Read/Write Operations";
            context = "systemd.service.disk.queued_iops";
            prio = NETDATA_CHART_PRIO_CGROUPS_SYSTEMD + 55;
        } else {
            title = "Queued I/O Operations (all disks)";
            context = k8s_is_kubepod(cg) ? "k8s.cgroup.queued_ops" : "cgroup.queued_ops";
            prio = NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 2000;
        }

        char buff[RRD_ID_LENGTH_MAX + 1];
        chart = cg->st_queued_ops = rrdset_create_localhost(
            cgroup_chart_type(buff, cg),
            "queued_ops",
            NULL,
            "disk",
            context,
            title,
            "operations",
            PLUGIN_CGROUPS_NAME,
            is_cgroup_systemd_service(cg) ? PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME : PLUGIN_CGROUPS_MODULE_CGROUPS_NAME,
            prio,
            cgroup_update_every,
            RRDSET_TYPE_LINE);

        rrdset_update_rrdlabels(chart, cg->chart_labels);
        rrddim_add(chart, "read", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        rrddim_add(chart, "write", NULL, -1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set(chart, "read", (collected_number)cg->io_queued.Read);
    rrddim_set(chart, "write", (collected_number)cg->io_queued.Write);
    rrdset_done(chart);
}

void update_io_merged_ops_chart(struct cgroup *cg) {
    RRDSET *chart = cg->st_merged_ops;

    if (unlikely(!cg->st_merged_ops)) {
        char *title;
        char *context;
        int prio;
        if (is_cgroup_systemd_service(cg)) {
            title = "Systemd Services Merged Disk Read/Write Operations";
            context = "systemd.service.disk.merged_iops";
            prio = NETDATA_CHART_PRIO_CGROUPS_SYSTEMD + 60;
        } else {
            title = "Merged I/O Operations (all disks)";
            context = k8s_is_kubepod(cg) ? "k8s.cgroup.merged_ops" : "cgroup.merged_ops";
            prio = NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 2100;
        }

        char buff[RRD_ID_LENGTH_MAX + 1];
        chart = cg->st_merged_ops = rrdset_create_localhost(
            cgroup_chart_type(buff, cg),
            "merged_ops",
            NULL,
            "disk",
            context,
            title,
            "operations/s",
            PLUGIN_CGROUPS_NAME,
            is_cgroup_systemd_service(cg) ? PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME : PLUGIN_CGROUPS_MODULE_CGROUPS_NAME,
            prio,
            cgroup_update_every,
            RRDSET_TYPE_LINE);

        rrdset_update_rrdlabels(chart, cg->chart_labels);
        rrddim_add(chart, "read", NULL, 1, 1024, RRD_ALGORITHM_INCREMENTAL);
        rrddim_add(chart, "write", NULL, -1, 1024, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set(chart, "read", (collected_number)cg->io_merged.Read);
    rrddim_set(chart, "write", (collected_number)cg->io_merged.Write);
    rrdset_done(chart);
}

void update_cpu_some_pressure_chart(struct cgroup *cg) {
    if (is_cgroup_systemd_service(cg))
        return;

    struct pressure *res = &cg->cpu_pressure;
    struct pressure_charts *pcs = &res->some;
    RRDSET *chart = pcs->share_time.st;

    if (unlikely(!pcs->share_time.st)) {
        char *title = "CPU some pressure";
        char *context = k8s_is_kubepod(cg) ? "k8s.cgroup.cpu_some_pressure" : "cgroup.cpu_some_pressure";
        int prio = NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 2200;

        char buff[RRD_ID_LENGTH_MAX + 1];
        chart = pcs->share_time.st = rrdset_create_localhost(
            cgroup_chart_type(buff, cg),
            "cpu_some_pressure",
            NULL,
            "cpu",
            context,
            title,
            "percentage",
            PLUGIN_CGROUPS_NAME,
            is_cgroup_systemd_service(cg) ? PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME : PLUGIN_CGROUPS_MODULE_CGROUPS_NAME,
            prio,
            cgroup_update_every,
            RRDSET_TYPE_LINE);

        rrdset_update_rrdlabels(chart, cg->chart_labels);
        pcs->share_time.rd10 = rrddim_add(chart, "some 10", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
        pcs->share_time.rd60 = rrddim_add(chart, "some 60", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
        pcs->share_time.rd300 = rrddim_add(chart, "some 300", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(chart, pcs->share_time.rd10, (collected_number)(pcs->share_time.value10 * 100));
    rrddim_set_by_pointer(chart, pcs->share_time.rd60, (collected_number)(pcs->share_time.value60 * 100));
    rrddim_set_by_pointer(chart, pcs->share_time.rd300, (collected_number)(pcs->share_time.value300 * 100));
    rrdset_done(chart);
}

void update_cpu_some_pressure_stall_time_chart(struct cgroup *cg) {
    if (is_cgroup_systemd_service(cg))
        return;

    struct pressure *res = &cg->cpu_pressure;
    struct pressure_charts *pcs = &res->some;
    RRDSET *chart = pcs->total_time.st;

    if (unlikely(!pcs->total_time.st)) {
        char *title = "CPU some pressure stall time";
        char *context =
            k8s_is_kubepod(cg) ? "k8s.cgroup.cpu_some_pressure_stall_time" : "cgroup.cpu_some_pressure_stall_time";
        int prio = NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 2220;

        char buff[RRD_ID_LENGTH_MAX + 1];
        chart = pcs->total_time.st = rrdset_create_localhost(
            cgroup_chart_type(buff, cg),
            "cpu_some_pressure_stall_time",
            NULL,
            "cpu",
            context,
            title,
            "ms",
            PLUGIN_CGROUPS_NAME,
            is_cgroup_systemd_service(cg) ? PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME : PLUGIN_CGROUPS_MODULE_CGROUPS_NAME,
            prio,
            cgroup_update_every,
            RRDSET_TYPE_LINE);
        rrdset_update_rrdlabels(chart, cg->chart_labels);
        pcs->total_time.rdtotal = rrddim_add(chart, "time", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(chart, pcs->total_time.rdtotal, (collected_number)(pcs->total_time.value_total));
    rrdset_done(chart);
}

void update_cpu_full_pressure_chart(struct cgroup *cg) {
    if (is_cgroup_systemd_service(cg))
        return;

    struct pressure *res = &cg->cpu_pressure;
    struct pressure_charts *pcs = &res->full;
    RRDSET *chart = pcs->share_time.st;

    if (unlikely(!pcs->share_time.st)) {
        char *title = "CPU full pressure";
        char *context = k8s_is_kubepod(cg) ? "k8s.cgroup.cpu_full_pressure" : "cgroup.cpu_full_pressure";
        int prio = NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 2240;

        char buff[RRD_ID_LENGTH_MAX + 1];
        chart = pcs->share_time.st = rrdset_create_localhost(
            cgroup_chart_type(buff, cg),
            "cpu_full_pressure",
            NULL,
            "cpu",
            context,
            title,
            "percentage",
            PLUGIN_CGROUPS_NAME,
            is_cgroup_systemd_service(cg) ? PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME : PLUGIN_CGROUPS_MODULE_CGROUPS_NAME,
            prio,
            cgroup_update_every,
            RRDSET_TYPE_LINE);

        rrdset_update_rrdlabels(chart, cg->chart_labels);
        pcs->share_time.rd10 = rrddim_add(chart, "full 10", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
        pcs->share_time.rd60 = rrddim_add(chart, "full 60", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
        pcs->share_time.rd300 = rrddim_add(chart, "full 300", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(chart, pcs->share_time.rd10, (collected_number)(pcs->share_time.value10 * 100));
    rrddim_set_by_pointer(chart, pcs->share_time.rd60, (collected_number)(pcs->share_time.value60 * 100));
    rrddim_set_by_pointer(chart, pcs->share_time.rd300, (collected_number)(pcs->share_time.value300 * 100));
    rrdset_done(chart);
}

void update_cpu_full_pressure_stall_time_chart(struct cgroup *cg) {
    if (is_cgroup_systemd_service(cg))
        return;

    struct pressure *res = &cg->cpu_pressure;
    struct pressure_charts *pcs = &res->full;
    RRDSET *chart = pcs->total_time.st;

    if (unlikely(!pcs->total_time.st)) {
        char *title = "CPU full pressure stall time";
        char *context =
            k8s_is_kubepod(cg) ? "k8s.cgroup.cpu_full_pressure_stall_time" : "cgroup.cpu_full_pressure_stall_time";
        int prio = NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 2260;

        char buff[RRD_ID_LENGTH_MAX + 1];
        chart = pcs->total_time.st = rrdset_create_localhost(
            cgroup_chart_type(buff, cg),
            "cpu_full_pressure_stall_time",
            NULL,
            "cpu",
            context,
            title,
            "ms",
            PLUGIN_CGROUPS_NAME,
            is_cgroup_systemd_service(cg) ? PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME : PLUGIN_CGROUPS_MODULE_CGROUPS_NAME,
            prio,
            cgroup_update_every,
            RRDSET_TYPE_LINE);

        rrdset_update_rrdlabels(chart, cg->chart_labels);
        pcs->total_time.rdtotal = rrddim_add(chart, "time", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(chart, pcs->total_time.rdtotal, (collected_number)(pcs->total_time.value_total));
    rrdset_done(chart);
}

void update_mem_some_pressure_chart(struct cgroup *cg) {
    if (is_cgroup_systemd_service(cg))
        return;

    struct pressure *res = &cg->memory_pressure;
    struct pressure_charts *pcs = &res->some;
    RRDSET *chart = pcs->share_time.st;

    if (unlikely(!pcs->share_time.st)) {
        char *title = "Memory some pressure";
        char *context = k8s_is_kubepod(cg) ? "k8s.cgroup.memory_some_pressure" : "cgroup.memory_some_pressure";
        int prio = NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 2300;

        char buff[RRD_ID_LENGTH_MAX + 1];
        chart = pcs->share_time.st = rrdset_create_localhost(
            cgroup_chart_type(buff, cg),
            "mem_some_pressure",
            NULL,
            "mem",
            context,
            title,
            "percentage",
            PLUGIN_CGROUPS_NAME,
            is_cgroup_systemd_service(cg) ? PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME : PLUGIN_CGROUPS_MODULE_CGROUPS_NAME,
            prio,
            cgroup_update_every,
            RRDSET_TYPE_LINE);

        rrdset_update_rrdlabels(chart, cg->chart_labels);
        pcs->share_time.rd10 = rrddim_add(chart, "some 10", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
        pcs->share_time.rd60 = rrddim_add(chart, "some 60", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
        pcs->share_time.rd300 = rrddim_add(chart, "some 300", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(chart, pcs->share_time.rd10, (collected_number)(pcs->share_time.value10 * 100));
    rrddim_set_by_pointer(chart, pcs->share_time.rd60, (collected_number)(pcs->share_time.value60 * 100));
    rrddim_set_by_pointer(chart, pcs->share_time.rd300, (collected_number)(pcs->share_time.value300 * 100));
    rrdset_done(chart);
}

void update_mem_some_pressure_stall_time_chart(struct cgroup *cg) {
    if (is_cgroup_systemd_service(cg))
        return;

    struct pressure *res = &cg->memory_pressure;
    struct pressure_charts *pcs = &res->some;
    RRDSET *chart = pcs->total_time.st;

    if (unlikely(!pcs->total_time.st)) {
        char *title = "Memory some pressure stall time";
        char *context = k8s_is_kubepod(cg) ? "k8s.cgroup.memory_some_pressure_stall_time" :
                                             "cgroup.memory_some_pressure_stall_time";
        int prio = NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 2320;

        char buff[RRD_ID_LENGTH_MAX + 1];
        chart = pcs->total_time.st = rrdset_create_localhost(
            cgroup_chart_type(buff, cg),
            "memory_some_pressure_stall_time",
            NULL,
            "mem",
            context,
            title,
            "ms",
            PLUGIN_CGROUPS_NAME,
            is_cgroup_systemd_service(cg) ? PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME : PLUGIN_CGROUPS_MODULE_CGROUPS_NAME,
            prio,
            cgroup_update_every,
            RRDSET_TYPE_LINE);

        rrdset_update_rrdlabels(chart, cg->chart_labels);
        pcs->total_time.rdtotal = rrddim_add(chart, "time", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(chart, pcs->total_time.rdtotal, (collected_number)(pcs->total_time.value_total));
    rrdset_done(chart);
}

void update_mem_full_pressure_chart(struct cgroup *cg) {
    if (is_cgroup_systemd_service(cg))
        return;

    struct pressure *res = &cg->memory_pressure;
    struct pressure_charts *pcs = &res->full;
    RRDSET *chart = pcs->share_time.st;

    if (unlikely(!pcs->share_time.st)) {
        char *title = "Memory full pressure";
        char *context = k8s_is_kubepod(cg) ? "k8s.cgroup.memory_full_pressure" : "cgroup.memory_full_pressure";
        int prio = NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 2340;

        char buff[RRD_ID_LENGTH_MAX + 1];
        chart = pcs->share_time.st = rrdset_create_localhost(
            cgroup_chart_type(buff, cg),
            "mem_full_pressure",
            NULL,
            "mem",
            context,
            title,
            "percentage",
            PLUGIN_CGROUPS_NAME,
            is_cgroup_systemd_service(cg) ? PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME : PLUGIN_CGROUPS_MODULE_CGROUPS_NAME,
            prio,
            cgroup_update_every,
            RRDSET_TYPE_LINE);

        rrdset_update_rrdlabels(chart, cg->chart_labels);
        pcs->share_time.rd10 = rrddim_add(chart, "full 10", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
        pcs->share_time.rd60 = rrddim_add(chart, "full 60", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
        pcs->share_time.rd300 = rrddim_add(chart, "full 300", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(chart, pcs->share_time.rd10, (collected_number)(pcs->share_time.value10 * 100));
    rrddim_set_by_pointer(chart, pcs->share_time.rd60, (collected_number)(pcs->share_time.value60 * 100));
    rrddim_set_by_pointer(chart, pcs->share_time.rd300, (collected_number)(pcs->share_time.value300 * 100));
    rrdset_done(chart);
}

void update_mem_full_pressure_stall_time_chart(struct cgroup *cg) {
    if (is_cgroup_systemd_service(cg))
        return;

    struct pressure *res = &cg->memory_pressure;
    struct pressure_charts *pcs = &res->full;
    RRDSET *chart = pcs->total_time.st;

    if (unlikely(!pcs->total_time.st)) {
        char *title = "Memory full pressure stall time";
        char *context = k8s_is_kubepod(cg) ? "k8s.cgroup.memory_full_pressure_stall_time" :
                                             "cgroup.memory_full_pressure_stall_time";
        int prio = NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 2360;

        char buff[RRD_ID_LENGTH_MAX + 1];
        chart = pcs->total_time.st = rrdset_create_localhost(
            cgroup_chart_type(buff, cg),
            "memory_full_pressure_stall_time",
            NULL,
            "mem",
            context,
            title,
            "ms",
            PLUGIN_CGROUPS_NAME,
            is_cgroup_systemd_service(cg) ? PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME : PLUGIN_CGROUPS_MODULE_CGROUPS_NAME,
            prio,
            cgroup_update_every,
            RRDSET_TYPE_LINE);
        rrdset_update_rrdlabels(chart, cg->chart_labels);
        pcs->total_time.rdtotal = rrddim_add(chart, "time", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(chart, pcs->total_time.rdtotal, (collected_number)(pcs->total_time.value_total));
    rrdset_done(chart);
}

void update_irq_some_pressure_chart(struct cgroup *cg) {
    if (is_cgroup_systemd_service(cg))
        return;

    struct pressure *res = &cg->irq_pressure;
    struct pressure_charts *pcs = &res->some;
    RRDSET *chart = pcs->share_time.st;

    if (unlikely(!pcs->share_time.st)) {
        char *title = "IRQ some pressure";
        char *context = k8s_is_kubepod(cg) ? "k8s.cgroup.irq_some_pressure" : "cgroup.irq_some_pressure";
        int prio = NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 2310;

        char buff[RRD_ID_LENGTH_MAX + 1];
        chart = pcs->share_time.st = rrdset_create_localhost(
            cgroup_chart_type(buff, cg),
            "irq_some_pressure",
            NULL,
            "interrupts",
            context,
            title,
            "percentage",
            PLUGIN_CGROUPS_NAME,
            is_cgroup_systemd_service(cg) ? PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME : PLUGIN_CGROUPS_MODULE_CGROUPS_NAME,
            prio,
            cgroup_update_every,
            RRDSET_TYPE_LINE);

        rrdset_update_rrdlabels(chart, cg->chart_labels);
        pcs->share_time.rd10 = rrddim_add(chart, "some 10", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
        pcs->share_time.rd60 = rrddim_add(chart, "some 60", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
        pcs->share_time.rd300 = rrddim_add(chart, "some 300", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(chart, pcs->share_time.rd10, (collected_number)(pcs->share_time.value10 * 100));
    rrddim_set_by_pointer(chart, pcs->share_time.rd60, (collected_number)(pcs->share_time.value60 * 100));
    rrddim_set_by_pointer(chart, pcs->share_time.rd300, (collected_number)(pcs->share_time.value300 * 100));
    rrdset_done(chart);
}

void update_irq_some_pressure_stall_time_chart(struct cgroup *cg) {
    if (is_cgroup_systemd_service(cg))
        return;

    struct pressure *res = &cg->irq_pressure;
    struct pressure_charts *pcs = &res->some;
    RRDSET *chart = pcs->total_time.st;

    if (unlikely(!pcs->total_time.st)) {
        char *title = "IRQ some pressure stall time";
        char *context =
            k8s_is_kubepod(cg) ? "k8s.cgroup.irq_some_pressure_stall_time" : "cgroup.irq_some_pressure_stall_time";
        int prio = NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 2330;

        char buff[RRD_ID_LENGTH_MAX + 1];
        chart = pcs->total_time.st = rrdset_create_localhost(
            cgroup_chart_type(buff, cg),
            "irq_some_pressure_stall_time",
            NULL,
            "interrupts",
            context,
            title,
            "ms",
            PLUGIN_CGROUPS_NAME,
            is_cgroup_systemd_service(cg) ? PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME : PLUGIN_CGROUPS_MODULE_CGROUPS_NAME,
            prio,
            cgroup_update_every,
            RRDSET_TYPE_LINE);

        rrdset_update_rrdlabels(chart, cg->chart_labels);
        pcs->total_time.rdtotal = rrddim_add(chart, "time", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(chart, pcs->total_time.rdtotal, (collected_number)(pcs->total_time.value_total));
    rrdset_done(chart);
}

void update_irq_full_pressure_chart(struct cgroup *cg) {
    if (is_cgroup_systemd_service(cg))
        return;

    struct pressure *res = &cg->irq_pressure;
    struct pressure_charts *pcs = &res->full;
    RRDSET *chart = pcs->share_time.st;

    if (unlikely(!pcs->share_time.st)) {
        char *title = "IRQ full pressure";
        char *context = k8s_is_kubepod(cg) ? "k8s.cgroup.irq_full_pressure" : "cgroup.irq_full_pressure";
        int prio = NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 2350;

        char buff[RRD_ID_LENGTH_MAX + 1];
        chart = pcs->share_time.st = rrdset_create_localhost(
            cgroup_chart_type(buff, cg),
            "irq_full_pressure",
            NULL,
            "interrupts",
            context,
            title,
            "percentage",
            PLUGIN_CGROUPS_NAME,
            is_cgroup_systemd_service(cg) ? PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME : PLUGIN_CGROUPS_MODULE_CGROUPS_NAME,
            prio,
            cgroup_update_every,
            RRDSET_TYPE_LINE);

        rrdset_update_rrdlabels(chart, cg->chart_labels);
        pcs->share_time.rd10 = rrddim_add(chart, "full 10", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
        pcs->share_time.rd60 = rrddim_add(chart, "full 60", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
        pcs->share_time.rd300 = rrddim_add(chart, "full 300", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(chart, pcs->share_time.rd10, (collected_number)(pcs->share_time.value10 * 100));
    rrddim_set_by_pointer(chart, pcs->share_time.rd60, (collected_number)(pcs->share_time.value60 * 100));
    rrddim_set_by_pointer(chart, pcs->share_time.rd300, (collected_number)(pcs->share_time.value300 * 100));
    rrdset_done(chart);
}

void update_irq_full_pressure_stall_time_chart(struct cgroup *cg) {
    if (is_cgroup_systemd_service(cg))
        return;

    struct pressure *res = &cg->irq_pressure;
    struct pressure_charts *pcs = &res->full;
    RRDSET *chart = pcs->total_time.st;

    if (unlikely(!pcs->total_time.st)) {
        char *title = "IRQ full pressure stall time";
        char *context =
            k8s_is_kubepod(cg) ? "k8s.cgroup.irq_full_pressure_stall_time" : "cgroup.irq_full_pressure_stall_time";
        int prio = NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 2370;

        char buff[RRD_ID_LENGTH_MAX + 1];
        chart = pcs->total_time.st = rrdset_create_localhost(
            cgroup_chart_type(buff, cg),
            "irq_full_pressure_stall_time",
            NULL,
            "interrupts",
            context,
            title,
            "ms",
            PLUGIN_CGROUPS_NAME,
            is_cgroup_systemd_service(cg) ? PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME : PLUGIN_CGROUPS_MODULE_CGROUPS_NAME,
            prio,
            cgroup_update_every,
            RRDSET_TYPE_LINE);

        rrdset_update_rrdlabels(chart, cg->chart_labels);
        pcs->total_time.rdtotal = rrddim_add(chart, "time", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(chart, pcs->total_time.rdtotal, (collected_number)(pcs->total_time.value_total));
    rrdset_done(chart);
}

void update_io_some_pressure_chart(struct cgroup *cg) {
    if (is_cgroup_systemd_service(cg))
        return;

    struct pressure *res = &cg->io_pressure;
    struct pressure_charts *pcs = &res->some;
    RRDSET *chart = pcs->share_time.st;

    if (unlikely(!pcs->share_time.st)) {
        char *title = "I/O some pressure";
        char *context = k8s_is_kubepod(cg) ? "k8s.cgroup.io_some_pressure" : "cgroup.io_some_pressure";
        int prio = NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 2400;

        char buff[RRD_ID_LENGTH_MAX + 1];
        chart = pcs->share_time.st = rrdset_create_localhost(
            cgroup_chart_type(buff, cg),
            "io_some_pressure",
            NULL,
            "disk",
            context,
            title,
            "percentage",
            PLUGIN_CGROUPS_NAME,
            is_cgroup_systemd_service(cg) ? PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME : PLUGIN_CGROUPS_MODULE_CGROUPS_NAME,
            prio,
            cgroup_update_every,
            RRDSET_TYPE_LINE);

        rrdset_update_rrdlabels(chart, cg->chart_labels);
        pcs->share_time.rd10 = rrddim_add(chart, "some 10", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
        pcs->share_time.rd60 = rrddim_add(chart, "some 60", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
        pcs->share_time.rd300 = rrddim_add(chart, "some 300", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(chart, pcs->share_time.rd10, (collected_number)(pcs->share_time.value10 * 100));
    rrddim_set_by_pointer(chart, pcs->share_time.rd60, (collected_number)(pcs->share_time.value60 * 100));
    rrddim_set_by_pointer(chart, pcs->share_time.rd300, (collected_number)(pcs->share_time.value300 * 100));
    rrdset_done(chart);
}

void update_io_some_pressure_stall_time_chart(struct cgroup *cg) {
    if (is_cgroup_systemd_service(cg))
        return;

    struct pressure *res = &cg->io_pressure;
    struct pressure_charts *pcs = &res->some;
    RRDSET *chart = pcs->total_time.st;

    if (unlikely(!pcs->total_time.st)) {
        char *title = "I/O some pressure stall time";
        char *context =
            k8s_is_kubepod(cg) ? "k8s.cgroup.io_some_pressure_stall_time" : "cgroup.io_some_pressure_stall_time";
        int prio = NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 2420;

        char buff[RRD_ID_LENGTH_MAX + 1];
        chart = pcs->total_time.st = rrdset_create_localhost(
            cgroup_chart_type(buff, cg),
            "io_some_pressure_stall_time",
            NULL,
            "disk",
            context,
            title,
            "ms",
            PLUGIN_CGROUPS_NAME,
            is_cgroup_systemd_service(cg) ? PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME : PLUGIN_CGROUPS_MODULE_CGROUPS_NAME,
            prio,
            cgroup_update_every,
            RRDSET_TYPE_LINE);
        rrdset_update_rrdlabels(chart, cg->chart_labels);
        pcs->total_time.rdtotal = rrddim_add(chart, "time", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(chart, pcs->total_time.rdtotal, (collected_number)(pcs->total_time.value_total));
    rrdset_done(chart);
}

void update_io_full_pressure_chart(struct cgroup *cg) {
    if (is_cgroup_systemd_service(cg))
        return;

    struct pressure *res = &cg->io_pressure;
    struct pressure_charts *pcs = &res->full;
    RRDSET *chart = pcs->share_time.st;

    if (unlikely(!pcs->share_time.st)) {
        char *title = "I/O full pressure";
        char *context = k8s_is_kubepod(cg) ? "k8s.cgroup.io_full_pressure" : "cgroup.io_full_pressure";
        int prio = NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 2440;

        char buff[RRD_ID_LENGTH_MAX + 1];
        chart = pcs->share_time.st = rrdset_create_localhost(
            cgroup_chart_type(buff, cg),
            "io_full_pressure",
            NULL,
            "disk",
            context,
            title,
            "percentage",
            PLUGIN_CGROUPS_NAME,
            is_cgroup_systemd_service(cg) ? PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME : PLUGIN_CGROUPS_MODULE_CGROUPS_NAME,
            prio,
            cgroup_update_every,
            RRDSET_TYPE_LINE);

        rrdset_update_rrdlabels(chart, cg->chart_labels);
        pcs->share_time.rd10 = rrddim_add(chart, "full 10", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
        pcs->share_time.rd60 = rrddim_add(chart, "full 60", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
        pcs->share_time.rd300 = rrddim_add(chart, "full 300", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(chart, pcs->share_time.rd10, (collected_number)(pcs->share_time.value10 * 100));
    rrddim_set_by_pointer(chart, pcs->share_time.rd60, (collected_number)(pcs->share_time.value60 * 100));
    rrddim_set_by_pointer(chart, pcs->share_time.rd300, (collected_number)(pcs->share_time.value300 * 100));
    rrdset_done(chart);
}

void update_io_full_pressure_stall_time_chart(struct cgroup *cg) {
    if (is_cgroup_systemd_service(cg))
        return;

    struct pressure *res = &cg->io_pressure;
    struct pressure_charts *pcs = &res->full;
    RRDSET *chart = pcs->total_time.st;

    if (unlikely(!pcs->total_time.st)) {
        char *title = "I/O full pressure stall time";
        char *context =
            k8s_is_kubepod(cg) ? "k8s.cgroup.io_full_pressure_stall_time" : "cgroup.io_full_pressure_stall_time";
        int prio = NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 2460;

        char buff[RRD_ID_LENGTH_MAX + 1];
        chart = pcs->total_time.st = rrdset_create_localhost(
            cgroup_chart_type(buff, cg),
            "io_full_pressure_stall_time",
            NULL,
            "disk",
            context,
            title,
            "ms",
            PLUGIN_CGROUPS_NAME,
            is_cgroup_systemd_service(cg) ? PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME : PLUGIN_CGROUPS_MODULE_CGROUPS_NAME,
            prio,
            cgroup_update_every,
            RRDSET_TYPE_LINE);

        rrdset_update_rrdlabels(chart, cg->chart_labels);
        pcs->total_time.rdtotal = rrddim_add(chart, "time", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(chart, pcs->total_time.rdtotal, (collected_number)(pcs->total_time.value_total));
    rrdset_done(chart);
}

void update_pids_current_chart(struct cgroup *cg) {
    RRDSET *chart = cg->st_pids;

    if (unlikely(!cg->st_pids)) {
        char *title;
        char *context;
        int prio;
        if (is_cgroup_systemd_service(cg)) {
            title = "Systemd Services Number of Processes";
            context = "systemd.service.pids.current";
            prio = NETDATA_CHART_PRIO_CGROUPS_SYSTEMD + 70;
        } else {
            title = "Number of processes";
            context = k8s_is_kubepod(cg) ? "k8s.cgroup.pids_current" : "cgroup.pids_current";
            prio = NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 2150;
        }

        char buff[RRD_ID_LENGTH_MAX + 1];
        chart = cg->st_pids = rrdset_create_localhost(
            cgroup_chart_type(buff, cg),
            "pids_current",
            NULL,
            "pids",
            context,
            title,
            "pids",
            PLUGIN_CGROUPS_NAME,
            is_cgroup_systemd_service(cg) ? PLUGIN_CGROUPS_MODULE_SYSTEMD_NAME : PLUGIN_CGROUPS_MODULE_CGROUPS_NAME,
            prio,
            cgroup_update_every,
            RRDSET_TYPE_LINE);

        rrdset_update_rrdlabels(chart, cg->chart_labels);
        cg->st_pids_rd_pids_current = rrddim_add(chart, "pids", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(chart, cg->st_pids_rd_pids_current, (collected_number)cg->pids_current.pids_current);
    rrdset_done(chart);
}
