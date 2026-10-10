package dic_server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/cjxpj/nebula/utils"
)

type HttpOpUiConfig_install struct {
	Component string            `json:"component"`
	Params    map[string]string `json:"params,omitempty"`
}

type HttpOpUiInstallResponse struct {
	Status string   `json:"status"`
	Output []string `json:"output,omitempty"`
	Error  string   `json:"error,omitempty"`
}

type HttpOpUiConfig_installStatus struct {
	Component string `json:"component"`
	TaskID    string `json:"task_id,omitempty"`
}

type HttpOpUiInstallStatusResponse struct {
	Installed bool     `json:"installed"`
	Output    []string `json:"output,omitempty"`
	Status    string   `json:"status,omitempty"`
	Error     string   `json:"error,omitempty"`
}

// ==================== 异步安装任务管理 ====================

type InstallTask struct {
	ID        string   `json:"id"`
	Component string   `json:"component"`
	Status    string   `json:"status"` // "running", "completed", "failed", "cancelled"
	Output    []string `json:"output"`
	Error     string   `json:"error,omitempty"`
	Progress  float64  `json:"progress"` // 下载进度 0-100

	cancelled bool
	mu        sync.RWMutex
}

func (t *InstallTask) Cancel() {
	t.mu.Lock()
	t.cancelled = true
	t.mu.Unlock()
}

func (t *InstallTask) IsCancelled() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.cancelled
}

func (t *InstallTask) addOutput(msg string) {
	t.mu.Lock()
	t.Output = append(t.Output, msg)
	t.mu.Unlock()
}

func (t *InstallTask) setProgress(p float64) {
	t.mu.Lock()
	if p > 100 {
		p = 100
	}
	t.Progress = p
	t.mu.Unlock()
}

func (t *InstallTask) snapshot() (status string, output []string, errMsg string, progress float64) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	status = t.Status
	output = make([]string, len(t.Output))
	copy(output, t.Output)
	errMsg = t.Error
	progress = t.Progress
	return
}

func (t *InstallTask) finish(err error) {
	t.mu.Lock()
	if t.cancelled {
		t.Status = "cancelled"
		t.Error = "用户取消"
	} else if err != nil {
		t.Status = "failed"
		t.Error = err.Error()
	} else {
		t.Status = "completed"
		t.Progress = 100
	}
	t.mu.Unlock()
	// 3 分钟后自动清理，给前端足够时间轮询到最终状态
	time.AfterFunc(3*time.Minute, func() {
		installTaskStore.Delete(t.ID)
	})
}

var installTaskStore sync.Map // map[string]*InstallTask

func generateTaskID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// findRunningTaskForComponent 检查指定组件是否已有正在运行的任务
func findRunningTaskForComponent(component string) *InstallTask {
	var found *InstallTask
	installTaskStore.Range(func(key, value any) bool {
		task, ok := value.(*InstallTask)
		if !ok {
			return true
		}
		task.mu.RLock()
		status := task.Status
		comp := task.Component
		task.mu.RUnlock()
		if comp == component && (status == "running") {
			found = task
			return false
		}
		return true
	})
	return found
}

// opuiAuthTable 管理面板登录密码在全局数据库中的表名（仅存 bcrypt 哈希，单向加密不存明文）

// 组件安装 / 卸载 API。
func init() {
	registerOpuiApi(opuiHandleInstallAPI,
		"install_php", "install_ffmpeg", "install_silk_v3", "install_napcat_bot",
		"install_python", "install_go", "get_install_status", "install_progress",
		"install_cancel", "uninstall", "uninstall_nebula",
	)
}

