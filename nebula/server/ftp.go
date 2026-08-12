package dic_server

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cjxpj/nebula/debugLog"
	"github.com/cjxpj/nebula/utils"
)

// ftpTlsConfig 全局 TLS 配置（由 StartFtp 设置）
var ftpTlsConfig *tls.Config

// runFtpServer 启动 FTP 服务端
func runFtpServer(ctx context.Context, port int, debug bool) error {
	addr := fmt.Sprintf(":%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("FTP 监听失败: %v", err)
	}
	ftpListener = listener

	defer func() {
		ftpListener = nil
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
				debugLog.Errorf("[FTP] 接受连接失败: %v", err)
			}
			continue
		}
		if debug {
			debugLog.Infof("[FTP] 新连接: %s", conn.RemoteAddr().String())
		}
		go handleFtpConn(conn, debug)
	}
}

// handleFtpConn 处理单个 FTP 客户端连接
func handleFtpConn(conn net.Conn, debug bool) {
	defer conn.Close()

	ftp := &ftpSession{
		conn:      conn,
		writer:    bufio.NewWriter(conn),
		reader:    bufio.NewReader(conn),
		debug:     debug,
		rootDir:   utils.FtpDir(),
		workDir:   "/",
		utf8:      true, // 默认 UTF-8
		dataType:  "A",  // 默认 ASCII（仅列表时用；文件传输用二进制）
		localAddr: conn.LocalAddr().String(),
	}
	defer ftp.closeDataConn()

	ftp.reply(220, "Nebula FTP Server ready")

	for {
		line, err := ftp.readLine()
		if err != nil {
			if err == io.EOF {
				if ftp.debug {
					debugLog.Infof(ftp.logPrefix()+" %s 连接正常关闭", conn.RemoteAddr())
				}
			} else {
				debugLog.Warnf(ftp.logPrefix()+" %s 连接断开: %v", conn.RemoteAddr(), err)
			}
			return
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			continue
		}

		if ftp.debug {
			debugLog.Infof(ftp.logPrefix()+" %s > %s", conn.RemoteAddr(), line)
		}

		parts := strings.SplitN(line, " ", 2)
		cmd := strings.ToUpper(parts[0])
		arg := ""
		if len(parts) > 1 {
			arg = parts[1]
		}

		switch cmd {
		case "USER":
			ftp.handleUSER(arg)
		case "PASS":
			ftp.handlePASS(arg)
		case "QUIT":
			ftp.reply(221, "Goodbye")
			return
		case "PWD":
			ftp.handlePWD()
		case "CWD":
			ftp.handleCWD(arg)
		case "CDUP":
			ftp.handleCWD("..")
		case "TYPE":
			ftp.handleTYPE(arg)
		case "PASV":
			ftp.handlePASV()
		case "PORT":
			ftp.handlePORT(arg)
		case "LIST", "NLST":
			ftp.handleLIST(arg)
		case "RETR":
			ftp.handleRETR(arg)
		case "STOR":
			ftp.handleSTOR(arg)
		case "DELE":
			ftp.handleDELE(arg)
		case "MKD":
			ftp.handleMKD(arg)
		case "RMD":
			ftp.handleRMD(arg)
		case "RNFR":
			ftp.handleRNFR(arg)
		case "RNTO":
			ftp.handleRNTO(arg)
		case "SIZE":
			ftp.handleSIZE(arg)
		case "SYST":
			ftp.reply(215, "UNIX Type: L8")
		case "FEAT":
			feat := "UTF8\r\n SIZE\r\n MDTM"
			if ftpTlsConfig != nil && !ftp.tlsEnabled {
				feat += "\r\n AUTH TLS"
			}
			if ftpTlsConfig != nil && ftp.tlsEnabled {
				feat += "\r\n PBSZ\r\n PROT"
			}
			feat += "\r\nEnd"
			ftp.reply(211, feat)
		case "AUTH":
			ftp.handleAUTH(arg)
		case "PBSZ":
			ftp.handlePBSZ(arg)
		case "PROT":
			ftp.handlePROT(arg)
		case "OPTS":
			ftp.handleOPTS(arg)
		case "NOOP":
			ftp.reply(200, "OK")
		case "HELP":
			ftp.reply(214, "Supported: USER PASS QUIT PWD CWD CDUP TYPE PASV PORT LIST RETR STOR DELE MKD RMD RNFR RNTO SIZE SYST FEAT NOOP")
		default:
			if ftp.debug {
				debugLog.Warnf(ftp.logPrefix()+" %s 未知命令: %s", conn.RemoteAddr(), cmd)
			}
			ftp.reply(502, "Command not implemented")
		}
	}
}

