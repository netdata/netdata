#ifndef NETDATA_GLOBAL_STATISTICS_H
#define NETDATA_GLOBAL_STATISTICS_H 1

// ----------------------------------------------------------------------------
// global statistics

struct global_statistics {
	unsigned long long connected_clients;
	unsigned long long web_requests;
	unsigned long long bytes_received;
	unsigned long long bytes_sent;

};

extern struct global_statistics global_statistics;

extern void global_statistics_lock(void);
extern void global_statistics_unlock(void);

#endif /* NETDATA_GLOBAL_STATISTICS_H */
