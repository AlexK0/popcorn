// tests-math.cpp
#include <gtest/gtest.h>

#include "my-math.h"

TEST(math_tests, test_sum) {
  ASSERT_EQ(sum(1, 2), 3);
}

TEST(math_tests, test_diff) {
  ASSERT_EQ(diff(1, 2), -1);
}
