cmake_minimum_required(VERSION 3.14)
project(streaming_test)

set(CMAKE_C_STANDARD 99)
# GoogleTest requires at least C++11
set(CMAKE_CXX_STANDARD 11)
include(FetchContent)
FetchContent_Declare(
  googletest
  URL https://github.com/google/googletest/archive/refs/tags/release-1.11.0.zip
)
# For Windows: Prevent overriding the parent project's compiler/linker settings
set(gtest_force_shared_crt ON CACHE BOOL "" FORCE)
FetchContent_MakeAvailable(googletest)

#include_directories(../../database)
#include_directories(../../libnetdata)
#include_directories(../../streaming)

#include_directories(../../*)

enable_testing()

add_executable(
  streaming_test_run
  tests.cc
  ../receiver.c
  ../sender.c
  ../rrdpush.c
  ../replication.c      
)

target_link_libraries(
  streaming_test_run
  gtest_main
)

include(GoogleTest)
gtest_discover_tests(streaming_test_run)
