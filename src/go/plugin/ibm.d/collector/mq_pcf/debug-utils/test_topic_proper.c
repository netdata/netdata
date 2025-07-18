// Proper MQCMD_INQUIRE_TOPIC test with correct parameters
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
    csp.AuthenticationType = MQCSP_AUTH_USER_ID_AND_PWD;
    csp.CSPUserIdPtr = user;
    csp.CSPPasswordPtr = password;
    csp.CSPUserIdLength = strlen(user);
    csp.CSPPasswordLength = strlen(password);
    
    cno.ClientConnPtr = &cd;
    cno.SecurityParmsPtr = &csp;
    cno.Version = MQCNO_VERSION_5;
    
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
    MQOPEN(hConn, &od, MQOO_OUTPUT | MQOO_FAIL_IF_QUIESCING, &hObj, &compCode, &reason);
    
    if (compCode != MQCC_OK) {
        printf("MQOPEN admin queue failed: CompCode=%d, Reason=%d\n", compCode, reason);
        MQDISC(&hConn, &compCode, &reason);
        return 1;
    }
    
    printf("Admin queue opened successfully\n");
    
    // Create reply queue
    MQOD replyOd = {MQOD_DEFAULT};
    strcpy(replyOd.ObjectName, "SYSTEM.DEFAULT.MODEL.QUEUE");
    strcpy(replyOd.DynamicQName, "TOPIC.REPLY.*");
    replyOd.ObjectType = MQOT_Q;
    MQOPEN(hConn, &replyOd, MQOO_INPUT_AS_Q_DEF | MQOO_FAIL_IF_QUIESCING, &hReplyQ, &compCode, &reason);
    
    if (compCode != MQCC_OK) {
        printf("MQOPEN reply queue failed: CompCode=%d, Reason=%d\n", compCode, reason);
        MQCLOSE(hConn, &hObj, MQCO_NONE, &compCode, &reason);
        MQDISC(&hConn, &compCode, &reason);
        return 1;
    }
    
    printf("Reply queue created: %s\n", replyOd.ObjectName);
    
    // Test 1: Try to inquire on a known topic
    printf("\n=== Test 1: Inquire on DEV.BASE.TOPIC ===\n");
    
    // Create PCF command message
    char buffer[4096];
    MQCFH *cfh = (MQCFH *)buffer;
    
    // PCF header
    cfh->Type = MQCFT_COMMAND;
    cfh->StrucLength = MQCFH_STRUC_LENGTH;
    cfh->Version = MQCFH_VERSION_1;
    cfh->Command = MQCMD_INQUIRE_TOPIC;
    cfh->MsgSeqNumber = 1;
    cfh->Control = MQCFC_LAST;
    cfh->ParameterCount = 1;
    
    // Add topic name parameter - use a specific topic that exists
    MQCFST *cfst = (MQCFST *)(buffer + MQCFH_STRUC_LENGTH);
    cfst->Type = MQCFT_STRING;
    cfst->StrucLength = MQCFST_STRUC_LENGTH_FIXED + 48;
    cfst->Parameter = MQCA_TOPIC_NAME;
    cfst->CodedCharSetId = MQCCSI_DEFAULT;
    cfst->StringLength = strlen("DEV.BASE.TOPIC");
    memset(cfst->String, ' ', 48);
    memcpy(cfst->String, "DEV.BASE.TOPIC", strlen("DEV.BASE.TOPIC"));
    
    // Send command with proper reply queue
    strcpy(md.Format, MQFMT_ADMIN);
    md.MsgType = MQMT_REQUEST;
    md.CodedCharSetId = 1208;  // UTF-8
    md.Encoding = MQENC_NATIVE;
    memcpy(md.ReplyToQ, replyOd.ObjectName, 48);
    
    MQLONG msgSize = MQCFH_STRUC_LENGTH + cfst->StrucLength;
    printf("PCF message size: %d bytes\n", msgSize);
    
    pmo.Options = MQPMO_NO_SYNCPOINT | MQPMO_FAIL_IF_QUIESCING;
    
    MQPUT(hConn, hObj, &md, &pmo, msgSize, buffer, &compCode, &reason);
    
    if (compCode != MQCC_OK) {
        printf("MQPUT failed: CompCode=%d, Reason=%d\n", compCode, reason);
        if (reason == 2115) {
            printf("MQRC_2115 (TARGET_CCSID_ERROR)\n");
        } else if (reason == 2027) {
            printf("MQRC_2027 (MSG_TOO_BIG_FOR_Q)\n");
        } else if (reason == 2035) {
            printf("MQRC_2035 (NOT_AUTHORIZED) - Need topic permissions\n");
        }
        goto cleanup;
    }
    
    printf("MQPUT successful! Now waiting for response...\n");
    
    // Get response
    memcpy(md.CorrelId, md.MsgId, sizeof(md.MsgId));
    memset(md.MsgId, 0, sizeof(md.MsgId));
    gmo.Options = MQGMO_WAIT | MQGMO_CONVERT;
    gmo.WaitInterval = 5000;
    
    MQLONG bufLen = sizeof(buffer);
    MQGET(hConn, hReplyQ, &md, &gmo, bufLen, buffer, &bufLen, &compCode, &reason);
    
    if (compCode != MQCC_OK) {
        printf("MQGET failed: CompCode=%d, Reason=%d\n", compCode, reason);
        if (reason == 2033) {
            printf("MQRC_2033 (NO_MSG_AVAILABLE) - No response received\n");
        } else if (reason == 2115) {
            printf("MQRC_2115 (TARGET_CCSID_ERROR) - Character set issue\n");
        }
        goto cleanup;
    }
    
    printf("Response received (%d bytes)\n", bufLen);
    
    // Parse response
    MQCFH *respCfh = (MQCFH *)buffer;
    printf("Response: Type=%d, CompCode=%d, Reason=%d, ParameterCount=%d\n",
           respCfh->Type, respCfh->CompCode, respCfh->Reason, respCfh->ParameterCount);
    
    if (respCfh->CompCode == MQCC_OK) {
        printf("✅ SUCCESS: MQCMD_INQUIRE_TOPIC worked!\n");
    } else {
        printf("❌ FAILED: MQCMD_INQUIRE_TOPIC returned CompCode=%d, Reason=%d\n", 
               respCfh->CompCode, respCfh->Reason);
    }
    
cleanup:
    if (hReplyQ != MQHO_UNUSABLE_HOBJ) {
        MQCLOSE(hConn, &hReplyQ, MQCO_DELETE, &compCode, &reason);
    }
    if (hObj != MQHO_UNUSABLE_HOBJ) {
        MQCLOSE(hConn, &hObj, MQCO_NONE, &compCode, &reason);
    }
    if (hConn != MQHC_UNUSABLE_HCONN) {
        MQDISC(&hConn, &compCode, &reason);
    }
    
    return 0;
}