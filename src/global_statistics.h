#ifndef NETDATA_GLOBAL_STATISTICS_H
#define NETDATA_GLOBAL_STATISTICS_H 1

// ----------------------------------------------------------------------------
// global statistics

struct global_statistics {
	unsigned long volatile connected_clients;
	unsigned long long volatile web_requests;
	unsigned long long volatile web_usec;
	unsigned long long volatile bytes_received;
	unsigned long long volatile bytes_sent;
};

extern struct global_statistics global_statistics;

extern void global_statistics_lock(void);
extern void global_statistics_unlock(void);

#endif /* NETDATA_GLOBAL_STATISTICS_H */
