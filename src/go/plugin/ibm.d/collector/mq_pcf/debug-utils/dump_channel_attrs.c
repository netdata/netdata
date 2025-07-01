// dump_channel_attrs.c - Dump all attributes returned by MQCMD_INQUIRE_CHANNEL
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <cmqc.h>
#include <cmqxc.h>
#include <cmqcfc.h>

// Helper to get attribute name
const char* getAttrName(MQLONG attr) {
    switch(attr) {
        // Channel identification
        case MQCACH_CHANNEL_NAME: return "MQCACH_CHANNEL_NAME";
        case MQCACH_DESC: return "MQCACH_DESC";
        case MQIACH_CHANNEL_TYPE: return "MQIACH_CHANNEL_TYPE";
        
        // Channel status attributes
        case MQIACH_CHANNEL_STATUS: return "MQIACH_CHANNEL_STATUS";
        case MQIACH_CHANNEL_INSTANCE_TYPE: return "MQIACH_CHANNEL_INSTANCE_TYPE";
        case MQIACH_CHANNEL_INSTANCE_ATTRS: return "MQIACH_CHANNEL_INSTANCE_ATTRS";
        case MQCACH_CHANNEL_START_DATE: return "MQCACH_CHANNEL_START_DATE";
        case MQCACH_CHANNEL_START_TIME: return "MQCACH_CHANNEL_START_TIME";
        
        // Message counts
        case MQIACH_MSGS: return "MQIACH_MSGS";
        case MQIACH_BYTES_SENT: return "MQIACH_BYTES_SENT";
        case MQIACH_BYTES_RECEIVED: return "MQIACH_BYTES_RECEIVED";
        case MQIACH_BATCHES: return "MQIACH_BATCHES";
        case MQIACH_BUFFERS_SENT: return "MQIACH_BUFFERS_SENT";
        case MQIACH_BUFFERS_RECEIVED: return "MQIACH_BUFFERS_RECEIVED";
        
        // Batch configuration
        case MQIACH_BATCH_SIZE: return "MQIACH_BATCH_SIZE";
        case MQIACH_BATCH_HB: return "MQIACH_BATCH_HB";
        case MQIACH_BATCH_INTERVAL: return "MQIACH_BATCH_INTERVAL";
        case MQIACH_NPM_SPEED: return "MQIACH_NPM_SPEED";
        
        // Retry configuration
        case MQIACH_SHORT_RETRY: return "MQIACH_SHORT_RETRY";
        case MQIACH_SHORT_TIMER: return "MQIACH_SHORT_TIMER";
        case MQIACH_LONG_RETRY: return "MQIACH_LONG_RETRY";
        case MQIACH_LONG_TIMER: return "MQIACH_LONG_TIMER";
        
        // Timeouts
        case MQIACH_DISC_INTERVAL: return "MQIACH_DISC_INTERVAL";
        case MQIACH_HB_INTERVAL: return "MQIACH_HB_INTERVAL";
        case MQIACH_KEEP_ALIVE_INTERVAL: return "MQIACH_KEEP_ALIVE_INTERVAL";
        
        // Other configuration
        case MQIACH_MCA_TYPE: return "MQIACH_MCA_TYPE";
        case MQIACH_MAX_MSG_LENGTH: return "MQIACH_MAX_MSG_LENGTH";
        case MQIACH_SHARING_CONVERSATIONS: return "MQIACH_SHARING_CONVERSATIONS";
        case MQIACH_NETWORK_PRIORITY: return "MQIACH_NETWORK_PRIORITY";
        case MQIACH_DATA_CONVERSION: return "MQIACH_DATA_CONVERSION";
        case MQIACH_MSG_SEQUENCE_NUMBER: return "MQIACH_MSG_SEQUENCE_NUMBER";
        case MQIACH_SSL_CLIENT_AUTH: return "MQIACH_SSL_CLIENT_AUTH";
        case MQIACH_PUT_AUTHORITY: return "MQIACH_PUT_AUTHORITY";
        case MQIACH_SEQUENCE_NUMBER_WRAP: return "MQIACH_SEQUENCE_NUMBER_WRAP";
        case MQIACH_MAX_INSTANCES: return "MQIACH_MAX_INSTANCES";
        case MQIACH_MAX_INSTS_PER_CLIENT: return "MQIACH_MAX_INSTS_PER_CLIENT";
        case MQIACH_CLWL_CHANNEL_RANK: return "MQIACH_CLWL_CHANNEL_RANK";
        case MQIACH_CLWL_CHANNEL_PRIORITY: return "MQIACH_CLWL_CHANNEL_PRIORITY";
        case MQIACH_CLWL_CHANNEL_WEIGHT: return "MQIACH_CLWL_CHANNEL_WEIGHT";
        case MQIACH_CHANNEL_DISP: return "MQIACH_CHANNEL_DISP";
        case MQIACH_INBOUND_DISP: return "MQIACH_INBOUND_DISP";
        case MQIACH_CHANNEL_TYPES: return "MQIACH_CHANNEL_TYPES";
        case MQIACH_AMQP_KEEP_ALIVE: return "MQIACH_AMQP_KEEP_ALIVE";
        case MQIACH_USE_CLIENT_ID: return "MQIACH_USE_CLIENT_ID";
        case MQIACH_CLIENT_CHANNEL_WEIGHT: return "MQIACH_CLIENT_CHANNEL_WEIGHT";
        case MQIACH_CONNECTION_AFFINITY: return "MQIACH_CONNECTION_AFFINITY";
        case MQIACH_RESET_REQUESTED: return "MQIACH_RESET_REQUESTED";
        case MQIACH_BATCH_DATA_LIMIT: return "MQIACH_BATCH_DATA_LIMIT";
        case MQIACH_MSG_HISTORY: return "MQIACH_MSG_HISTORY";
        case MQIACH_MULTICAST_PROPERTIES: return "MQIACH_MULTICAST_PROPERTIES";
        case MQIACH_NEW_SUBSCRIBER_HISTORY: return "MQIACH_NEW_SUBSCRIBER_HISTORY";
        case MQIACH_MC_HB_INTERVAL: return "MQIACH_MC_HB_INTERVAL";
        case MQIACH_PORT: return "MQIACH_PORT";
        case MQIACH_COMPRESSION_RATE: return "MQIACH_COMPRESSION_RATE";
        case MQIACH_COMPRESSION_TIME: return "MQIACH_COMPRESSION_TIME";
        case MQIACH_EXIT_TIME_INDICATOR: return "MQIACH_EXIT_TIME_INDICATOR";
        case MQIACH_HDR_COMPRESSION: return "MQIACH_HDR_COMPRESSION";
        case MQIACH_MSG_COMPRESSION: return "MQIACH_MSG_COMPRESSION";
        case MQIACH_CHANNEL_SUMMARY: return "MQIACH_CHANNEL_SUMMARY";
        case MQIACH_XMITQ_TIME_INDICATOR: return "MQIACH_XMITQ_TIME_INDICATOR";
        case MQIACH_IN_DOUBT: return "MQIACH_IN_DOUBT";
        case MQIACH_MCA_JOB_TYPE: return "MQIACH_MCA_JOB_TYPE";
        case MQIACH_NETWORK_TIME_INDICATOR: return "MQIACH_NETWORK_TIME_INDICATOR";
        case MQIACH_STOP_REQUESTED: return "MQIACH_STOP_REQUESTED";
        case MQIACH_MR_COUNT: return "MQIACH_MR_COUNT";
        case MQIACH_MR_INTERVAL: return "MQIACH_MR_INTERVAL";
        case MQIACH_NPM_SPEEDS: return "MQIACH_NPM_SPEEDS";
        case MQIACH_HB_INTERVAL: return "MQIACH_HB_INTERVAL";
        case MQIACH_BATCH_INTERVAL: return "MQIACH_BATCH_INTERVAL";
        case MQIACH_NETWORK_PRIORITY: return "MQIACH_NETWORK_PRIORITY";
        case MQIACH_DISC_INTERVAL: return "MQIACH_DISC_INTERVAL";
        case MQIACH_SHORT_TIMER: return "MQIACH_SHORT_TIMER";
        case MQIACH_SHORT_RETRY: return "MQIACH_SHORT_RETRY";
        case MQIACH_LONG_TIMER: return "MQIACH_LONG_TIMER";
        case MQIACH_LONG_RETRY: return "MQIACH_LONG_RETRY";
        case MQIACH_PUT_AUTHORITY: return "MQIACH_PUT_AUTHORITY";
        case MQIACH_CHANNEL_SUBSTATE: return "MQIACH_CHANNEL_SUBSTATE";
        case MQIACH_SSL_RETURN_CODE: return "MQIACH_SSL_RETURN_CODE";
        case MQIACH_XMITQ_MSGS_AVAILABLE: return "MQIACH_XMITQ_MSGS_AVAILABLE";
        case MQIACH_ACTIVE_CHL: return "MQIACH_ACTIVE_CHL";
        case MQIACH_AVG_BATCH_SIZE: return "MQIACH_AVG_BATCH_SIZE";
        case MQIACH_CUR_BATCH_SIZE: return "MQIACH_CUR_BATCH_SIZE";
        case MQIACH_CUR_SEQ_NUMBER: return "MQIACH_CUR_SEQ_NUMBER";
        case MQIACH_IN_DOUBT_IN: return "MQIACH_IN_DOUBT_IN";
        case MQIACH_IN_DOUBT_OUT: return "MQIACH_IN_DOUBT_OUT";
        case MQIACH_LAST_SEQ_NUMBER: return "MQIACH_LAST_SEQ_NUMBER";
        case MQIACH_LONG_RETRIES_LEFT: return "MQIACH_LONG_RETRIES_LEFT";
        case MQIACH_MCA_STATUS: return "MQIACH_MCA_STATUS";
        case MQIACH_MSGS_RCVD: return "MQIACH_MSGS_RCVD";
        case MQIACH_MSGS_SENT: return "MQIACH_MSGS_SENT";
        case MQIACH_PENDING_COMMITS: return "MQIACH_PENDING_COMMITS";
        case MQIACH_RUNNING_MCA: return "MQIACH_RUNNING_MCA";
        case MQIACH_SHORT_RETRIES_LEFT: return "MQIACH_SHORT_RETRIES_LEFT";
        case MQIACH_BATCHES: return "MQIACH_BATCHES";
        case MQIACH_BUFFERS_RCVD: return "MQIACH_BUFFERS_RCVD";
        case MQIACH_BUFFERS_SENT: return "MQIACH_BUFFERS_SENT";
        case MQIACH_BYTES_RCVD: return "MQIACH_BYTES_RCVD";
        case MQIACH_BYTES_SENT: return "MQIACH_BYTES_SENT";
        case MQIACH_INDOUBT_STATUS: return "MQIACH_INDOUBT_STATUS";
        
        // String attributes
        case MQCACH_CONNECTION_NAME: return "MQCACH_CONNECTION_NAME";
        case MQCACH_XMIT_Q_NAME: return "MQCACH_XMIT_Q_NAME";
        case MQCACH_MCA_NAME: return "MQCACH_MCA_NAME";
        case MQCACH_MCA_USER_ID: return "MQCACH_MCA_USER_ID";
        case MQCACH_SSL_CIPHER_SPEC: return "MQCACH_SSL_CIPHER_SPEC";
        case MQCACH_SSL_PEER_NAME: return "MQCACH_SSL_PEER_NAME";
        case MQCACH_SSL_HANDSHAKE_STAGE: return "MQCACH_SSL_HANDSHAKE_STAGE";
        case MQCACH_SSL_SHORT_PEER_NAME: return "MQCACH_SSL_SHORT_PEER_NAME";
        case MQCACH_REMOTE_APPL_TAG: return "MQCACH_REMOTE_APPL_TAG";
        case MQCACH_CLUSTER_NAME: return "MQCACH_CLUSTER_NAME";
        case MQCACH_CLUSTER_NAMELIST: return "MQCACH_CLUSTER_NAMELIST";
        case MQCACH_NETWORK_APPLID: return "MQCACH_NETWORK_APPLID";
        case MQCACH_EXIT_NAME: return "MQCACH_EXIT_NAME";
        case MQCACH_MSG_EXIT_NAME: return "MQCACH_MSG_EXIT_NAME";
        case MQCACH_SEND_EXIT_NAME: return "MQCACH_SEND_EXIT_NAME";
        case MQCACH_RCV_EXIT_NAME: return "MQCACH_RCV_EXIT_NAME";
        case MQCACH_CHANNEL_NAMES: return "MQCACH_CHANNEL_NAMES";
        case MQCACH_LAST_MSG_TIME: return "MQCACH_LAST_MSG_TIME";
        case MQCACH_LAST_MSG_DATE: return "MQCACH_LAST_MSG_DATE";
        case MQCACH_MCA_JOB_NAME: return "MQCACH_MCA_JOB_NAME";
        case MQCACH_STOP_TIME: return "MQCACH_STOP_TIME";
        case MQCACH_STOP_DATE: return "MQCACH_STOP_DATE";
        case MQCACH_REMOTE_Q_MGR_NAME: return "MQCACH_REMOTE_Q_MGR_NAME";
        case MQCACH_MCA_SECURITY_ID: return "MQCACH_MCA_SECURITY_ID";
        case MQCACH_LU_NAME: return "MQCACH_LU_NAME";
        case MQCACH_IP_ADDRESS: return "MQCACH_IP_ADDRESS";
        case MQCACH_TCP_NAME: return "MQCACH_TCP_NAME";
        case MQCACH_LOCAL_ADDRESS: return "MQCACH_LOCAL_ADDRESS";
        case MQCACH_LOCAL_NAME: return "MQCACH_LOCAL_NAME";
        case MQCACH_REMOTE_ADDRESS: return "MQCACH_REMOTE_ADDRESS";
        case MQCACH_REMOTE_NAME: return "MQCACH_REMOTE_NAME";
        case MQCACH_REMOTE_PRODUCT: return "MQCACH_REMOTE_PRODUCT";
        case MQCACH_REMOTE_VERSION: return "MQCACH_REMOTE_VERSION";
        case MQCACH_CURRENT_LUWID: return "MQCACH_CURRENT_LUWID";
        case MQCACH_LAST_LUWID: return "MQCACH_LAST_LUWID";
        case MQCACH_PASSWORD: return "MQCACH_PASSWORD";
        case MQCACH_SSL_KEY_PASSPHRASE: return "MQCACH_SSL_KEY_PASSPHRASE";
        case MQCACH_JAAS_CONFIG: return "MQCACH_JAAS_CONFIG";
        case MQCACH_SSL_KEY_RESET_DATE: return "MQCACH_SSL_KEY_RESET_DATE";
        case MQCACH_SSL_KEY_RESET_TIME: return "MQCACH_SSL_KEY_RESET_TIME";
        case MQCACH_CURRENT_MSGS: return "MQCACH_CURRENT_MSGS";
        case MQCACH_INDOUBT_MSGS: return "MQCACH_INDOUBT_MSGS";
        case MQCACH_FORMAT_NAME: return "MQCACH_FORMAT_NAME";
        case MQCACH_MR_EXIT_NAME: return "MQCACH_MR_EXIT_NAME";
        case MQCACH_MR_EXIT_USER_DATA: return "MQCACH_MR_EXIT_USER_DATA";
        case MQCACH_MSG_EXIT_NAME: return "MQCACH_MSG_EXIT_NAME";
        case MQCACH_MSG_EXIT_USER_DATA: return "MQCACH_MSG_EXIT_USER_DATA";
        case MQCACH_MSG_USER_DATA: return "MQCACH_MSG_USER_DATA";
        case MQCACH_RCV_EXIT_NAME: return "MQCACH_RCV_EXIT_NAME";
        case MQCACH_RCV_EXIT_USER_DATA: return "MQCACH_RCV_EXIT_USER_DATA";
        case MQCACH_SEC_EXIT_NAME: return "MQCACH_SEC_EXIT_NAME";
        case MQCACH_SEC_EXIT_USER_DATA: return "MQCACH_SEC_EXIT_USER_DATA";
        case MQCACH_SEND_EXIT_NAME: return "MQCACH_SEND_EXIT_NAME";
        case MQCACH_SEND_EXIT_USER_DATA: return "MQCACH_SEND_EXIT_USER_DATA";
        case MQCACH_USER_ID: return "MQCACH_USER_ID";
        
        default: return NULL;
    }
}

