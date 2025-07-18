// Test MQCMD_INQUIRE_TOPIC_STATUS against real MQ to see what data is available
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
    
    printf("=== Testing MQCMD_INQUIRE_TOPIC_STATUS Against Real MQ ===\n\n");
    
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
    MQOPEN(hConn, &od, MQOO_OUTPUT | MQOO_FAIL_IF_QUIESCING, &hObj, &compCode, &reason);
    if (compCode != MQCC_OK) {
        printf("MQOPEN admin queue failed: CompCode=%d, Reason=%d\n", compCode, reason);
        MQDISC(&hConn, &compCode, &reason);
        return 1;
    }
    printf("Admin queue opened\n");
    
    // Create reply queue
    MQOD replyOd = {MQOD_DEFAULT};
    strcpy(replyOd.ObjectName, "SYSTEM.DEFAULT.MODEL.QUEUE");
    strcpy(replyOd.DynamicQName, "TOPIC.STATUS.*");
    replyOd.ObjectType = MQOT_Q;
    MQOPEN(hConn, &replyOd, MQOO_INPUT_AS_Q_DEF | MQOO_FAIL_IF_QUIESCING, &hReplyQ, &compCode, &reason);
    if (compCode != MQCC_OK) {
        printf("MQOPEN reply queue failed: CompCode=%d, Reason=%d\n", compCode, reason);
        MQCLOSE(hConn, &hObj, MQCO_NONE, &compCode, &reason);
        MQDISC(&hConn, &compCode, &reason);
        return 1;
    }
    printf("Reply queue created: %s\n", replyOd.ObjectName);
    
    // Test 1: Try MQCMD_INQUIRE_TOPIC_STATUS with wildcard
    printf("\n=== Test 1: MQCMD_INQUIRE_TOPIC_STATUS with wildcard ===\n");
    
    char buffer[65536];
    MQCFH *cfh = (MQCFH *)buffer;
    
    // PCF header
    cfh->Type = MQCFT_COMMAND;
    cfh->StrucLength = MQCFH_STRUC_LENGTH;
    cfh->Version = MQCFH_VERSION_1;
    cfh->Command = MQCMD_INQUIRE_TOPIC_STATUS;
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
    md.CodedCharSetId = 1208;  // UTF-8 
    md.Encoding = MQENC_NATIVE;
    memcpy(md.ReplyToQ, replyOd.ObjectName, 48);
    
    MQLONG msgSize = MQCFH_STRUC_LENGTH + cfst->StrucLength;
    printf("Sending MQCMD_INQUIRE_TOPIC_STATUS with wildcard (message size: %d bytes)\n", msgSize);
    
    pmo.Options = MQPMO_NO_SYNCPOINT | MQPMO_FAIL_IF_QUIESCING;
    
    MQPUT(hConn, hObj, &md, &pmo, msgSize, buffer, &compCode, &reason);
    
    if (compCode != MQCC_OK) {
        printf("MQPUT failed: CompCode=%d, Reason=%d\n", compCode, reason);
        if (reason == 2035) {
            printf("  MQRC_NOT_AUTHORIZED - Need topic inquiry permissions\n");
        } else if (reason == 2085) {
            printf("  MQRC_UNKNOWN_OBJECT_NAME - Topic doesn't exist or command not supported\n");
        } else if (reason == 2068) {
            printf("  MQRC_OBJECT_IN_USE - Topic is in use\n");
        } else if (reason == 2027) {
            printf("  MQRC_MSG_TOO_BIG_FOR_Q - Message too big (PCF marshaling issue)\n");
        } else if (reason == 2115) {
            printf("  MQRC_TARGET_CCSID_ERROR - Character set conversion issue\n");
        } else if (reason == 3008) {
            printf("  MQRCCF_COMMAND_FAILED - Command failed\n");
        } else if (reason == 3013) {
            printf("  MQRCCF_OBJECT_ALREADY_EXISTS - Object already exists\n");
        } else if (reason == 3024) {
            printf("  MQRCCF_OBJECT_OPEN_ERROR - Object open error\n");
        } else if (reason == 3065) {
            printf("  MQRCCF_OBJECT_NOT_FOUND - Object not found\n");
        }
        goto cleanup;
    }
    
    printf("MQPUT successful! Waiting for response...\n");
    
    // Get response
    memcpy(md.CorrelId, md.MsgId, sizeof(md.MsgId));
    memset(md.MsgId, 0, sizeof(md.MsgId));
    gmo.Options = MQGMO_WAIT | MQGMO_CONVERT;
    gmo.WaitInterval = 10000;  // 10 seconds
    
    MQLONG bufLen = sizeof(buffer);
    MQGET(hConn, hReplyQ, &md, &gmo, bufLen, buffer, &bufLen, &compCode, &reason);
    
    if (compCode != MQCC_OK) {
        printf("MQGET failed: CompCode=%d, Reason=%d\n", compCode, reason);
        if (reason == 2033) {
            printf("  MQRC_NO_MSG_AVAILABLE - No response received (timeout)\n");
        }
        goto cleanup;
    }
    
    printf("Response received (%d bytes)\n", bufLen);
    
    // Parse response
    MQCFH *respCfh = (MQCFH *)buffer;
    printf("Response: Type=%d, CompCode=%d, Reason=%d, ParameterCount=%d\n",
           respCfh->Type, respCfh->CompCode, respCfh->Reason, respCfh->ParameterCount);
    
    if (respCfh->CompCode == MQCC_OK) {
        printf("✅ SUCCESS: MQCMD_INQUIRE_TOPIC_STATUS command works!\n");
        
        // Try to parse parameters to see what's available
        printf("\nParsing response parameters...\n");
        char *paramPtr = buffer + MQCFH_STRUC_LENGTH;
        
        for (int i = 0; i < respCfh->ParameterCount; i++) {
            MQCFH *param = (MQCFH *)paramPtr;
            
            if (param->Type == MQCFT_INTEGER) {
                MQCFIN *cfin = (MQCFIN *)paramPtr;
                printf("[%2d] INTEGER: Parameter=%d, Value=%d", i+1, cfin->Parameter, cfin->Value);
                
                // Check if this is one of our expected metrics
                if (cfin->Parameter == MQIA_PUB_COUNT) {
                    printf(" (MQIA_PUB_COUNT - Publisher count!)");
                } else if (cfin->Parameter == MQIA_SUB_COUNT) {
                    printf(" (MQIA_SUB_COUNT - Subscriber count!)");
                } else if (cfin->Parameter == MQIAMO_PUBLISH_MSG_COUNT) {
                    printf(" (MQIAMO_PUBLISH_MSG_COUNT - Published message count!)");
                }
                printf("\n");
                
                paramPtr += cfin->StrucLength;
            } else if (param->Type == MQCFT_STRING) {
                MQCFST *cfst = (MQCFST *)paramPtr;
                printf("[%2d] STRING: Parameter=%d, Length=%d", i+1, cfst->Parameter, cfst->StringLength);
                
                if (cfst->Parameter == MQCA_TOPIC_NAME) {
                    printf(" (MQCA_TOPIC_NAME)");
                    if (cfst->StringLength > 0) {
                        char topicName[257];
                        memcpy(topicName, cfst->String, cfst->StringLength);
                        topicName[cfst->StringLength] = '\0';
                        printf(" = '%s'", topicName);
                    }
                }
                printf("\n");
                
                paramPtr += cfst->StrucLength;
            } else {
                printf("[%2d] UNKNOWN TYPE: %d\n", i+1, param->Type);
                break;
            }
        }
    } else {
        printf("❌ FAILED: MQCMD_INQUIRE_TOPIC_STATUS returned CompCode=%d, Reason=%d\n", 
               respCfh->CompCode, respCfh->Reason);
        
        if (respCfh->Reason == 2085) {
            printf("  This suggests topic status inquiry is not supported or no topics exist\n");
        } else if (respCfh->Reason == 2035) {
            printf("  This suggests insufficient permissions for topic status inquiry\n");
        }
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
    
    printf("\n=== Summary ===\n");
    printf("This test verifies if MQCMD_INQUIRE_TOPIC_STATUS actually works against real MQ.\n");
    printf("If it fails, the collector's topic monitoring expectations are incorrect.\n");
    
    return 0;
}