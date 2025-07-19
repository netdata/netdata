// test_queue_stats.c - Test MQCMD_RESET_Q_STATS to get queue runtime statistics
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
        
        // Statistics attributes
        case MQIA_MSG_ENQ_COUNT: return "MQIA_MSG_ENQ_COUNT";
        case MQIA_MSG_DEQ_COUNT: return "MQIA_MSG_DEQ_COUNT";
        case MQIAMO_Q_TIME_AVG: return "MQIAMO_Q_TIME_AVG";
        case MQIAMO_Q_TIME_MAX: return "MQIAMO_Q_TIME_MAX";
        case MQIAMO_Q_TIME_MIN: return "MQIAMO_Q_TIME_MIN";
        
        // Performance attributes
        case MQIA_HIGH_Q_DEPTH: return "MQIA_HIGH_Q_DEPTH";
        case MQIA_TIME_SINCE_RESET: return "MQIA_TIME_SINCE_RESET";
        
        default: return NULL;
    }
}

void print_usage(const char *program) {
    printf("Usage: %s <queue_manager> <queue_name> [host] [port] [channel] [user] [password]\n", program);
    printf("  queue_manager: Name of the queue manager (required)\n");
    printf("  queue_name:    Name of the queue to reset stats (required)\n");
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
    od.Version = MQOD_VERSION_1;
    strcpy(od.ObjectName, "SYSTEM.DEFAULT.MODEL.QUEUE");
    strcpy(od.DynamicQName, "MQPCF.*");
    MQOPEN(hConn, &od, MQOO_INPUT_AS_Q_DEF, &hReplyQ, &compCode, &reason);
    if (compCode != MQCC_OK) {
        printf("MQOPEN reply queue failed: CompCode=%d, Reason=%d\n", compCode, reason);
        MQCLOSE(hConn, &hObj, MQCO_NONE, &compCode, &reason);
        MQDISC(&hConn, &compCode, &reason);
        return 1;
    }
    
    // Build RESET_Q_STATS PCF command
    char buffer[65536];
    MQCFH *cfh = (MQCFH *)buffer;
    cfh->Type = MQCFT_COMMAND;
    cfh->StrucLength = MQCFH_STRUC_LENGTH;
    cfh->Version = MQCFH_VERSION_1;
    cfh->Command = MQCMD_RESET_Q_STATS;
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
    strcpy(md.Format, MQFMT_ADMIN);
    md.MsgType = MQMT_REQUEST;
    memcpy(md.ReplyToQ, od.ObjectName, 48);
    
    printf("\n=== Sending MQCMD_RESET_Q_STATS for: %s ===\n", queueName);
    
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
    printf("\n=== MQCMD_RESET_Q_STATS Response ===\n");
    printf("CompCode=%d, Reason=%d, Parameters=%d\n\n", 
           cfh->CompCode, cfh->Reason, cfh->ParameterCount);
    
    if (cfh->CompCode != MQCC_OK) {
        printf("Command failed!\n");
        if (cfh->Reason == 2334) {
            printf("Error: Queue statistics not available (MQRC_Q_STATS_NOT_AVAILABLE)\n");
            printf("Make sure STATQ is enabled for this queue or at queue manager level.\n");
        } else if (cfh->Reason == 2085) {
            printf("Error: Unknown object name (MQRC_UNKNOWN_OBJECT_NAME)\n");
            printf("Queue '%s' does not exist.\n", queueName);
        }
        goto cleanup;
    }
    
    int offset = MQCFH_STRUC_LENGTH;
    int count = 0;
    int intCount = 0, strCount = 0, unknownCount = 0;
    
    printf("=== Statistics Returned ===\n");
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
                    printf(" (peak depth since reset)");
                } else if (cfin->Parameter == MQIA_TIME_SINCE_RESET) {
                    printf(" (seconds)");
                } else if (cfin->Parameter == MQIAMO_Q_TIME_AVG) {
                    printf(" (microseconds)");
                } else if (cfin->Parameter == MQIAMO_Q_TIME_MAX) {
                    printf(" (microseconds)");
                } else if (cfin->Parameter == MQIAMO_Q_TIME_MIN) {
                    printf(" (microseconds)");
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
    printf("- MSG_ENQ_COUNT/MSG_DEQ_COUNT are runtime counters since last reset\n");
    printf("- HIGH_Q_DEPTH is the peak queue depth since last reset\n");
    printf("- Q_TIME_* values are in microseconds\n");
    printf("- TIME_SINCE_RESET is in seconds\n");
    printf("- Use ALTER QMGR STATQ(ON) to enable statistics\n");
    printf("- Statistics are reset after this command completes\n");
    
cleanup:
    MQCLOSE(hConn, &hReplyQ, MQCO_DELETE_PURGE, &compCode, &reason);
    MQCLOSE(hConn, &hObj, MQCO_NONE, &compCode, &reason);
    MQDISC(&hConn, &compCode, &reason);
    return 0;
}