#ifndef NETDATA_GLOBAL_STATISTICS_H
#define NETDATA_GLOBAL_STATISTICS_H 1

// ----------------------------------------------------------------------------
/// global statistics
struct global_statistics {
    volatile uint16_t connected_clients; ///< number of connected clients

    volatile uint64_t web_requests;            ///< Number of web requests.
    /// \brief Total duration to serve web_requests.
    ///
    /// Summed duration from the reception of a request, to the dispatch of the last byte.
    volatile uint64_t web_usec;
    /// \brief Maximum duration of a request for the last iteration.
    ///
    /// The max time to serve a request. This is reset to zero every time the chart is updated, so it shows the max time for each iteration.
    volatile uint64_t web_usec_max;
    volatile uint64_t bytes_received;          ///< Number of bytes received.
    volatile uint64_t bytes_sent;              ///< Number of bytes sent.
    volatile uint64_t content_size;            ///< Size of contetnt.
    volatile uint64_t compressed_content_size; ///< Size of compressed content.
};

extern volatile struct global_statistics global_statistics;

extern void global_statistics_lock(void);
extern void global_statistics_unlock(void);
extern void finished_web_request_statistics(uint64_t dt,
                                     uint64_t bytes_received,
                                     uint64_t bytes_sent,
                                     uint64_t content_size,
                                     uint64_t compressed_content_size);

extern void web_client_connected(void);
extern void web_client_disconnected(void);

#define GLOBAL_STATS_RESET_WEB_USEC_MAX 0x01
extern void global_statistics_copy(struct global_statistics *gs, uint8_t options);
extern void global_statistics_charts(void);

#endif /* NETDATA_GLOBAL_STATISTICS_H */
