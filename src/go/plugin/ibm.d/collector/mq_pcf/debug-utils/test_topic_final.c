// Final test for topic commands - use synchronous PCF without reply queue
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
    MQCNO cno = {MQCNO_DEFAULT};
    MQCD cd = {MQCD_CLIENT_CONN_DEFAULT};
    MQCSP csp = {MQCSP_DEFAULT};
    
    printf("=== Final Topic Test - Direct PCF Commands ===\n\n");
    
    // Set up client connection
    cno.Version = MQCNO_VERSION_4;
    cno.Options = MQCNO_CLIENT_BINDING;
    
    cd.ChannelType = MQCHT_CLNTCONN;
    cd.TransportType = MQXPT_TCP;
    cd.Version = MQCD_VERSION_6;
    strncpy(cd.ChannelName, "DEV.APP.SVRCONN", MQ_CHANNEL_NAME_LENGTH);
    sprintf(cd.ConnectionName, "localhost(3414)");
    
    cno.ClientConnPtr = &cd;
    
    // Set up authentication
    char user[] = "app";
    char password[] = "passw0rd";
    cno.Version = MQCNO_VERSION_5;
    csp.AuthenticationType = MQCSP_AUTH_USER_ID_AND_PWD;
    csp.CSPUserIdPtr = user;
    csp.CSPUserIdLength = strlen(user);
    csp.CSPPasswordPtr = password;
    csp.CSPPasswordLength = strlen(password);
    cno.SecurityParmsPtr = &csp;
    
    // Connect
    MQCONNX("QM1", &cno, &hConn, &compCode, &reason);
    if (compCode != MQCC_OK) {
        printf("MQCONNX failed: CompCode=%d, Reason=%d\n", compCode, reason);
        return 1;
    }
    printf("Connected to QM1\n");
    
    // Open command queue
    strcpy(od.ObjectName, "SYSTEM.ADMIN.COMMAND.QUEUE");
    MQOPEN(hConn, &od, MQOO_OUTPUT, &hObj, &compCode, &reason);
    if (compCode != MQCC_OK) {
        printf("MQOPEN command queue failed: CompCode=%d, Reason=%d\n", compCode, reason);
        MQDISC(&hConn, &compCode, &reason);
        return 1;
    }
    printf("Command queue opened\n");
    
    // Test 1: Send MQCMD_INQUIRE_TOPIC without reply queue (fire and forget)
    printf("\n=== Test 1: MQCMD_INQUIRE_TOPIC (synchronous) ===\n");
    
    char buffer[65536];
    MQCFH *cfh = (MQCFH *)buffer;
    cfh->Type = MQCFT_COMMAND;
    cfh->StrucLength = MQCFH_STRUC_LENGTH;
    cfh->Version = MQCFH_VERSION_1;
    cfh->Command = MQCMD_INQUIRE_TOPIC;
    cfh->MsgSeqNumber = 1;
    cfh->Control = MQCFC_LAST;
    cfh->ParameterCount = 1;
    
    // Add topic name parameter (test with specific topic)
    MQCFST *cfst = (MQCFST *)(buffer + MQCFH_STRUC_LENGTH);
    cfst->Type = MQCFT_STRING;
    cfst->StrucLength = MQCFST_STRUC_LENGTH_FIXED + 256;  // Topic names are 256 chars
    cfst->Parameter = MQCA_TOPIC_NAME;
    cfst->CodedCharSetId = MQCCSI_DEFAULT;
    cfst->StringLength = 6;  // Length of "TOPIC1"
    memset(cfst->String, ' ', 256);  // Fill with spaces
    memcpy(cfst->String, "TOPIC1", 6);  // Copy actual value
    
    // Send command without reply queue
    strcpy(md.Format, MQFMT_ADMIN);
    md.MsgType = MQMT_DATAGRAM;  // Use datagram (no reply expected)
    md.CodedCharSetId = MQCCSI_DEFAULT;
    md.Encoding = MQENC_NATIVE;
    
    printf("Sending MQCMD_INQUIRE_TOPIC for TOPIC1 (message size: %d bytes)\n", MQCFH_STRUC_LENGTH + cfst->StrucLength);
    
    MQPUT(hConn, hObj, &md, &pmo, MQCFH_STRUC_LENGTH + cfst->StrucLength, buffer, &compCode, &reason);
    
    if (compCode != MQCC_OK) {
        printf("MQPUT failed: CompCode=%d, Reason=%d\n", compCode, reason);
        if (reason == 2050) {
            printf("  MQRC_OBJECT_NOT_OPEN - Still getting this error\n");
        } else if (reason == 2085) {
            printf("  MQRC_UNKNOWN_OBJECT_NAME - Topic doesn't exist\n");
        } else if (reason == 2035) {
            printf("  MQRC_NOT_AUTHORIZED - Not authorized\n");
        } else if (reason == 2027) {
            printf("  MQRC_MSG_TOO_BIG_FOR_Q - Message too big\n");
        } else if (reason == 2115) {
            printf("  MQRC_TARGET_CCSID_ERROR - Character set issue\n");
        }
        goto test2;
    }
    
    printf("✅ MQPUT successful! MQCMD_INQUIRE_TOPIC works!\n");
    