void print_usage(const char *program) {
    printf("Usage: %s <queue_manager> <channel_name> [host] [port] [channel] [user] [password]\n", program);
    printf("  queue_manager: Name of the queue manager (required)\n");
    printf("  channel_name:  Name of the channel to inquire (required)\n");
    printf("  host:          Host name (default: localhost)\n");
    printf("  port:          Port number (default: 1414)\n");
    printf("  channel:       Channel name (default: SYSTEM.DEF.SVRCONN)\n");
    printf("  user:          User name (optional)\n");
    printf("  password:      Password (optional)\n");
}

int main(int argc, char *argv[]) {
    if (argc < 3) {
        print_usage(argv[0]);
        return 1;
    }
    
    // Parse arguments
    char *qmgr = argv[1];
    char *channelName = argv[2];
    char *host = argc > 3 ? argv[3] : "localhost";
    int port = argc > 4 ? atoi(argv[4]) : 1414;
    char *channel = argc > 5 ? argv[5] : "SYSTEM.DEF.SVRCONN";
    char *user = argc > 6 ? argv[6] : NULL;
    char *password = argc > 7 ? argv[7] : NULL;
    
    MQHCONN hConn = MQHC_UNUSABLE_HCONN;
    MQHOBJ hObj = MQHO_UNUSABLE_HOBJ;
    MQHOBJ hReplyQ = MQHO_UNUSABLE_HOBJ;
    MQLONG compCode, reason;
    MQOD od = {MQOD_DEFAULT};
    MQMD md = {MQMD_DEFAULT};
    MQPMO pmo = {MQPMO_DEFAULT};
    MQGMO gmo = {MQGMO_DEFAULT};
    MQCNO cno = {MQCNO_DEFAULT};
    MQCD cd = {MQCD_CLIENT_CONN_DEFAULT};
    MQCSP csp = {MQCSP_DEFAULT};
    
    // Set up client connection
    cno.Version = MQCNO_VERSION_4;
    cno.Options = MQCNO_CLIENT_BINDING;
    
    cd.ChannelType = MQCHT_CLNTCONN;
    cd.TransportType = MQXPT_TCP;
    cd.Version = MQCD_VERSION_6;
    strncpy(cd.ChannelName, channel, MQ_CHANNEL_NAME_LENGTH);
    sprintf(cd.ConnectionName, "%s(%d)", host, port);
    
    cno.ClientConnPtr = &cd;
    
    // Set up authentication if provided
    if (user && password) {
        cno.Version = MQCNO_VERSION_5;
        csp.AuthenticationType = MQCSP_AUTH_USER_ID_AND_PWD;
        csp.CSPUserIdPtr = user;
        csp.CSPUserIdLength = strlen(user);
        csp.CSPPasswordPtr = password;
        csp.CSPPasswordLength = strlen(password);
        cno.SecurityParmsPtr = &csp;
    }
    
    // Connect
    MQCONNX(qmgr, &cno, &hConn, &compCode, &reason);
    if (compCode != MQCC_OK) {
        printf("MQCONNX failed: CompCode=%d, Reason=%d\n", compCode, reason);
        return 1;
    }
    printf("Connected to %s on %s:%d via %s\n", qmgr, host, port, channel);
    
    // Open command queue
    strcpy(od.ObjectName, "SYSTEM.ADMIN.COMMAND.QUEUE");
    MQOPEN(hConn, &od, MQOO_OUTPUT, &hObj, &compCode, &reason);
    if (compCode != MQCC_OK) {
        printf("MQOPEN command queue failed: CompCode=%d, Reason=%d\n", compCode, reason);
        MQDISC(&hConn, &compCode, &reason);
        return 1;
    }
    
    // Open reply queue
    memset(&od, 0, sizeof(od));
    strcpy(od.ObjectName, "SYSTEM.DEFAULT.MODEL.QUEUE");
    strcpy(od.DynamicQName, "DEBUG.REPLY.*");
    MQOPEN(hConn, &od, MQOO_INPUT_AS_Q_DEF, &hReplyQ, &compCode, &reason);
    if (compCode != MQCC_OK) {
        printf("MQOPEN reply queue failed: CompCode=%d, Reason=%d\n", compCode, reason);
        MQCLOSE(hConn, &hObj, MQCO_NONE, &compCode, &reason);
        MQDISC(&hConn, &compCode, &reason);
        return 1;
    }
    
    // Build INQUIRE_CHANNEL PCF command
    char buffer[65536];
    MQCFH *cfh = (MQCFH *)buffer;
    cfh->Type = MQCFT_COMMAND;
    cfh->StrucLength = MQCFH_STRUC_LENGTH;
    cfh->Version = MQCFH_VERSION_1;
    cfh->Command = MQCMD_INQUIRE_CHANNEL;
    cfh->MsgSeqNumber = 1;
    cfh->Control = MQCFC_LAST;
    cfh->ParameterCount = 1;
    
    // Add channel name parameter
    MQCFST *cfst = (MQCFST *)(buffer + MQCFH_STRUC_LENGTH);
    cfst->Type = MQCFT_STRING;
    cfst->StrucLength = MQCFST_STRUC_LENGTH_FIXED + 20;  // Fixed part + 20 chars
    cfst->Parameter = MQCACH_CHANNEL_NAME;
    cfst->CodedCharSetId = MQCCSI_DEFAULT;
    cfst->StringLength = strlen(channelName);
    memset(cfst->String, ' ', 20);
    memcpy(cfst->String, channelName, strlen(channelName));
    
    // Send command
    strcpy(md.Format, MQFMT_ADMIN);
    md.MsgType = MQMT_REQUEST;
    memcpy(md.ReplyToQ, od.ObjectName, 48);
    
    MQPUT(hConn, hObj, &md, &pmo, MQCFH_STRUC_LENGTH + cfst->StrucLength, 
          buffer, &compCode, &reason);
    if (compCode != MQCC_OK) {
        printf("MQPUT failed: CompCode=%d, Reason=%d\n", compCode, reason);
        goto cleanup;
    }
    
    // Get response
    memcpy(md.CorrelId, md.MsgId, sizeof(md.MsgId));
    memset(md.MsgId, 0, sizeof(md.MsgId));
    gmo.Options = MQGMO_WAIT | MQGMO_CONVERT;
    gmo.WaitInterval = 5000;
    
    MQLONG bufLen = sizeof(buffer);
    MQGET(hConn, hReplyQ, &md, &gmo, bufLen, buffer, &bufLen, &compCode, &reason);
    if (compCode != MQCC_OK) {
        printf("MQGET failed: CompCode=%d, Reason=%d\n", compCode, reason);
        goto cleanup;
    }
    
    // Parse response
    cfh = (MQCFH *)buffer;
    printf("\n=== MQCMD_INQUIRE_CHANNEL Response for: %s ===\n", channelName);
    printf("CompCode=%d, Reason=%d, Parameters=%d\n\n", 
           cfh->CompCode, cfh->Reason, cfh->ParameterCount);
    
    if (cfh->CompCode != MQCC_OK) {
        printf("Command failed!\n");
        goto cleanup;
    }
    
    int offset = MQCFH_STRUC_LENGTH;
    int count = 0;
    int intCount = 0, strCount = 0, unknownCount = 0;
    
    printf("=== Attributes ===\n");
    for (int i = 0; i < cfh->ParameterCount && offset < bufLen; i++) {
        MQLONG type = *(MQLONG *)(buffer + offset);
        
        if (type == MQCFT_INTEGER) {
            MQCFIN *cfin = (MQCFIN *)(buffer + offset);
            const char *name = getAttrName(cfin->Parameter);
            if (name) {
                printf("[%3d] INTEGER: %-40s (%4d) = %d\n", 
                       ++count, name, cfin->Parameter, cfin->Value);
            } else {
                printf("[%3d] INTEGER: UNKNOWN_ATTR_%d             (%4d) = %d\n", 
                       ++count, cfin->Parameter, cfin->Parameter, cfin->Value);
            }
            intCount++;
            offset += cfin->StrucLength;
        }
        else if (type == MQCFT_STRING) {
            MQCFST *cfst = (MQCFST *)(buffer + offset);
            char str[256] = {0};
            int len = cfst->StringLength < 255 ? cfst->StringLength : 255;
            memcpy(str, (char *)(buffer + offset + MQCFST_STRUC_LENGTH_FIXED), len);
            
            // Trim trailing spaces
            while (len > 0 && str[len-1] == ' ') {
                str[--len] = '\0';
            }
            
            const char *name = getAttrName(cfst->Parameter);
            if (name) {
                printf("[%3d] STRING:  %-40s (%4d) = '%s'\n", 
                       ++count, name, cfst->Parameter, str);
            } else {
                printf("[%3d] STRING:  UNKNOWN_ATTR_%d             (%4d) = '%s'\n", 
                       ++count, cfst->Parameter, cfst->Parameter, str);
            }
            strCount++;
            offset += cfst->StrucLength;
        }
        else {
            printf("[%3d] Unknown type %d at offset %d\n", ++count, type, offset);
            unknownCount++;
            break;
        }
    }
    
    printf("\n=== Summary ===\n");
    printf("Total attributes: %d\n", count);
    printf("  Integer attributes: %d\n", intCount);
    printf("  String attributes: %d\n", strCount);
    if (unknownCount > 0) {
        printf("  Unknown types: %d\n", unknownCount);
    }
    
cleanup:
    MQCLOSE(hConn, &hReplyQ, MQCO_DELETE_PURGE, &compCode, &reason);
    MQCLOSE(hConn, &hObj, MQCO_NONE, &compCode, &reason);
    MQDISC(&hConn, &compCode, &reason);
    return 0;
}