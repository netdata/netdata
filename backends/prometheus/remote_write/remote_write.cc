// SPDX-License-Identifier: GPL-3.0-or-later

#include <snappy.h>
#include "remote_write.pb.h"
#include "remote_write.h"

using namespace prometheus;


google::protobuf::Arena arena;
WriteRequest *write_request;

void init_write_request() {
    GOOGLE_PROTOBUF_VERIFY_VERSION;
    write_request = google::protobuf::Arena::CreateMessage<WriteRequest>(&arena);
}

void clear_write_request() {
    write_request->clear_timeseries();
}

void add_host_info(const char *name, const char *instance, const char *application, const char *version, const int64_t timestamp) {
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

    if(application) {
        label = timeseries->add_labels();
        label->set_name("application");
        label->set_value(application);
    }

    if(version) {
        label = timeseries->add_labels();
        label->set_name("version");
        label->set_value(version);
    }

    sample = timeseries->add_samples();
    sample->set_value(1);
    sample->set_timestamp(timestamp);
}

// adds tag to the last created timeseries
void add_tag(char *tag, char *value) {
    TimeSeries *timeseries;
    Label *label;

    timeseries = write_request->mutable_timeseries(write_request->timeseries_size() - 1);

    label = timeseries->add_labels();
    label->set_name(tag);
    label->set_value(value);
}

void add_metric(const char *name, const char *chart, const char *family, const char *dimension, const char *instance, const double value, const int64_t timestamp) {
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

    if(dimension) {
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

size_t get_write_request_size(){
#if GOOGLE_PROTOBUF_VERSION < 3001000
    size_t size = (size_t)snappy::MaxCompressedLength(write_request->ByteSize());
#else
    size_t size = (size_t)snappy::MaxCompressedLength(write_request->ByteSizeLong());
#endif

    return (size < INT_MAX)?size:0;
}

int pack_write_request(char *buffer, size_t *size) {
    std::string uncompressed_write_request;
    if(write_request->SerializeToString(&uncompressed_write_request) == false) return 1;

    snappy::RawCompress(uncompressed_write_request.data(), uncompressed_write_request.size(), buffer, size);

    return 0;
}

void protocol_buffers_shutdown() {
    google::protobuf::ShutdownProtobufLibrary();
}
