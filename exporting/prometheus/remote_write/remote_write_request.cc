// SPDX-License-Identifier: GPL-3.0-or-later

#include <snappy.h>
#include "remote_write.pb.h"
#include "remote_write_request.h"

using namespace prometheus;

google::protobuf::Arena arena;

/**
 * Initialize a write request
 *
 * @return Returns a new write request
 */
void *init_write_request()
{
    GOOGLE_PROTOBUF_VERIFY_VERSION;
    WriteRequest *write_request = google::protobuf::Arena::CreateMessage<WriteRequest>(&arena);
    return (void *)write_request;
}

/**
 * Adds information about a host to a write request
 *
 * @param write_request_p the write request
 * @param name the name of a metric which is used for providing the host information
 * @param instance the name of the host itself
 * @param application the name of a program which sends the information
 * @param version the version of the program
 * @param timestamp the timestamp for the metric in milliseconds
 */
void add_host_info(
    void *write_request_p,
    const char *name, const char *instance, const char *application, const char *version, const int64_t timestamp)
{
    WriteRequest *write_request = (WriteRequest *)write_request_p;
    TimeSeries *timeseries;
    Sample *sample;
    Label *label;

    timeseries = write_request->add_timeseries();

    label = timeseries->add_labels();
    label->set_name("__name__");
    label->set_value(name);

    label = timeseries->add_labels();
    label->set_name("instance");
    label->set_value(instance);

    if (application) {
        label = timeseries->add_labels();
        label->set_name("application");
        label->set_value(application);
    }

    if (version) {
        label = timeseries->add_labels();
        label->set_name("version");
        label->set_value(version);
    }

    sample = timeseries->add_samples();
    sample->set_value(1);
    sample->set_timestamp(timestamp);
}

/**
 * Adds a label to the last created timeseries
 *
 * @param write_request_p the write request with the timeseries
 * @param key the key of the label
 * @param value the value of the label
 */
void add_label(void *write_request_p, char *key, char *value)
{
    WriteRequest *write_request = (WriteRequest *)write_request_p;
    TimeSeries *timeseries;
    Label *label;

    timeseries = write_request->mutable_timeseries(write_request->timeseries_size() - 1);

    label = timeseries->add_labels();
    label->set_name(key);
    label->set_value(value);
}

/**
 * Adds a metric to a write request
 *
 * @param write_request_p the write request
 * @param name the name of the metric
 * @param chart the chart, the metric belongs to
 * @param family the family, the metric belongs to
 * @param dimension the dimension, the metric belongs to
 * @param instance the name of the host, the metric belongs to
 * @param value the value of the metric
 * @param timestamp the timestamp for the metric in milliseconds
 */
void add_metric(
    void *write_request_p,
    const char *name, const char *chart, const char *family, const char *dimension, const char *instance,
    const double value, const int64_t timestamp)
{
    WriteRequest *write_request = (WriteRequest *)write_request_p;
    TimeSeries *timeseries;
    Sample *sample;
    Label *label;

    timeseries = write_request->add_timeseries();

    label = timeseries->add_labels();
    label->set_name("__name__");
    label->set_value(name);

    label = timeseries->add_labels();
    label->set_name("chart");
    label->set_value(chart);

    label = timeseries->add_labels();
    label->set_name("family");
    label->set_value(family);

    if (dimension) {
        label = timeseries->add_labels();
        label->set_name("dimension");
        label->set_value(dimension);
    }

    label = timeseries->add_labels();
    label->set_name("instance");
    label->set_value(instance);

    sample = timeseries->add_samples();
    sample->set_value(value);
    sample->set_timestamp(timestamp);
}

/**
 * Gets the size of a write request
 *
 * @param write_request_p the write request
 * @return Returns the size of the write request
 */
size_t get_write_request_size(void *write_request_p)
{
    WriteRequest *write_request = (WriteRequest *)write_request_p;

#if GOOGLE_PROTOBUF_VERSION < 3001000
    size_t size = (size_t)snappy::MaxCompressedLength(write_request->ByteSize());
#else
    size_t size = (size_t)snappy::MaxCompressedLength(write_request->ByteSizeLong());
#endif

    return (size < INT_MAX) ? size : 0;
}

/**
 * Packs a write request into a buffer and clears the request
 *
 * @param write_request_p the write request
 * @param buffer a buffer, where compressed data is written
 * @param size gets the size of the write request, returns the size of the compressed data
 * @return Returns 0 on success, 1 on failure
 */
int pack_and_clear_write_request(void *write_request_p, char *buffer, size_t *size)
{
    WriteRequest *write_request = (WriteRequest *)write_request_p;
    std::string uncompressed_write_request;

    if (write_request->SerializeToString(&uncompressed_write_request) == false)
        return 1;
    write_request->clear_timeseries();
    snappy::RawCompress(uncompressed_write_request.data(), uncompressed_write_request.size(), buffer, size);

    return 0;
}

/**
 * Shuts down the Protobuf library
 */
void protocol_buffers_shutdown()
{
    google::protobuf::ShutdownProtobufLibrary();
}
