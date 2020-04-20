// SPDX-License-Identifier: GPL-3.0-or-later

#include <google/pubsub/v1/pubsub.grpc.pb.h>
#include <grpcpp/grpcpp.h>
#include <stdexcept>
#include "pubsub_publish.h"

#define EVENT_CHECK_TIMEOUT 50

struct response {
    google::pubsub::v1::PublishResponse *publish_response;
    size_t tag;
    grpc::Status *status;
};

/**
 * Initialize a Pub/Sub client and a data structure for responses.
 *
 * @param pubsub_specific_data_p a pointer to a structure with instance-wide data.
 * @param pubsub_specific_config_p a pointer to a structure with configuration strings.
 * @return Returns 0 on success, 1 on failure.
 */
int pubsub_init(void *pubsub_specific_data_p, char *project_id, char *topic_id)
{
    struct pubsub_specific_data *pubsub_specific_data = (struct pubsub_specific_data *)pubsub_specific_data_p;

    try {
        std::unique_ptr<google::pubsub::v1::Publisher::Stub> stub = google::pubsub::v1::Publisher::NewStub(
            grpc::CreateChannel("pubsub.googleapis.com", grpc::GoogleDefaultCredentials()));

        if (!stub) {
            std::cerr << "EXPORTING: Can't create a Pub/Sub stub" << std::endl;
            return 1;
        }

        pubsub_specific_data->stub = (void *)&stub;

        google::pubsub::v1::PublishRequest *request = new google::pubsub::v1::PublishRequest;
        request->set_topic(std::string("projects/") + project_id + "/topics/" + topic_id);

        pubsub_specific_data->request = (void *)request;
        pubsub_specific_data->completion_queue = (void *)new grpc::CompletionQueue;
        pubsub_specific_data->responses = (void *)new std::vector<struct response *>;
        pubsub_specific_data->last_tag = 0;

    } catch (std::exception const &ex) {
        std::cerr << "EXPORTING: Standard exception raised: " << ex.what() << std::endl;
        return 1;
    }

    return 0;
}

void pubsub_add_message(void *pubsub_specific_data_p, char *data)
{
    struct pubsub_specific_data *pubsub_specific_data = (struct pubsub_specific_data *)pubsub_specific_data_p;
    google::pubsub::v1::PublishRequest *request = (google::pubsub::v1::PublishRequest *)pubsub_specific_data->request;

    google::pubsub::v1::PubsubMessage *message = request->add_messages();
    message->set_data(data);
}

void pubsub_clear_messages(void *pubsub_specific_data_p)
{
    struct pubsub_specific_data *pubsub_specific_data = (struct pubsub_specific_data *)pubsub_specific_data_p;
    google::pubsub::v1::PublishRequest *request = (google::pubsub::v1::PublishRequest *)pubsub_specific_data->request;

    request->clear_messages();
}

/**
 * Send data to the Pub/Sub service
 *
 * @param pubsub_specific_data_p a pointer to a structure with client and request outcome information.
 * @param stream_name the name of a stream to send to.
 * @param partition_key a partition key which automatically maps data to a specific stream.
 * @param data a data buffer to send to the stream.
 * @param data_len the length of the data buffer.
 */
void pubsub_publish(void *pubsub_specific_data_p)
{
    struct pubsub_specific_data *pubsub_specific_data = (struct pubsub_specific_data *)pubsub_specific_data_p;
    std::unique_ptr<google::pubsub::v1::Publisher::Stub> *stub =
        (std::unique_ptr<google::pubsub::v1::Publisher::Stub> *)pubsub_specific_data->stub;
    google::pubsub::v1::PublishRequest *request = (google::pubsub::v1::PublishRequest *)pubsub_specific_data->request;
    grpc::CompletionQueue *completion_queue = (grpc::CompletionQueue *)pubsub_specific_data->completion_queue;

    grpc::ClientContext context;
    std::unique_ptr<grpc::ClientAsyncResponseReader<google::pubsub::v1::PublishResponse> > rpc(
        (*stub)->AsyncPublish(&context, *request, completion_queue));

    struct response *response = new struct response;
    response->publish_response = new google::pubsub::v1::PublishResponse;
    response->status = new grpc::Status;
    response->tag = pubsub_specific_data->last_tag++;

    rpc->Finish(response->publish_response, response->status, (void *)response->tag);

    ((std::vector<struct response *> *)(pubsub_specific_data->responses))->push_back(response);
}

/**
 * Get results from service responces
 *
 * @param request_outcomes_p request outcome information.
 * @param error_message report error message to a caller.
 * @param sent_bytes report to a caller how many bytes was successfuly sent.
 * @param lost_bytes report to a caller how many bytes was lost during transmission.
 * @return Returns 0 if all data was sent successfully, 1 when data was lost on transmission
 */
int pubsub_get_result(void *pubsub_specific_data_p, char *error_message, size_t *sent_bytes, size_t *lost_bytes)
{
    struct pubsub_specific_data *pubsub_specific_data = (struct pubsub_specific_data *)pubsub_specific_data_p;
    grpc::CompletionQueue *completion_queue = (grpc::CompletionQueue *)pubsub_specific_data->completion_queue;
    std::vector<struct response *> *responses = (std::vector<struct response *> *)pubsub_specific_data->responses;
    *sent_bytes = 0;
    *lost_bytes = 0;

    while (1) {
        grpc_impl::CompletionQueue::NextStatus next_status;
        void *got_tag;
        bool ok = false;

        auto deadline = std::chrono::system_clock::now() + std::chrono::milliseconds(EVENT_CHECK_TIMEOUT);
        next_status = completion_queue->AsyncNext(&got_tag, &ok, deadline);

        if (next_status == grpc::CompletionQueue::GOT_EVENT) {
            struct response *response = nullptr;
            google::pubsub::v1::PublishResponse *publish_response = nullptr;

            for (auto &current_response : *responses) {
                if ((void *)current_response->tag == got_tag) {
                    publish_response = current_response->publish_response;
                    response = current_response;
                    break;
                }
            }

            if (!response) {
                strncpy(error_message, "EXPORTING: Cannot get Pub/Sub response", ERROR_LINE_MAX);
                return 1;
            }

            if (ok) {
                // *sent_metrics +=
                publish_response->message_ids_size();
                // *sent_bytes += response->data_len;             // TODO
            } else {
                // *lost_metrics +=
                publish_response->message_ids_size();
                // *lost_bytes += response->data_len;             // TODO
                response->status->error_message().copy(error_message, ERROR_LINE_MAX);
            }

            std::remove(responses->begin(), responses->end(), response);
            delete response;
        } else {
            break;
        }
    }

    if (*lost_bytes) {
        return 1;
    }

    return 0;
}
