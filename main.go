package main

import (
	"context"
	"embed"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"sangforpm/internal/httpapi"
	"sangforpm/internal/store"
)

//go:embed all:web
var webFS embed.FS

const version = "1.0.0"

func main() {
	port := flag.Int("port", 8787, "监听端口")
	host := flag.String("host", "0.0.0.0", "监听地址；0.0.0.0 允许局域网访问，127.0.0.1 仅本机")
	noOpen := flag.Bool("no-open", false, "启动后不自动打开浏览器")
	dataDir := flag.String("data", "", "数据目录（留空用默认：Windows %APPDATA%\\SangforPM，Linux/macOS ~/.sangfor-pm）")
	flag.Parse()

	log.SetFlags(log.Ltime)
	fmt.Println("==================================================")
	fmt.Println("  项目管理 · 深信服实施交付个人工作台  v" + version)
	fmt.Println("  平台:", runtime.GOOS+"/"+runtime.GOARCH)
	fmt.Println("==================================================")

	st, err := store.OpenAt(*dataDir)
	if err != nil {
		fatal("打开数据库失败", err)
	}
	fmt.Println("数据目录:", st.DataDir)
	fmt.Println("数据库  :", filepath.Join(st.DataDir, "data.db"))
	if tmp := prepareTempDir(st.DataDir); tmp != "" {
		fmt.Println("临时目录:", tmp)
	}

	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		fatal("加载前端资源失败", err)
	}
	srv := httpapi.New(st, sub)
	srv.StartBackupScheduler()

	ln, addr, err := listen(*host, *port)
	if err != nil {
		fatal("端口被占用，请用 -port 指定其他端口", err)
	}
	actualPort := fmt.Sprint(addr.(*net.TCPAddr).Port)
	localURL := "http://127.0.0.1:" + actualPort
	lanURLs := lanURLs(*host, actualPort)

	fmt.Println("本机访问:", localURL)
	if len(lanURLs) > 0 {
		fmt.Println("局域网访问（其他主机可用）:")
		for _, u := range lanURLs {
			fmt.Println("   ", u)
		}
	} else if isLoopbackHost(*host) {
		fmt.Println("当前仅监听本机（-host 127.0.0.1），其他主机无法访问。")
	}
	if !isLoopbackHost(*host) {
		fmt.Println("--------------------------------------------------")
		fmt.Println("⚠ 已开放给局域网：请确保账号密码足够复杂；")
		fmt.Println("  附件与导出的网络信息含明文账号密码，仅在可信网络使用。")
		fmt.Println("  仅本机使用可加参数：-host 127.0.0.1")
		printFirewallHint(actualPort)
	}
	fmt.Println("--------------------------------------------------")
	if runtime.GOOS == "windows" {
		fmt.Println("按 Ctrl+C 停止服务（也可直接关闭本窗口）。")
	} else {
		fmt.Println("按 Ctrl+C 停止服务。")
	}
	fmt.Println("--------------------------------------------------")

	if !*noOpen {
		go func() {
			time.Sleep(600 * time.Millisecond)
			openBrowser(localURL)
		}()
	}

	httpSrv := &http.Server{
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 15 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM)
		<-c
		fmt.Println("\n正在停止服务…")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(ctx)
	}()

	if err := httpSrv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Printf("服务异常退出: %v", err)
	}
	if err := st.Close(); err != nil {
		log.Printf("关闭数据库: %v", err)
	}
	fmt.Println("服务已停止。")
}

// prepareTempDir 把上传分片目录放到数据目录所在磁盘。
// Linux 上 /tmp 常是 tmpfs，300MB 附件如果先落到 /tmp 会直接吃内存；
// 顺手清理 24 小时前进程被杀留下的残留分片。
func prepareTempDir(dataDir string) string {
	if runtime.GOOS == "windows" {
		return ""
	}
	if v := os.Getenv("TMPDIR"); v != "" {
		return v
	}
	tmp := filepath.Join(dataDir, "tmp")
	if err := os.MkdirAll(tmp, 0o700); err != nil {
		return ""
	}
	_ = os.Setenv("TMPDIR", tmp)

	if entries, err := os.ReadDir(tmp); err == nil {
		cutoff := time.Now().Add(-24 * time.Hour)
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			if info, ierr := e.Info(); ierr == nil && info.ModTime().Before(cutoff) {
				_ = os.Remove(filepath.Join(tmp, e.Name()))
			}
		}
	}
	return tmp
}

