# SQLite

`$打开sqlite$` 返回「SQLite 连接对象」（面对像），后续通过 `$对象.方法$` 操作，文件记得使用 `.db` 后缀。

```
连接:$打开sqlite <文件|:内存:> <默认值>$    // 文件为空或 :内存: 表示内存数据库；打开失败返回第 2 个参数默认值
连接:$打开sqlite database/data.db$
连接:$打开sqlite :内存:$
$连接.写 <表单> <键> <值>$
$连接.读$                        // 获取全部表单（JSON 数组）
$连接.读 <表单>$                  // 获取全部键（JSON 数组）
$连接.读 <表单> <键>$             // 读取值
$连接.读 <表单> <键> <默认值>$     // 读取，不存在返回默认值
$连接.删除文件 <key>$             // 删除 fs_files 表中记录
$连接.删除文件夹 <key>$           // 删除 fs_files 表中记录
$连接.执行 <sql> <绑定参数>...$    // 执行 SQL，返回 JSON，结构同 MySQL
$连接.关闭$                       // 返回 "true" / "false"
```

执行语句示例

```
查询语句
c:"""
SELECT name, email FROM users
"""
$连接.执行 %c%$

插入语句
b:"""
INSERT INTO users (name, email) VALUES (?, ?)
"""
$连接.执行 %b% a b$

创建表语句
a:"""
CREATE TABLE IF NOT EXISTS users (
	name TEXT NOT NULL,
	email TEXT NOT NULL UNIQUE
)
"""
$连接.执行 %a%$
```

快捷读写（无需手动打开连接，默认在 fs_files 表单下读写）

```
$读sqlite <文件> <键> <默认值>$
$读sqlite <文件>$        // 获取全部键
$写sqlite <文件> <键> <值>$
```
