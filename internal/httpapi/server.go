package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"sangforpm/internal/auth"
	"sangforpm/internal/store"
)

type Server struct {
	st   *store.Store
	sess *auth.Sessions
	web  fs.FS
	mux  *http.ServeMux

	loginMu    sync.Mutex
	loginFails map[string]*failStat
}

type failStat struct {
	count int
	until time.Time
}

const (
	maxLoginFails = 5
	loginLockDur  = 60 * time.Second
)

func New(st *store.Store, web fs.FS) *Server {
	s := &Server{
		st: st, sess: auth.NewSessions(), web: web, mux: http.NewServeMux(),
		loginFails: map[string]*failStat{},
	}
	s.routes()
	return s
}

func (s *Server) loginLocked(ip string) (time.Duration, bool) {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	f, okf := s.loginFails[ip]
	if !okf {
		return 0, false
	}
	if time.Now().Before(f.until) {
		return time.Until(f.until), true
	}
	if f.count >= maxLoginFails {
		delete(s.loginFails, ip)
	}
	return 0, false
}

func (s *Server) loginFailed(ip string) {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	f, okf := s.loginFails[ip]
	if !okf {
		f = &failStat{}
		s.loginFails[ip] = f
	}
	f.count++
	if f.count >= maxLoginFails {
		f.until = time.Now().Add(loginLockDur)
		f.count = 0
	}
}

func (s *Server) loginOK(ip string) {
	s.loginMu.Lock()
	delete(s.loginFails, ip)
	s.loginMu.Unlock()
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func isLoopbackReq(r *http.Request) bool {
	ip := net.ParseIP(clientIP(r))
	if ip == nil {
		return false
	}
	if ip.IsLoopback() {
		return true
	}
	// 本机通过自己的局域网 IP 访问也算本机
	ifaces, err := net.Interfaces()
	if err != nil {
		return false
	}
	for _, ifc := range ifaces {
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			if ipnet, okip := a.(*net.IPNet); okip && ipnet.IP.Equal(ip) {
				return true
			}
		}
	}
	return false
}

func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("panic: %v", rec)
				writeErr(w, 500, "服务器内部错误")
			}
		}()
		w.Header().Set("X-Content-Type-Options", "nosniff")
		s.mux.ServeHTTP(w, r)
	})
}