// ftpSession 表示一个 FTP 会话
type ftpSession struct {
	conn          net.Conn
	writer        *bufio.Writer
	reader        *bufio.Reader
	debug         bool
	rootDir       string // FTP 根目录（物理路径）
	workDir       string // 当前工作目录（虚拟路径，相对于 rootDir）
	utf8          bool
	dataType      string // I=Binary, A=ASCII
	localAddr     string // 服务端地址
	user          string // 用户名
	authenticated bool
	dataConn      net.Conn     // 数据连接 (PASV/PORT)
	dataListener  net.Listener // PASV 监听器
	rnfrPath      string       // RNFR 暂存路径
	tlsEnabled    bool         // 是否启用 TLS
	tlsProtData   bool         // PROT P: 数据连接也加密
}

// logPrefix 返回日志前缀，TLS 启用时显示 [FTPS]
func (ftp *ftpSession) logPrefix() string {
	if ftp.tlsEnabled {
		return "[FTPS]"
	}
	return "[FTP]"
}

// reply 发送 FTP 响应（自动加 \r\n）
func (ftp *ftpSession) reply(code int, msg string) {
	lines := strings.Split(msg, "\n")
	for i, line := range lines {
		prefix := fmt.Sprintf("%d ", code)
		if i < len(lines)-1 {
			prefix = fmt.Sprintf("%d-", code)
		}
		resp := prefix + line + "\r\n"
		ftp.writer.WriteString(resp)
		if ftp.debug {
			debugLog.Infof(ftp.logPrefix()+" %s < %s", ftp.conn.RemoteAddr(), strings.TrimRight(resp, "\r\n"))
		}
	}
	ftp.conn.SetWriteDeadline(time.Now().Add(ftpWriteTimeout))
	ftp.writer.Flush()
}

// ftpIdleTimeout FTP 控制连接空闲超时时间
const ftpIdleTimeout = 5 * time.Minute

// ftpDataConnTimeout PASV 数据连接 Accept 超时时间
const ftpDataConnTimeout = 30 * time.Second

// ftpWriteTimeout 控制连接写入超时时间
const ftpWriteTimeout = 10 * time.Second

// readLine 使用 bufio.Reader 读取一行，每次读取前刷新读超时
func (ftp *ftpSession) readLine() (string, error) {
	ftp.conn.SetReadDeadline(time.Now().Add(ftpIdleTimeout))
	line, err := ftp.reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// resolvePath 将虚拟路径转为物理路径，并检查路径穿越
func (ftp *ftpSession) resolvePath(vpath string) (string, error) {
	// 相对路径拼接当前工作目录
	if !strings.HasPrefix(vpath, "/") {
		vpath = ftp.workDir + "/" + vpath
	}
	if !strings.HasPrefix(vpath, "/") {
		vpath = "/" + vpath
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(vpath)))
	// 根目录则直接返回
	rootClean := filepath.Clean(ftp.rootDir)
	if clean == "/" || clean == "." {
		return rootClean, nil
	}
	// 检查是否在根目录内
	abs := filepath.Join(rootClean, filepath.FromSlash(strings.TrimPrefix(clean, "/")))
	if !strings.HasPrefix(filepath.Clean(abs), rootClean) {
		return "", fmt.Errorf("路径越权: %s", clean)
	}
	return abs, nil
}

