package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/cjxpj/nebula/dto"
)

// runCmd 静默执行外部命令，失败时返回完整日志
func runCmd(name string, args ...string) error {
	var out bytes.Buffer
	cmd := exec.Command(name, args...)
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s 运行失败: %s\n输出:\n%s", name, err.Error(), out.String())
	}
	return nil
}

// mp3ToSilk 将 MP3 文件转换为 SILK 文件
func mp3ToSilk(mp3Path, silkPath string) error {
	// 生成临时 WAV 文件
	tmpFile, err := os.CreateTemp("", "temp_audio_*.wav")
	if err != nil {
		return fmt.Errorf("生成临时文件失败: %w", err)
	}
	tmpWav := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpWav) // 函数结束自动删除

	ffmpegExe := dto.GV.GetStr("_Ffmpeg_")
	if ffmpegExe == "" {
		return fmt.Errorf("未设置ffmpeg环境")
	}

	silkExe := dto.GV.GetStr("_SilkPath_")
	if silkExe == "" {
		return fmt.Errorf("未设置silk环境")
	}
	silkExe = filepath.Join(silkExe, "silk_v3_encoder.exe")

	// MP3 -> WAV
	if err := runCmd(ffmpegExe, "-y", "-i", mp3Path, "-ar", "16000", "-ac", "1", "-f", "wav", tmpWav); err != nil {
		return err
	}

	// WAV -> SILK
	if err := runCmd(silkExe, tmpWav, silkPath, "-Fs_API", "16000", "-tencent"); err != nil {
		return err
	}

	return nil
}
