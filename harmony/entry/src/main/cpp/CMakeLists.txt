cmake_minimum_required(VERSION 3.5.0)
project(nebula)

set(NATIVERENDER_ROOT_PATH ${CMAKE_CURRENT_SOURCE_DIR})

include_directories(${NATIVERENDER_ROOT_PATH}/include)

# NAPI 桥接层，输出 libentry.so（ArkTS 通过 'libentry.so' 导入）
add_library(entry SHARED napi_init.cpp)

# 找到系统 NAPI 库
find_library(NAPI_LIB NAMES ace_napi.z)

# 链接 Go 编译出的 libnebula.so（位于 src/main/libs/${OHOS_ARCH}，由 build_ohos_so.ps1 生成）
target_link_directories(entry PUBLIC ${NATIVERENDER_ROOT_PATH}/../libs/${OHOS_ARCH})
target_link_libraries(entry PUBLIC ${NAPI_LIB} nebula)

# hvigor 只打包 CMake 输出目录里的 .so，而 libnebula.so 是预编译产物，
# 需要手动复制到 libentry.so 同一输出目录，否则不会被纳入 HAP。
add_custom_command(TARGET entry POST_BUILD
    COMMAND ${CMAKE_COMMAND} -E copy_if_different
        ${NATIVERENDER_ROOT_PATH}/../libs/${OHOS_ARCH}/libnebula.so
        $<TARGET_FILE_DIR:entry>)
