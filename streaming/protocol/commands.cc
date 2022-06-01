#include <google/protobuf/io/zero_copy_stream_impl.h>
#include "proto/command.pb.h"
#include "commands.h"

struct CommandEncoder {
    std::string Buf;
};

command_encoder_t command_encoder_new() {
    return new CommandEncoder();
}

void command_encoder_delete(command_encoder_t cmd_encoder) {
    CommandEncoder *CmdEncoder = reinterpret_cast<CommandEncoder *>(cmd_encoder);
    delete CmdEncoder;
}

bool command_request_fill_gap(command_encoder_t cmd_encoder, time_t After, time_t Before) {
#if 0
    CommandEncoder *CmdEncoder = reinterpret_cast<CommandEncoder *>(cmd_encoder);

    protocol::Command Cmd;
    Cmd.set_type(protocol::CommandType::REQUEST_FILL_GAP);

    protocol::RequestFillGap *RFG = Cmd.mutable_fillgaprequest();
    RFG->set_after(After);
    RFG->set_before(Before);

    google::protobuf::io::StringOutputStream SOS(&CmdEncoder->Buf);
    google::protobuf::io::CodedOutputStream COS(&SOS);

    COS.WriteVarint32(Cmd.ByteSizeLong());
    Cmd.SerializeToCodedStream(&COS);

    return true;
#else
    UNUSED(cmd_encoder);
    UNUSED(After);
    UNUSED(Before);
    return true;
#endif
}
