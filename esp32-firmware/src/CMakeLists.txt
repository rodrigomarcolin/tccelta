# Todo o código do app vira o componente do ESP-IDF. O build_src_filter do
# PlatformIO não vale com ESP-IDF: arquivos específicos de um env ficam
# protegidos por #if (ex.: Mcp2515Can.cpp com USE_MCP2515).
FILE(GLOB_RECURSE app_sources ${CMAKE_SOURCE_DIR}/src/*.*)
idf_component_register(SRCS ${app_sources})