// ======================== 认证 ========================

// mustAuth 检查是否已认证且（若启用 TLS）已 TLS 加密，未通过返回 false 并发送错误
func (ftp *ftpSession) mustAuth() bool {
	if !ftp.authenticated {
		ftp.reply(530, "Not logged in")
		return false
	}
	if ftpTlsConfig != nil && !ftp.tlsEnabled {
		ftp.reply(530, "TLS required, send AUTH TLS first")
		return false
	}
	return true
}

func (ftp *ftpSession) handleUSER(arg string) {
	ftp.user = arg
	ftp.authenticated = false
	if arg == "anonymous" || arg == "ftp" || (ftpUser == "" && ftpPass == "") {
		ftp.authenticated = true
		ftp.reply(230, "Login accepted")
		return
	}
	ftp.reply(331, "Password required")
}

func (ftp *ftpSession) handlePASS(arg string) {
	if ftp.user == "anonymous" || ftp.user == "ftp" || (ftpUser == "" && ftpPass == "") {
		ftp.authenticated = true
		ftp.reply(230, "Login accepted")
		return
	}
	if ftpTlsConfig != nil && !ftp.tlsEnabled {
		ftp.reply(530, "TLS required, send AUTH TLS first")
		return
	}
	if ftp.user == ftpUser && arg == ftpPass {
		ftp.authenticated = true
		ftp.reply(230, "Login successful")
		return
	}
	ftp.reply(530, "Login incorrect")
}

// ======================== 目录操作 ========================

func (ftp *ftpSession) handlePWD() {
	if !ftp.mustAuth() {
		return
	}
	ftp.reply(257, fmt.Sprintf(`"%s"`, ftp.workDir))
}

func (ftp *ftpSession) handleCWD(arg string) {
	if !ftp.mustAuth() {
		return
	}
	abs, err := ftp.resolvePath(arg)
	if err != nil {
		ftp.reply(550, err.Error())
		return
	}
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		ftp.reply(550, "Directory not found")
		return
	}
	// 更新 workDir
	rel, _ := filepath.Rel(ftp.rootDir, abs)
	ftp.workDir = "/" + filepath.ToSlash(rel)
	if ftp.workDir == "/." {
		ftp.workDir = "/"
	}
	ftp.reply(250, "Directory changed")
}

func (ftp *ftpSession) handleMKD(arg string) {
	if !ftp.mustAuth() {
		return
	}
	abs, err := ftp.resolvePath(arg)
	if err != nil {
		ftp.reply(550, err.Error())
		return
	}
	if err := os.MkdirAll(abs, 0755); err != nil {
		if ftp.debug {
			debugLog.Errorf(ftp.logPrefix()+" MKD 失败: %v", err)
		}
		ftp.reply(550, fmt.Sprintf("MKD failed: %v", err))
		return
	}
	ftp.reply(257, fmt.Sprintf(`"%s" created`, arg))
}

func (ftp *ftpSession) handleRMD(arg string) {
	if !ftp.mustAuth() {
		return
	}
	abs, err := ftp.resolvePath(arg)
	if err != nil {
		ftp.reply(550, err.Error())
		return
	}
	if err := os.Remove(abs); err != nil {
		if ftp.debug {
			debugLog.Errorf(ftp.logPrefix()+" RMD 失败: %v", err)
		}
		ftp.reply(550, fmt.Sprintf("RMD failed: %v", err))
		return
	}
	ftp.reply(250, "Directory removed")
}

// ======================== 传输模式 ========================

func (ftp *ftpSession) handleTYPE(arg string) {
	if !ftp.mustAuth() {
		return
	}
	switch strings.ToUpper(arg) {
	case "I":
		ftp.dataType = "I"
		ftp.reply(200, "Type set to Binary")
	case "A", "A N":
		ftp.dataType = "A"
		ftp.reply(200, "Type set to ASCII")
	default:
		ftp.reply(504, "Type not supported")
	}
}

// ======================== PASV 被动模式 ========================

