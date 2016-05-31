#ifndef NETDATA_GLOBAL_STATISTICS_H
#define NETDATA_GLOBAL_STATISTICS_H 1

// ----------------------------------------------------------------------------
// global statistics

struct global_statistics {
	volatile unsigned long volatile connected_clients;
	volatile unsigned long long volatile web_requests;
	volatile unsigned long long volatile web_usec;
	volatile unsigned long long volatile bytes_received;
	volatile unsigned long long volatile bytes_sent;
	volatile unsigned long long volatile content_size;
	volatile unsigned long long volatile compressed_content_size;
};

extern struct global_statistics global_statistics;

extern void global_statistics_lock(void);
extern void global_statistics_unlock(void);

#endif /* NETDATA_GLOBAL_STATISTICS_H */