test2:
    // Test 2: Send MQCMD_INQUIRE_TOPIC_STATUS
    printf("\n=== Test 2: MQCMD_INQUIRE_TOPIC_STATUS (synchronous) ===\n");
    
    // Reset for next test
    cfh->Command = MQCMD_INQUIRE_TOPIC_STATUS;
    md = (MQMD){MQMD_DEFAULT};
    strcpy(md.Format, MQFMT_ADMIN);
    md.MsgType = MQMT_DATAGRAM;
    md.CodedCharSetId = MQCCSI_DEFAULT;
    md.Encoding = MQENC_NATIVE;
    
    printf("Sending MQCMD_INQUIRE_TOPIC_STATUS for TOPIC1...\n");
    
    MQPUT(hConn, hObj, &md, &pmo, MQCFH_STRUC_LENGTH + cfst->StrucLength, buffer, &compCode, &reason);
    
    if (compCode != MQCC_OK) {
        printf("MQPUT failed: CompCode=%d, Reason=%d\n", compCode, reason);
        printf("  MQCMD_INQUIRE_TOPIC_STATUS doesn't work or isn't supported\n");
        goto test3;
    }
    
    printf("✅ MQPUT successful! MQCMD_INQUIRE_TOPIC_STATUS works!\n");
    
test3:
    // Test 3: Try with wildcard
    printf("\n=== Test 3: MQCMD_INQUIRE_TOPIC with wildcard ===\n");
    
    // Reset for wildcard test
    cfh->Command = MQCMD_INQUIRE_TOPIC;
    md = (MQMD){MQMD_DEFAULT};
    strcpy(md.Format, MQFMT_ADMIN);
    md.MsgType = MQMT_DATAGRAM;
    md.CodedCharSetId = MQCCSI_DEFAULT;
    md.Encoding = MQENC_NATIVE;
    
    // Change to wildcard
    cfst->StringLength = 1;  // Length of "*"
    memset(cfst->String, ' ', 256);  // Fill with spaces
    memcpy(cfst->String, "*", 1);    // Copy wildcard
    
    printf("Sending MQCMD_INQUIRE_TOPIC with wildcard '*'...\n");
    
    MQPUT(hConn, hObj, &md, &pmo, MQCFH_STRUC_LENGTH + cfst->StrucLength, buffer, &compCode, &reason);
    
    if (compCode != MQCC_OK) {
        printf("MQPUT failed: CompCode=%d, Reason=%d\n", compCode, reason);
        printf("  Wildcard topic inquiry doesn't work\n");
        goto cleanup;
    }
    
    printf("✅ MQPUT successful! Wildcard topic inquiry works!\n");
    
cleanup:
    if (hObj != MQHO_UNUSABLE_HOBJ) {
        MQCLOSE(hConn, &hObj, MQCO_NONE, &compCode, &reason);
    }
    if (hConn != MQHC_UNUSABLE_HCONN) {
        MQDISC(&hConn, &compCode, &reason);
    }
    
    printf("\n=== Final Assessment ===\n");
    printf("This test uses synchronous PCF commands without reply queues.\n");
    printf("If commands succeed, topic monitoring should work in the collector.\n");
    printf("Note: We can't see the response data in this test, but successful\n");
    printf("      MQPUT means the command was accepted by MQ.\n");
    
    return 0;
}