func (ftp *ftpSession) handlePASV() {
	if !ftp.mustAuth() {
		return
	}
	// 关闭旧的数据连接和监听器
	ftp.closeDataConn()

	// 在限定范围内监听，方便防火墙放行
	var listener net.Listener
	var err error
	for port := ftpPasvPortMin; port <= ftpPasvPortMax; port++ {
		listener, err = net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err == nil {
			break
		}
	}
	if err != nil {
		if ftp.debug {
			debugLog.Errorf(ftp.logPrefix()+" PASV 监听失败: %v", err)
		}
		ftp.reply(425, "Cannot open data connection")
		return
	}
	ftp.dataListener = listener

	_, portStr, _ := net.SplitHostPort(listener.Addr().String())
	dataPort, _ := strconv.Atoi(portStr)
	p1 := dataPort / 256
	p2 := dataPort % 256

	// 获取实际局域网 IP 返回给客户端
	host, _, _ := net.SplitHostPort(ftp.localAddr)
	if host == "0.0.0.0" || host == "::" {
		host = getLanIP()
	}
	ip := strings.ReplaceAll(host, ".", ",")
	ftp.reply(227, fmt.Sprintf("Entering Passive Mode (%s,%d,%d)", ip, p1, p2))
}

// getLanIP 获取首选局域网 IPv4 地址
// 优先返回常见局域网网段 (192.168.x.x > 10.x.x.x > 172.16-31.x.x)，避免返回 Docker/虚拟网卡的地址
func getLanIP() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "127.0.0.1"
	}

	var fallback string
	var ip172 string // 172.16-31.x.x（常见局域网）
	var ip10 string
	var ip192 string

	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipNet.IP.To4()
			if ip == nil {
				continue
			}
			ipStr := ip.String()

			if ip[0] == 192 && ip[1] == 168 {
				ip192 = ipStr
			} else if ip[0] == 10 {
				ip10 = ipStr
			} else if ip[0] == 172 && ip[1] >= 16 && ip[1] <= 31 {
				ip172 = ipStr
			} else if fallback == "" {
				fallback = ipStr
			}
		}
	}

	// 按优先级返回
	if ip192 != "" {
		return ip192
	}
	if ip10 != "" {
		return ip10
	}
	if ip172 != "" {
		return ip172
	}
	if fallback != "" {
		return fallback
	}
	return "127.0.0.1"
}

// ======================== PORT 主动模式 ========================

func (ftp *ftpSession) handlePORT(arg string) {
	if !ftp.mustAuth() {
		return
	}
	parts := strings.Split(arg, ",")
	if len(parts) != 6 {
		ftp.reply(501, "Invalid PORT format")
		return
	}
	var nums [6]int
	for i, part := range parts {
		var err error
		nums[i], err = strconv.Atoi(part)
		if err != nil {
			ftp.reply(501, "Invalid PORT format: non-numeric value")
			return
		}
	}
	h1, h2, h3, h4 := nums[0], nums[1], nums[2], nums[3]
	p1, p2 := nums[4], nums[5]

	host := fmt.Sprintf("%d.%d.%d.%d", h1, h2, h3, h4)
	port := p1*256 + p2

	// 关闭旧的数据连接和监听器
	ftp.closeDataConn()

	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, fmt.Sprintf("%d", port)), time.Second*10)
	if err != nil {
		if ftp.debug {
			debugLog.Errorf(ftp.logPrefix()+" PORT 连接失败 %s:%d: %v", host, port, err)
		}
		ftp.reply(425, "Cannot open data connection")
		return
	}
	ftp.dataConn = conn
	ftp.reply(200, "PORT command successful")
}

// ======================== 获取数据连接 ========================