// printFirewallHint 按平台给出放行端口的提示。
func printFirewallHint(port string) {
	switch runtime.GOOS {
	case "windows":
		fmt.Println("  若其他主机仍打不开，需放行防火墙（运行「允许局域网访问.bat」，管理员权限）：")
		fmt.Printf("    netsh advfirewall firewall add rule name=\"项目管理\" dir=in action=allow protocol=TCP localport=%s\n", port)
	case "linux":
		fmt.Println("  若其他主机仍打不开，需放行防火墙（任选其一，需 root）：")
		fmt.Printf("    sudo ufw allow %s/tcp\n", port)
		fmt.Printf("    sudo firewall-cmd --add-port=%s/tcp --permanent && sudo firewall-cmd --reload\n", port)
	default:
		fmt.Printf("  若其他主机仍打不开，请在系统防火墙放行 TCP %s 端口。\n", port)
	}
}

func listen(host string, port int) (net.Listener, net.Addr, error) {
	var lastErr error
	for i := 0; i < 20; i++ {
		addr := fmt.Sprintf("%s:%d", host, port+i)
		ln, err := net.Listen("tcp", addr)
		if err == nil {
			return ln, ln.Addr(), nil
		}
		lastErr = err
	}
	return nil, nil, lastErr
}

func isLoopbackHost(h string) bool {
	if h == "" {
		return false
	}
	if h == "localhost" {
		return true
	}
	ip := net.ParseIP(strings.Trim(h, "[]"))
	return ip != nil && ip.IsLoopback()
}

// lanURLs 返回其他主机可用的访问地址；仅监听回环时返回空。
// 优先返回物理网卡地址，虚拟网卡（VMware/Hyper-V/VPN 等）作为兜底，避免刷屏。
func lanURLs(host, port string) []string {
	if isLoopbackHost(host) {
		return nil
	}
	if ip := net.ParseIP(strings.Trim(host, "[]")); ip != nil && !ip.IsUnspecified() {
		return []string{"http://" + ip.String() + ":" + port}
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	virtualKeys := []string{"vmware", "virtualbox", "hyper-v", "vethernet", "loopback",
		"tap", "tun", "wsl", "docker", "tailscale", "zerotier", "radmin", "npcap", "bluetooth",
		// Linux 容器/网桥：veth*、docker 生成的 br-<id>、KVM 的 virbr、K8s 的 cni/cali/flannel
		"veth", "br-", "virbr", "cni", "cali", "flannel", "kube"}
	primary := make([]string, 0, 4)
	fallback := make([]string, 0, 4)
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		name := strings.ToLower(ifc.Name)
		isVirtual := false
		for _, k := range virtualKeys {
			if strings.Contains(name, k) {
				isVirtual = true
				break
			}
		}
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipnet, okip := a.(*net.IPNet)
			if !okip {
				continue
			}
			ip4 := ipnet.IP.To4()
			if ip4 == nil || ip4.IsLoopback() || ip4.IsLinkLocalUnicast() {
				continue
			}
			u := "http://" + ip4.String() + ":" + port
			if isVirtual {
				fallback = append(fallback, u)
			} else {
				primary = append(primary, u)
			}
		}
	}
	if len(primary) > 0 {
		return primary
	}
	return fallback
}

// openBrowser 在桌面环境下打开默认浏览器；服务器 / 无图形界面时静默跳过。
func openBrowser(url string) {
	if runtime.GOOS == "linux" && os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		return
	}
	var bin string
	var args []string
	switch runtime.GOOS {
	case "windows":
		bin, args = "rundll32", []string{"url.dll,FileProtocolHandler", url}
	case "darwin":
		bin, args = "open", []string{url}
	default:
		bin, args = "xdg-open", []string{url}
	}
	if _, err := exec.LookPath(bin); err != nil {
		return
	}
	_ = exec.Command(bin, args...).Start()
}

func fatal(msg string, err error) {
	fmt.Println("[错误]", msg)
	if err != nil {
		fmt.Println("     ", err)
	}
	waitForEnter()
	os.Exit(1)
}

// waitForEnter 只在交互终端里等待回车，避免 systemd / docker / 管道环境下卡死。
func waitForEnter() {
	fi, err := os.Stdin.Stat()
	if err != nil || fi.Mode()&os.ModeCharDevice == 0 {
		return
	}
	fmt.Println("\n按回车键退出…")
	var b [1]byte
	_, _ = os.Stdin.Read(b[:])
}
