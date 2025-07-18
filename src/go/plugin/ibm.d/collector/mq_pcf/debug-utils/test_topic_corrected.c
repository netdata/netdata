// Test topic commands using the exact same pattern as working queue tests
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
    
    printf("=== Testing Topic Commands with Working Queue Pattern ===\n\n");
    
    // Set up client connection (exact same as working test)
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
    
    // Open command queue (exact same as working test)
    strcpy(od.ObjectName, "SYSTEM.ADMIN.COMMAND.QUEUE");
    MQOPEN(hConn, &od, MQOO_OUTPUT, &hObj, &compCode, &reason);
    if (compCode != MQCC_OK) {
        printf("MQOPEN command queue failed: CompCode=%d, Reason=%d\n", compCode, reason);
        MQDISC(&hConn, &compCode, &reason);
        return 1;
    }
    printf("Command queue opened\n");
    
    // Open reply queue (exact same as working test)
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
    printf("Reply queue created: %s\n", od.ObjectName);
    
    // Test 1: MQCMD_INQUIRE_TOPIC (using exact same pattern as working queue test)
    printf("\n=== Test 1: MQCMD_INQUIRE_TOPIC (using working pattern) ===\n");
    
    char buffer[65536];
    MQCFH *cfh = (MQCFH *)buffer;
    cfh->Type = MQCFT_COMMAND;
    cfh->StrucLength = MQCFH_STRUC_LENGTH;
    cfh->Version = MQCFH_VERSION_1;
    cfh->Command = MQCMD_INQUIRE_TOPIC;
    cfh->MsgSeqNumber = 1;
    cfh->Control = MQCFC_LAST;
    cfh->ParameterCount = 1;
    
    // Add topic name parameter (follow working pattern exactly)
    MQCFST *cfst = (MQCFST *)(buffer + MQCFH_STRUC_LENGTH);
    cfst->Type = MQCFT_STRING;
    cfst->StrucLength = MQCFST_STRUC_LENGTH_FIXED + 256;  // Topic names are 256 chars
    cfst->Parameter = MQCA_TOPIC_NAME;
    cfst->CodedCharSetId = MQCCSI_DEFAULT;
    cfst->StringLength = 1;  // Length of "*"
    memset(cfst->String, ' ', 256);  // Fill with spaces
    memcpy(cfst->String, "*", 1);    // Copy actual value
    
    // Send command (exact same as working test)
    strcpy(md.Format, MQFMT_ADMIN);
    md.MsgType = MQMT_REQUEST;
    memcpy(md.ReplyToQ, od.ObjectName, 48);
    
    printf("Sending MQCMD_INQUIRE_TOPIC (message size: %d bytes)\n", MQCFH_STRUC_LENGTH + cfst->StrucLength);
    
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
        }
        goto cleanup;
    }
    
    printf("✅ MQPUT successful! Getting response...\n");
    
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
    
    printf("✅ Response received (%d bytes)\n", bufLen);
    
    // Parse response
    MQCFH *respCfh = (MQCFH *)buffer;
    printf("Response: Type=%d, CompCode=%d, Reason=%d, ParameterCount=%d\n",
           respCfh->Type, respCfh->CompCode, respCfh->Reason, respCfh->ParameterCount);
    
    if (respCfh->CompCode == MQCC_OK) {
        printf("✅ SUCCESS: MQCMD_INQUIRE_TOPIC works!\n");
        
        // Try to find topics in the response
        if (respCfh->ParameterCount > 0) {
            printf("Found %d topic(s) in response\n", respCfh->ParameterCount);
            
            // Now test MQCMD_INQUIRE_TOPIC_STATUS with a specific topic
            printf("\n=== Test 2: MQCMD_INQUIRE_TOPIC_STATUS (after confirming topics exist) ===\n");
            
            // Reset for next test
            cfh->Command = MQCMD_INQUIRE_TOPIC_STATUS;
            md = (MQMD){MQMD_DEFAULT};
            strcpy(md.Format, MQFMT_ADMIN);
            md.MsgType = MQMT_REQUEST;
            memcpy(md.ReplyToQ, od.ObjectName, 48);
            
            printf("Sending MQCMD_INQUIRE_TOPIC_STATUS...\n");
            
            MQPUT(hConn, hObj, &md, &pmo, MQCFH_STRUC_LENGTH + cfst->StrucLength, buffer, &compCode, &reason);
            
            if (compCode != MQCC_OK) {
                printf("MQPUT failed: CompCode=%d, Reason=%d\n", compCode, reason);
                printf("  MQCMD_INQUIRE_TOPIC_STATUS doesn't work\n");
            } else {
                printf("✅ MQPUT successful! Getting topic status response...\n");
                
                // Get response
                memcpy(md.CorrelId, md.MsgId, sizeof(md.MsgId));
                memset(md.MsgId, 0, sizeof(md.MsgId));
                gmo.Options = MQGMO_WAIT | MQGMO_CONVERT;
                gmo.WaitInterval = 5000;
                
                bufLen = sizeof(buffer);
                MQGET(hConn, hReplyQ, &md, &gmo, bufLen, buffer, &bufLen, &compCode, &reason);
                
                if (compCode == MQCC_OK) {
                    respCfh = (MQCFH *)buffer;
                    printf("✅ Topic Status Response: CompCode=%d, Reason=%d, Parameters=%d\n",
                           respCfh->CompCode, respCfh->Reason, respCfh->ParameterCount);
                    
                    if (respCfh->CompCode == MQCC_OK) {
                        printf("✅ SUCCESS: MQCMD_INQUIRE_TOPIC_STATUS works!\n");
                        printf("  This means topic monitoring is feasible\n");
                    } else {
                        printf("❌ Topic status command failed: CompCode=%d, Reason=%d\n",
                               respCfh->CompCode, respCfh->Reason);
                    }
                } else {
                    printf("MQGET failed: CompCode=%d, Reason=%d\n", compCode, reason);
                }
            }
        }
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
    
    printf("\n=== Final Assessment ===\n");
    printf("This test uses the exact same pattern as the working queue tests.\n");
    printf("If topic commands still fail, topic monitoring may not be supported in this MQ setup.\n");
    
    return 0;
}