func (ftp *ftpSession) getDataConn() (net.Conn, error) {
	var dc net.Conn
	var err error

	if ftp.dataListener != nil {
		defer func() {
			ftp.dataListener.Close()
			ftp.dataListener = nil
		}()
		// 设置 Accept 超时，防止客户端不连接导致 goroutine 永久阻塞
		if tl, ok := ftp.dataListener.(*net.TCPListener); ok {
			tl.SetDeadline(time.Now().Add(ftpDataConnTimeout))
		}
		dc, err = ftp.dataListener.Accept()
		if err != nil {
			return nil, err
		}
	} else if ftp.dataConn != nil {
		dc = ftp.dataConn
		ftp.dataConn = nil
	} else {
		return nil, fmt.Errorf("no data connection established")
	}

	// PROT P: 数据连接也加密
	if ftp.tlsProtData && ftpTlsConfig != nil {
		tlsDc := tls.Server(dc, ftpTlsConfig)
		if err := tlsDc.Handshake(); err != nil {
			dc.Close()
			if ftp.debug {
				debugLog.Errorf(ftp.logPrefix()+" 数据连接 TLS 握手失败: %v", err)
			}
			return nil, fmt.Errorf("TLS handshake failed: %v", err)
		}
		dc = tlsDc
	}

	ftp.dataConn = dc
	return dc, nil
}

func (ftp *ftpSession) closeDataConn() {
	if ftp.dataConn != nil {
		ftp.dataConn.Close()
		ftp.dataConn = nil
	}
	if ftp.dataListener != nil {
		ftp.dataListener.Close()
		ftp.dataListener = nil
	}
}

// ======================== 文件列表 ========================

func (ftp *ftpSession) handleLIST(arg string) {
	if !ftp.mustAuth() {
		return
	}
	dir := ftp.workDir
	if arg != "" && arg != "-a" && arg != "-la" && arg != "-al" {
		var err error
		_, err = ftp.resolvePath(arg)
		if err == nil {
			dir = arg
		}
	}

	abs, err := ftp.resolvePath(dir)
	if err != nil {
		ftp.reply(550, err.Error())
		return
	}

	ftp.reply(150, "Opening data connection for directory listing")

	dc, err := ftp.getDataConn()
	if err != nil {
		ftp.reply(425, "Cannot open data connection")
		return
	}

	entries, err := os.ReadDir(abs)
	if err != nil {
		ftp.closeDataConn()
		if ftp.debug {
			debugLog.Errorf(ftp.logPrefix()+" LIST 失败: %v", err)
		}
		ftp.reply(550, fmt.Sprintf("LIST failed: %v", err))
		return
	}

	totalSize := int64(0)
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		line := formatFtpListLine(info)
		if _, err := fmt.Fprint(dc, line+"\r\n"); err != nil {
			break
		}
		totalSize += int64(len(line) + 2)
	}
	ftp.closeDataConn()

	if ftp.debug {
		debugLog.Infof(ftp.logPrefix()+" LIST 完成: %d 条, %d 字节", len(entries), totalSize)
	}

	ftp.reply(226, "Transfer complete")
}

// formatFtpListLine 格式化为类 Unix ls -l 的输出
func formatFtpListLine(info os.FileInfo) string {
	mode := info.Mode()
	perm := mode.String()
	// 首位替换为文件类型标识
	if mode.IsDir() {
		perm = "d" + perm[1:]
	} else {
		perm = "-" + perm[1:]
	}
	t := info.ModTime()
	return fmt.Sprintf("%s 1 ftp ftp %8d %s %02d %02d %s",
		perm,
		info.Size(),
		t.Month().String()[:3],
		t.Day(),
		t.Year(),
		info.Name(),
	)
}

// ======================== 文件下载 (RETR) ========================

