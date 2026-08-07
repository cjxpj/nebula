# AGENTS.md

## Code Review（代码审查规则）

### 路径白名单

以下路径在代码审查中 **必须跳过**，不读取、不评论，即使出现在 diff、MR/PR 或指定文件范围中：

- `c:\Users\admin\Documents\vue\nebulaOpUI` — opui 前端源码项目（Vue），无需代码审查
