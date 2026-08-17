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
	ID       string `json:"id"`
	DicPath  string `json:"dic_path"`
	Trigger  string `json:"trigger"`
	Interval string `json:"interval"`
	Once     bool   `json:"once"`
}

// ScheduledTask 定时任务运行时状态
type ScheduledTask struct {
	ID       string
	DicPath  string
	Trigger  string
	Interval string
	Once     bool
	cancel   chan struct{}
}

// scheduledTasks 进程内定时任务存储（重启后清空）
var scheduledTasks sync.Map // map[string]*ScheduledTask

// AddScheduledTask 添加定时任务并启动调度，返回唯一编号；once 为 true 时仅执行一次
func AddScheduledTask(dicPath, trigger, interval string, once bool) (string, error) {
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
		ID:       id,
		DicPath:  dicPath,
		Trigger:  trigger,
		Interval: interval,
		Once:     once,
		cancel:   make(chan struct{}),
	}
	scheduledTasks.Store(id, task)
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
			ID:       task.ID,
			DicPath:  task.DicPath,
			Trigger:  task.Trigger,
			Interval: task.Interval,
			Once:     task.Once,
		})
		return true
	})
	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
	return list
}

// run 定时调度循环：每次等待间隔后执行指定词库的触发词，直到被取消或（一次性任务）执行完毕
func (t *ScheduledTask) run() {
	for {
		d, err := parseInterval(t.Interval)
		if err != nil {
			debugLog.Infof("定时任务 %s 停止：%v", t.ID, err)
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
			// 一次性任务执行完后自动移除自身
			scheduledTasks.Delete(t.ID)
			return
		}
	}
}

// execute 加载并执行一次词库
func (t *ScheduledTask) execute() {
	data, err := utils.NewFileQueue(t.DicPath).ReadFromFile()
	if err != nil {
		debugLog.Infof("定时任务 %s 读取词库失败: %v", t.ID, err)
		return
	}
	dic := dic_dto.NewDic(t.DicPath, data)
	defer dic.Close()
	dic.Val.P.Set("_词库路径_", t.DicPath)
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

// $添加定时任务(时间, 触发词, 词库路径, 一次性)$
func addScheduledTaskFunc(d *dto.DicInputs) (any, error) {
	interval := d.Inputs.String(1)
	trigger := d.Inputs.StringDefault(2, "Main")
	dicPath := d.Inputs.String(3)
	if dicPath == "" {
		// 词库路径留空时默认执行当前词库
		if p, ok := d.V.Get("_词库路径_").(string); ok {
			dicPath = p
		}
	}
	once := d.Inputs.Bool(4)
	return AddScheduledTask(dicPath, trigger, interval, once)
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
