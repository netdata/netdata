// SPDX-License-Identifier: GPL-3.0-or-later

// Heavily inspired from proc_net_dev.c
#include "plugin_proc.h"

#define PLUGIN_PROC_MODULE_INFINIBAND_NAME "/sys/class/infiniband"
#define CONFIG_SECTION_PLUGIN_SYS_CLASS_INFINIBAND "plugin:" PLUGIN_PROC_CONFIG_NAME ":" PLUGIN_PROC_MODULE_INFINIBAND_NAME

// ib_device::name[IB_DEVICE_NAME_MAX(64)] + "-" + ib_device::phys_port_cnt[u8 = 3 chars]
#define IBNAME_MAX 68

// ----------------------------------------------------------------------------
// infiniband & omnipath list

// I use macro as there's no single file acting as summary, but a lot of different files, so can't use helpers like
// procfile(). Also, omnipath generates other counters, that are not provided by infiniband
#define FOREACH_COUNTER(GEN, ...) \
	FOREACH_COUNTER_BYTES(GEN,   __VA_ARGS__) \
	FOREACH_COUNTER_PACKETS(GEN, __VA_ARGS__) \
	FOREACH_COUNTER_ERRORS(GEN,  __VA_ARGS__)

#define FOREACH_COUNTER_BYTES(GEN, ...) \
	GEN(port_rcv_data,                   bytes,  "Received", 1, __VA_ARGS__) \
	GEN(port_xmit_data,                  bytes,  "Sent",    -1, __VA_ARGS__)

#define FOREACH_COUNTER_PACKETS(GEN, ...) \
	GEN(port_rcv_packets,                packets, "Received",    1, __VA_ARGS__) \
	GEN(port_xmit_packets,               packets, "Sent",       -1, __VA_ARGS__) \
	GEN(multicast_rcv_packets,           packets, "Mcast rcvd",  1, __VA_ARGS__) \
	GEN(multicast_xmit_packets,          packets, "Mcast sent", -1, __VA_ARGS__) \
	GEN(unicast_rcv_packets,             packets, "Ucast rcvd",  1, __VA_ARGS__) \
	GEN(unicast_xmit_packets,            packets, "Ucast sent", -1, __VA_ARGS__) \

#define FOREACH_COUNTER_ERRORS(GEN, ...) \
	GEN(port_rcv_errors,                 errors, "Pkts malformated",      1, __VA_ARGS__) \
	GEN(port_rcv_constraint_errors,      errors, "Pkts rcvd discarded ",  1, __VA_ARGS__) \
	GEN(port_xmit_discards,              errors, "Pkts sent discarded",   1, __VA_ARGS__) \
	GEN(port_xmit_wait,                  errors, "Tick Wait to send",     1, __VA_ARGS__) \
	GEN(VL15_dropped,                    errors, "Pkts missed ressource", 1, __VA_ARGS__) \
	GEN(excessive_buffer_overrun_errors, errors, "Buffer overrun",        1, __VA_ARGS__) \
	GEN(link_downed,                     errors, "Link Downed",           1, __VA_ARGS__) \
	GEN(link_error_recovery,             errors, "Link recovered",        1, __VA_ARGS__) \
	GEN(local_link_integrity_errors,     errors, "Link integrity err",    1, __VA_ARGS__) \
	GEN(symbol_error,                    errors, "Link minor errors",     1, __VA_ARGS__) \
	GEN(port_rcv_remote_physical_errors, errors, "Pkts rcvd with EBP",    1, __VA_ARGS__) \
	GEN(port_rcv_switch_relay_errors,    errors, "Pkts rcvd discarded by switch",  1, __VA_ARGS__) \
	GEN(port_xmit_constraint_errors,     errors, "Pkts sent discarded by switch",  1, __VA_ARGS__)

// Common definitions used more than once
#define GEN_RRD_DIM_ADD(NAME,GRP,DESC,DIR, PORT, ...) \
	PORT->rd_##NAME = rrddim_add(PORT->st_##GRP, DESC, NULL, DIR, 1, RRD_ALGORITHM_INCREMENTAL);

