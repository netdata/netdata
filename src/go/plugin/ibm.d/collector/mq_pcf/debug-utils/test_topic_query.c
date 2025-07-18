// test_topic_query.c - Test MQCMD_INQUIRE_TOPIC to debug MQRC_2115
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <cmqc.h>
#include <cmqxc.h>
#include <cmqcfc.h>

int main() {
    // Connect to queue manager
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
    strcpy(cd.ChannelName, "DEV.APP.SVRCONN");
    strcpy(cd.ConnectionName, "localhost(3414)");
    cd.TransportType = MQXPT_TCP;
    
    // Set up authentication
    char user[] = "app";
    char password[] = "passw0rd";
    csp.CSPUserIdPtr = user;
    csp.CSPPasswordPtr = password;
    csp.CSPUserIdLength = 3;
    csp.CSPPasswordLength = 8;
    csp.AuthenticationType = MQCSP_AUTH_USER_ID_AND_PWD;
    
    // Set up connection options
    cno.ClientConnPtr = &cd;
    cno.SecurityParmsPtr = &csp;
    cno.Version = MQCNO_VERSION_5;
    cno.Options = MQCNO_CLIENT_BINDING;
    
    // Connect to queue manager
    MQCONNX("QM1", &cno, &hConn, &compCode, &reason);
    if (compCode != MQCC_OK) {
        printf("MQCONNX failed: CompCode=%d, Reason=%d\n", compCode, reason);
        return 1;
    }
    
    // Open admin queue
    memset(&od, 0, sizeof(od));
    strcpy(od.ObjectName, "SYSTEM.ADMIN.COMMAND.QUEUE");
    od.ObjectType = MQOT_Q;
    MQOPEN(hConn, &od, MQOO_OUTPUT | MQOO_FAIL_IF_QUIESCING, &hObj, &compCode, &reason);
    if (compCode != MQCC_OK) {
        printf("MQOPEN admin queue failed: CompCode=%d, Reason=%d\n", compCode, reason);
        MQDISC(&hConn, &compCode, &reason);
        return 1;
    }
    
    // Create temporary reply queue
    memset(&od, 0, sizeof(od));
    strcpy(od.ObjectName, "SYSTEM.DEFAULT.MODEL.QUEUE");
    strcpy(od.DynamicQName, "TOPIC.TEST.*");
    od.ObjectType = MQOT_Q;
    MQOPEN(hConn, &od, MQOO_INPUT_AS_Q_DEF | MQOO_FAIL_IF_QUIESCING, &hReplyQ, &compCode, &reason);
    if (compCode != MQCC_OK) {
        printf("MQOPEN reply queue failed: CompCode=%d, Reason=%d\n", compCode, reason);
        MQCLOSE(hConn, &hObj, MQCO_NONE, &compCode, &reason);
        MQDISC(&hConn, &compCode, &reason);
        return 1;
    }
    
    printf("Reply queue created: %s\n", od.ObjectName);
    
    // Create MQCMD_INQUIRE_TOPIC command
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
    
    // Add topic name parameter
    MQCFST *cfst = (MQCFST *)(buffer + MQCFH_STRUC_LENGTH);
    cfst->Type = MQCFT_STRING;
    cfst->StrucLength = MQCFST_STRUC_LENGTH_FIXED + 48;  // Use 48 chars like queue names
    cfst->Parameter = MQCA_TOPIC_NAME;
    cfst->CodedCharSetId = MQCCSI_DEFAULT;
    cfst->StringLength = 1;  // Length of "*"
    memset(cfst->String, ' ', 48);
    memcpy(cfst->String, "*", 1);
    
    // Send command
    strcpy(md.Format, MQFMT_ADMIN);
    md.MsgType = MQMT_REQUEST;
    md.CodedCharSetId = MQCCSI_Q_MGR;  // Use same as Go code
    md.Encoding = MQENC_NATIVE;
    memcpy(md.ReplyToQ, od.ObjectName, 48);
    
    MQPUT(hConn, hObj, &md, &pmo, MQCFH_STRUC_LENGTH + cfst->StrucLength, 
          buffer, &compCode, &reason);
    if (compCode != MQCC_OK) {
        printf("MQPUT failed: CompCode=%d, Reason=%d\n", compCode, reason);
        goto cleanup;
    }
    
    printf("MQPUT successful, waiting for response...\n");
    
    // Get response
    memcpy(md.CorrelId, md.MsgId, sizeof(md.MsgId));
    memset(md.MsgId, 0, sizeof(md.MsgId));
    gmo.Options = MQGMO_WAIT;
    gmo.WaitInterval = 5000;
    
    MQLONG bufLen = sizeof(buffer);
    MQGET(hConn, hReplyQ, &md, &gmo, bufLen, buffer, &bufLen, &compCode, &reason);
    if (compCode != MQCC_OK) {
        printf("MQGET failed: CompCode=%d, Reason=%d\n", compCode, reason);
        if (reason == 2115) {
            printf("MQRC_2115 (TARGET_CCSID_ERROR) - This is the error we're debugging!\n");
        }
        goto cleanup;
    }
    
    printf("MQGET successful, response received (%d bytes)\n", bufLen);
    
    // Parse response
    cfh = (MQCFH *)buffer;
    printf("Response: Type=%d, CompCode=%d, Reason=%d, ParameterCount=%d\n",
           cfh->Type, cfh->CompCode, cfh->Reason, cfh->ParameterCount);
    
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