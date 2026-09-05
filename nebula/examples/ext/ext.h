/*
 * Nebula 扩展动态库 C ABI 接口声明。
 *
 * 扩展是一个原生 C 动态库（Windows .dll / Linux/Android/鸿蒙 .so）。
 * 主程序（nebula）在运行时加载它，并调用以下固定符号枚举与执行字典函数，
 * 使词库可用 $函数名$ 调用扩展能力。
 *
 * 符号须以 C 链接导出（C++ 下用 extern "C" 包裹）：
 *   int         ext_init(void)
 *   void        ext_close(void)
 *   int         ext_count(void)
 *   const char* ext_name(int index)
 *   const char* ext_l(int index)
 *   const char* ext_call(const char* name, const char* const* args, int argc, const char** err)
 *
 * 返回字符串（ext_name / ext_l / ext_call 的结果与错误信息）指向的缓冲区
 * 必须在下一次调用本扩展或 ext_close 之前保持有效；服务端会立即复制，
 * 因此使用单个静态缓冲区即可。服务端串行调用扩展，无需考虑多线程并发。
 */
#ifndef NEBULA_EXT_H
#define NEBULA_EXT_H

#ifdef __cplusplus
extern "C" {
#endif

int ext_init(void);
void ext_close(void);
int ext_count(void);
const char* ext_name(int index);
const char* ext_l(int index);
const char* ext_call(const char* name, const char* const* args, int argc, const char** err);

#ifdef __cplusplus
}
#endif

#endif /* NEBULA_EXT_H */
