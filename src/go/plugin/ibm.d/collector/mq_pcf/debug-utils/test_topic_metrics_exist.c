// Test if expected topic metrics actually exist in MQ
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <cmqc.h>
#include <cmqxc.h>
#include <cmqcfc.h>

int main() {
    printf("=== Testing Topic Metrics Existence ===\n\n");
    
    // Test 1: Check if MQCMD_INQUIRE_TOPIC_STATUS command exists
    printf("1. Testing MQCMD_INQUIRE_TOPIC_STATUS command:\n");
    #ifdef MQCMD_INQUIRE_TOPIC_STATUS
        printf("   ✓ MQCMD_INQUIRE_TOPIC_STATUS is defined (value: %d)\n", MQCMD_INQUIRE_TOPIC_STATUS);
    #else
        printf("   ✗ MQCMD_INQUIRE_TOPIC_STATUS is NOT defined\n");
    #endif
    
    // Test 2: Check if publisher count attribute exists
    printf("\n2. Testing MQIA_PUB_COUNT attribute:\n");
    #ifdef MQIA_PUB_COUNT
        printf("   ✓ MQIA_PUB_COUNT is defined (value: %d)\n", MQIA_PUB_COUNT);
    #else
        printf("   ✗ MQIA_PUB_COUNT is NOT defined\n");
    #endif
    
    // Test 3: Check if subscriber count attribute exists
    printf("\n3. Testing MQIA_SUB_COUNT attribute:\n");
    #ifdef MQIA_SUB_COUNT
        printf("   ✓ MQIA_SUB_COUNT is defined (value: %d)\n", MQIA_SUB_COUNT);
    #else
        printf("   ✗ MQIA_SUB_COUNT is NOT defined\n");
    #endif
    
    // Test 4: Check if publish message count attribute exists
    printf("\n4. Testing MQIAMO_PUBLISH_MSG_COUNT attribute:\n");
    #ifdef MQIAMO_PUBLISH_MSG_COUNT
        printf("   ✓ MQIAMO_PUBLISH_MSG_COUNT is defined (value: %d)\n", MQIAMO_PUBLISH_MSG_COUNT);
    #else
        printf("   ✗ MQIAMO_PUBLISH_MSG_COUNT is NOT defined\n");
    #endif
    
    // Test 5: Check what topic-related commands ARE available
    printf("\n5. Available topic-related commands:\n");
    #ifdef MQCMD_INQUIRE_TOPIC
        printf("   ✓ MQCMD_INQUIRE_TOPIC is defined (value: %d)\n", MQCMD_INQUIRE_TOPIC);
    #else
        printf("   ✗ MQCMD_INQUIRE_TOPIC is NOT defined\n");
    #endif
    
    #ifdef MQCMD_CREATE_TOPIC
        printf("   ✓ MQCMD_CREATE_TOPIC is defined (value: %d)\n", MQCMD_CREATE_TOPIC);
    #else
        printf("   ✗ MQCMD_CREATE_TOPIC is NOT defined\n");
    #endif
    
    #ifdef MQCMD_DELETE_TOPIC
        printf("   ✓ MQCMD_DELETE_TOPIC is defined (value: %d)\n", MQCMD_DELETE_TOPIC);
    #else
        printf("   ✗ MQCMD_DELETE_TOPIC is NOT defined\n");
    #endif
    
    // Test 6: Check what subscription-related commands ARE available
    printf("\n6. Available subscription-related commands:\n");
    #ifdef MQCMD_INQUIRE_SUB
        printf("   ✓ MQCMD_INQUIRE_SUB is defined (value: %d)\n", MQCMD_INQUIRE_SUB);
    #else
        printf("   ✗ MQCMD_INQUIRE_SUB is NOT defined\n");
    #endif
    
    #ifdef MQCMD_INQUIRE_SUB_STATUS
        printf("   ✓ MQCMD_INQUIRE_SUB_STATUS is defined (value: %d)\n", MQCMD_INQUIRE_SUB_STATUS);
    #else
        printf("   ✗ MQCMD_INQUIRE_SUB_STATUS is NOT defined\n");
    #endif
    
    // Test 7: Check what topic attributes ARE available
    printf("\n7. Available topic attributes:\n");
    #ifdef MQCA_TOPIC_NAME
        printf("   ✓ MQCA_TOPIC_NAME is defined (value: %d)\n", MQCA_TOPIC_NAME);
    #else
        printf("   ✗ MQCA_TOPIC_NAME is NOT defined\n");
    #endif
    
    #ifdef MQCA_TOPIC_STRING
        printf("   ✓ MQCA_TOPIC_STRING is defined (value: %d)\n", MQCA_TOPIC_STRING);
    #else
        printf("   ✗ MQCA_TOPIC_STRING is NOT defined\n");
    #endif
    
    #ifdef MQIA_TOPIC_TYPE
        printf("   ✓ MQIA_TOPIC_TYPE is defined (value: %d)\n", MQIA_TOPIC_TYPE);
    #else
        printf("   ✗ MQIA_TOPIC_TYPE is NOT defined\n");
    #endif
    
    printf("\n=== Summary ===\n");
    printf("This test checks if the MQ constants used in the Go collector actually exist.\n");
    printf("If key constants are missing, the collector is trying to use non-existent metrics.\n");
    
    return 0;
}