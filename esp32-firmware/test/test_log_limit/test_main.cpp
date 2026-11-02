#include <unity.h>
#include "collector/LogLimit.h"

using namespace collector;

namespace {
constexpr size_t   k2MB = 2097152;
constexpr uint32_t kCeiling = 256 * kBlocksPerSegment;
}

void setUp() {}
void tearDown() {}

void test_registro_maximo() {
    TEST_ASSERT_EQUAL_UINT32(1029, maxRecordBytes(0));
    TEST_ASSERT_EQUAL_UINT32(1061, maxRecordBytes(32));
    TEST_ASSERT_EQUAL_UINT32(1093, maxRecordBytes(64));
}

void test_2mb_10pct_tag32() {
    // margem 209715 B; (2097152 - 209715) / 1061 = 1778
    TEST_ASSERT_EQUAL_UINT32(1778, maxBlocksFor(k2MB, 10, 32, kCeiling));
}

void test_tag0_e_tag64() {
    TEST_ASSERT_EQUAL_UINT32(1887437 / 1029, maxBlocksFor(k2MB, 10, 0, kCeiling));
    TEST_ASSERT_EQUAL_UINT32(1887437 / 1093, maxBlocksFor(k2MB, 10, 64, kCeiling));
}

void test_margem_zero() {
    TEST_ASSERT_EQUAL_UINT32(k2MB / 1061, maxBlocksFor(k2MB, 0, 32, kCeiling));
}

void test_piso() {
    TEST_ASSERT_EQUAL_UINT32(128, maxBlocksFor(k2MB, 99, 32, kCeiling));
    TEST_ASSERT_EQUAL_UINT32(128, maxBlocksFor(0, 10, 32, kCeiling));
}

void test_teto() {
    TEST_ASSERT_EQUAL_UINT32(16384, maxBlocksFor(64u * 1024 * 1024, 10, 32, kCeiling));
}

int main() {
    UNITY_BEGIN();
    RUN_TEST(test_registro_maximo);
    RUN_TEST(test_2mb_10pct_tag32);
    RUN_TEST(test_tag0_e_tag64);
    RUN_TEST(test_margem_zero);
    RUN_TEST(test_piso);
    RUN_TEST(test_teto);
    return UNITY_END();
}