func (s *Server) routes() {
	m := s.mux

	// --- 公共 ---
	m.HandleFunc("GET /api/state", s.hState)
	m.HandleFunc("POST /api/setup", s.hSetup)
	m.HandleFunc("POST /api/login", s.hLogin)
	m.HandleFunc("POST /api/logout", s.hLogout)
	m.HandleFunc("POST /api/password/reset-token", s.hResetToken)
	m.HandleFunc("POST /api/password/reset", s.hResetPassword)

	// --- 需登录 ---
	p := func(pattern string, h http.HandlerFunc) { m.Handle(pattern, s.auth(h)) }

	p("GET /api/me", s.hMe)
	p("GET /api/meta", s.hMeta)
	p("POST /api/upload", s.hUpload)
	p("GET /api/blob/{name...}", s.hBlob)
	p("POST /api/password/change", s.hChangePassword)
	p("GET /api/settings", s.hGetSettings)
	p("PUT /api/settings", s.hPutSettings)

	// 客户
	p("GET /api/customers", s.hCustomerList)
	p("POST /api/customers", s.hCustomerCreate)
	p("GET /api/customers/{id}", s.hCustomerGet)
	p("PUT /api/customers/{id}", s.hCustomerUpdate)
	p("DELETE /api/customers/{id}", s.hCustomerDelete)
	p("GET /api/customers/{id}/contacts", s.hContactList)
	p("POST /api/customers/{id}/contacts", s.hContactCreate)
	p("PUT /api/contacts/{id}", s.hContactUpdate)
	p("DELETE /api/contacts/{id}", s.hContactDelete)
	p("GET /api/customers/{id}/net-versions", s.hNetVersionList)
	p("POST /api/customers/{id}/net-versions", s.hNetVersionCreate)
	p("GET /api/net-versions/{id}", s.hNetVersionGet)
	p("GET /api/net-versions/{id}/changes", s.hNetChanges)
	p("GET /api/customers/{id}/devices", s.hDeviceList)
	p("POST /api/customers/{id}/devices", s.hDeviceCreate)
	p("GET /api/devices/{id}", s.hDeviceGet)
	p("PUT /api/devices/{id}", s.hDeviceUpdate)
	p("DELETE /api/devices/{id}", s.hDeviceDelete)
	p("GET /api/devices/{id}/history", s.hDeviceHistory)

	// 项目
	p("GET /api/projects", s.hProjectList)
	p("POST /api/projects", s.hProjectCreate)
	p("POST /api/projects/convert", s.hProjectConvert)
	p("POST /api/projects/batch/status", s.hBatchStatus)
	p("GET /api/projects/{id}", s.hProjectGet)
	p("PUT /api/projects/{id}", s.hProjectUpdate)
	p("DELETE /api/projects/{id}", s.hProjectDelete)
	p("GET /api/projects/{id}/related", s.hProjectRelated)
	p("POST /api/projects/{id}/relation", s.hProjectRelationAdd)
	p("POST /api/projects/{id}/stages/init", s.hStageInit)
	p("GET /api/projects/{id}/stages", s.hStageList)
	p("POST /api/projects/{id}/stages", s.hStageCreate)
	p("PUT /api/stages/{id}", s.hStageUpdate)
	p("DELETE /api/stages/{id}", s.hStageDelete)

	p("GET /api/projects/{id}/devices", s.hSnapshotList)
	p("POST /api/projects/{id}/devices", s.hSnapshotCreate)
	p("POST /api/projects/{id}/devices/inherit", s.hSnapshotInherit)
	p("PUT /api/snapshots/{id}", s.hSnapshotUpdate)
	p("DELETE /api/snapshots/{id}", s.hSnapshotDelete)
	p("GET /api/snapshots/{id}/history", s.hSnapshotHistory)

	p("GET /api/projects/{id}/net-ref", s.hNetRefGet)
	p("POST /api/projects/{id}/net-ref", s.hNetRefSet)

	p("GET /api/projects/{id}/attachments", s.hAttachList)
	p("POST /api/projects/{id}/attachments", s.hAttachUpload)
	p("POST /api/attachments/{id}/versions", s.hAttachUploadVersion)
	p("GET /api/attachments/{id}/versions", s.hAttachVersions)
	p("DELETE /api/attachments/{id}", s.hAttachDelete)

	// 进度与问题
	p("GET /api/projects/{id}/progress", s.hProgressList)
	p("POST /api/projects/{id}/progress", s.hProgressCreate)
	p("GET /api/progress", s.hProgressAll)
	p("PUT /api/progress/{id}", s.hProgressUpdate)
	p("DELETE /api/progress/{id}", s.hProgressDelete)
	p("GET /api/projects/{id}/issues", s.hIssueList)
	p("POST /api/projects/{id}/issues", s.hIssueCreate)
	p("GET /api/issues", s.hIssueAll)
	p("PUT /api/issues/{id}", s.hIssueUpdate)
	p("DELETE /api/issues/{id}", s.hIssueDelete)

	// 维保
	p("GET /api/projects/{id}/maintenance", s.hMaintGet)
	p("PUT /api/projects/{id}/maintenance", s.hMaintUpdate)
	p("GET /api/maintenance", s.hMaintList)

	// 看板 / 提醒 / 搜索
	p("GET /api/dashboard", s.hDashboard)
	p("GET /api/reminders", s.hReminderList)
	p("POST /api/reminders/scan", s.hReminderScan)
	p("PUT /api/reminders/{id}", s.hReminderUpdate)
	p("GET /api/search", s.hSearch)

	// 导出 / 批量
	p("GET /api/export/excel", s.hExportExcel)
	p("GET /api/export/pdf", s.hExportPDF)
	p("GET /api/export/logs", s.hExportLogs)

	// 备份
	p("GET /api/backup/config", s.hBackupConfigGet)
	p("PUT /api/backup/config", s.hBackupConfigPut)
	p("POST /api/backup/run", s.hBackupRun)
	p("GET /api/backup/list", s.hBackupList)
	p("GET /api/backup/download", s.hBackupDownload)

	// 文件流（附件预览 / 下载）
	p("GET /api/files/{vid}", s.hFileServe)

	// --- 静态资源（SPA） ---
	m.HandleFunc("/", s.hStatic)
}

func (s *Server) auth(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie("pmsess")
		if err != nil || !s.sess.Valid(c.Value) {
			writeErr(w, 401, "未登录")
			return
		}
		next(w, r)
	})
}

func (s *Server) hStatic(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/")
	if name == "" {
		name = "index.html"
	}
	if f, err := s.web.Open(name); err == nil {
		f.Close()
		http.ServeFileFS(w, r, s.web, name)
		return
	}
	// SPA 回退
	http.ServeFileFS(w, r, s.web, "index.html")
}

// ---------- 工具 ----------

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) { writeJSON(w, code, map[string]any{"error": msg}) }

func ok(w http.ResponseWriter, data any) { writeJSON(w, 200, map[string]any{"data": data}) }

func readJSON(r *http.Request, v any) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		return err
	}
	if len(body) == 0 {
		return nil
	}
	return json.Unmarshal(body, v)
}

func pathID(r *http.Request, key string) int64 {
	v := r.PathValue(key)
	n, _ := strconv.ParseInt(v, 10, 64)
	return n
}

func reqID(w http.ResponseWriter, r *http.Request, key string) (int64, bool) {
	id := pathID(r, key)
	if id <= 0 {
		writeErr(w, 400, "无效的 ID")
		return 0, false
	}
	return id, true
}

func (s *Server) currentUser(r *http.Request) string {
	m, err := s.st.QueryRowMap(`SELECT username FROM users ORDER BY id LIMIT 1`)
	if err != nil {
		return "admin"
	}
	return store.Pick(m, "username")
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func abs(p string) string {
	a, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return a
}

var errNotFound = errors.New("not found")

func nowTime() time.Time { return time.Now() }
