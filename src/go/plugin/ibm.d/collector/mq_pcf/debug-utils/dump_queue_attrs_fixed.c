// dump_queue_attrs_fixed.c - Version using dedicated reply queue
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <cmqc.h>
#include <cmqxc.h>
#include <cmqcfc.h>

// Helper to get attribute name
const char* getAttrName(MQLONG attr) {
    switch(attr) {
        // Queue identification
        case MQCA_Q_NAME: return "MQCA_Q_NAME";
        case MQCA_Q_DESC: return "MQCA_Q_DESC";
        case MQIA_Q_TYPE: return "MQIA_Q_TYPE";
        
        // Queue depth and limits
        case MQIA_CURRENT_Q_DEPTH: return "MQIA_CURRENT_Q_DEPTH";
        case MQIA_MAX_Q_DEPTH: return "MQIA_MAX_Q_DEPTH";
        case MQIA_Q_DEPTH_HIGH_LIMIT: return "MQIA_Q_DEPTH_HIGH_LIMIT";
        case MQIA_Q_DEPTH_LOW_LIMIT: return "MQIA_Q_DEPTH_LOW_LIMIT";
        case MQIA_Q_DEPTH_MAX_EVENT: return "MQIA_Q_DEPTH_MAX_EVENT";
        case MQIA_Q_DEPTH_HIGH_EVENT: return "MQIA_Q_DEPTH_HIGH_EVENT";
        case MQIA_Q_DEPTH_LOW_EVENT: return "MQIA_Q_DEPTH_LOW_EVENT";
        
        // Message counts
        case MQIA_MSG_ENQ_COUNT: return "MQIA_MSG_ENQ_COUNT";
        case MQIA_MSG_DEQ_COUNT: return "MQIA_MSG_DEQ_COUNT";
        
        // Open handles
        case MQIA_OPEN_INPUT_COUNT: return "MQIA_OPEN_INPUT_COUNT";
        case MQIA_OPEN_OUTPUT_COUNT: return "MQIA_OPEN_OUTPUT_COUNT";
        
        // Queue configuration
        case MQIA_BACKOUT_THRESHOLD: return "MQIA_BACKOUT_THRESHOLD";
        case MQIA_SHAREABILITY: return "MQIA_SHAREABILITY";
        case MQIA_DEF_INPUT_OPEN_OPTION: return "MQIA_DEF_INPUT_OPEN_OPTION";
        case MQIA_DEF_PERSISTENCE: return "MQIA_DEF_PERSISTENCE";
        case MQIA_DEF_PRIORITY: return "MQIA_DEF_PRIORITY";
        case MQIA_INHIBIT_GET: return "MQIA_INHIBIT_GET";
        case MQIA_INHIBIT_PUT: return "MQIA_INHIBIT_PUT";
        
        // Triggering
        case MQIA_TRIGGER_CONTROL: return "MQIA_TRIGGER_CONTROL";
        case MQIA_TRIGGER_TYPE: return "MQIA_TRIGGER_TYPE";
        case MQIA_TRIGGER_DEPTH: return "MQIA_TRIGGER_DEPTH";
        case MQIA_TRIGGER_MSG_PRIORITY: return "MQIA_TRIGGER_MSG_PRIORITY";
        case MQCA_TRIGGER_DATA: return "MQCA_TRIGGER_DATA";
        
        // Events and monitoring
        case MQIA_Q_SERVICE_INTERVAL: return "MQIA_Q_SERVICE_INTERVAL";
        case MQIA_Q_SERVICE_INTERVAL_EVENT: return "MQIA_Q_SERVICE_INTERVAL_EVENT";
        case MQIA_ACCOUNTING_Q: return "MQIA_ACCOUNTING_Q";
        case MQIA_MONITORING_Q: return "MQIA_MONITORING_Q";
        case MQIA_STATISTICS_Q: return "MQIA_STATISTICS_Q";
        
        // Other attributes
        case MQIA_USAGE: return "MQIA_USAGE";
        case MQIA_MAX_MSG_LENGTH: return "MQIA_MAX_MSG_LENGTH";
        case MQIA_RETENTION_INTERVAL: return "MQIA_RETENTION_INTERVAL";
        case MQIA_MSG_DELIVERY_SEQUENCE: return "MQIA_MSG_DELIVERY_SEQUENCE";
        case MQIA_DIST_LISTS: return "MQIA_DIST_LISTS";
        case MQIA_INDEX_TYPE: return "MQIA_INDEX_TYPE";
        case MQIA_DEF_BIND: return "MQIA_DEF_BIND";
        case MQIA_DEF_PUT_RESPONSE_TYPE: return "MQIA_DEF_PUT_RESPONSE_TYPE";
        case MQIA_HARDEN_GET_BACKOUT: return "MQIA_HARDEN_GET_BACKOUT";
        case MQIA_NPM_CLASS: return "MQIA_NPM_CLASS";
        case MQIA_DEF_READ_AHEAD: return "MQIA_DEF_READ_AHEAD";
        case MQIA_PROPERTY_CONTROL: return "MQIA_PROPERTY_CONTROL";
        case MQIA_BASE_TYPE: return "MQIA_BASE_TYPE";
        case MQIA_CLWL_Q_RANK: return "MQIA_CLWL_Q_RANK";
        case MQIA_CLWL_Q_PRIORITY: return "MQIA_CLWL_Q_PRIORITY";
        case MQIA_CLWL_USEQ: return "MQIA_CLWL_USEQ";
        case MQIA_SCOPE: return "MQIA_SCOPE";
        
        // String attributes
        case MQCA_BASE_Q_NAME: return "MQCA_BASE_Q_NAME";
        case MQCA_CLUSTER_NAME: return "MQCA_CLUSTER_NAME";
        case MQCA_CLUSTER_NAMELIST: return "MQCA_CLUSTER_NAMELIST";
        case MQCA_ALTERATION_DATE: return "MQCA_ALTERATION_DATE";
        case MQCA_ALTERATION_TIME: return "MQCA_ALTERATION_TIME";
        case MQCA_CREATION_DATE: return "MQCA_CREATION_DATE";
        case MQCA_CREATION_TIME: return "MQCA_CREATION_TIME";
        
        default: return NULL;
    }
}

