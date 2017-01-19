#ifndef NETDATA_GLOBAL_STATISTICS_H
#define NETDATA_GLOBAL_STATISTICS_H 1

/**
 * @file global_statistics.h
 * @brief The global web server statistics.
 */

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

/**
 * Update the statistics after a finished web reqest.
 *
 * @param dt Duration of the request.
 * @param bytes_received on the request
 * @param bytes_sent as response
 * @param content_size of the response
 * @param compressed_content_size of the response
 */
extern void finished_web_request_statistics(uint64_t dt,
                                     uint64_t bytes_received,
                                     uint64_t bytes_sent,
                                     uint64_t content_size,
                                     uint64_t compressed_content_size);

/**
 * Update the statistics after a web client connected.
 */
extern void web_client_connected(void);
/**
 * Update the statistics after a web client disconnected.
 */
extern void web_client_disconnected(void);

/**
 * Reset the max response time.
 *
 * The caller will set this to indicate that the max value has been used and it can now be reset.
 * Without there was no way to find the max duration per second.
 */
#define GLOBAL_STATS_RESET_WEB_USEC_MAX 0x01
/**
 * Copy the current web server statistics.
 *
 * After this `gs` can be read.
 *
 * @param gs Pointer to write to.
 * @param options GLOBAL_STATS_*
 */
extern void global_statistics_copy(struct global_statistics *gs, uint8_t options);
/**
 * Update global statistic charts.
 *
 * This updates the global statistic netdata charts exposed to clients.
 */
extern void global_statistics_charts(void);

#endif /* NETDATA_GLOBAL_STATISTICS_H */
