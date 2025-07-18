// Simple connection test
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <cmqc.h>
#include <cmqxc.h>

int main() {
    MQHCONN hConn = MQHC_UNUSABLE_HCONN;
    MQLONG compCode, reason;
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
    
    printf("Successfully connected to QM1!\n");
    
    // Disconnect
    MQDISC(&hConn, &compCode, &reason);
    printf("Disconnected\n");
    
    return 0;
}