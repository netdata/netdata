#include <stdbool.h>
#include "plugin_proc.h"

#define PLUGIN_PROC_MODULE_NETWIRELESS_NAME "/proc/net/wireless"

#define CONFIG_SECTION_PLUGIN_PROC_NETWIRELESS "plugin:" PLUGIN_PROC_CONFIG_NAME ":" PLUGIN_PROC_MODULE_NETWIRELESS_NAME


static struct netwireless {
	char *name;
	uint32_t hash;

	//flags
	bool configured;
	struct timeval updated;

	int do_status;
	int do_quality;
	int do_discarded_packets;
	int do_missed_beacon;

	// Data collected
	// status
	kernel_uint_t status;

	// Quality
	kernel_uint_t link;
	kernel_uint_t level;
	kernel_uint_t noise;

	// Discarded packets
	kernel_uint_t nwid;
	kernel_uint_t crypt;
	kernel_uint_t frag;
	kernel_uint_t retry;
	kernel_uint_t misc;

	// missed beacon
	kernel_uint_t missed_beacon;

	const char *chart_id_net_status;
	const char *chart_id_net_link;
	const char *chart_id_net_level;
	const char *chart_id_net_noise;
	const char *chart_id_net_discarded_packets;
	const char *chart_id_net_missed_beacon;

	const char *chart_family;

	// charts
	// satus
	RRDSET *st_status;

	// Quality
	RRDSET *st_link;
	RRDSET *st_level;
	RRDSET *st_noise;

	// Discarded Packets
	RRDSET *st_discarded_packets;
	// Missed beacon
	RRDSET *st_missed_beacon;

	// Dimensions
	// status
	RRDDIM *rd_status;

	// Quality
	RRDDIM *rd_link;
	RRDDIM *rd_level;
	RRDDIM *rd_noise;

	// Discarded packets
	RRDDIM *rd_nwid;
	RRDDIM *rd_crypt;
	RRDDIM *rd_frag;
	RRDDIM *rd_retry;
	RRDDIM *rd_misc;

	// missed beacon
	RRDDIM *rd_missed_beacon;


	struct netwireless *next;
} *netwireless_root = NULL;


static void netwireless_free_st(struct netwireless *wireless_dev) {
	if(wireless_dev->st_status) rrdset_is_obsolete(wireless_dev->st_status);
	if(wireless_dev->st_link) rrdset_is_obsolete(wireless_dev->st_link);
	if(wireless_dev->st_level) rrdset_is_obsolete(wireless_dev->st_level);
	if(wireless_dev->st_noise) rrdset_is_obsolete(wireless_dev->st_noise);
	if(wireless_dev->st_discarded_packets) rrdset_is_obsolete(wireless_dev->st_discarded_packets);
	if(wireless_dev->st_missed_beacon) rrdset_is_obsolete(wireless_dev->st_missed_beacon);

	wireless_dev->st_status = NULL;
	wireless_dev->st_link = NULL;
	wireless_dev->st_level = NULL;
	wireless_dev->st_noise = NULL;
	wireless_dev->st_discarded_packets = NULL;
	wireless_dev->st_missed_beacon = NULL;
}

static void netwireless_free(struct netwireless *wireless_dev) {

		wireless_dev->next = NULL;
		freez((void *)wireless_dev->name);
		netwireless_free_st(wireless_dev);
		freez((void *)wireless_dev->chart_id_net_status);
		freez((void *)wireless_dev->chart_id_net_link);
		freez((void *)wireless_dev->chart_id_net_level);
		freez((void *)wireless_dev->chart_id_net_noise);
		freez((void *)wireless_dev->chart_id_net_discarded_packets);
		freez((void *)wireless_dev->chart_id_net_missed_beacon);

		freez((void *)wireless_dev);

}

