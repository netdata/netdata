// Simple MQCMD_INQUIRE_TOPIC test - TEST UTF-8 CCSID FIX
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <cmqc.h>
#include <cmqxc.h>
#include <cmqcfc.h>

int main() {
    MQHCONN hConn = MQHC_UNUSABLE_HCONN;
    MQHOBJ hObj = MQHO_UNUSABLE_HOBJ;
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
    
    // Create PCF command message
    char buffer[1024];
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
    cfst->StrucLength = MQCFST_STRUC_LENGTH_FIXED + 48;
    cfst->Parameter = MQCA_TOPIC_NAME;
    cfst->CodedCharSetId = MQCCSI_DEFAULT;
    cfst->StringLength = 1;  // Length of "*"
    memset(cfst->String, ' ', 48);
    memcpy(cfst->String, "*", 1);
    
    // Send command - TEST UTF-8 CCSID
    strcpy(md.Format, MQFMT_ADMIN);
    md.MsgType = MQMT_REQUEST;
    md.CodedCharSetId = 1208;  // UTF-8 - THIS IS THE FIX WE'RE TESTING
    md.Encoding = MQENC_NATIVE;
    
    printf("Testing MQCMD_INQUIRE_TOPIC with UTF-8 CCSID (1208)...\n");
    
    // Check message size
    MQLONG msgSize = MQCFH_STRUC_LENGTH + cfst->StrucLength;
    printf("PCF message size: %d bytes (admin queue max: 32762)\n", msgSize);
    
    // Use synchronous call without reply queue for simplicity
    pmo.Options = MQPMO_NO_SYNCPOINT | MQPMO_FAIL_IF_QUIESCING | MQPMO_NEW_MSG_ID;
    
    MQPUT(hConn, hObj, &md, &pmo, msgSize, buffer, &compCode, &reason);
    
    if (compCode != MQCC_OK) {
        printf("MQPUT failed: CompCode=%d, Reason=%d\n", compCode, reason);
        if (reason == 2115) {
            printf("MQRC_2115 (TARGET_CCSID_ERROR) - UTF-8 CCSID did NOT fix the issue!\n");
        } else {
            printf("Different error than expected MQRC_2115\n");
        }
    } else {
        printf("MQPUT successful! UTF-8 CCSID (1208) FIXED the MQRC_2115 error!\n");
    }
    
    // Cleanup
    MQCLOSE(hConn, &hObj, MQCO_NONE, &compCode, &reason);
    MQDISC(&hConn, &compCode, &reason);
    
    return 0;
}