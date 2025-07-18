// Test what topic commands actually work in MQ
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <cmqc.h>
#include <cmqxc.h>
#include <cmqcfc.h>

int main() {
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
    
    printf("=== Testing What Topic Commands Actually Work ===\n\n");
    
    // Set up client connection
    cno.Version = MQCNO_VERSION_4;
    cno.Options = MQCNO_CLIENT_BINDING;
    
    cd.ChannelType = MQCHT_CLNTCONN;
    cd.TransportType = MQXPT_TCP;
    cd.Version = MQCD_VERSION_6;
    strcpy(cd.ChannelName, "DEV.APP.SVRCONN");
    strcpy(cd.ConnectionName, "localhost(3414)");
    
    // Set up authentication
    char user[] = "app";
    char password[] = "passw0rd";
    cno.Version = MQCNO_VERSION_5;
    csp.AuthenticationType = MQCSP_AUTH_USER_ID_AND_PWD;
    csp.CSPUserIdPtr = user;
    csp.CSPUserIdLength = strlen(user);
    csp.CSPPasswordPtr = password;
    csp.CSPPasswordLength = strlen(password);
    cno.ClientConnPtr = &cd;
    cno.SecurityParmsPtr = &csp;
    
    // Connect to queue manager
    MQCONNX("QM1", &cno, &hConn, &compCode, &reason);
    if (compCode != MQCC_OK) {
        printf("MQCONNX failed: CompCode=%d, Reason=%d\n", compCode, reason);
        return 1;
    }
    printf("Connected to QM1\n");
    
    // Open admin queue
    strcpy(od.ObjectName, "SYSTEM.ADMIN.COMMAND.QUEUE");
    od.ObjectType = MQOT_Q;
    MQOPEN(hConn, &od, MQOO_OUTPUT, &hObj, &compCode, &reason);
    if (compCode != MQCC_OK) {
        printf("MQOPEN admin queue failed: CompCode=%d, Reason=%d\n", compCode, reason);
        MQDISC(&hConn, &compCode, &reason);
        return 1;
    }
    printf("Admin queue opened\n");
    
    // Create reply queue using the same pattern as working tests
    MQOD replyOd = {MQOD_DEFAULT};
    strcpy(replyOd.ObjectName, "SYSTEM.DEFAULT.MODEL.QUEUE");
    strcpy(replyOd.DynamicQName, "MQTOPIC.*");
    replyOd.ObjectType = MQOT_Q;
    MQOPEN(hConn, &replyOd, MQOO_INPUT_AS_Q_DEF, &hReplyQ, &compCode, &reason);
    if (compCode != MQCC_OK) {
        printf("MQOPEN reply queue failed: CompCode=%d, Reason=%d\n", compCode, reason);
        MQCLOSE(hConn, &hObj, MQCO_NONE, &compCode, &reason);
        MQDISC(&hConn, &compCode, &reason);
        return 1;
    }
    printf("Reply queue created: %s\n", replyOd.ObjectName);
    
    // Test 1: First try basic MQCMD_INQUIRE_TOPIC (known to work)
    printf("\n=== Test 1: MQCMD_INQUIRE_TOPIC (basic topic inquiry) ===\n");
    
    char buffer[65536];
    MQCFH *cfh = (MQCFH *)buffer;
    
    // PCF header
    cfh->Type = MQCFT_COMMAND;
    cfh->StrucLength = MQCFH_STRUC_LENGTH;
    cfh->Version = MQCFH_VERSION_1;
    cfh->Command = MQCMD_INQUIRE_TOPIC;
    cfh->MsgSeqNumber = 1;
    cfh->Control = MQCFC_LAST;
    cfh->ParameterCount = 1;
    
    // Add topic name parameter with wildcard
    MQCFST *cfst = (MQCFST *)(buffer + MQCFH_STRUC_LENGTH);
    cfst->Type = MQCFT_STRING;
    cfst->StrucLength = MQCFST_STRUC_LENGTH_FIXED + 256;  // Topic names are 256 chars
    cfst->Parameter = MQCA_TOPIC_NAME;
    cfst->CodedCharSetId = MQCCSI_DEFAULT;
    cfst->StringLength = 1;  // Length of "*"
    memset(cfst->String, ' ', 256);
    memcpy(cfst->String, "*", 1);
    
    // Send command
    strcpy(md.Format, MQFMT_ADMIN);
    md.MsgType = MQMT_REQUEST;
    md.CodedCharSetId = MQCCSI_DEFAULT;  // Use default instead of UTF-8
    md.Encoding = MQENC_NATIVE;
    memcpy(md.ReplyToQ, replyOd.ObjectName, 48);
    
    MQLONG msgSize = MQCFH_STRUC_LENGTH + cfst->StrucLength;
    printf("Sending MQCMD_INQUIRE_TOPIC (message size: %d bytes)\n", msgSize);
    
    pmo.Options = MQPMO_NO_SYNCPOINT | MQPMO_FAIL_IF_QUIESCING | MQPMO_NEW_MSG_ID;
    
    MQPUT(hConn, hObj, &md, &pmo, msgSize, buffer, &compCode, &reason);
    
    if (compCode != MQCC_OK) {
        printf("MQPUT failed: CompCode=%d, Reason=%d\n", compCode, reason);
        printf("  This suggests basic topic inquiry doesn't work\n");
    } else {
        printf("MQPUT successful! Topic inquiry works\n");
        
        // Get response
        memcpy(md.CorrelId, md.MsgId, sizeof(md.MsgId));
        memset(md.MsgId, 0, sizeof(md.MsgId));
        gmo.Options = MQGMO_WAIT | MQGMO_CONVERT;
        gmo.WaitInterval = 5000;
        
        MQLONG bufLen = sizeof(buffer);
        MQGET(hConn, hReplyQ, &md, &gmo, bufLen, buffer, &bufLen, &compCode, &reason);
        
        if (compCode == MQCC_OK) {
            MQCFH *respCfh = (MQCFH *)buffer;
            printf("✅ MQCMD_INQUIRE_TOPIC works! Response: CompCode=%d, Reason=%d, Parameters=%d\n",
                   respCfh->CompCode, respCfh->Reason, respCfh->ParameterCount);
        } else {
            printf("MQGET failed: CompCode=%d, Reason=%d\n", compCode, reason);
        }
    }
    
    // Test 2: Now try MQCMD_INQUIRE_TOPIC_STATUS
    printf("\n=== Test 2: MQCMD_INQUIRE_TOPIC_STATUS (topic status inquiry) ===\n");
    
    // Reset message descriptor
    md = (MQMD){MQMD_DEFAULT};
    
    // PCF header
    cfh->Type = MQCFT_COMMAND;
    cfh->StrucLength = MQCFH_STRUC_LENGTH;
    cfh->Version = MQCFH_VERSION_1;
    cfh->Command = MQCMD_INQUIRE_TOPIC_STATUS;
    cfh->MsgSeqNumber = 1;
    cfh->Control = MQCFC_LAST;
    cfh->ParameterCount = 1;
    
    // Same topic name parameter
    cfst->Type = MQCFT_STRING;
    cfst->StrucLength = MQCFST_STRUC_LENGTH_FIXED + 256;
    cfst->Parameter = MQCA_TOPIC_NAME;
    cfst->CodedCharSetId = MQCCSI_DEFAULT;
    cfst->StringLength = 1;
    memset(cfst->String, ' ', 256);
    memcpy(cfst->String, "*", 1);
    
    // Send command
    strcpy(md.Format, MQFMT_ADMIN);
    md.MsgType = MQMT_REQUEST;
    md.CodedCharSetId = MQCCSI_DEFAULT;
    md.Encoding = MQENC_NATIVE;
    memcpy(md.ReplyToQ, replyOd.ObjectName, 48);
    
    printf("Sending MQCMD_INQUIRE_TOPIC_STATUS (message size: %d bytes)\n", msgSize);
    
    pmo.Options = MQPMO_NO_SYNCPOINT | MQPMO_FAIL_IF_QUIESCING | MQPMO_NEW_MSG_ID;
    
    MQPUT(hConn, hObj, &md, &pmo, msgSize, buffer, &compCode, &reason);
    
    if (compCode != MQCC_OK) {
        printf("MQPUT failed: CompCode=%d, Reason=%d\n", compCode, reason);
        if (reason == 2050) {
            printf("  MQRC_OBJECT_NOT_OPEN - Object not open error\n");
        } else if (reason == 2085) {
            printf("  MQRC_UNKNOWN_OBJECT_NAME - Command not supported or object not found\n");
        } else if (reason == 2035) {
            printf("  MQRC_NOT_AUTHORIZED - Not authorized\n");
        }
        printf("  This suggests MQCMD_INQUIRE_TOPIC_STATUS doesn't work or isn't supported\n");
    } else {
        printf("MQPUT successful! Topic status inquiry works\n");
        
        // Get response
        memcpy(md.CorrelId, md.MsgId, sizeof(md.MsgId));
        memset(md.MsgId, 0, sizeof(md.MsgId));
        gmo.Options = MQGMO_WAIT | MQGMO_CONVERT;
        gmo.WaitInterval = 5000;
        
        MQLONG bufLen = sizeof(buffer);
        MQGET(hConn, hReplyQ, &md, &gmo, bufLen, buffer, &bufLen, &compCode, &reason);
        
        if (compCode == MQCC_OK) {
            MQCFH *respCfh = (MQCFH *)buffer;
            printf("✅ MQCMD_INQUIRE_TOPIC_STATUS works! Response: CompCode=%d, Reason=%d, Parameters=%d\n",
                   respCfh->CompCode, respCfh->Reason, respCfh->ParameterCount);
        } else {
            printf("MQGET failed: CompCode=%d, Reason=%d\n", compCode, reason);
        }
    }
    
    // Test 3: Try MQCMD_INQUIRE_SUB_STATUS (subscription status)
    printf("\n=== Test 3: MQCMD_INQUIRE_SUB_STATUS (subscription status) ===\n");
    
    // Reset message descriptor
    md = (MQMD){MQMD_DEFAULT};
    
    // PCF header
    cfh->Type = MQCFT_COMMAND;
    cfh->StrucLength = MQCFH_STRUC_LENGTH;
    cfh->Version = MQCFH_VERSION_1;
    cfh->Command = MQCMD_INQUIRE_SUB_STATUS;
    cfh->MsgSeqNumber = 1;
    cfh->Control = MQCFC_LAST;
    cfh->ParameterCount = 1;
    
    // Try with subscription name wildcard
    cfst->Type = MQCFT_STRING;
    cfst->StrucLength = MQCFST_STRUC_LENGTH_FIXED + 256;
    cfst->Parameter = MQCACF_SUB_NAME;  // Subscription name
    cfst->CodedCharSetId = MQCCSI_DEFAULT;
    cfst->StringLength = 1;
    memset(cfst->String, ' ', 256);
    memcpy(cfst->String, "*", 1);
    
    // Send command
    strcpy(md.Format, MQFMT_ADMIN);
    md.MsgType = MQMT_REQUEST;
    md.CodedCharSetId = MQCCSI_DEFAULT;
    md.Encoding = MQENC_NATIVE;
    memcpy(md.ReplyToQ, replyOd.ObjectName, 48);
    
    printf("Sending MQCMD_INQUIRE_SUB_STATUS (message size: %d bytes)\n", msgSize);
    
    pmo.Options = MQPMO_NO_SYNCPOINT | MQPMO_FAIL_IF_QUIESCING | MQPMO_NEW_MSG_ID;
    
    MQPUT(hConn, hObj, &md, &pmo, msgSize, buffer, &compCode, &reason);
    
    if (compCode != MQCC_OK) {
        printf("MQPUT failed: CompCode=%d, Reason=%d\n", compCode, reason);
        printf("  This suggests MQCMD_INQUIRE_SUB_STATUS doesn't work\n");
    } else {
        printf("MQPUT successful! Subscription status inquiry works\n");
        
        // Get response
        memcpy(md.CorrelId, md.MsgId, sizeof(md.MsgId));
        memset(md.MsgId, 0, sizeof(md.MsgId));
        gmo.Options = MQGMO_WAIT | MQGMO_CONVERT;
        gmo.WaitInterval = 5000;
        
        MQLONG bufLen = sizeof(buffer);
        MQGET(hConn, hReplyQ, &md, &gmo, bufLen, buffer, &bufLen, &compCode, &reason);
        
        if (compCode == MQCC_OK) {
            MQCFH *respCfh = (MQCFH *)buffer;
            printf("✅ MQCMD_INQUIRE_SUB_STATUS works! Response: CompCode=%d, Reason=%d, Parameters=%d\n",
                   respCfh->CompCode, respCfh->Reason, respCfh->ParameterCount);
        } else {
            printf("MQGET failed: CompCode=%d, Reason=%d\n", compCode, reason);
        }
    }
    
    // Cleanup
    if (hReplyQ != MQHO_UNUSABLE_HOBJ) {
        MQCLOSE(hConn, &hReplyQ, MQCO_DELETE, &compCode, &reason);
    }
    if (hObj != MQHO_UNUSABLE_HOBJ) {
        MQCLOSE(hConn, &hObj, MQCO_NONE, &compCode, &reason);
    }
    if (hConn != MQHC_UNUSABLE_HCONN) {
        MQDISC(&hConn, &compCode, &reason);
    }
    
    printf("\n=== Summary ===\n");
    printf("This test determines which topic-related commands actually work in MQ.\n");
    printf("Commands that work can be used for monitoring. Commands that fail are not usable.\n");
    
    return 0;
}