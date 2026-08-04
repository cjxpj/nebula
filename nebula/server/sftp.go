package dic_server

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cjxpj/nebula/debugLog"
	"github.com/cjxpj/nebula/utils"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// runSftpServer 启动 SFTP 服务端
func runSftpServer(ctx context.Context, port int, debug bool) error {
	addr := fmt.Sprintf(":%d", port)

	// 生成 SSH 服务端配置
	config := &ssh.ServerConfig{
		PasswordCallback: func(conn ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
			if conn.User() == sftpUser && string(password) == sftpPass {
				return nil, nil
			}
			return nil, fmt.Errorf("密码错误")
		},
	}

	// 生成主机密钥
	privateKey, err := generateHostKey()
	if err != nil {
		return fmt.Errorf("SFTP 生成主机密钥失败: %v", err)
	}
	config.AddHostKey(privateKey)

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("SFTP 监听失败: %v", err)
	}
	sftpListener = listener

	defer func() {
		sftpListener = nil
	}()

	// 监听取消信号
	go func() {
		<-ctx.Done()
		listener.Close()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
			}
			if debug {
				debugLog.Errorf("[SFTP] 接受连接失败: %v", err)
			}
			continue
		}
		if debug {
			debugLog.Infof("[SFTP] 新连接: %s", conn.RemoteAddr().String())
		}
		go handleSftpConn(conn, config, debug)
	}
}

const sftpKeyDir = "private/sftp"

// generateHostKey 生成或加载 SSH 主机密钥
func generateHostKey() (ssh.Signer, error) {
	// 密钥目录相对于应用数据目录
	keyDir := filepath.Join(utils.GetAppDir(), sftpKeyDir)
	keyPath := filepath.Join(keyDir, "sftp_host_key")

	// 尝试加载已有的密钥
	if keyData, err := os.ReadFile(keyPath); err == nil {
		return ssh.ParsePrivateKey(keyData)
	}

	// 确保密钥目录存在
	if err := os.MkdirAll(keyDir, 0700); err != nil {
		return nil, fmt.Errorf("创建 SFTP 密钥目录失败: %v", err)
	}

	// 生成新的 2048 位 RSA 密钥
	privateKey, err := utils.GenerateSSHHostKey()
	if err != nil {
		return nil, err
	}

	// 转换为 SSH Signer
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		return nil, err
	}

	// 保存私钥到文件（先写私钥再写公钥，避免只有公钥无对应私钥）
	pemBlock := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}
	pemBytes := pem.EncodeToMemory(pemBlock)
	if err := os.WriteFile(keyPath, pemBytes, 0600); err != nil {
		return nil, fmt.Errorf("保存 SFTP 私钥失败: %v", err)
	}

	// 保存公钥到文件
	pubKeyBytes := ssh.MarshalAuthorizedKey(signer.PublicKey())
	os.WriteFile(keyPath+".pub", pubKeyBytes, 0600)

	return signer, nil
}

// sftpHandshakeTimeout SSH 握手超时时间
const sftpHandshakeTimeout = 30 * time.Second

// handleSftpConn 处理单个 SFTP 客户端连接
func handleSftpConn(conn net.Conn, config *ssh.ServerConfig, debug bool) {
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(sftpHandshakeTimeout))
	sshConn, chans, reqs, err := ssh.NewServerConn(conn, config)
	if err != nil {
		if debug {
			debugLog.Infof("[SFTP] %s SSH 握手失败: %v", conn.RemoteAddr(), err)
		}
		return
	}
	conn.SetDeadline(time.Time{})
	defer sshConn.Close()

	if debug {
		debugLog.Infof("[SFTP] %s 认证成功 (用户: %s)", conn.RemoteAddr(), sshConn.User())
	}

	// 处理全局请求
	go ssh.DiscardRequests(reqs)

	// 处理通道请求
	for newChannel := range chans {
		if newChannel.ChannelType() != "session" {
			newChannel.Reject(ssh.UnknownChannelType, "仅支持 session 通道")
			continue
		}

		channel, requests, err := newChannel.Accept()
		if err != nil {
			if debug {
				debugLog.Infof("[SFTP] 接受通道失败: %v", err)
			}
			continue
		}

		go func() {
			for req := range requests {
				switch req.Type {
				case "subsystem":
					if string(req.Payload[4:]) == "sftp" {
						req.Reply(true, nil)
						go serveSftp(channel, debug)
						return
					}
					req.Reply(false, nil)
				case "exec", "shell":
					req.Reply(false, nil)
				default:
					req.Reply(false, nil)
				}
			}
		}()
	}
}