// opuiHandleInstallAPI 处理本模块注册的 OPUI API 类型。
func opuiHandleInstallAPI(w http.ResponseWriter, r *http.Request, h *HttpOpUiData) {
	// 扩展部署：php/ffmpeg/silk_v3/napcat/go 仅支持 Windows，python 全平台支持
	switch h.Type {
	case "install_php", "install_ffmpeg", "install_silk_v3", "install_napcat_bot", "install_go":
		if runtime.GOOS != "windows" {
			jsonResp, _ := json.Marshal(map[string]string{"status": "error", "error": "该组件仅支持 Windows 端"})
			w.Write(jsonResp)
			return
		}
	}

	switch h.Type {
	case "install_php":
		appDir := utils.GetAppDir()
		destDir := filepath.Join(appDir, "private", "extensions", "php")
		if fileExists(filepath.Join(destDir, "php.exe")) {
			resp := HttpOpUiInstallResponse{
				Status: "ok",
				Output: []string{"PHP 已安装"},
			}
			jsonResp, _ := json.Marshal(resp)
			w.Write(jsonResp)
			return
		}
		// 防止重复安装：检查是否已有同组件运行中的任务
		if existingTask := findRunningTaskForComponent("php"); existingTask != nil {
			jsonResp, _ := json.Marshal(map[string]string{"status": "ok", "task_id": existingTask.ID})
			w.Write(jsonResp)
			return
		}
		taskID := generateTaskID()
		task := &InstallTask{
			ID:        taskID,
			Component: "php",
			Status:    "running",
			Progress:  0,
		}
		installTaskStore.Store(taskID, task)
		go func() {
			var output []string
			progressFn := func(p float64) { task.setProgress(p) }
			err := installPHP(destDir, &output, progressFn)
			for _, line := range output {
				task.addOutput(line)
			}
			if task.IsCancelled() {
				task.addOutput("⚠ 安装已取消")
				task.finish(nil)
				return
			}
			task.finish(err)
		}()
		jsonResp, _ := json.Marshal(map[string]string{"status": "ok", "task_id": taskID})
		w.Write(jsonResp)
		return

	case "install_ffmpeg":
		appDir := utils.GetAppDir()
		destDir := filepath.Join(appDir, "private", "extensions", "ffmpeg")
		if utils.FindFfmpegExe(destDir) != "" {
			resp := HttpOpUiInstallResponse{
				Status: "ok",
				Output: []string{"FFmpeg 已安装"},
			}
			jsonResp, _ := json.Marshal(resp)
			w.Write(jsonResp)
			return
		}
		// 防止重复安装
		if existingTask := findRunningTaskForComponent("ffmpeg"); existingTask != nil {
			jsonResp, _ := json.Marshal(map[string]string{"status": "ok", "task_id": existingTask.ID})
			w.Write(jsonResp)
			return
		}
		taskID := generateTaskID()
		task := &InstallTask{
			ID:        taskID,
			Component: "ffmpeg",
			Status:    "running",
			Progress:  0,
		}
		installTaskStore.Store(taskID, task)
		go func() {
			var output []string
			progressFn := func(p float64) { task.setProgress(p) }
			err := installFFmpeg(destDir, &output, progressFn)
			for _, line := range output {
				task.addOutput(line)
			}
			if task.IsCancelled() {
				task.addOutput("⚠ 安装已取消")
				task.finish(nil)
				return
			}
			task.finish(err)
		}()
		jsonResp, _ := json.Marshal(map[string]string{"status": "ok", "task_id": taskID})
		w.Write(jsonResp)
		return

	case "install_silk_v3":
		appDir := utils.GetAppDir()
		destDir := filepath.Join(appDir, "private", "extensions")
		if fileExists(filepath.Join(destDir, "silk_v3", "silk_v3_encoder.exe")) {
			resp := HttpOpUiInstallResponse{
				Status: "ok",
				Output: []string{"silk_v3 已安装"},
			}
			jsonResp, _ := json.Marshal(resp)
			w.Write(jsonResp)
			return
		}
		// 防止重复安装
		if existingTask := findRunningTaskForComponent("silk_v3"); existingTask != nil {
			jsonResp, _ := json.Marshal(map[string]string{"status": "ok", "task_id": existingTask.ID})
			w.Write(jsonResp)
			return
		}
		taskID := generateTaskID()
		task := &InstallTask{
			ID:        taskID,
			Component: "silk_v3",
			Status:    "running",
			Progress:  0,
		}
		installTaskStore.Store(taskID, task)
		go func() {
			var output []string
			progressFn := func(p float64) { task.setProgress(p) }
			err := installSilkV3(destDir, &output, progressFn)
			for _, line := range output {
				task.addOutput(line)
			}
			if task.IsCancelled() {
				task.addOutput("⚠ 安装已取消")
				task.finish(nil)
				return
			}
			task.finish(err)
		}()
		jsonResp, _ := json.Marshal(map[string]string{"status": "ok", "task_id": taskID})
		w.Write(jsonResp)
		return

	case "install_napcat_bot":
		var config HttpOpUiConfig_install
		if err := json.Unmarshal(h.Data, &config); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		qq, ok := config.Params["qq"]
		if !ok || qq == "" {
			w.Write([]byte(`{"status":"error","error":"missing qq parameter"}`))
			return
		}
		appDir := utils.GetAppDir()
		destDir := filepath.Join(appDir, "private", "extensions", "NapCat.Shell")
		if fileExists(filepath.Join(destDir, "launcher.bat")) {
			resp := HttpOpUiInstallResponse{
				Status: "ok",
				Output: []string{"napcat_bot 已安装"},
			}
			jsonResp, _ := json.Marshal(resp)
			w.Write(jsonResp)
			return
		}
		// 防止重复安装
		if existingTask := findRunningTaskForComponent("napcat_bot"); existingTask != nil {
			jsonResp, _ := json.Marshal(map[string]string{"status": "ok", "task_id": existingTask.ID})
			w.Write(jsonResp)
			return
		}
		taskID := generateTaskID()
		task := &InstallTask{
			ID:        taskID,
			Component: "napcat_bot",
			Status:    "running",
			Progress:  0,
		}
		installTaskStore.Store(taskID, task)
		go func() {
			var output []string
			progressFn := func(p float64) { task.setProgress(p) }
			err := installNapCatBot(destDir, qq, &output, progressFn)
			for _, line := range output {
				task.addOutput(line)
			}
			if task.IsCancelled() {
				task.addOutput("⚠ 安装已取消")
				task.finish(nil)
				return
			}
			task.finish(err)
		}()
		jsonResp, _ := json.Marshal(map[string]string{"status": "ok", "task_id": taskID})
		w.Write(jsonResp)
		return

	case "install_python":
		appDir := utils.GetAppDir()
		destDir := filepath.Join(appDir, "private", "extensions", "python")
		if isPythonInstalled(destDir) {
			resp := HttpOpUiInstallResponse{
				Status: "ok",
				Output: []string{"Python 已安装"},
			}
			jsonResp, _ := json.Marshal(resp)
			w.Write(jsonResp)
			return
		}
		// Linux 等非 Windows 平台使用系统 python3，无法通过此处下载安装
		if runtime.GOOS != "windows" {
			resp := HttpOpUiInstallResponse{
				Status: "error",
				Error:  "未检测到 Python 运行环境，请通过系统包管理器安装 python3",
			}
			jsonResp, _ := json.Marshal(resp)
			w.Write(jsonResp)
			return
		}
		// 防止重复安装
		if existingTask := findRunningTaskForComponent("python"); existingTask != nil {
			jsonResp, _ := json.Marshal(map[string]string{"status": "ok", "task_id": existingTask.ID})
			w.Write(jsonResp)
			return
		}
		taskID := generateTaskID()
		task := &InstallTask{
			ID:        taskID,
			Component: "python",
			Status:    "running",
			Progress:  0,
		}
		installTaskStore.Store(taskID, task)
		go func() {
			var output []string
			progressFn := func(p float64) { task.setProgress(p) }
			err := installPython(destDir, &output, progressFn)
			for _, line := range output {
				task.addOutput(line)
			}
			if task.IsCancelled() {
				task.addOutput("⚠ 安装已取消")
				task.finish(nil)
				return
			}
			task.finish(err)
		}()
		jsonResp, _ := json.Marshal(map[string]string{"status": "ok", "task_id": taskID})
		w.Write(jsonResp)
		return

	case "install_go":
		appDir := utils.GetAppDir()
		extDir := filepath.Join(appDir, "private", "extensions")
		if fileExists(filepath.Join(extDir, "go", "bin", "go.exe")) {
			resp := HttpOpUiInstallResponse{
				Status: "ok",
				Output: []string{"Go 工具链已安装"},
			}
			jsonResp, _ := json.Marshal(resp)
			w.Write(jsonResp)
			return
		}
		// 防止重复安装
		if existingTask := findRunningTaskForComponent("go"); existingTask != nil {
			jsonResp, _ := json.Marshal(map[string]string{"status": "ok", "task_id": existingTask.ID})
			w.Write(jsonResp)
			return
		}
		taskID := generateTaskID()
		task := &InstallTask{
			ID:        taskID,
			Component: "go",
			Status:    "running",
			Progress:  0,
		}
		installTaskStore.Store(taskID, task)
		go func() {
			var output []string
			progressFn := func(p float64) { task.setProgress(p) }
			err := installGo(extDir, &output, progressFn)
			for _, line := range output {
				task.addOutput(line)
			}
			if task.IsCancelled() {
				task.addOutput("⚠ 安装已取消")
				task.finish(nil)
				return
			}
			task.finish(err)
		}()
		jsonResp, _ := json.Marshal(map[string]string{"status": "ok", "task_id": taskID})
		w.Write(jsonResp)
		return

	case "get_install_status":
		appDir := utils.GetAppDir()
		extDir := filepath.Join(appDir, "private", "extensions")
		allStatus := map[string]bool{
			"php":        fileExists(filepath.Join(extDir, "php", "php.exe")),
			"python":     isPythonInstalled(filepath.Join(extDir, "python")),
			"napcat_bot": fileExists(filepath.Join(extDir, "NapCat.Shell", "launcher.bat")),
			"ffmpeg":     utils.FindFfmpegExe(filepath.Join(extDir, "ffmpeg")) != "",
			"silk_v3":    fileExists(filepath.Join(extDir, "silk_v3", "silk_v3_encoder.exe")),
			"go":         fileExists(filepath.Join(extDir, "go", "bin", "go.exe")),
		}
		jsonResp, _ := json.Marshal(allStatus)
		w.Write(jsonResp)
		return

	case "install_progress":
		var j HttpOpUiConfig_installStatus
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if val, ok := installTaskStore.Load(j.TaskID); ok {
			task, ok := val.(*InstallTask)
			if !ok {
				http.Error(w, `{"status":"error","error":"invalid task"}`, http.StatusInternalServerError)
				return
			}
			status, output, errMsg, progress := task.snapshot()
			resp := map[string]any{
				"status":    status,
				"component": task.Component,
				"output":    output,
				"error":     errMsg,
				"progress":  progress,
			}
			jsonResp, _ := json.Marshal(resp)
			w.Write(jsonResp)
		} else {
			w.Write([]byte(`{"status":"not_found","error":"task not found"}`))
		}
		return

	case "install_cancel":
		var j HttpOpUiConfig_installStatus
		if err := json.Unmarshal(h.Data, &j); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if val, ok := installTaskStore.Load(j.TaskID); ok {
			task, ok := val.(*InstallTask)
			if !ok {
				http.Error(w, `{"status":"error","error":"invalid task"}`, http.StatusInternalServerError)
				return
			}
			task.Cancel()
			w.Write([]byte(`{"status":"ok"}`))
		} else {
			w.Write([]byte(`{"status":"not_found","error":"task not found"}`))
		}
		return

	case "uninstall":
		var config HttpOpUiConfig_install
		if err := json.Unmarshal(h.Data, &config); err != nil {
			http.Error(w, `{"status":"error","error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		appDir := utils.GetAppDir()
		// 非 Windows 仅支持卸载 Python
		if runtime.GOOS != "windows" && config.Component != "python" {
			resp := HttpOpUiInstallResponse{Status: "error", Error: "该组件仅支持 Windows 端"}
			jsonResp, _ := json.Marshal(resp)
			w.Write(jsonResp)
			return
		}
		var rmDir string
		switch config.Component {
		case "php":
			rmDir = filepath.Join(appDir, "private", "extensions", "php")
		case "ffmpeg":
			rmDir = filepath.Join(appDir, "private", "extensions", "ffmpeg")
		case "silk_v3":
			rmDir = filepath.Join(appDir, "private", "extensions", "silk_v3")
		case "napcat_bot":
			rmDir = filepath.Join(appDir, "private", "extensions", "NapCat.Shell")
		case "go":
			rmDir = filepath.Join(appDir, "private", "extensions", "go")
		case "python":
			// Linux 使用系统 python3，仅当存在内置扩展时才可删除
			if runtime.GOOS != "windows" && !fileExists(filepath.Join(appDir, "private", "extensions", "python", "python3")) {
				resp := HttpOpUiInstallResponse{Status: "error", Error: "当前使用系统 python3，请通过系统包管理器卸载"}
				jsonResp, _ := json.Marshal(resp)
				w.Write(jsonResp)
				return
			}
			rmDir = filepath.Join(appDir, "private", "extensions", "python")
		default:
			http.Error(w, `{"status":"error","error":"unknown component"}`, http.StatusBadRequest)
			return
		}
		if err := os.RemoveAll(rmDir); err != nil {
			resp := HttpOpUiInstallResponse{Status: "error", Error: "卸载失败: " + err.Error()}
			jsonResp, _ := json.Marshal(resp)
			w.Write(jsonResp)
			return
		}
		resp := HttpOpUiInstallResponse{Status: "ok", Output: []string{config.Component + " 已卸载"}}
		jsonResp, _ := json.Marshal(resp)
		w.Write(jsonResp)
		return

	case "uninstall_nebula":
		// 卸载主程序：删除程序本体 exe，remove_data 为真时连同数据目录一并删除，随后退出进程。
		// 需校验管理面板登录密码；先返回响应、延迟执行删除，保证前端能收到结果（进程退出后 WS 即断开）。
		var j struct {
			RemoveData bool   `json:"remove_data"`
			Password   string `json:"password"`
		}
		_ = json.Unmarshal(h.Data, &j)
		// 未设置登录密码时放行（面板本身无鉴权），已设置则必须匹配登录密码
		if ok, err := opuiVerifyPassword(j.Password); err != nil {
			jsonResp, _ := json.Marshal(map[string]string{"status": "error", "error": "密码校验失败: " + err.Error()})
			w.Write(jsonResp)
			return
		} else if !ok {
			jsonResp, _ := json.Marshal(map[string]string{"status": "error", "error": "登录密码错误"})
			w.Write(jsonResp)
			return
		}
		if err := startUninstall(j.RemoveData); err != nil {
			jsonResp, _ := json.Marshal(map[string]string{"status": "error", "error": err.Error()})
			w.Write(jsonResp)
			return
		}
		w.Write([]byte(`{"status":"ok"}`))
		return

	}
}
