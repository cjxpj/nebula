package funcs

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cjxpj/nebula/debugLog"
	dic_api "github.com/cjxpj/nebula/dic/api"
	dic_dto "github.com/cjxpj/nebula/dic/dto"
	"github.com/cjxpj/nebula/dto"
	"github.com/cjxpj/nebula/utils"
)

// ScheduledTaskInfo 定时任务的对外信息（供函数列表与前端展示）
type ScheduledTaskInfo struct {
	ID         string `json:"id"`
	DicPath    string `json:"dic_path"`
	Trigger    string `json:"trigger"`
	Interval   string `json:"interval"`
	Once       bool   `json:"once"`
	RunAtStart bool   `json:"run_at_start"`
}

// ScheduledTask 定时任务运行时状态
type ScheduledTask struct {
	ID         string
	DicPath    string
	Trigger    string
	Interval   string
	Once       bool
	RunAtStart bool
	cancel     chan struct{}
}

// scheduledTasks 进程内定时任务存储（内存态），持久化副本位于全局数据库 scheduled_tasks 表
var scheduledTasks sync.Map // map[string]*ScheduledTask

// scheduledTaskTable 定时任务在全局数据库中的持久化表名
const scheduledTaskTable = "scheduled_tasks"

// AddScheduledTask 添加定时任务并启动调度，返回唯一编号；once 为 true 时仅执行一次，runAtStart 为 true 时启动立即触发一次
func AddScheduledTask(dicPath, trigger, interval string, once, runAtStart bool) (string, error) {
	dicPath = strings.TrimSpace(dicPath)
	if dicPath == "" {
		return "", errors.New("定时任务：词库路径不能为空")
	}
	interval = strings.TrimSpace(interval)
	if interval == "" {
		return "", errors.New("定时任务：执行间隔不能为空")
	}
	if _, err := parseInterval(interval); err != nil {
		return "", err
	}
	if trigger = strings.TrimSpace(trigger); trigger == "" {
		trigger = "Main"
	}

	id := fmt.Sprintf("%d", time.Now().UnixNano())
	task := &ScheduledTask{
		ID:         id,
		DicPath:    dicPath,
		Trigger:    trigger,
		Interval:   interval,
		Once:       once,
		RunAtStart: runAtStart,
		cancel:     make(chan struct{}),
	}
	scheduledTasks.Store(id, task)
	persistScheduledTask(task)
	go task.run()
	return id, nil
}

// DelScheduledTask 删除并停止指定编号的定时任务
func DelScheduledTask(id string) error {
	if id == "" {
		return errors.New("定时任务：编号不能为空")
	}
	v, loaded := scheduledTasks.LoadAndDelete(id)
	if !loaded {
		return errors.New("定时任务：编号不存在 " + id)
	}
	if task, ok := v.(*ScheduledTask); ok {
		close(task.cancel)
	}
	removeScheduledTaskFromDB(id)
	return nil
}

// ListScheduledTasks 返回全部定时任务信息，按编号升序
func ListScheduledTasks() []ScheduledTaskInfo {
	list := make([]ScheduledTaskInfo, 0)
	scheduledTasks.Range(func(key, value any) bool {
		task, ok := value.(*ScheduledTask)
		if !ok {
			return true
		}
		list = append(list, ScheduledTaskInfo{
			ID:         task.ID,
			DicPath:    task.DicPath,
			Trigger:    task.Trigger,
			Interval:   task.Interval,
			Once:       task.Once,
			RunAtStart: task.RunAtStart,
		})
		return true
	})
	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
	return list
}

// persistScheduledTask 将任务写入全局数据库，失败仅记日志不影响内存调度
func persistScheduledTask(task *ScheduledTask) {
	db, err := GetGlobalDB()
	if err != nil {
		debugLog.Infof("定时任务 %s 持久化失败（数据库不可用）: %v", task.ID, err)
		return
	}
	if err := EnsureFsTable(db, scheduledTaskTable); err != nil {
		debugLog.Infof("定时任务 %s 持久化失败（建表）: %v", task.ID, err)
		return
	}
	b, err := json.Marshal(ScheduledTaskInfo{
		ID:         task.ID,
		DicPath:    task.DicPath,
		Trigger:    task.Trigger,
		Interval:   task.Interval,
		Once:       task.Once,
		RunAtStart: task.RunAtStart,
	})
	if err != nil {
		debugLog.Infof("定时任务 %s 序列化失败: %v", task.ID, err)
		return
	}
	_, err = db.Exec(fmt.Sprintf(`
		INSERT INTO "%s" (key, data, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET
			data = excluded.data,
			updated_at = excluded.updated_at
	`, scheduledTaskTable), task.ID, string(b), time.Now().Unix())
	if err != nil {
		debugLog.Infof("定时任务 %s 持久化写入失败: %v", task.ID, err)
	}
}

// removeScheduledTaskFromDB 从全局数据库删除任务，失败仅记日志
func removeScheduledTaskFromDB(id string) {
	db, err := GetGlobalDB()
	if err != nil {
		return
	}
	_ = EnsureFsTable(db, scheduledTaskTable)
	if _, err := db.Exec(fmt.Sprintf(`DELETE FROM "%s" WHERE key=?`, scheduledTaskTable), id); err != nil {
		debugLog.Infof("定时任务 %s 删除持久化失败: %v", id, err)
	}
}

// loadScheduledTasksOnce 保证启动加载仅执行一次
var loadScheduledTasksOnce sync.Once