// serveSftp 启动 SFTP 子系统
func serveSftp(channel ssh.Channel, debug bool) {
	defer channel.Close()

	rootDir := utils.FtpDir()
	// 如果根目录是相对路径，相对于可执行文件目录解析
	if !filepath.IsAbs(rootDir) {
		exe, err := os.Executable()
		if err == nil {
			rootDir = filepath.Join(filepath.Dir(exe), rootDir)
		}
	}
	absDir, err := filepath.Abs(rootDir)
	if err != nil {
		if debug {
			debugLog.Errorf("[SFTP] 解析根目录路径失败: %v", err)
		}
		return
	}
	absDir = filepath.Clean(absDir)
	// 确保根目录存在
	if err := os.MkdirAll(absDir, 0755); err != nil {
		if debug {
			debugLog.Errorf("[SFTP] 创建根目录失败: %v", err)
		}
		return
	}

	h := &sftpJail{root: absDir}
	server := sftp.NewRequestServer(channel, sftp.Handlers{
		FileGet:  h,
		FilePut:  h,
		FileCmd:  h,
		FileList: h,
	})

	if debug {
		debugLog.Infof("[SFTP] SFTP 子系统已启动，根目录: %s", absDir)
	}

	if err := server.Serve(); err != nil && err != io.EOF {
		if debug {
			debugLog.Infof("[SFTP] SFTP 会话结束: %v", err)
		}
	}
}

// sftpJail 实现 SFTP Handlers，限制所有文件操作在 root 目录内
type sftpJail struct {
	root string
}

// resolvePath 解析路径并校验不超出根目录
func (j *sftpJail) resolvePath(p string) (string, error) {
	// 去掉前导 / 和 \，防止 filepath.Join 重置到盘符根目录
	p = strings.TrimLeft(p, "/\\")
	// 清理路径中的 .. 等
	p = filepath.Clean(p)
	// 禁止绝对路径（盘符开头如 C:）
	if filepath.IsAbs(p) {
		return "", fmt.Errorf("access denied: %s", p)
	}
	abs := filepath.Join(j.root, p)
	abs, err := filepath.Abs(abs)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	root := filepath.Clean(j.root)
	if !strings.HasPrefix(abs, root) && abs != root {
		return "", fmt.Errorf("access denied: %s", p)
	}
	return abs, nil
}

func (j *sftpJail) Fileread(r *sftp.Request) (io.ReaderAt, error) {
	p, err := j.resolvePath(r.Filepath)
	if err != nil {
		return nil, err
	}
	return os.Open(p)
}

func (j *sftpJail) Filewrite(r *sftp.Request) (io.WriterAt, error) {
	p, err := j.resolvePath(r.Filepath)
	if err != nil {
		return nil, err
	}
	flags := os.O_WRONLY | os.O_CREATE
	if r.Pflags().Append {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
	}
	return os.OpenFile(p, flags, 0644)
}

func (j *sftpJail) Filecmd(r *sftp.Request) error {
	switch r.Method {
	case "Setstat":
		p, err := j.resolvePath(r.Filepath)
		if err != nil {
			return err
		}
		if r.AttrFlags().Size {
			return os.Truncate(p, int64(r.Attributes().Size))
		}
		if r.AttrFlags().Permissions {
			return os.Chmod(p, os.FileMode(r.Attributes().Mode))
		}
		return nil
	case "Rename":
		old, err := j.resolvePath(r.Filepath)
		if err != nil {
			return err
		}
		new, err := j.resolvePath(r.Target)
		if err != nil {
			return err
		}
		return os.Rename(old, new)
	case "Rmdir":
		p, err := j.resolvePath(r.Filepath)
		if err != nil {
			return err
		}
		return os.Remove(p)
	case "Mkdir":
		p, err := j.resolvePath(r.Filepath)
		if err != nil {
			return err
		}
		return os.Mkdir(p, 0755)
	case "Remove":
		p, err := j.resolvePath(r.Filepath)
		if err != nil {
			return err
		}
		return os.Remove(p)
	case "Symlink":
		target, err := j.resolvePath(r.Filepath)
		if err != nil {
			return err
		}
		link, err := j.resolvePath(r.Target)
		if err != nil {
			return err
		}
		return os.Symlink(target, link)
	}
	return fmt.Errorf("unsupported: %s", r.Method)
}

func (j *sftpJail) Filelist(r *sftp.Request) (sftp.ListerAt, error) {
	p, err := j.resolvePath(r.Filepath)
	if err != nil {
		return nil, err
	}
	switch r.Method {
	case "List":
		entries, err := os.ReadDir(p)
		if err != nil {
			return nil, err
		}
		infos := make([]os.FileInfo, len(entries))
		for i, e := range entries {
			infos[i], err = e.Info()
			if err != nil {
				return nil, err
			}
		}
		return listerat(infos), nil
	case "Stat":
		fi, err := os.Stat(p)
		if err != nil {
			return nil, err
		}
		return listerat{fi}, nil
	}
	return nil, fmt.Errorf("unsupported: %s", r.Method)
}

func (j *sftpJail) Lstat(r *sftp.Request) (sftp.ListerAt, error) {
	p, err := j.resolvePath(r.Filepath)
	if err != nil {
		return nil, err
	}
	fi, err := os.Lstat(p)
	if err != nil {
		return nil, err
	}
	return listerat{fi}, nil
}

// listerat 实现 sftp.ListerAt 接口
type listerat []os.FileInfo

func (f listerat) ListAt(ls []os.FileInfo, offset int64) (int, error) {
	if offset >= int64(len(f)) {
		return 0, io.EOF
	}
	n := copy(ls, f[offset:])
	if n < len(ls) {
		return n, io.EOF
	}
	return n, nil
}
