package funcs

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"

	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
)

// EnsureMoneyTable 确保全局数据库存在 money 表，类型+账号作为联合主键（索引）。
func EnsureMoneyTable(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS "money" (
			type TEXT NOT NULL,
			account TEXT NOT NULL,
			balance INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (type, account)
		)
	`)
	return err
}

// moneyBalance 读取指定类型/账号的余额，返回字符串；不存在返回 "0"。
func moneyBalance(db *sql.DB, typ, account string) (any, error) {
	var balance int64
	err := db.QueryRow(
		`SELECT balance FROM "money" WHERE type=? AND account=?`,
		typ, account,
	).Scan(&balance)
	if err != nil {
		return "0", nil
	}
	return strconv.FormatInt(balance, 10), nil
}

// moneyBalanceAndRank 查询指定账号的余额与当前排名，返回 {"余额":...,"排名":...}。
// 排名按余额降序、同余额账号升序计算，与 db_查询排名 一致；账号不存在时余额视为 0。
func moneyBalanceAndRank(db *sql.DB, typ, account string) (any, error) {
	var balance int64
	err := db.QueryRow(
		`SELECT balance FROM "money" WHERE type=? AND account=?`,
		typ, account,
	).Scan(&balance)
	if err != nil {
		balance = 0
	}

	var rank int64
	err = db.QueryRow(`
		SELECT COUNT(*) FROM "money"
		WHERE type=? AND (balance > ? OR (balance = ? AND account < ?))
	`, typ, balance, balance, account).Scan(&rank)
	if err != nil {
		return "{}", nil
	}

	return utils.AnyToString(map[string]any{
		"余额": balance,
		"排名": rank + 1,
	}), nil
}

// db_添加 <类型> <账号> <整数>：增加余额，返回操作后的余额。
func dbMoneyAdd(d *dto.DicInputs) (any, error) {
	db, err := GetGlobalDB()
	if err != nil {
		return nil, fmt.Errorf("全局数据库初始化失败: %w", err)
	}
	if err = EnsureMoneyTable(db); err != nil {
		return nil, err
	}

	typ := d.Inputs.String(1)
	account := d.Inputs.String(2)
	amount := d.Inputs.Int(3)
	if typ == "" || account == "" {
		return nil, errors.New("类型和账号不能为空")
	}

	_, err = db.Exec(`
		INSERT INTO "money" (type, account, balance)
		VALUES (?, ?, ?)
		ON CONFLICT(type, account) DO UPDATE SET
			balance = balance + excluded.balance
	`, typ, account, amount)
	if err != nil {
		return nil, err
	}

	return moneyBalance(db, typ, account)
}

// db_减少 <类型> <账号> <整数>：减少余额，返回操作后的余额。
func dbMoneySub(d *dto.DicInputs) (any, error) {
	db, err := GetGlobalDB()
	if err != nil {
		return nil, fmt.Errorf("全局数据库初始化失败: %w", err)
	}
	if err = EnsureMoneyTable(db); err != nil {
		return nil, err
	}

	typ := d.Inputs.String(1)
	account := d.Inputs.String(2)
	amount := d.Inputs.Int(3)
	if typ == "" || account == "" {
		return nil, errors.New("类型和账号不能为空")
	}

	_, err = db.Exec(`
		INSERT INTO "money" (type, account, balance)
		VALUES (?, ?, ?)
		ON CONFLICT(type, account) DO UPDATE SET
			balance = balance - excluded.balance
	`, typ, account, amount)
	if err != nil {
		return nil, err
	}

	return moneyBalance(db, typ, account)
}

// db_设置 <类型> <账号> <整数或负数>：直接设置余额，返回操作后的余额。
func dbMoneySet(d *dto.DicInputs) (any, error) {
	db, err := GetGlobalDB()
	if err != nil {
		return nil, fmt.Errorf("全局数据库初始化失败: %w", err)
	}
	if err = EnsureMoneyTable(db); err != nil {
		return nil, err
	}

	typ := d.Inputs.String(1)
	account := d.Inputs.String(2)
	balance := d.Inputs.Int(3)
	if typ == "" || account == "" {
		return nil, errors.New("类型和账号不能为空")
	}

	_, err = db.Exec(`
		INSERT INTO "money" (type, account, balance)
		VALUES (?, ?, ?)
		ON CONFLICT(type, account) DO UPDATE SET
			balance = excluded.balance
	`, typ, account, balance)
	if err != nil {
		return nil, err
	}

	return moneyBalance(db, typ, account)
}

// db_查询 <类型> <账号>：账号留空返回 {} 数据（账号->余额），否则返回 {"余额":...,"排名":...}。
func dbMoneyQuery(d *dto.DicInputs) (any, error) {
	db, err := GetGlobalDB()
	if err != nil {
		return nil, fmt.Errorf("全局数据库初始化失败: %w", err)
	}
	if err = EnsureMoneyTable(db); err != nil {
		return nil, err
	}

	typ := d.Inputs.String(1)
	if typ == "" {
		return nil, errors.New("类型不能为空")
	}

	account := d.Inputs.String(2)
	if account == "" {
		rows, err := db.Query(
			`SELECT account, balance FROM "money" WHERE type=? ORDER BY account ASC`,
			typ,
		)
		if err != nil {
			return "{}", nil
		}
		defer rows.Close()

		result := make(map[string]int64)
		for rows.Next() {
			var acc string
			var bal int64
			if err := rows.Scan(&acc, &bal); err == nil {
				result[acc] = bal
			}
		}
		return utils.AnyToString(result), nil
	}

	return moneyBalanceAndRank(db, typ, account)
}

// db_查询排名 <类型> <页数> <显示数量>：按余额降序返回排名列表；页数留空返回全部，显示数量默认 10。
func dbMoneyRank(d *dto.DicInputs) (any, error) {
	db, err := GetGlobalDB()
	if err != nil {
		return nil, fmt.Errorf("全局数据库初始化失败: %w", err)
	}
	if err = EnsureMoneyTable(db); err != nil {
		return nil, err
	}

	typ := d.Inputs.String(1)
	if typ == "" {
		return nil, errors.New("类型不能为空")
	}

	// 页数留空：返回全部列表
	all := d.Inputs.Len() < 2
	page := d.Inputs.IntDefault(2, 1)
	count := d.Inputs.IntDefault(3, 10)
	if page < 1 {
		page = 1
	}
	if count < 1 {
		count = 10
	}

	var rows *sql.Rows
	rank := 1
	if all {
		rows, err = db.Query(
			`SELECT account, balance FROM "money" WHERE type=? ORDER BY balance DESC, account ASC`,
			typ,
		)
	} else {
		offset := (page - 1) * count
		rank = offset + 1
		rows, err = db.Query(
			`SELECT account, balance FROM "money" WHERE type=? ORDER BY balance DESC, account ASC LIMIT ? OFFSET ?`,
			typ, count, offset,
		)
	}
	if err != nil {
		return "[]", nil
	}
	defer rows.Close()

	list := make([]map[string]any, 0)
	for rows.Next() {
		var acc string
		var bal int64
		if err := rows.Scan(&acc, &bal); err == nil {
			list = append(list, map[string]any{
				"排名": rank,
				"账号": acc,
				"余额": bal,
			})
			rank++
		}
	}
	return utils.AnyToString(list), nil
}

// db_清空经济系统 <类型>：清空指定类型的所有记录；类型留空则清空整个 money 表。
func dbMoneyClear(d *dto.DicInputs) (any, error) {
	db, err := GetGlobalDB()
	if err != nil {
		return nil, fmt.Errorf("全局数据库初始化失败: %w", err)
	}
	if err = EnsureMoneyTable(db); err != nil {
		return nil, err
	}

	typ := d.Inputs.String(1)
	if typ == "" {
		_, err = db.Exec(`DELETE FROM "money"`)
	} else {
		_, err = db.Exec(`DELETE FROM "money" WHERE type=?`, typ)
	}
	if err != nil {
		return nil, err
	}
	return nil, nil
}