func (ftp *ftpSession) handleRETR(arg string) {
	if !ftp.mustAuth() {
		return
	}
	if arg == "" {
		ftp.reply(501, "Filename required")
		return
	}
	abs, err := ftp.resolvePath(arg)
	if err != nil {
		ftp.reply(550, err.Error())
		return
	}
	info, err := os.Stat(abs)
	if err != nil || info.IsDir() {
		ftp.reply(550, "File not found")
		return
	}

	ftp.reply(150, "Opening data connection for file download")
	if ftp.debug {
		debugLog.Infof(ftp.logPrefix()+" 开始下载: %s (%d 字节)", abs, info.Size())
	}

	dc, err := ftp.getDataConn()
	if err != nil {
		ftp.reply(425, "Cannot open data connection")
		return
	}

	file, err := os.Open(abs)
	if err != nil {
		ftp.closeDataConn()
		if ftp.debug {
			debugLog.Errorf(ftp.logPrefix()+" RETR 打开文件失败: %v", err)
		}
		ftp.reply(550, fmt.Sprintf("Cannot open file: %v", err))
		return
	}
	defer file.Close()

	n, err := io.Copy(dc, file)
	ftp.closeDataConn()
	if err != nil && ftp.debug {
		debugLog.Errorf(ftp.logPrefix()+" RETR 传输错误: %v", err)
	}
	if ftp.debug {
		debugLog.Infof(ftp.logPrefix()+" 下载完成: %s, 传输 %d 字节", arg, n)
	}

	ftp.reply(226, "Transfer complete")
}

// ======================== 文件上传 (STOR) ========================

func (ftp *ftpSession) handleSTOR(arg string) {
	if !ftp.mustAuth() {
		return
	}
	if arg == "" {
		ftp.reply(501, "Filename required")
		return
	}
	abs, err := ftp.resolvePath(arg)
	if err != nil {
		ftp.reply(550, err.Error())
		return
	}

	ftp.reply(150, "Opening data connection for file upload")
	if ftp.debug {
		debugLog.Infof(ftp.logPrefix()+" 开始上传: %s", abs)
	}

	dc, err := ftp.getDataConn()
	if err != nil {
		ftp.reply(425, "Cannot open data connection")
		return
	}

	// 确保目录存在
	if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
		ftp.closeDataConn()
		if ftp.debug {
			debugLog.Errorf(ftp.logPrefix()+" STOR 创建目录失败: %v", err)
		}
		ftp.reply(550, fmt.Sprintf("Cannot create directory: %v", err))
		return
	}

	file, err := os.Create(abs)
	if err != nil {
		ftp.closeDataConn()
		if ftp.debug {
			debugLog.Errorf(ftp.logPrefix()+" STOR 创建文件失败: %v", err)
		}
		ftp.reply(550, fmt.Sprintf("Cannot create file: %v", err))
		return
	}
	defer file.Close()

	n, err := io.Copy(file, dc)
	ftp.closeDataConn()
	if err != nil && ftp.debug {
		debugLog.Errorf(ftp.logPrefix()+" STOR 传输错误: %v", err)
	}
	if ftp.debug {
		debugLog.Infof(ftp.logPrefix()+" 上传完成: %s, 接收 %d 字节", arg, n)
	}

	ftp.reply(226, "Transfer complete")
}

// ======================== 删除文件 ========================

func (ftp *ftpSession) handleDELE(arg string) {
	if !ftp.mustAuth() {
		return
	}
	if arg == "" {
		ftp.reply(501, "Filename required")
		return
	}
	abs, err := ftp.resolvePath(arg)
	if err != nil {
		ftp.reply(550, err.Error())
		return
	}
	if err := os.Remove(abs); err != nil {
		if ftp.debug {
			debugLog.Errorf(ftp.logPrefix()+" DELE 失败: %v", err)
		}
		ftp.reply(550, fmt.Sprintf("DELE failed: %v", err))
		return
	}
	if ftp.debug {
		debugLog.Infof(ftp.logPrefix()+" 删除文件: %s", abs)
	}
	ftp.reply(250, "File deleted")
}

// ======================== 重命名 ========================

func (ftp *ftpSession) handleRNFR(arg string) {
	if !ftp.mustAuth() {
		return
	}
	if arg == "" {
		ftp.reply(501, "Filename required")
		return
	}
	abs, err := ftp.resolvePath(arg)
	if err != nil {
		ftp.reply(550, err.Error())
		return
	}
	if _, err := os.Stat(abs); err != nil {
		ftp.reply(550, "File not found")
		return
	}
	ftp.rnfrPath = abs
	ftp.reply(350, "Ready for RNTO")
}