#define GEN_RRD_DIM_SETP(NAME,GRP,DESC,DIR, PORT, ...) \
	rrddim_set_by_pointer(PORT->st_##GRP, PORT->rd_##NAME, (collected_number)PORT->NAME);


// https://community.mellanox.com/s/article/understanding-mlx5-linux-counters-and-status-parameters
// https://community.mellanox.com/s/article/infiniband-port-counters
static struct ibport {
	char *name;
	char *counters_path;
	int  len;

	// flags
	int configured;
	int enabled;
	int updated;

	int do_bytes;
	int do_packets;
	int do_errors;

	const char *chart_type_bytes;
	const char *chart_type_packets;
	const char *chart_type_errors;

	const char *chart_id_bytes;
	const char *chart_id_packets;
	const char *chart_id_errors;

	const char *chart_family;

	unsigned long priority;

	// Stats from /$verb/ports/$portid/counters
	// as drivers/infiniband/hw/qib/qib_verbs.h
	// All uint64 except vl15_dropped, local_link_integrity_errors, excessive_buffer_overrun_errors uint32
	// Will generate 2 elements for each counter:
	// - uint64_t to store the value
	// - char*    to store the filename path
	#define GEN_DEF_COUNTER_VALUE(NAME, ...) uint64_t  NAME;
	FOREACH_COUNTER(GEN_DEF_COUNTER_VALUE)

	#define GEN_DEF_COUNTER_FILE(NAME, ...)  char     *file_##NAME;
	FOREACH_COUNTER(GEN_DEF_COUNTER_FILE)



	// charts and dim
	RRDSET *st_bytes;
	RRDSET *st_packets;
	RRDSET *st_errors;

	#define GEN_DEF_RRD_DIM(NAME, ...)       RRDDIM   *rd_##NAME;
	FOREACH_COUNTER(GEN_DEF_RRD_DIM)

	usec_t speed_last_collected_usec;

	struct ibport *next;
} *ibport_root = NULL, *ibport_last_used = NULL;


// ----------------------------------------------------------------------------


static struct ibport *get_ibport(const char *verb, const char *port) {
	struct ibport *p;

	char name[IBNAME_MAX+1];
	snprintfz(name, IBNAME_MAX, "%s-%s", verb, port);


	// search it, resuming from the last position in sequence
	for(p = ibport_last_used ; p ; p = p->next) {
		if(unlikely(!strcmp(name, p->name))) {
			ibport_last_used = p->next;
			return p;
		}
	}

	// new round, from the beginning to the last position used this time
	for(p = ibport_root ; p != ibport_last_used ; p = p->next) {
		if(unlikely(!strcmp(name, p->name))) {
			ibport_last_used = p->next;
			return p;
		}
	}

	// create a new one
	p       = callocz(1, sizeof(struct ibport));
	p->name = strdupz(name);
	p->len  = strlen(p->name);


	p->chart_type_bytes    = strdupz("Infiniband");
	p->chart_type_packets  = strdupz("Infiniband");
	p->chart_type_errors   = strdupz("Infiniband");

	char buffer[RRD_ID_LENGTH_MAX + 1];
	snprintfz(buffer, RRD_ID_LENGTH_MAX, "ib_bytes_%s", p->name);
	p->chart_id_bytes   = strdupz(buffer);
	snprintfz(buffer, RRD_ID_LENGTH_MAX, "ib_packets_%s", p->name);
	p->chart_id_packets = strdupz(buffer);
	snprintfz(buffer, RRD_ID_LENGTH_MAX, "ib_errors_%s", p->name);
	p->chart_id_errors  = strdupz(buffer);

	p->chart_family        = strdupz(p->name);
	p->priority            = NETDATA_CHART_PRIO_INFINIBAND;

	// Link current ibport to last one in the list
	if (ibport_root) {
		struct ibport *t;
		for (t = ibport_root; t->next ; t = t->next) ;
		t->next = p;
	}
	else
		ibport_root = p;

	return p;

}


int do_sys_class_infiniband(int update_every, usec_t dt) {
	(void)dt;
	static SIMPLE_PATTERN *disabled_list = NULL;
	static int initialized = 0;
	static int enable_new_ports = -1;
	static int do_bytes = -1, do_packets = -1, do_errors = -1;
	static char *sys_class_infiniband_dirname = NULL;

	static long long int dt_to_refresh_speed = 0;
	
	if(unlikely(enable_new_ports == -1)) {
		char dirname[FILENAME_MAX + 1];

		snprintfz(dirname, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/class/infiniband");
		sys_class_infiniband_dirname = config_get(CONFIG_SECTION_PLUGIN_SYS_CLASS_INFINIBAND, "dirname to monitor", dirname);

		do_bytes   = config_get_boolean_ondemand(CONFIG_SECTION_PLUGIN_SYS_CLASS_INFINIBAND, "bandwidth for all infiniband ports", CONFIG_BOOLEAN_AUTO);
		do_packets = config_get_boolean_ondemand(CONFIG_SECTION_PLUGIN_SYS_CLASS_INFINIBAND, "packets for all infiniband ports", CONFIG_BOOLEAN_AUTO);
		do_errors  = config_get_boolean_ondemand(CONFIG_SECTION_PLUGIN_SYS_CLASS_INFINIBAND, "errors for all infiniband ports", CONFIG_BOOLEAN_AUTO);

		disabled_list = simple_pattern_create(config_get(CONFIG_SECTION_PLUGIN_SYS_CLASS_INFINIBAND, "disable by default interfaces matching", ""), NULL, SIMPLE_PATTERN_EXACT);

		dt_to_refresh_speed = config_get_number(CONFIG_SECTION_PLUGIN_SYS_CLASS_INFINIBAND, "refresh interface speed every seconds", 10) * USEC_PER_SEC;
		if(dt_to_refresh_speed < 0) dt_to_refresh_speed = 0;

		enable_new_ports = config_get_boolean_ondemand(CONFIG_SECTION_PLUGIN_SYS_CLASS_INFINIBAND, "errors for all infiniband ports", CONFIG_BOOLEAN_AUTO);
	}


	// init listing of /sys/class/infiniband/
	if(unlikely(!initialized)) {

		// If folder does not exists, return 1 to disable
		DIR *verbs_dir = opendir(sys_class_infiniband_dirname);
		if(unlikely(!verbs_dir)) return 1;

		// Work on all verbs (card) available
		struct dirent *verb_dent;
		while ( (verb_dent = readdir(verbs_dir)) ) {

			if (!strcmp(verb_dent->d_name, "..") || !strcmp(verb_dent->d_name, "."))
				continue;

			// /sys/class/infiniband/<verb>/ports
			char ports_dirname[FILENAME_MAX +1];
			snprintfz(ports_dirname, FILENAME_MAX, "%s/%s/%s", sys_class_infiniband_dirname, verb_dent->d_name, "ports");

			DIR *ports_dir = opendir(ports_dirname);
			if(unlikely(!ports_dir)) continue;

			struct dirent *port_dent;
			while ( (port_dent = readdir(ports_dir)) ) {

				if (!strcmp(port_dent->d_name, "..") || !strcmp(port_dent->d_name, "."))
					continue;

				// Check if counters are available
				// /sys/class/infiniband/<verb>/ports/<port>/counters
				char counters_dirname[FILENAME_MAX +1];
				snprintfz(counters_dirname, FILENAME_MAX, "%s/%s/%s", ports_dirname, port_dent->d_name, "counters");
				DIR *counters_dir = opendir(counters_dirname);
				if (!counters_dir) continue;

				// Get new ibport
				struct ibport *p = get_ibport(verb_dent->d_name, port_dent->d_name);
				if(!p) continue;

				p->updated = 1;

				// Prepare configuration
				if (!p->configured) {
					p->configured = 1;

					p->counters_path = strdupz(counters_dirname);

					p->enabled = enable_new_ports;

					if (p->enabled)
						p->enabled = !simple_pattern_matches(disabled_list, p->name);

					char buffer[FILENAME_MAX + 1];
					snprintfz(buffer, FILENAME_MAX, "plugin:proc:/sys/class/infiniband:%s", p->name);
					p->do_bytes   = config_get_boolean_ondemand(buffer, "bytes",   do_bytes);
					p->do_packets = config_get_boolean_ondemand(buffer, "packets", do_packets);
					p->do_errors  = config_get_boolean_ondemand(buffer, "errors",  do_errors);


					// Gen filename allocation and concatenation
					#define GEN_DO_COUNTER_NAME(NAME,GRP,DESC,DIR,PORT, ...) \
						PORT->file_##NAME = callocz(1, strlen(PORT->counters_path) + sizeof(#NAME) +3); \
						strcat(PORT->file_##NAME, PORT->counters_path); \
						strcat(PORT->file_##NAME, "/"#NAME);
					FOREACH_COUNTER(GEN_DO_COUNTER_NAME, p)

					// Dispatch the reads

				}
				
			}
			closedir(ports_dir);
		}
		closedir(verbs_dir);

		initialized = 1;
	}

	// Update all ports values
	struct ibport *port;
	for (port = ibport_root; port ; port = port->next) {


		// Read each counter
		#define GEN_DO_COUNTER_READ(NAME,GRP,DESC,DIR,PORT, ...) \
			if (PORT->do_##GRP != CONFIG_BOOLEAN_NO && PORT->file_##NAME) { \
				if (read_single_number_file(PORT->file_##NAME, (unsigned long long *) &PORT->NAME)) { \
					error("cannot read iface '%s' counter '"#NAME"'", PORT->name); \
					PORT->file_##NAME = NULL; \
				} \
			}
		FOREACH_COUNTER(GEN_DO_COUNTER_READ, port)


		// Update charts
		if (port->do_bytes != CONFIG_BOOLEAN_NO) {
			// First creation of RRD Set (charts)
			if(unlikely(!port->st_bytes)) {
				port->st_bytes = rrdset_create_localhost(
					port->chart_type_bytes
					, port->chart_id_bytes
					, NULL
					, port->chart_family
					, "ib.bytes"
					, "Bytes"
					, "Bytes/s"
					, PLUGIN_PROC_NAME
					, PLUGIN_PROC_MODULE_INFINIBAND_NAME
					, port->priority + 1
					, update_every
					, RRDSET_TYPE_AREA
				);

				rrdset_flag_set(port->st_bytes, RRDSET_FLAG_DETAIL);
				FOREACH_COUNTER_BYTES(GEN_RRD_DIM_ADD, port)

			}
			else
				rrdset_next(port->st_bytes);

			FOREACH_COUNTER_BYTES(GEN_RRD_DIM_SETP, port)
			rrdset_done(port->st_bytes);
		}

		if (port->do_packets != CONFIG_BOOLEAN_NO) {
			// First creation of RRD Set (charts)
			if(unlikely(!port->st_packets)) {
				port->st_packets = rrdset_create_localhost(
					port->chart_type_packets
					, port->chart_id_packets
					, NULL
					, port->chart_family
					, "ib.packets"
					, "Packets"
					, "Packets/s"
					, PLUGIN_PROC_NAME
					, PLUGIN_PROC_MODULE_INFINIBAND_NAME
					, port->priority + 1
					, update_every
					, RRDSET_TYPE_AREA
				);

				rrdset_flag_set(port->st_packets, RRDSET_FLAG_DETAIL);
				FOREACH_COUNTER_PACKETS(GEN_RRD_DIM_ADD, port)
			}
			else
				rrdset_next(port->st_packets);

			FOREACH_COUNTER_PACKETS(GEN_RRD_DIM_SETP, port)
			rrdset_done(port->st_packets);
		}

		if (port->do_errors != CONFIG_BOOLEAN_NO) {
			// First creation of RRD Set (charts)
			if(unlikely(!port->st_errors)) {
				port->st_errors = rrdset_create_localhost(
					port->chart_type_errors
					, port->chart_id_errors
					, NULL
					, port->chart_family
					, "ib.errors"
					, "errors"
					, "errors/s"
					, PLUGIN_PROC_NAME
					, PLUGIN_PROC_MODULE_INFINIBAND_NAME
					, port->priority + 1
					, update_every
					, RRDSET_TYPE_LINE
				);

				rrdset_flag_set(port->st_errors, RRDSET_FLAG_DETAIL);
				FOREACH_COUNTER_ERRORS(GEN_RRD_DIM_ADD, port)
			}
			else
				rrdset_next(port->st_errors);

			FOREACH_COUNTER_ERRORS(GEN_RRD_DIM_SETP, port)
			rrdset_done(port->st_errors);
		}

	}


	return 0;
}