void print_usage(const char *program) {
    printf("Usage: %s <queue_manager> <queue_name> [host] [port] [channel] [user] [password]\n", program);
    printf("  queue_manager: Name of the queue manager (required)\n");
    printf("  queue_name:    Name of the queue to inquire (required)\n");
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
    char *queueName = argv[2];
    char *host = argc > 3 ? argv[3] : "localhost";
    int port = argc > 4 ? atoi(argv[4]) : 1414;
    char *channel = argc > 5 ? argv[5] : "DEV.APP.SVRCONN";
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
    od.ObjectType = MQOT_Q;
    MQOPEN(hConn, &od, MQOO_OUTPUT | MQOO_FAIL_IF_QUIESCING, &hObj, &compCode, &reason);
    if (compCode != MQCC_OK) {
        printf("MQOPEN command queue failed: CompCode=%d, Reason=%d\n", compCode, reason);
        MQDISC(&hConn, &compCode, &reason);
        return 1;
    }
    
    // Open reply queue (use dedicated queue)
    od = (MQOD){MQOD_DEFAULT};
    strcpy(od.ObjectName, "NETDATA.PCF.REPLY");
    MQOPEN(hConn, &od, MQOO_INPUT_AS_Q_DEF, &hReplyQ, &compCode, &reason);
    if (compCode != MQCC_OK) {
        printf("MQOPEN reply queue failed: CompCode=%d, Reason=%d\n", compCode, reason);
        MQCLOSE(hConn, &hObj, MQCO_NONE, &compCode, &reason);
        MQDISC(&hConn, &compCode, &reason);
        return 1;
    }
    printf("Opened reply queue: NETDATA.PCF.REPLY\n");
    
    // Build INQUIRE_Q PCF command
    char buffer[65536];
    MQCFH *cfh = (MQCFH *)buffer;
    cfh->Type = MQCFT_COMMAND;
    cfh->StrucLength = MQCFH_STRUC_LENGTH;
    cfh->Version = MQCFH_VERSION_1;
    cfh->Command = MQCMD_INQUIRE_Q;
    cfh->MsgSeqNumber = 1;
    cfh->Control = MQCFC_LAST;
    cfh->ParameterCount = 1;
    
    // Add queue name parameter
    MQCFST *cfst = (MQCFST *)(buffer + MQCFH_STRUC_LENGTH);
    cfst->Type = MQCFT_STRING;
    cfst->StrucLength = MQCFST_STRUC_LENGTH_FIXED + 48;  // Fixed part + 48 chars
    cfst->Parameter = MQCA_Q_NAME;
    cfst->CodedCharSetId = MQCCSI_DEFAULT;
    cfst->StringLength = strlen(queueName);
    memset(cfst->String, ' ', 48);
    memcpy(cfst->String, queueName, strlen(queueName));
    
    // Send command
    md = (MQMD){MQMD_DEFAULT};
    strcpy(md.Format, MQFMT_ADMIN);
    md.MsgType = MQMT_REQUEST;
    memset(md.ReplyToQ, ' ', 48);
    memcpy(md.ReplyToQ, "NETDATA.PCF.REPLY", strlen("NETDATA.PCF.REPLY"));
    
    MQPUT(hConn, hObj, &md, &pmo, MQCFH_STRUC_LENGTH + cfst->StrucLength, 
          buffer, &compCode, &reason);
    if (compCode != MQCC_OK) {
        printf("MQPUT failed: CompCode=%d, Reason=%d\n", compCode, reason);
        goto cleanup;
    }
    printf("Sent MQCMD_INQUIRE_Q command\n");
    
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
    printf("\n=== MQCMD_INQUIRE_Q Response for: %s ===\n", queueName);
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
                printf("[%3d] INTEGER: %-30s (%4d) = %d\n", 
                       ++count, name, cfin->Parameter, cfin->Value);
            } else {
                printf("[%3d] INTEGER: UNKNOWN_ATTR_%d         (%4d) = %d\n", 
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
                printf("[%3d] STRING:  %-30s (%4d) = '%s'\n", 
                       ++count, name, cfst->Parameter, str);
            } else {
                printf("[%3d] STRING:  UNKNOWN_ATTR_%d         (%4d) = '%s'\n", 
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
    MQCLOSE(hConn, &hReplyQ, MQCO_NONE, &compCode, &reason);
    MQCLOSE(hConn, &hObj, MQCO_NONE, &compCode, &reason);
    MQDISC(&hConn, &compCode, &reason);
    return 0;
}