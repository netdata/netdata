#ifndef NETDATA_BACKENDS_H
#define NETDATA_BACKENDS_H 1

#define BACKEND_SOURCE_DATA_AS_COLLECTED 0x00000001
#define BACKEND_SOURCE_DATA_AVERAGE      0x00000002
#define BACKEND_SOURCE_DATA_SUM          0x00000004

#define BACKEND_SOURCE_BITS (BACKEND_SOURCE_DATA_AS_COLLECTED|BACKEND_SOURCE_DATA_AVERAGE|BACKEND_SOURCE_DATA_SUM)

extern int backend_send_names;
extern int backend_update_every;
extern uint32_t backend_options;
extern const char *backend_prefix;

extern void *backends_main(void *ptr);

extern uint32_t backend_parse_data_source(const char *source, uint32_t mode);

extern calculated_number backend_calculate_value_from_stored_data(
        RRDSET *st                  // the chart
        , RRDDIM *rd                // the dimension
        , time_t after              // the start timestamp
        , time_t before             // the end timestamp
        , uint32_t options          // BACKEND_SOURCE_* bitmap
);

#endif /* NETDATA_BACKENDS_H */
