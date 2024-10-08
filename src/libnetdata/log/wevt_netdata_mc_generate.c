#include <stdio.h>
#include <inttypes.h>
#include <ctype.h>
#include <stdbool.h>

// from winnt.h
#define EVENTLOG_SUCCESS 0x0000
#define EVENTLOG_ERROR_TYPE 0x0001
#define EVENTLOG_WARNING_TYPE 0x0002
#define EVENTLOG_INFORMATION_TYPE 0x0004
#define EVENTLOG_AUDIT_SUCCESS 0x0008
#define EVENTLOG_AUDIT_FAILURE 0x0010

// the severities we define in .mc file
#define STATUS_SEVERITY_INFORMATIONAL 0x1
#define STATUS_SEVERITY_WARNING 0x2
#define STATUS_SEVERITY_ERROR 0x3

#define FACILITY_APPLICATION 0x0fff

#include "nd_log-common.h"
#include "nd_log-to-windows-common.h"

int main(int argc, const char **argv) {
    (void)argc; (void)argv;

    const char *header =
            "MessageIdTypedef=DWORD\r\n"
            "\r\n"
            "SeverityNames=(\r\n"
            "                Informational=0x1:STATUS_SEVERITY_INFORMATIONAL\r\n"
            "                Warning=0x2:STATUS_SEVERITY_WARNING\r\n"
            "                Error=0x3:STATUS_SEVERITY_ERROR\r\n"
            "              )\r\n"
            "\r\n"
            "FacilityNames=(\r\n"
            "                Application=0x0FFF:FACILITY_APPLICATION\r\n"
            "              )\r\n"
            "\r\n"
            "LanguageNames=(\r\n"
            "                English=0x409:MSG00409\r\n"
            "              )\r\n"
            "\r\n";

    bool done[UINT16_MAX] = { 0 };

    printf("%s", header);
    for(size_t src = 1; src < _NDLS_MAX ;src++) {
        for(size_t pri = 0; pri < _NDLP_MAX ;pri++) {
            uint8_t severity = get_severity_from_priority(pri);

            for(size_t msg = 1; msg < _MSGID_MAX ;msg++) {

                if(src >= 16) {
                    fprintf(stderr, "\n\nSource %zu is bigger than 4 bits!\n\n", src);
                    return 1;
                }

                if(pri >= 16) {
                    fprintf(stderr, "\n\nPriority %zu is bigger than 4 bits!\n\n", pri);
                    return 1;
                }

                if(msg >= 256) {
                    fprintf(stderr, "\n\nMessageID %zu is bigger than 8 bits!\n\n", msg);
                    return 1;
                }

                uint16_t eventID = construct_event_code(src, pri, msg);
                if((eventID & 0xFFFF) != eventID) {
                    fprintf(stderr, "\n\nEventID 0x%x is bigger than 16 bits!\n\n", eventID);
                    return 1;
                }

                if(done[eventID]) continue;
                done[eventID] = true;

                const char *pri_txt;
                switch(pri) {
                    case NDLP_EMERG:
                        pri_txt = "EMERG";
                        break;

                    case NDLP_CRIT:
                        pri_txt = "CRIT";
                        break;

                    case NDLP_ALERT:
                        pri_txt = "ALERT";
                        break;

                    case NDLP_ERR:
                        pri_txt = "ERR";
                        break;

                    case NDLP_WARNING:
                        pri_txt = "WARN";
                        break;

                    case NDLP_INFO:
                        pri_txt = "INFO";
                        break;

                    case NDLP_NOTICE:
                        pri_txt = "NOTICE";
                        break;

                    case NDLP_DEBUG:
                        pri_txt = "DEBUG";
                        break;

                    default:
                        fprintf(stderr, "\n\nInvalid priority %zu!\n\n\n", pri);
                        return 1;
                }

                const char *src_txt;
                switch(src) {
                    case NDLS_COLLECTORS:
                        src_txt = "COLLECTORS";
                        break;

                    case NDLS_ACCESS:
                        src_txt = "ACCESS";
                        break;

                    case NDLS_HEALTH:
                        src_txt = "HEALTH";
                        break;

                    case NDLS_DEBUG:
                        src_txt = "DEBUG";
                        break;

                    case NDLS_DAEMON:
                        src_txt = "DAEMON";
                        break;

                    case NDLS_ACLK:
                        src_txt = "ACLK";
                        break;

                    default:
                        fprintf(stderr, "\n\nInvalid source %zu!\n\n\n", src);
                        return 1;
                }

                const char *msg_txt;
                switch(msg) {
                    case MSGID_MESSAGE_ONLY:
                        msg_txt = "MESSAGE_ONLY";
                        break;

                    case MSGID_REQUEST_ONLY:
                        msg_txt = "REQUEST_ONLY";
                        break;

                    case MSGID_ACCESS:
                        msg_txt = "ACCESS";
                        break;

                    case MSGID_ACCESS_USER:
                        msg_txt = "ACCESS_USER";
                        break;

                    case MSGID_ACCESS_FORWARDER:
                        msg_txt = "ACCESS_FORWARDER";
                        break;

                    case MSGID_ACCESS_FORWARDER_USER:
                        msg_txt = "ACCESS_FORWARDER_USER";
                        break;

                    case MSGID_ALERT_TRANSITION:
                        msg_txt = "ALERT_TRANSITION";
                        break;

                    default:
                        fprintf(stderr, "\n\nInvalid message id %zu!\n\n\n", msg);
                        return 1;
                }

                const char *severity_txt;
                switch (severity) {
                    case STATUS_SEVERITY_INFORMATIONAL:
                        severity_txt = "Informational";
                        break;

                    case STATUS_SEVERITY_ERROR:
                        severity_txt = "Error";
                        break;

                    case STATUS_SEVERITY_WARNING:
                        severity_txt = "Warning";
                        break;

                    default:
                        fprintf(stderr, "\n\nInvalid severity id %u!\n\n\n", severity);
                        return 1;
                }

                char symbol[1024];

                snprintf(symbol, sizeof(symbol), "%s_%s_%s", src_txt, pri_txt, msg_txt);

                printf("MessageId=0x%x\r\n"
                       "Severity=%s\r\n"
                       "Facility=Application\r\n"
                       "SymbolicName=%s\r\n"
                       "Language=English\r\n"
                       "%%1\r\n"
                       ".\r\n"
                       "\r\n",
                       eventID, severity_txt, symbol);
            }
        }
    }
}