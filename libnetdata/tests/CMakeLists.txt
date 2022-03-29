cmake_minimum_required(VERSION 3.14)
project(queue_test)

set(CMAKE_C_STANDARD 99)
# GoogleTest requires at least C++11
set(CMAKE_CXX_STANDARD 11)
include(FetchContent)
FetchContent_Declare(
  googletest
  URL https://github.com/google/googletest/archive/609281088cfefc76f9d0ce82e1ff6c30cc3591e5.zip
)
# For Windows: Prevent overriding the parent project's compiler/linker settings
set(gtest_force_shared_crt ON CACHE BOOL "" FORCE)
FetchContent_MakeAvailable(googletest)

#include_directories(..)
#include_directories(../queue)

enable_testing()

add_executable(
  test_queue_run
  test_queue.cc
  ../queue/queue.c
)

target_link_libraries(
  test_queue_run
  gtest_main
)

include(GoogleTest)
gtest_discover_tests(test_queue_run)
