//go:build !js

package funcs

// 注册 SQLite 驱动。浏览器（wasm）环境不支持这两个驱动，故仅在非 js 平台引入。
import (
	_ "github.com/mattn/go-sqlite3"
	_ "modernc.org/sqlite"
)
