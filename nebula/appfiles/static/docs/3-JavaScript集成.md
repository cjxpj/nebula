# JavaScript 集成

## 基本用法

**支持库**：`Buffer`、`setTimeout`、`setInterval`、`console`、`url`、`require`；词库参数以逗号分隔、从参数 0 开始（如 `a,b,c`）：

```
a:1
--js
a+a
--end
```

## 回调词库函数

在 JavaScript 中用 `runDic` 回调词库函数：

```
a:1
--js
var result = runDic('函数名', '参数1', '参数2');   // 同步调用：返回执行结果
runDic('异步函数', '参数', function(r) { console.log(r); });   // 异步调用：结果走回调
--end
```

参数：第一个为词库函数名，后续为传给词库函数的参数，最后一个（可选）为接收异步结果的回调函数。返回值：同步调用返回执行结果；异步调用无返回值，结果通过回调函数获取。
