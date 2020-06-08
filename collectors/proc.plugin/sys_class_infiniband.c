// SPDX-License-Identifier: GPL-3.0-or-later

// Heavily inspired from proc_net_dev.c
#include "plugin_proc.h"

#define PLUGIN_PROC_MODULE_INFINIBAND_NAME "/sys/class/infiniband"
#define CONFIG_SECTION_PLUGIN_SYS_CLASS_INFINIBAND "plugin:" PLUGIN_PROC_CONFIG_NAME ":" PLUGIN_PROC_MODULE_INFINIBAND_NAME

// ib_device::name[IB_DEVICE_NAME_MAX(64)] + "-" + ib_device::phys_port_cnt[u8 = 3 chars]
#define IBNAME_MAX 68

// ----------------------------------------------------------------------------
// infiniband & omnipath standard counters

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



//
// Hardware Counters
//

// List of implemented hardware vendors
#define FOREACH_HWCOUNTER_NAME(GEN, ...) \
	GEN(mlx, __VA_ARGS__)

// HW Counters for Mellanox ConnectX Devices
#define FOREACH_HWCOUNTER_MLX(GEN, ...) \
	FOREACH_HWCOUNTER_MLX_PACKETS(GEN, __VA_ARGS__) \
	FOREACH_HWCOUNTER_MLX_ERRORS(GEN, __VA_ARGS__)

#define FOREACH_HWCOUNTER_MLX_PACKETS(GEN, ...) \
	GEN(np_cnp_sent,                 packets, "RoCEv2 Congestion sent",  1, __VA_ARGS__) \
	GEN(np_ecn_marked_roce_packets,  packets, "RoCEv2 Congestion rcvd", -1, __VA_ARGS__) \
	GEN(rp_cnp_handled,              packets, "IB Congestion handled",   1, __VA_ARGS__) \
	GEN(rx_atomic_requests,          packets, "ATOMIC req. rcvd",        1, __VA_ARGS__) \
	GEN(rx_dct_connect,              packets, "Connection req. rcvd",    1, __VA_ARGS__) \
	GEN(rx_read_requests,            packets, "Read req. rcvd",          1, __VA_ARGS__) \
	GEN(rx_write_requests,           packets, "Write req. rcvd",         1, __VA_ARGS__) \
	GEN(roce_adp_retrans,            packets, "RoCE retrans adaptive",   1, __VA_ARGS__) \
	GEN(roce_adp_retrans_to,         packets, "RoCE retrans timeout",    1, __VA_ARGS__) \
	GEN(roce_slow_restart,           packets, "RoCE slow restart",       1, __VA_ARGS__) \
	GEN(roce_slow_restart_cnps,      packets, "RoCE slow restart congestion",  1, __VA_ARGS__) \
	GEN(roce_slow_restart_trans,     packets, "RoCE slow restart count", 1, __VA_ARGS__)

#define FOREACH_HWCOUNTER_MLX_ERRORS(GEN, ...) \
	GEN(duplicate_request,           errors, "Duplicated packets",   -1, __VA_ARGS__) \
	GEN(implied_nak_seq_err,         errors, "Pkt Seq Num gap",       1, __VA_ARGS__) \
	GEN(local_ack_timeout_err,       errors, "Ack timer expired",     1, __VA_ARGS__) \
	GEN(out_of_buffer,               errors, "Drop missing buffer",   1, __VA_ARGS__) \
	GEN(out_of_sequence,             errors, "Drop out of sequence",  1, __VA_ARGS__) \
	GEN(packet_seq_err,              errors, "NAK sequence rcvd",     1, __VA_ARGS__) \
	GEN(req_cqe_error,               errors, "CQE err Req",           1, __VA_ARGS__) \
	GEN(resp_cqe_error,              errors, "CQE err Resp",          1, __VA_ARGS__) \
	GEN(req_cqe_flush_error,         errors, "CQE Flushed err Req",   1, __VA_ARGS__) \
	GEN(resp_cqe_flush_error,        errors, "CQE Flushed err Resp",  1, __VA_ARGS__) \
	GEN(req_remote_access_errors,    errors, "Remote access err Req", 1, __VA_ARGS__) \
	GEN(resp_remote_access_errors,   errors, "Remote access err Resp",1, __VA_ARGS__) \
	GEN(req_remote_invalid_request,  errors, "Remote invalid req",    1, __VA_ARGS__) \
	GEN(resp_local_length_error,     errors, "Local length err Resp", 1, __VA_ARGS__) \
	GEN(rnr_nak_retry_err,           errors, "RNR NAK Packets",       1, __VA_ARGS__) \
	GEN(rp_cnp_ignored,              errors, "CNP Pkts ignored",      1, __VA_ARGS__) \
	GEN(rx_icrc_encapsulated,        errors, "RoCE ICRC Errors",      1, __VA_ARGS__)


