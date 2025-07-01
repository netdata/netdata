// test_queue_status.c - Test MQCMD_INQUIRE_Q_STATUS for non-destructive runtime metrics
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
        
        // Status attributes
        case MQIACF_MONITORING: return "MQIACF_MONITORING";
        case MQIACF_Q_STATUS_TYPE: return "MQIACF_Q_STATUS_TYPE";
        case MQIACF_Q_HANDLE: return "MQIACF_Q_HANDLE";
        
        // Runtime metrics
        case MQIA_MSG_ENQ_COUNT: return "MQIA_MSG_ENQ_COUNT";
        case MQIA_MSG_DEQ_COUNT: return "MQIA_MSG_DEQ_COUNT";
        case MQIA_HIGH_Q_DEPTH: return "MQIA_HIGH_Q_DEPTH";
        case MQIA_TIME_SINCE_RESET: return "MQIA_TIME_SINCE_RESET";
        
        // Open handles
        case MQIA_OPEN_INPUT_COUNT: return "MQIA_OPEN_INPUT_COUNT";
        case MQIA_OPEN_OUTPUT_COUNT: return "MQIA_OPEN_OUTPUT_COUNT";
        
        // Current depth
        case MQIA_CURRENT_Q_DEPTH: return "MQIA_CURRENT_Q_DEPTH";
        
        // Timestamps
        case MQCACF_LAST_GET_DATE: return "MQCACF_LAST_GET_DATE";
        case MQCACF_LAST_GET_TIME: return "MQCACF_LAST_GET_TIME";
        case MQCACF_LAST_PUT_DATE: return "MQCACF_LAST_PUT_DATE";
        case MQCACF_LAST_PUT_TIME: return "MQCACF_LAST_PUT_TIME";
        
        default: return NULL;
    }
}