func (ftp *ftpSession) handleRNTO(arg string) {
	if !ftp.mustAuth() {
		return
	}
	if ftp.rnfrPath == "" {
		ftp.reply(503, "RNFR required first")
		return
	}
	if arg == "" {
		ftp.reply(501, "Filename required")
		return
	}
	newPath, err := ftp.resolvePath(arg)
	if err != nil {
		ftp.reply(550, err.Error())
		return
	}
	if err := os.Rename(ftp.rnfrPath, newPath); err != nil {
		if ftp.debug {
			debugLog.Errorf(ftp.logPrefix()+" RNTO 失败: %v", err)
		}
		ftp.reply(550, fmt.Sprintf("Rename failed: %v", err))
	} else {
		if ftp.debug {
			debugLog.Infof(ftp.logPrefix()+" 重命名: %s -> %s", ftp.rnfrPath, newPath)
		}
		ftp.reply(250, "File renamed")
	}
	ftp.rnfrPath = ""
}

// ======================== 文件大小 ========================

func (ftp *ftpSession) handleSIZE(arg string) {
	if !ftp.mustAuth() {
		return
	}
	if arg == "" {
		ftp.reply(501, "Filename required")
		return
	}
	abs, err := ftp.resolvePath(arg)
	if err != nil {
		ftp.reply(550, err.Error())
		return
	}
	info, err := os.Stat(abs)
	if err != nil {
		ftp.reply(550, "File not found")
		return
	}
	ftp.reply(213, strconv.FormatInt(info.Size(), 10))
}

// ======================== OPTS ========================

func (ftp *ftpSession) handleOPTS(arg string) {
	upper := strings.ToUpper(arg)
	if strings.HasPrefix(upper, "UTF8") {
		ftp.utf8 = !strings.Contains(upper, "OFF")
		ftp.reply(200, "UTF8 mode set")
		return
	}
	ftp.reply(501, "Option not supported")
}

// ======================== TLS (FTPS) ========================

func (ftp *ftpSession) handleAUTH(arg string) {
	if strings.ToUpper(arg) != "TLS" {
		ftp.reply(504, "Only AUTH TLS supported")
		return
	}
	if ftpTlsConfig == nil {
		ftp.reply(502, "TLS not configured")
		return
	}
	if ftp.tlsEnabled {
		ftp.reply(200, "Already in TLS mode")
		return
	}
	// 升级控制连接到 TLS（先握手再回复，握手失败不发送 234）
	tlsConn := tls.Server(ftp.conn, ftpTlsConfig)
	if err := tlsConn.Handshake(); err != nil {
		ftp.conn.Close()
		if ftp.debug {
			debugLog.Errorf(ftp.logPrefix()+" TLS 握手失败: %v", err)
		}
		return
	}
	ftp.reply(234, "AUTH TLS successful")
	ftp.conn = tlsConn
	ftp.writer = bufio.NewWriter(tlsConn)
	ftp.reader = bufio.NewReader(tlsConn)
	ftp.tlsEnabled = true

	if ftp.debug {
		debugLog.Infof("%s", ftp.logPrefix()+" TLS 会话已建立")
	}
}

func (ftp *ftpSession) handlePBSZ(_ string) {
	if !ftp.tlsEnabled {
		ftp.reply(503, "TLS required for PBSZ")
		return
	}
	ftp.reply(200, "PBSZ=0")
}

func (ftp *ftpSession) handlePROT(arg string) {
	if !ftp.tlsEnabled {
		ftp.reply(503, "TLS required for PROT")
		return
	}
	switch strings.ToUpper(arg) {
	case "P":
		ftp.tlsProtData = true
		ftp.reply(200, "Protection level set to Private")
	case "C":
		ftp.tlsProtData = false
		ftp.reply(200, "Protection level set to Clear")
	default:
		ftp.reply(504, "Protection level not supported")
	}
}