// HW Counters for Intel Omnipath devices
// #define FOREACH_HWCOUNTER_HFI_ERRORS(GEN, ...) \




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
	char *hwcounters_path;
	int  len;

	// flags
	int configured;
	int enabled;
	int updated;

	int do_bytes;
	int do_packets;
	int do_errors;
	int do_hwpackets;
	int do_hwerrors;

	const char *chart_type_bytes;
	const char *chart_type_packets;
	const char *chart_type_errors;
	const char *chart_type_hwpackets;
	const char *chart_type_hwerrors;

	const char *chart_id_bytes;
	const char *chart_id_packets;
	const char *chart_id_errors;
	const char *chart_id_hwpackets;
	const char *chart_id_hwerrors;

	const char *chart_family;

	unsigned long priority;

	// Stats from /$device/ports/$portid/counters
	// as drivers/infiniband/hw/qib/qib_verbs.h
	// All uint64 except vl15_dropped, local_link_integrity_errors, excessive_buffer_overrun_errors uint32
	// Will generate 2 elements for each counter:
	// - uint64_t to store the value
	// - char*    to store the filename path
	#define GEN_DEF_COUNTER(NAME, ...) \
		uint64_t  NAME; \
		char      *file_##NAME;
	FOREACH_COUNTER(GEN_DEF_COUNTER)

	// Vendor specific hwcounters from /$device/ports/$portid/hwcounters
	// We will generate one struct pointer per vendor to avoid future casting
	#define GEN_DEF_HWCOUNTER_PTR(VENDOR, ...) \
		struct ibporthw_##VENDOR *hwcounters_##VENDOR;
	FOREACH_HWCOUNTER_NAME(GEN_DEF_HWCOUNTER_PTR)

	// Function pointer to the "infiniband_parse_hwcounter_<vendor>" function
	void (*parse_hwcounters)(struct ibport *);


	// charts and dim
	RRDSET *st_bytes;
	RRDSET *st_packets;
	RRDSET *st_errors;
	RRDSET *st_hwpackets;
	RRDSET *st_hwerrors;

	#define GEN_DEF_RRD_DIM(NAME, ...)       RRDDIM   *rd_##NAME;
	FOREACH_COUNTER(GEN_DEF_RRD_DIM)

	usec_t speed_last_collected_usec;

	struct ibport *next;
} *ibport_root = NULL, *ibport_last_used = NULL;


//
// Vendor specific
//

#define GEN_DEF_HWCOUNTER(NAME, ...) \
	uint64_t  NAME; \
	char      *file_##NAME;

