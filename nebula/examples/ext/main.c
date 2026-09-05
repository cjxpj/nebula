/*
 * 示例扩展：演示 Nebula C 扩展（原生动态库）的各种用法。
 *
 * 编译（Windows 用 MinGW-w64 gcc，Linux/Android/鸿蒙用系统 gcc）：
 *   Windows：            gcc -shared -Os -s -ffunction-sections -fdata-sections -Wl,--gc-sections -fno-asynchronous-unwind-tables -fno-stack-protector -o example.dll main.c
 *   Linux/Android/鸿蒙： gcc -shared -fPIC -Os -s -ffunction-sections -fdata-sections -Wl,--gc-sections -fno-asynchronous-unwind-tables -fno-stack-protector -o example.so main.c
 *
 * 或直接运行 build.ps1。将编译产物放入主程序数据目录 private/plugins/ 下，
 * 重启主程序即可在词库中用 $扩展测试$ / $扩展加法 1 2$ 等调用。
 */
#include <stdio.h>
#include <string.h>
#include <stdlib.h>

#include "ext.h"

/* 函数描述表：name 为词库调用的函数名，l 为参数数量规则（"0"、"1|2"、"2.." 等）。 */
static const struct {
    const char* name;
    const char* l;
} funcs[] = {
    {"扩展测试", "0"},
    {"扩展回显", "1"},
    {"扩展加法", "2"},
    {"扩展长度", "1"},
    {"扩展报错", "0"},
};

#define FUNC_COUNT ((int)(sizeof(funcs) / sizeof(funcs[0])))

/* 结果缓冲区。服务端串行调用扩展，单缓冲区即可；返回后服务端会立即复制。 */
static char result_buf[4096];

int ext_init(void) {
    printf("example 扩展已加载（%d 个函数）\n", FUNC_COUNT);
    return 0;
}
void ext_close(void) {}

int ext_count(void) {
    return FUNC_COUNT;
}

const char* ext_name(int index) {
    if (index < 0 || index >= FUNC_COUNT) {
        return NULL;
    }
    return funcs[index].name;
}

const char* ext_l(int index) {
    if (index < 0 || index >= FUNC_COUNT) {
        return NULL;
    }
    return funcs[index].l;
}

const char* ext_call(const char* name, const char* const* args, int argc, const char** err) {
    *err = NULL;
    if (name == NULL) {
        *err = "未知函数";
        return NULL;
    }

    if (strcmp(name, "扩展测试") == 0) {
        snprintf(result_buf, sizeof(result_buf), "扩展加载成功");
        return result_buf;
    }

    if (strcmp(name, "扩展回显") == 0) {
        snprintf(result_buf, sizeof(result_buf), "%s", (argc > 0 && args[0]) ? args[0] : "");
        return result_buf;
    }

    if (strcmp(name, "扩展加法") == 0) {
        double a = (argc > 0 && args[0]) ? atof(args[0]) : 0.0;
        double b = (argc > 1 && args[1]) ? atof(args[1]) : 0.0;
        snprintf(result_buf, sizeof(result_buf), "%g", a + b);
        return result_buf;
    }

    if (strcmp(name, "扩展长度") == 0) {
        const char* s = (argc > 0 && args[0]) ? args[0] : "";
        /* 返回 UTF-8 字节数 */
        snprintf(result_buf, sizeof(result_buf), "%zu", strlen(s));
        return result_buf;
    }

    if (strcmp(name, "扩展报错") == 0) {
        *err = "这是一个测试错误";
        return NULL;
    }

    *err = "未知函数";
    return NULL;
}