void print_usage(const char *program) {
    printf("Usage: %s <queue_manager> <queue_name> [host] [port] [channel] [user] [password]\n", program);
    printf("  queue_manager: Name of the queue manager (required)\n");
    printf("  queue_name:    Name of the queue to inquire status (required)\n");
    printf("  host:          Host name (default: localhost)\n");
    printf("  port:          Port number (default: 1414)\n");
    printf("  channel:       Channel name (default: DEV.APP.SVRCONN)\n");
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
    
    // Initialize PMO properly
    pmo.Options = MQPMO_NO_SYNCPOINT | MQPMO_FAIL_IF_QUIESCING;
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
    
    // Build INQUIRE_Q_STATUS PCF command
    char buffer[65536];
    MQCFH *cfh = (MQCFH *)buffer;
    cfh->Type = MQCFT_COMMAND;
    cfh->StrucLength = MQCFH_STRUC_LENGTH;
    cfh->Version = MQCFH_VERSION_1;
    cfh->Command = MQCMD_INQUIRE_Q_STATUS;
    cfh->MsgSeqNumber = 1;
    cfh->Control = MQCFC_LAST;
    cfh->ParameterCount = 2;  // Changed to 2 parameters
    
    // Add queue name parameter
    MQCFST *cfst = (MQCFST *)(buffer + MQCFH_STRUC_LENGTH);
    cfst->Type = MQCFT_STRING;
    cfst->StrucLength = MQCFST_STRUC_LENGTH_FIXED + 48;  // Fixed part + 48 chars
    cfst->Parameter = MQCA_Q_NAME;
    cfst->CodedCharSetId = MQCCSI_DEFAULT;
    cfst->StringLength = strlen(queueName);
    memset(cfst->String, ' ', 48);
    memcpy(cfst->String, queueName, strlen(queueName));
    
    // Add status type parameter to request all information
    MQCFIN *cfin = (MQCFIN *)(buffer + MQCFH_STRUC_LENGTH + cfst->StrucLength);
    cfin->Type = MQCFT_INTEGER;
    cfin->StrucLength = MQCFIN_STRUC_LENGTH;
    cfin->Parameter = MQIACF_Q_STATUS_TYPE;
    cfin->Value = MQIACF_Q_STATUS;  // Request general queue status
    
    // Send command
    md = (MQMD){MQMD_DEFAULT};
    strcpy(md.Format, MQFMT_ADMIN);
    md.MsgType = MQMT_REQUEST;
    md.Priority = MQPRI_PRIORITY_AS_Q_DEF;  // Use queue default priority
    memset(md.ReplyToQ, ' ', 48);
    memcpy(md.ReplyToQ, "NETDATA.PCF.REPLY", strlen("NETDATA.PCF.REPLY"));
    memset(md.ReplyToQMgr, ' ', 48);
    memcpy(md.ReplyToQMgr, qmgr, strlen(qmgr));
    
    printf("\n=== Sending MQCMD_INQUIRE_Q_STATUS for: %s ===\n", queueName);
    
    MQPUT(hConn, hObj, &md, &pmo, MQCFH_STRUC_LENGTH + cfst->StrucLength + cfin->StrucLength, 
          buffer, &compCode, &reason);
    if (compCode != MQCC_OK) {
        printf("MQPUT failed: CompCode=%d, Reason=%d\n", compCode, reason);
        goto cleanup;
    }
    printf("Sent MQCMD_INQUIRE_Q_STATUS command\n");
    
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
    printf("\n=== MQCMD_INQUIRE_Q_STATUS Response ===\n");
    printf("CompCode=%d, Reason=%d, Parameters=%d\n\n", 
           cfh->CompCode, cfh->Reason, cfh->ParameterCount);
    
    if (cfh->CompCode != MQCC_OK) {
        printf("Command failed!\n");
        if (cfh->Reason == 2334) {
            printf("Error: Queue status not available (MQRC_Q_STATUS_NOT_AVAILABLE)\n");
            printf("This typically means no processes have the queue open.\n");
        } else if (cfh->Reason == 2085) {
            printf("Error: Unknown object name (MQRC_UNKNOWN_OBJECT_NAME)\n");
            printf("Queue '%s' does not exist.\n", queueName);
        }
        goto cleanup;
    }
    
    int offset = MQCFH_STRUC_LENGTH;
    int count = 0;
    int intCount = 0, strCount = 0, unknownCount = 0;
    
    printf("=== Attributes Returned ===\n");
    for (int i = 0; i < cfh->ParameterCount && offset < bufLen; i++) {
        MQLONG type = *(MQLONG *)(buffer + offset);
        
        if (type == MQCFT_INTEGER) {
            MQCFIN *cfin = (MQCFIN *)(buffer + offset);
            const char *name = getAttrName(cfin->Parameter);
            if (name) {
                printf("[%3d] INTEGER: %-30s (%4d) = %d", 
                       ++count, name, cfin->Parameter, cfin->Value);
                
                // Add helpful notes for specific attributes
                if (cfin->Parameter == MQIA_MSG_ENQ_COUNT) {
                    printf(" (messages put)");
                } else if (cfin->Parameter == MQIA_MSG_DEQ_COUNT) {
                    printf(" (messages gotten)");
                } else if (cfin->Parameter == MQIA_HIGH_Q_DEPTH) {
                    printf(" (peak depth)");
                } else if (cfin->Parameter == MQIA_TIME_SINCE_RESET) {
                    printf(" (seconds)");
                } else if (cfin->Parameter == MQIA_OPEN_INPUT_COUNT) {
                    printf(" (open for GET)");
                } else if (cfin->Parameter == MQIA_OPEN_OUTPUT_COUNT) {
                    printf(" (open for PUT)");
                }
                printf("\n");
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
    
    printf("\n=== Notes ===\n");
    printf("- MQCMD_INQUIRE_Q_STATUS returns current runtime information\n");
    printf("- Does NOT reset statistics (non-destructive read)\n");
    printf("- Returns data only if queue has processes with it open\n");
    printf("- For message counts, might need MONQ(HIGH) or STATQ(ON)\n");
    
cleanup:
    MQCLOSE(hConn, &hReplyQ, MQCO_NONE, &compCode, &reason);
    MQCLOSE(hConn, &hObj, MQCO_NONE, &compCode, &reason);
    MQDISC(&hConn, &compCode, &reason);
    return 0;
}