#define GEN_DO_HWCOUNTER_READ(NAME,GRP,DESC,DIR,PORT,HW, ...) \
	if (HW->file_##NAME) { \
		if (read_single_number_file(HW->file_##NAME, (unsigned long long *) &HW->NAME)) { \
			error("cannot read iface '%s' hwcounter '"#HW"'", PORT->name); \
			HW->file_##NAME = NULL; \
		} \
	}


// Mellanox
struct ibporthw_mlx {
	FOREACH_HWCOUNTER_MLX(GEN_DEF_HWCOUNTER)
};
void infiniband_parse_hwcounters_mlx(struct ibport *port) {
	if (port->do_hwerrors != CONFIG_BOOLEAN_NO) {
		FOREACH_HWCOUNTER_MLX_ERRORS(GEN_DO_HWCOUNTER_READ, port, port->hwcounters_mlx)
	}
	if (port->do_hwpackets != CONFIG_BOOLEAN_NO) {
		FOREACH_HWCOUNTER_MLX_PACKETS(GEN_DO_HWCOUNTER_READ, port, port->hwcounters_mlx)
	}
}




// ----------------------------------------------------------------------------


static struct ibport *get_ibport(const char *dev, const char *port) {
	struct ibport *p;

	char name[IBNAME_MAX+1];
	snprintfz(name, IBNAME_MAX, "%s-%s", dev, port);


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


	p->chart_type_bytes     = strdupz("infiniband_cnt_bytes");
	p->chart_type_packets   = strdupz("infiniband_cnt_packets");
	p->chart_type_errors    = strdupz("infiniband_cnt_errors");
	p->chart_type_hwpackets = strdupz("infiniband_hwc_packets");
	p->chart_type_hwerrors  = strdupz("infiniband_hwc_errors");

	p->chart_id_bytes     = strdupz(p->name);
	p->chart_id_packets   = strdupz(p->name);
	p->chart_id_errors    = strdupz(p->name);
	p->chart_id_hwpackets = strdupz(p->name);
	p->chart_id_hwerrors  = strdupz(p->name);

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
	static int do_bytes = -1, do_packets = -1, do_errors = -1, do_hwpackets = -1, do_hwerrors= -1;
	static char *sys_class_infiniband_dirname = NULL;

	static long long int dt_to_refresh_speed = 0;
	
	if(unlikely(enable_new_ports == -1)) {
		char dirname[FILENAME_MAX + 1];

		snprintfz(dirname, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/class/infiniband");
		sys_class_infiniband_dirname = config_get(CONFIG_SECTION_PLUGIN_SYS_CLASS_INFINIBAND, "dirname to monitor", dirname);

		do_bytes     = config_get_boolean_ondemand(CONFIG_SECTION_PLUGIN_SYS_CLASS_INFINIBAND, "bandwidth counters", CONFIG_BOOLEAN_AUTO);
		do_packets   = config_get_boolean_ondemand(CONFIG_SECTION_PLUGIN_SYS_CLASS_INFINIBAND, "packets counters", CONFIG_BOOLEAN_AUTO);
		do_errors    = config_get_boolean_ondemand(CONFIG_SECTION_PLUGIN_SYS_CLASS_INFINIBAND, "errors counters", CONFIG_BOOLEAN_AUTO);
		do_hwpackets = config_get_boolean_ondemand(CONFIG_SECTION_PLUGIN_SYS_CLASS_INFINIBAND, "hardware packets counters", CONFIG_BOOLEAN_AUTO);
		do_hwerrors  = config_get_boolean_ondemand(CONFIG_SECTION_PLUGIN_SYS_CLASS_INFINIBAND, "hardware errors counters", CONFIG_BOOLEAN_AUTO);

		disabled_list = simple_pattern_create(config_get(CONFIG_SECTION_PLUGIN_SYS_CLASS_INFINIBAND, "disable by default interfaces matching", ""), NULL, SIMPLE_PATTERN_EXACT);

		dt_to_refresh_speed = config_get_number(CONFIG_SECTION_PLUGIN_SYS_CLASS_INFINIBAND, "refresh interface speed every seconds", 10) * USEC_PER_SEC;
		if(dt_to_refresh_speed < 0) dt_to_refresh_speed = 0;

		enable_new_ports = config_get_boolean_ondemand(CONFIG_SECTION_PLUGIN_SYS_CLASS_INFINIBAND, "Monitor ports going online during runtime", CONFIG_BOOLEAN_AUTO);
	}


	// init listing of /sys/class/infiniband/ (or rediscovery)
	if(unlikely(!initialized)) {

		// If folder does not exists, return 1 to disable
		DIR *devices_dir = opendir(sys_class_infiniband_dirname);
		if(unlikely(!devices_dir)) return 1;

		// Work on all device available
		struct dirent *dev_dent;
		while ( (dev_dent = readdir(devices_dir)) ) {

			// Skip special folders
			if (!strcmp(dev_dent->d_name, "..") || !strcmp(dev_dent->d_name, "."))
				continue;

			// /sys/class/infiniband/<dev>/ports
			char ports_dirname[FILENAME_MAX +1];
			snprintfz(ports_dirname, FILENAME_MAX, "%s/%s/%s", sys_class_infiniband_dirname, dev_dent->d_name, "ports");

			DIR *ports_dir = opendir(ports_dirname);
			if(unlikely(!ports_dir)) continue;

			struct dirent *port_dent;
			while ( (port_dent = readdir(ports_dir)) ) {

				// Skip special folders
				if (!strcmp(port_dent->d_name, "..") || !strcmp(port_dent->d_name, "."))
					continue;

				// Check if counters are availablea (mandatory)
				// /sys/class/infiniband/<device>/ports/<port>/counters
				char counters_dirname[FILENAME_MAX +1];
				snprintfz(counters_dirname, FILENAME_MAX, "%s/%s/%s", ports_dirname, port_dent->d_name, "counters");
				DIR *counters_dir = opendir(counters_dirname);
				// Standard counters are mandatory
				if (!counters_dir) continue;

				// Nearly same with hardware counters
				char hwcounters_dirname[FILENAME_MAX +1];
				snprintfz(hwcounters_dirname, FILENAME_MAX, "%s/%s/%s", ports_dirname, port_dent->d_name, "hwcounters");
				DIR *hwcounters_dir = opendir(hwcounters_dirname);

				// Get new ibport
				struct ibport *p = get_ibport(dev_dent->d_name, port_dent->d_name);
				if(!p) continue;

				p->updated = 1;

				// Prepare configuration
				if (!p->configured) {
					p->configured = 1;

					p->counters_path = strdupz(counters_dirname);
					p->hwcounters_path = strdupz(hwcounters_dirname);

					p->enabled = enable_new_ports;

					if (p->enabled)
						p->enabled = !simple_pattern_matches(disabled_list, p->name);

					char buffer[FILENAME_MAX + 1];
					snprintfz(buffer, FILENAME_MAX, "plugin:proc:/sys/class/infiniband:%s", p->name);

					// Standard counters
					p->do_bytes   = config_get_boolean_ondemand(buffer, "bytes",   do_bytes);
					p->do_packets = config_get_boolean_ondemand(buffer, "packets", do_packets);
					p->do_errors  = config_get_boolean_ondemand(buffer, "errors",  do_errors);

					// Gen filename allocation and concatenation
					#define GEN_DO_COUNTER_NAME(NAME,GRP,DESC,DIR,PORT, ...) \
						PORT->file_##NAME = callocz(1, strlen(PORT->counters_path) + sizeof(#NAME) +3); \
						strcat(PORT->file_##NAME, PORT->counters_path); \
						strcat(PORT->file_##NAME, "/"#NAME);
					FOREACH_COUNTER(GEN_DO_COUNTER_NAME, p)


					// Check HW Counters vendor dependent
					if (hwcounters_dir) {

						// By default set standard
						p->do_hwpackets = config_get_boolean_ondemand(buffer, "hwpackets", do_hwpackets);
						p->do_hwerrors  = config_get_boolean_ondemand(buffer, "hwerrors",  do_hwerrors);


						// VENDOR: Mellanox
						if (strncmp(dev_dent->d_name, "mlx", 3) == 0) {

							// Allocate the vendor specific struct
							p->hwcounters_mlx = callocz(1, sizeof(struct ibporthw_mlx));

							// Allocate the chars to the filenames
							#define GEN_DO_HWCOUNTER_NAME(NAME,GRP,DESC,DIR,PORT,HW, ...) \
								HW->file_##NAME = callocz(1, strlen(PORT->hwcounters_path) + sizeof(#NAME) +3); \
								strcat(HW->file_##NAME, PORT->hwcounters_path); \
								strcat(HW->file_##NAME, "/"#NAME);
							FOREACH_HWCOUNTER_MLX(GEN_DO_HWCOUNTER_NAME, p, p->hwcounters_mlx)

							// Set the function pointer for hwcounter parsing
							p->parse_hwcounters = &infiniband_parse_hwcounters_mlx;
						}

						// VENDOR: Unknown
						else {
							p->do_hwpackets = CONFIG_BOOLEAN_NO;
							p->do_hwerrors  = CONFIG_BOOLEAN_NO;
						}
					}

				}
			}
			closedir(ports_dir);
		}
		closedir(devices_dir);

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


		// Call the function for parsing hwcounters
		if (port->parse_hwcounters) {
			(*port->parse_hwcounters)(port);
		}



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

		// TODO: Add logic for hwcounters

	}


	return 0;
}