static void netwireless_cleanup(struct timeval *timestamp) {

	struct netwireless *previous = NULL;
    // search it, from begining to the end
    for(struct netwireless* current = netwireless_root; current;) {

		if(timercmp(&current->updated, timestamp, <)) {

			struct netwireless *to_free = current;
			current = current->next;
			netwireless_free(to_free);

			if(previous) {
				previous->next = current;
			}
			else {
				netwireless_root = current;
			}

		}
		else {
			previous = current;
			current = current->next;
		}

    }
}



// finds an existing interface or creates a new entry
static struct netwireless *find_or_create_wireless(const char *name) {
    struct netwireless *wireless;

    uint32_t hash = simple_hash(name);

    // search it, from begining to the end
    for(wireless = netwireless_root ; wireless ; wireless = wireless->next) {
        if(unlikely(hash == wireless->hash && !strcmp(name, wireless->name))) {
            return wireless;
        }
    }

    // create a new one
	wireless = callocz(1, sizeof(struct netwireless));
	wireless->name = strdupz(name);
	wireless->hash = simple_hash(name);
	wireless->next = NULL;

    // link it to the end
    if(netwireless_root) {
        struct netwireless *last_node;
        for(last_node = netwireless_root; last_node->next ; last_node = last_node->next);
        last_node->next = wireless;
    }
    else
        netwireless_root = wireless;

    return wireless;
}

static bool configure_device(int do_status, int do_quality, int do_discarded_packets, int do_missed,
							 struct netwireless *wireless_dev) {
	wireless_dev->do_status = do_status;
	wireless_dev->do_quality = do_quality;
	wireless_dev->do_discarded_packets = do_discarded_packets;
	wireless_dev->do_missed_beacon = do_missed;
	wireless_dev->configured = true;

	char buffer[RRD_ID_LENGTH_MAX + 1];

	snprintfz(buffer, RRD_ID_LENGTH_MAX, "%s", wireless_dev->name);
	wireless_dev->chart_id_net_status = strdupz(buffer);

	snprintfz(buffer, RRD_ID_LENGTH_MAX, "%s", wireless_dev->name);
	wireless_dev->chart_id_net_link = strdupz(buffer);

	snprintfz(buffer, RRD_ID_LENGTH_MAX, "%s", wireless_dev->name);
	wireless_dev->chart_id_net_level = strdupz(buffer);

	snprintfz(buffer, RRD_ID_LENGTH_MAX, "%s", wireless_dev->name);
	wireless_dev->chart_id_net_noise = strdupz(buffer);

	snprintfz(buffer, RRD_ID_LENGTH_MAX, "%s", wireless_dev->name);
	wireless_dev->chart_id_net_discarded_packets = strdupz(buffer);

	snprintfz(buffer, RRD_ID_LENGTH_MAX, "%s", wireless_dev->name);
	wireless_dev->chart_id_net_missed_beacon = strdupz(buffer);

	return true;
}

