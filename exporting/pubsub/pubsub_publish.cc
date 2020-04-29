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
 * @param project_id a project ID.
 * @param topic_id a topic ID.
 * @return Returns 0 on success, 1 on failure.
 */
int pubsub_init(void *pubsub_specific_data_p, char *project_id, char *topic_id)
{
    struct pubsub_specific_data *pubsub_specific_data = (struct pubsub_specific_data *)pubsub_specific_data_p;

    try {
        setenv("GOOGLE_APPLICATION_CREDENTIALS", "/etc/netdata/netdata-analytics-ml-921ae1ba188f.json", 0);

        std::shared_ptr<grpc::ChannelCredentials> credentials = grpc::GoogleDefaultCredentials();
        if (credentials == nullptr) {
            std::cerr << "EXPORTING: Can't load credentials" << std::endl;
            return 1;
        }

        std::shared_ptr<grpc::Channel> channel = grpc::CreateChannel("pubsub.googleapis.com", credentials);

        google::pubsub::v1::Publisher::Stub *stub = new google::pubsub::v1::Publisher::Stub(channel);

        if (!stub)
            std::cerr << "EXPORTING: Can't create a publisher stub" << std::endl;

        pubsub_specific_data->stub = stub;

        google::pubsub::v1::PublishRequest *request = new google::pubsub::v1::PublishRequest;
        pubsub_specific_data->request = request;
        ((google::pubsub::v1::PublishRequest *)(pubsub_specific_data->request))
            ->set_topic(std::string("projects/") + project_id + "/topics/" + topic_id);

        grpc::CompletionQueue *cq = new grpc::CompletionQueue;
        pubsub_specific_data->completion_queue = cq;

        pubsub_specific_data->responses = new std::vector<struct response>;

        return 0;
    } catch (std::exception const &ex) {
        std::cerr << "EXPORTING: Standard exception raised: " << ex.what() << std::endl;
        return 1;
    }

    return 0;
}

/**
 * Add data to a Pub/Sub request message.
 *
 * @param pubsub_specific_data_p a pointer to a structure with instance-wide data.
 * @param data a text buffer with metrics.
 */
void pubsub_add_message(void *pubsub_specific_data_p, char *data)
{
    struct pubsub_specific_data *pubsub_specific_data = (struct pubsub_specific_data *)pubsub_specific_data_p;

    google::pubsub::v1::PubsubMessage *message =
        ((google::pubsub::v1::PublishRequest *)(pubsub_specific_data->request))->add_messages();

    message->set_data(data);
}

/**
 * Remove all messages from a Pub/Sub request.
 *
 * @param pubsub_specific_data_p a pointer to a structure with instance-wide data.
 */
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
 */
void pubsub_publish(void *pubsub_specific_data_p)
{
    struct pubsub_specific_data *pubsub_specific_data = (struct pubsub_specific_data *)pubsub_specific_data_p;

    grpc::ClientContext *context = new grpc::ClientContext;

    std::cerr << "EXPORTING: Publish" << std::endl;
    std::unique_ptr<grpc::ClientAsyncResponseReader<google::pubsub::v1::PublishResponse> > rpc(
        ((google::pubsub::v1::Publisher::Stub *)(pubsub_specific_data->stub))
            ->AsyncPublish(
                context, (*(google::pubsub::v1::PublishRequest *)(pubsub_specific_data->request)),
                ((grpc::CompletionQueue *)(pubsub_specific_data->completion_queue))));

    struct response response;
    response.publish_response = new google::pubsub::v1::PublishResponse;
    response.tag = pubsub_specific_data->last_tag++;
    response.status = new grpc::Status;

    rpc->Finish(response.publish_response, response.status, (void *)response.tag);

    ((google::pubsub::v1::PublishRequest *)(pubsub_specific_data->request))->clear_messages();

    ((std::vector<struct response> *)(pubsub_specific_data->responses))->push_back(response);
}

/**
 * Get results from service responces
 *
 * @param pubsub_specific_data_p a pointer to a structure with instance-wide data.
 * @param error_message report error message to a caller.
 * @param sent_bytes report to a caller how many bytes was successfuly sent.
 * @param lost_bytes report to a caller how many bytes was lost during transmission.
 * @return Returns 0 if all data was sent successfully, 1 when data was lost on transmission.
 */
int pubsub_get_result(
    void *pubsub_specific_data_p, char *error_message,
    size_t *sent_metrics, size_t *sent_bytes, size_t *lost_metrics, size_t *lost_bytes)
{
    struct pubsub_specific_data *pubsub_specific_data = (struct pubsub_specific_data *)pubsub_specific_data_p;
    std::vector<struct response> *responses = (std::vector<struct response> *)pubsub_specific_data->responses;
    grpc_impl::CompletionQueue::NextStatus next_status;

    *sent_metrics = 0;
    *sent_bytes = 0;
    *lost_metrics = 0;
    *lost_bytes = 0;

    do {
        std::vector<struct response>::iterator response;
        void *got_tag;
        bool ok = false;

        auto deadline = std::chrono::system_clock::now() + std::chrono::milliseconds(50);
        next_status =
            (*(grpc::CompletionQueue *)(pubsub_specific_data->completion_queue)).AsyncNext(&got_tag, &ok, deadline);
        std::cerr << "EXPORTING: Status = " << next_status << std::endl;

        if (next_status == grpc::CompletionQueue::GOT_EVENT) {
            for (response = responses->begin(); response != responses->end(); ++response) {
                if ((void *)response->tag == got_tag)
                    break;
            }

            if (response == responses->end()) {
                strncpy(error_message, "EXPORTING: Cannot get Pub/Sub response", ERROR_LINE_MAX);
                return 1;
            }

            if (ok) {
                std::cerr << "EXPORTING: Got right tag and status is OK" << std::endl;
                for (auto s : response->publish_response->message_ids())
                    std::cerr << "EXPORTING: Message id = " << s << std::endl;
                *sent_metrics += response->publish_response->message_ids_size();
                // *sent_bytes += response->data_len;             // TODO
            } else {
                std::cerr << "EXPORTING: Status is not OK" << std::endl;
                std::cerr << "EXPORTING: " << response->status->error_code() << ": " << response->status->error_message() << std::endl;
                *lost_metrics += response->publish_response->message_ids_size();
                // *lost_bytes += response->data_len;             // TODO
                response->status->error_message().copy(error_message, ERROR_LINE_MAX);
            }

            responses->erase(response);
        }

        if (next_status == grpc::CompletionQueue::SHUTDOWN) {
            std::cerr << "EXPORTING: Shutdown" << std::endl;
            return 1;
        }

    } while (next_status == grpc::CompletionQueue::GOT_EVENT);


    if (*lost_bytes) {
        return 1;
    }

    return 0;
}
