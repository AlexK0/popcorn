cmake_minimum_required(VERSION 3.16)
project(example)

set(CMAKE_CXX_STANDARD 17)

find_package(GTest QUIET)

if(GTest_FOUND)
    include(GoogleTest)
else()
    include(FetchContent)
    FetchContent_Declare(
            googletest
            GIT_REPOSITORY https://github.com/google/googletest.git
            GIT_TAG        release-1.10.0
    )
    FetchContent_MakeAvailable(googletest)
endif()

add_compile_options(-Wall -Wextra)
include_directories(src)
add_executable(example src/main.cpp src/my-math.cpp)

add_executable(unittests tests/tests-math.cpp src/my-math.cpp)
target_link_libraries(unittests PRIVATE gtest gtest_main -lpthread)
