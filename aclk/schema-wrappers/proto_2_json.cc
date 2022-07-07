#include <google/protobuf/message.h>
#include <google/protobuf/util/json_util.h>

#include "proto/alarm/v1/config.pb.h"
#include "proto/alarm/v1/stream.pb.h"
#include "proto/aclk/v1/lib.pb.h"
#include "proto/chart/v1/config.pb.h"
#include "proto/chart/v1/stream.pb.h"
#include "proto/agent/v1/connection.pb.h"
#include "proto/agent/v1/disconnect.pb.h"
#include "proto/nodeinstance/connection/v1/connection.pb.h"
#include "proto/nodeinstance/create/v1/creation.pb.h"
#include "proto/nodeinstance/info/v1/info.pb.h"

#include "libnetdata/libnetdata.h"

#include "proto_2_json.h"

using namespace google::protobuf::util;

static google::protobuf::Message *msg_name_to_protomsg(const char *msgname)
{
//tx side
    if (!strcmp(msgname, "UpdateAgentConnection"))
        return new agent::v1::UpdateAgentConnection;
    if (!strcmp(msgname, "UpdateNodeInstanceConnection"))
        return new nodeinstance::v1::UpdateNodeInstanceConnection;
    if (!strcmp(msgname, "CreateNodeInstance"))
        return new nodeinstance::create::v1::CreateNodeInstance;
    if (!strcmp(msgname, "ChartsAndDimensionsUpdated"))
        return new chart::v1::ChartsAndDimensionsUpdated;
    if (!strcmp(msgname, "ChartConfigsUpdated"))
        return new chart::v1::ChartConfigsUpdated;
    if (!strcmp(msgname, "ResetChartMessages"))
        return new chart::v1::ResetChartMessages;
    if (!strcmp(msgname, "RetentionUpdated"))
        return new chart::v1::RetentionUpdated;
    if (!strcmp(msgname, "UpdateNodeInfo"))
        return new nodeinstance::info::v1::UpdateNodeInfo;
    if (!strcmp(msgname, "AlarmLogHealth"))
        return new alarms::v1::AlarmLogHealth;
    if (!strcmp(msgname, "ProvideAlarmConfiguration"))
        return new alarms::v1::ProvideAlarmConfiguration;
    if (!strcmp(msgname, "AlarmSnapshot"))
        return new alarms::v1::AlarmSnapshot;
    if (!strcmp(msgname, "AlarmLogEntry"))
        return new alarms::v1::AlarmLogEntry;

//rx side
    if (!strcmp(msgname, "CreateNodeInstanceResult"))
        return new nodeinstance::create::v1::CreateNodeInstanceResult;
    if (!strcmp(msgname, "SendNodeInstances"))
        return new agent::v1::SendNodeInstances;
    if (!strcmp(msgname, "StreamChartsAndDimensions"))
        return new chart::v1::StreamChartsAndDimensions;
    if (!strcmp(msgname, "ChartsAndDimensionsAck"))
        return new chart::v1::ChartsAndDimensionsAck;
    if (!strcmp(msgname, "UpdateChartConfigs"))
        return new chart::v1::UpdateChartConfigs;
    if (!strcmp(msgname, "StartAlarmStreaming"))
        return new alarms::v1::StartAlarmStreaming;
    if (!strcmp(msgname, "SendAlarmLogHealth"))
        return new alarms::v1::SendAlarmLogHealth;
    if (!strcmp(msgname, "SendAlarmConfiguration"))
        return new alarms::v1::SendAlarmConfiguration;
    if (!strcmp(msgname, "SendAlarmSnapshot"))
        return new alarms::v1::SendAlarmSnapshot;
    if (!strcmp(msgname, "DisconnectReq"))
        return new agent::v1::DisconnectReq;

    return NULL;
}

char *protomsg_to_json(const void *protobin, size_t len, const char *msgname)
{
    google::protobuf::Message *msg = msg_name_to_protomsg(msgname);
    if (msg == NULL)
        return strdupz("Don't know this message type by name.");

    if (!msg->ParseFromArray(protobin, len))
        return strdupz("Can't parse this message. Malformed or wrong parser used.");

    JsonPrintOptions options;

    std::string output;
    google::protobuf::util::MessageToJsonString(*msg, &output, options);
    delete msg;
    return strdupz(output.c_str());
}
