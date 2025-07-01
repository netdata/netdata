// dump_channel_attrs_simple.c - Simple version to dump channel attributes from MQCMD_INQUIRE_CHANNEL
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <cmqc.h>
#include <cmqxc.h>
#include <cmqcfc.h>

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
    
    printf("=== Attributes (Raw) ===\n");
    printf("%-5s %-10s %-10s %s\n", "Idx", "Type", "Param ID", "Value");
    printf("%-5s %-10s %-10s %s\n", "---", "----", "---------", "-----");
    
    for (int i = 0; i < cfh->ParameterCount && offset < bufLen; i++) {
        MQLONG type = *(MQLONG *)(buffer + offset);
        
        if (type == MQCFT_INTEGER) {
            MQCFIN *cfin = (MQCFIN *)(buffer + offset);
            printf("[%3d] %-10s %10d %d\n", 
                   ++count, "INTEGER", cfin->Parameter, cfin->Value);
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
            
            printf("[%3d] %-10s %10d '%s'\n", 
                   ++count, "STRING", cfst->Parameter, str);
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
    printf("- Parameter IDs starting with 3000s are channel-specific integers (MQIACH_*)\n");
    printf("- Parameter IDs starting with 3500s are channel-specific strings (MQCACH_*)\n");
    printf("- Use cmqcfc.h constants to map parameter IDs to names\n");
    
cleanup:
    MQCLOSE(hConn, &hReplyQ, MQCO_DELETE_PURGE, &compCode, &reason);
    MQCLOSE(hConn, &hObj, MQCO_NONE, &compCode, &reason);
    MQDISC(&hConn, &compCode, &reason);
    return 0;
}