int do_proc_net_wireless(int update_every, usec_t dt) {
	(void)dt;
	static procfile *ff = NULL;
	static int do_status = -1, do_quality = -1, do_discarded_packets = -1, do_missed = -1;
	static unsigned long long int dt_to_refresh_speed = 0;
	static char *proc_net_wireless_filename = NULL;
	static int enable_new_interfaces = -1;

	if(unlikely(enable_new_interfaces == -1)) {
		char filename[FILENAME_MAX + 1];
		snprintfz(filename, FILENAME_MAX, "%s", "/proc/net/wireless");

		proc_net_wireless_filename = config_get(CONFIG_SECTION_PLUGIN_PROC_NETWIRELESS,
												"filename to monitor", filename);
        enable_new_interfaces = config_get_boolean_ondemand(CONFIG_SECTION_PLUGIN_PROC_NETWIRELESS,
															"enable new interfaces detected at runtime",
															CONFIG_BOOLEAN_AUTO);

        do_status = config_get_boolean_ondemand(CONFIG_SECTION_PLUGIN_PROC_NETWIRELESS,
													  "status for all interfaces", CONFIG_BOOLEAN_AUTO);

        do_quality = config_get_boolean_ondemand(CONFIG_SECTION_PLUGIN_PROC_NETWIRELESS,
													  "quality for all interfaces", CONFIG_BOOLEAN_AUTO);

        do_discarded_packets = config_get_boolean_ondemand(CONFIG_SECTION_PLUGIN_PROC_NETWIRELESS,
															  "discarded packets for all interfaces",
															  CONFIG_BOOLEAN_AUTO);

        do_missed = config_get_boolean_ondemand(CONFIG_SECTION_PLUGIN_PROC_NETWIRELESS,
													  "missed for all interfaces", CONFIG_BOOLEAN_AUTO);

        dt_to_refresh_speed = config_get_number(CONFIG_SECTION_PLUGIN_PROC_NETWIRELESS,
												"refresh interface speed every seconds", 10) * USEC_PER_SEC;
        if(dt_to_refresh_speed < 0) dt_to_refresh_speed = 0;

	}

	if(unlikely(!ff)) {
		ff = procfile_open(proc_net_wireless_filename, " \t,|", PROCFILE_FLAG_DEFAULT);
		if(unlikely(!ff)) return 1;
	}

	ff = procfile_readall(ff);
	if(unlikely(!ff)) return 1;

	size_t lines = procfile_lines(ff);
	struct timeval timestamp;
	gettimeofday(&timestamp, NULL);

	for(size_t l = 2; l < lines; l++) {
		if(unlikely(procfile_linewords(ff, l) < 11)) continue;
		char *name = procfile_lineword(ff, l, 0);
		size_t len = strlen(name);
		if(name[len - 1] == ':') name[len - 1] = '\0';

		struct netwireless *wireless_dev = find_or_create_wireless(name);

		if(unlikely(!wireless_dev->configured)) {
			configure_device(do_status, do_quality, do_discarded_packets,
							 do_missed, wireless_dev);
		}

		if(likely(do_status != CONFIG_BOOLEAN_NO)) {
			wireless_dev->status = str2kernel_uint_t(procfile_lineword(ff, l, 1));

			if(unlikely(!wireless_dev->st_status)) {
		        wireless_dev->st_status = rrdset_create_localhost(
												"ap_status"
												, wireless_dev->chart_id_net_status
												, NULL
												, wireless_dev->name
												, "ap.status"
												, "Status"
										        , "status"
												, PLUGIN_PROC_NAME
												, PLUGIN_PROC_MODULE_NETWIRELESS_NAME
												, NETDATA_CHART_PRIO_WIRELESS_IFACE
												, update_every
												, RRDSET_TYPE_LINE
												);
				rrdset_flag_set(wireless_dev->st_status, RRDSET_FLAG_DETAIL);

				wireless_dev->rd_status = rrddim_add(wireless_dev->st_status, "status", NULL, 1, 1,
													 RRD_ALGORITHM_ABSOLUTE);
			}
			else {
				rrdset_next(wireless_dev->st_status);
			}

			rrddim_set_by_pointer(wireless_dev->st_status, wireless_dev->rd_status,
								  (collected_number)wireless_dev->status);
			rrdset_done(wireless_dev->st_status);

		}

		if(likely(do_quality != CONFIG_BOOLEAN_NO)) {
			wireless_dev->link = str2kernel_uint_t(procfile_lineword(ff, l, 2));
			wireless_dev->level = str2kernel_uint_t(procfile_lineword(ff, l, 3));
			wireless_dev->noise = str2kernel_uint_t(procfile_lineword(ff, l, 4));

			if(unlikely(!wireless_dev->st_link)) {

				wireless_dev->st_link = rrdset_create_localhost(
												"ap_quality_link"
												, wireless_dev->chart_id_net_link
												, NULL
												, wireless_dev->name
												, "ap.quality.link"
												, "Link"
												, "dB"
												, PLUGIN_PROC_NAME
												, PLUGIN_PROC_MODULE_NETWIRELESS_NAME
												, NETDATA_CHART_PRIO_WIRELESS_IFACE + 1
												, update_every
												, RRDSET_TYPE_LINE
												);
				rrdset_flag_set(wireless_dev->st_link, RRDSET_FLAG_DETAIL);

				wireless_dev->rd_link = rrddim_add(wireless_dev->st_link, "link", NULL, 1, 1,
												   RRD_ALGORITHM_ABSOLUTE);
			}
			else {
				rrdset_next(wireless_dev->st_link);
			}



			if(unlikely(!wireless_dev->st_level)) {

				wireless_dev->st_level = rrdset_create_localhost(
												"ap_quality_level"
												, wireless_dev->chart_id_net_level
												, NULL
												, wireless_dev->name
												, "ap.quality.level"
												, "Signal level"
												, "dB"
												, PLUGIN_PROC_NAME
												, PLUGIN_PROC_MODULE_NETWIRELESS_NAME
												, NETDATA_CHART_PRIO_WIRELESS_IFACE + 2
												, update_every
												, RRDSET_TYPE_LINE
												);
				rrdset_flag_set(wireless_dev->st_level, RRDSET_FLAG_DETAIL);

				wireless_dev->rd_level = rrddim_add(wireless_dev->st_level, "level", NULL, 1, 1,
												  RRD_ALGORITHM_ABSOLUTE);
			}
			else {
				rrdset_next(wireless_dev->st_level);
			}

			if(unlikely(!wireless_dev->st_noise)) {

				wireless_dev->st_noise = rrdset_create_localhost(
												"ap_quality_noise"
												, wireless_dev->chart_id_net_noise
												, NULL
												, wireless_dev->name
												, "ap.quality.noise"
												, "Noise"
												, "dB"
												, PLUGIN_PROC_NAME
												, PLUGIN_PROC_MODULE_NETWIRELESS_NAME
												, NETDATA_CHART_PRIO_WIRELESS_IFACE + 3
												, update_every
												, RRDSET_TYPE_LINE
												);
				rrdset_flag_set(wireless_dev->st_noise, RRDSET_FLAG_DETAIL);

				wireless_dev->rd_noise = rrddim_add(wireless_dev->st_noise, "noise", NULL, 1, 1,
												  RRD_ALGORITHM_ABSOLUTE);
			}
			else {
				rrdset_next(wireless_dev->st_noise);
			}


			rrddim_set_by_pointer(wireless_dev->st_link, wireless_dev->rd_link,
								  (collected_number)wireless_dev->link);
			rrdset_done(wireless_dev->st_link);

			rrddim_set_by_pointer(wireless_dev->st_level, wireless_dev->rd_level,
								  (collected_number)wireless_dev->level);
			rrdset_done(wireless_dev->st_level);

			rrddim_set_by_pointer(wireless_dev->st_noise, wireless_dev->rd_noise,
								  (collected_number)wireless_dev->noise);
			rrdset_done(wireless_dev->st_noise);
		}

		if(likely(do_discarded_packets)) {
			wireless_dev->nwid = str2kernel_uint_t(procfile_lineword(ff, l, 5));
			wireless_dev->crypt = str2kernel_uint_t(procfile_lineword(ff, l, 6));
			wireless_dev->frag = str2kernel_uint_t(procfile_lineword(ff, l, 7));
			wireless_dev->retry = str2kernel_uint_t(procfile_lineword(ff, l, 8));
			wireless_dev->misc = str2kernel_uint_t(procfile_lineword(ff, l, 9));


			if(unlikely(!wireless_dev->st_discarded_packets)) {

				wireless_dev->st_discarded_packets = rrdset_create_localhost(
												"ap_discarded"
												, wireless_dev->chart_id_net_discarded_packets
												, NULL
												, wireless_dev->name
												, "ap.discarded"
												, "Discarded Packets"
												, "packets/s"
												, PLUGIN_PROC_NAME
												, PLUGIN_PROC_MODULE_NETWIRELESS_NAME
												, NETDATA_CHART_PRIO_WIRELESS_IFACE + 4
												, update_every
												, RRDSET_TYPE_LINE
												);

				rrdset_flag_set(wireless_dev->st_discarded_packets, RRDSET_FLAG_DETAIL);

				wireless_dev->rd_nwid = rrddim_add(wireless_dev->st_discarded_packets, "nwid", NULL, 1, 1,
												  RRD_ALGORITHM_ABSOLUTE);
				wireless_dev->rd_crypt = rrddim_add(wireless_dev->st_discarded_packets, "crypt", NULL, 1, 1,
												  RRD_ALGORITHM_ABSOLUTE);
				wireless_dev->rd_frag = rrddim_add(wireless_dev->st_discarded_packets, "frag", NULL, 1, 1,
												  RRD_ALGORITHM_ABSOLUTE);
				wireless_dev->rd_retry = rrddim_add(wireless_dev->st_discarded_packets, "retry", NULL, 1, 1,
												   RRD_ALGORITHM_ABSOLUTE);
				wireless_dev->rd_misc = rrddim_add(wireless_dev->st_discarded_packets, "misc", NULL, 1, 1,
												  RRD_ALGORITHM_ABSOLUTE);
			}
			else {
				rrdset_next(wireless_dev->st_discarded_packets);
			}

			rrddim_set_by_pointer(wireless_dev->st_discarded_packets, wireless_dev->rd_nwid,
								  (collected_number)wireless_dev->nwid);

			rrddim_set_by_pointer(wireless_dev->st_discarded_packets, wireless_dev->rd_crypt,
								  (collected_number)wireless_dev->crypt);

			rrddim_set_by_pointer(wireless_dev->st_discarded_packets, wireless_dev->rd_frag,
								  (collected_number)wireless_dev->frag);

			rrddim_set_by_pointer(wireless_dev->st_discarded_packets, wireless_dev->rd_retry,
								  (collected_number)wireless_dev->retry);

			rrddim_set_by_pointer(wireless_dev->st_discarded_packets, wireless_dev->rd_misc,
								  (collected_number)wireless_dev->misc);

			rrdset_done(wireless_dev->st_discarded_packets);
		}

		if(likely(do_missed)) {
			wireless_dev->missed_beacon = str2kernel_uint_t(procfile_lineword(ff, l, 10));

			if(unlikely(!wireless_dev->st_missed_beacon)) {

				wireless_dev->st_missed_beacon = rrdset_create_localhost(
												"ap_missed"
												, wireless_dev->chart_id_net_missed_beacon
												, NULL
												, wireless_dev->name
												, "ap.missed"
												, "Missed beacon"
												, "packets/s"
												, PLUGIN_PROC_NAME
												, PLUGIN_PROC_MODULE_NETWIRELESS_NAME
												, NETDATA_CHART_PRIO_WIRELESS_IFACE + 5
												, update_every
												, RRDSET_TYPE_LINE
												);
				rrdset_flag_set(wireless_dev->st_missed_beacon, RRDSET_FLAG_DETAIL);

				wireless_dev->rd_missed_beacon = rrddim_add(wireless_dev->st_missed_beacon, "missed beacon", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

			}
			else {
				rrdset_next(wireless_dev->st_missed_beacon);
			}
			rrddim_set_by_pointer(wireless_dev->st_missed_beacon, wireless_dev->rd_missed_beacon,
								  (collected_number)wireless_dev->missed_beacon);
			rrdset_done(wireless_dev->st_missed_beacon);

		}

		wireless_dev->updated = timestamp;
	}

	netwireless_cleanup(&timestamp);
	return 0;
}