// LoadScheduledTasks 从全局数据库恢复定时任务并重启调度，进程启动时调用一次
func LoadScheduledTasks() {
	loadScheduledTasksOnce.Do(func() {
		db, err := GetGlobalDB()
		if err != nil {
			debugLog.Infof("定时任务恢复失败（数据库不可用）: %v", err)
			return
		}
		if err := EnsureFsTable(db, scheduledTaskTable); err != nil {
			debugLog.Infof("定时任务恢复失败（建表）: %v", err)
			return
		}
		rows, err := db.Query(fmt.Sprintf(`SELECT key, data FROM "%s"`, scheduledTaskTable))
		if err != nil {
			debugLog.Infof("定时任务恢复失败（查询）: %v", err)
			return
		}
		defer rows.Close()
		for rows.Next() {
			var id, data string
			if err := rows.Scan(&id, &data); err != nil {
				continue
			}
			var info ScheduledTaskInfo
			if err := json.Unmarshal([]byte(data), &info); err != nil {
				debugLog.Infof("定时任务 %s 反序列化失败: %v", id, err)
				continue
			}
			if id == "" {
				continue
			}
			// 以数据库主键为准，避免脏数据导致 ID 不一致
			info.ID = id
			task := &ScheduledTask{
				ID:         info.ID,
				DicPath:    info.DicPath,
				Trigger:    info.Trigger,
				Interval:   info.Interval,
				Once:       info.Once,
				RunAtStart: info.RunAtStart,
				cancel:     make(chan struct{}),
			}
			scheduledTasks.Store(task.ID, task)
			go task.run()
		}
	})
}

// run 定时调度循环：启动时可选立即触发一次，之后每次等待间隔后执行指定词库的触发词，直到被取消或（一次性任务）执行完毕
func (t *ScheduledTask) run() {
	if t.RunAtStart {
		t.execute()
		if t.Once {
			// 一次性任务执行完后自动移除自身（内存 + 数据库）
			scheduledTasks.Delete(t.ID)
			removeScheduledTaskFromDB(t.ID)
			return
		}
	}
	for {
		d, err := parseInterval(t.Interval)
		if err != nil {
			debugLog.Infof("定时任务 %s 停止：%v", t.ID, err)
			// 间隔非法（如历史脏数据）：同步清理内存与数据库，避免僵尸任务反复加载
			scheduledTasks.Delete(t.ID)
			removeScheduledTaskFromDB(t.ID)
			return
		}
		timer := time.NewTimer(d)
		select {
		case <-timer.C:
		case <-t.cancel:
			timer.Stop()
			return
		}
		t.execute()
		if t.Once {
			// 一次性任务执行完后自动移除自身（内存 + 数据库）
			scheduledTasks.Delete(t.ID)
			removeScheduledTaskFromDB(t.ID)
			return
		}
	}
}

// execute 加载并执行一次词库
func (t *ScheduledTask) execute() {
	// 词库执行由独立 goroutine 承载，panic 未恢复会拖垮整个进程，这里兜底拦截
	defer func() {
		if r := recover(); r != nil {
			debugLog.Infof("定时任务 %s 执行 panic: %v", t.ID, r)
		}
	}()
	data, err := utils.NewFileQueue(t.DicPath).ReadFromFile()
	if err != nil {
		debugLog.Infof("定时任务 %s 读取词库失败: %v", t.ID, err)
		return
	}
	dic := dic_dto.NewDic(t.DicPath, data)
	defer dic.Close()
	if out := dic_api.Api.DicRun(dic, t.Trigger); out != "" {
		debugLog.Infof("定时任务 %s 输出: %v", t.ID, out)
	}
}

// parseInterval 解析执行间隔为距下一次执行的时长
func parseInterval(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("定时任务：执行间隔不能为空")
	}
	if ms, err := strconv.Atoi(s); err == nil {
		if ms <= 0 {
			return 0, errors.New("定时任务：执行间隔必须大于 0")
		}
		return time.Duration(ms) * time.Millisecond, nil
	}
	switch s {
	case "s", "一秒":
		return time.Second, nil
	case "m", "一分":
		return time.Minute, nil
	case "h", "一小时":
		return time.Hour, nil
	case "d", "一天":
		return 24 * time.Hour, nil
	case "整点":
		return time.Until(time.Now().Truncate(time.Hour).Add(time.Hour)), nil
	case "整分":
		return time.Until(time.Now().Truncate(time.Minute).Add(time.Minute)), nil
	default:
		return 0, errors.New("定时任务：无法识别的执行间隔 " + s)
	}
}

// $添加定时任务(时间, 触发词, 词库路径, 一次性, 启动触发一次)$
func addScheduledTaskFunc(d *dto.DicInputs) (any, error) {
	interval := d.Inputs.String(1)
	trigger := d.Inputs.StringDefault(2, "Main")
	dicPath := d.Inputs.String(3)
	if dicPath == "" {
		// 词库路径留空时默认执行当前词库
		if p := d.V.G.GetStr("_词库路径_"); p != "" {
			dicPath = p
		}
	}
	once := d.Inputs.Bool(4)
	runAtStart := d.Inputs.Bool(5)
	return AddScheduledTask(dicPath, trigger, interval, once, runAtStart)
}

// $删除定时任务(编号)$
func delScheduledTaskFunc(d *dto.DicInputs) (any, error) {
	if err := DelScheduledTask(d.Inputs.String(1)); err != nil {
		return "", err
	}
	return "", nil
}

// $定时任务列表$
func listScheduledTaskFunc(d *dto.DicInputs) (any, error) {
	b, err := json.Marshal(ListScheduledTasks())
	if err != nil {
		return "", err
	}
	return string(b), nil
}
