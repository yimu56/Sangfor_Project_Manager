package httpapi

import (
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"sangforpm/internal/auth"
	"sangforpm/internal/store"
)

func (s *Server) hState(w http.ResponseWriter, r *http.Request) {
	_, err := s.st.QueryRowMap(`SELECT id FROM users LIMIT 1`)
	initialized := err == nil
	ok(w, map[string]any{
		"initialized": initialized,
		"app_name":    s.st.GetSetting("app_name"),
		"subtitle":    s.st.GetSetting("app_subtitle"),
		"version":     "1.0.0",
	})
}

func (s *Server) hSetup(w http.ResponseWriter, r *http.Request) {
	if _, err := s.st.QueryRowMap(`SELECT id FROM users LIMIT 1`); err == nil {
		writeErr(w, 400, "系统已初始化")
		return
	}
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	body.Username = strings.TrimSpace(body.Username)
	if len(body.Username) < 2 || len(body.Password) < 6 {
		writeErr(w, 400, "用户名至少2位，密码至少6位")
		return
	}
	now := store.Now()
	_, err := s.st.Exec(`INSERT INTO users(username,password_hash,created_at,updated_at) VALUES(?,?,?,?)`,
		body.Username, auth.Hash(body.Password), now, now)
	if err != nil {
		writeErr(w, 500, "创建账号失败: "+err.Error())
		return
	}
	tok := s.sess.Create()
	http.SetCookie(w, &http.Cookie{Name: "pmsess", Value: tok, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 30 * 24 * 3600})
	ok(w, map[string]any{"username": body.Username})
}

func (s *Server) hLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	ip := clientIP(r)
	if wait, locked := s.loginLocked(ip); locked {
		w.Header().Set("Retry-After", fmt.Sprint(int(wait.Seconds())))
		writeErr(w, 429, fmt.Sprintf("密码错误次数过多，请 %d 秒后再试", int(wait.Seconds())))
		return
	}
	m, err := s.st.QueryRowMap(`SELECT id,username,password_hash FROM users ORDER BY id LIMIT 1`)
	if err != nil {
		writeErr(w, 400, "系统尚未初始化")
		return
	}
	if store.Pick(m, "username") != strings.TrimSpace(body.Username) || !auth.Verify(body.Password, store.Pick(m, "password_hash")) {
		s.loginFailed(ip)
		writeErr(w, 400, "用户名或密码错误")
		return
	}
	s.loginOK(ip)
	tok := s.sess.Create()
	http.SetCookie(w, &http.Cookie{Name: "pmsess", Value: tok, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 30 * 24 * 3600})
	ok(w, map[string]any{"username": store.Pick(m, "username")})
}

func (s *Server) hLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie("pmsess"); err == nil {
		s.sess.Drop(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: "pmsess", Value: "", Path: "/", MaxAge: -1})
	ok(w, true)
}

func (s *Server) hMe(w http.ResponseWriter, r *http.Request) {
	ok(w, map[string]any{"username": s.currentUser(r)})
}

func (s *Server) hChangePassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Old string `json:"old"`
		New string `json:"new"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	if len(body.New) < 6 {
		writeErr(w, 400, "新密码至少6位")
		return
	}
	m, err := s.st.QueryRowMap(`SELECT id,password_hash FROM users ORDER BY id LIMIT 1`)
	if err != nil {
		writeErr(w, 400, "系统尚未初始化")
		return
	}
	if !auth.Verify(body.Old, store.Pick(m, "password_hash")) {
		writeErr(w, 400, "原密码错误")
		return
	}
	_, err = s.st.Exec(`UPDATE users SET password_hash=?,updated_at=? WHERE id=?`,
		auth.Hash(body.New), store.Now(), store.PickInt(m, "id"))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ok(w, true)
}

// hResetToken 生成一次性重置令牌。仅允许本机发起，避免同网段他人直接接管账号。
func (s *Server) hResetToken(w http.ResponseWriter, r *http.Request) {
	if !isLoopbackReq(r) {
		writeErr(w, 403, "出于安全考虑，重置令牌只能在运行服务的本机生成（其他主机请到服务器上操作）")
		return
	}
	if _, err := s.st.QueryRowMap(`SELECT id FROM users LIMIT 1`); err != nil {
		writeErr(w, 400, "系统尚未初始化")
		return
	}
	tok := auth.NewResetToken()
	exp := time.Now().Add(30 * time.Minute).Format("2006-01-02 15:04:05")
	if _, err := s.st.Exec(`UPDATE users SET reset_token=?,reset_expire=?`, tok, exp); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	p := filepath.Join(s.st.DataDir, "RESET_TOKEN.txt")
	_ = os.WriteFile(p, []byte("密码重置令牌: "+tok+"\n有效期至: "+exp+"\n"), 0o600)
	writeJSON(w, 200, map[string]any{"data": map[string]any{"token": tok, "expire_at": exp, "file": p}})
}

func (s *Server) hResetPassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token string `json:"token"`
		New   string `json:"new"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	if len(body.New) < 6 {
		writeErr(w, 400, "新密码至少6位")
		return
	}
	m, err := s.st.QueryRowMap(`SELECT id,reset_token,reset_expire FROM users ORDER BY id LIMIT 1`)
	if err != nil {
		writeErr(w, 400, "系统尚未初始化")
		return
	}
	saved := strings.TrimSpace(store.Pick(m, "reset_token"))
	if saved == "" || !strings.EqualFold(saved, strings.TrimSpace(body.Token)) {
		writeErr(w, 400, "重置令牌无效")
		return
	}
	exp, err := time.ParseInLocation("2006-01-02 15:04:05", store.Pick(m, "reset_expire"), time.Local)
	if err != nil || time.Now().After(exp) {
		writeErr(w, 400, "重置令牌已过期，请重新获取")
		return
	}
	if _, err := s.st.Exec(`UPDATE users SET password_hash=?,reset_token=NULL,reset_expire=NULL,updated_at=? WHERE id=?`,
		auth.Hash(body.New), store.Now(), store.PickInt(m, "id")); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.sess.DropAll()
	_ = os.Remove(filepath.Join(s.st.DataDir, "RESET_TOKEN.txt"))
	ok(w, true)
}

// ---------- 系统设置 ----------

var settingKeys = []string{
	"app_name", "app_subtitle", "max_attach_mb", "warranty_notice",
	"stale_days", "accept_notice", "longrun_days",
}

func (s *Server) hGetSettings(w http.ResponseWriter, r *http.Request) {
	out := map[string]string{}
	for _, k := range settingKeys {
		out[k] = s.st.GetSetting(k)
	}
	out["data_dir"] = s.st.DataDir
	out["db_path"] = filepath.Join(s.st.DataDir, "data.db")
	ok(w, out)
}

func (s *Server) hPutSettings(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	for _, k := range settingKeys {
		if v, okv := body[k]; okv {
			if err := s.st.SetSetting(k, strings.TrimSpace(store.Pick(map[string]any{"v": v}, "v"))); err != nil {
				writeErr(w, 500, err.Error())
				return
			}
		}
	}
	ok(w, true)
}

// ---------- 备份 ----------

func (s *Server) hBackupConfigGet(w http.ResponseWriter, r *http.Request) {
	m, err := s.st.QueryRowMap(`SELECT * FROM backup_config WHERE id=1`)
	if err == sql.ErrNoRows {
		ok(w, map[string]any{})
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ok(w, m)
}

var backupFields = []string{
	"target_type", "local_path", "s3_endpoint", "s3_region", "s3_bucket",
	"s3_access_key", "s3_secret_key", "s3_prefix", "frequency",
	"frequency_value", "keep_count", "include_attachments", "enabled",
}

func (s *Server) hBackupConfigPut(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	if err := s.st.Update("backup_config", 1, body, backupFields); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	// 重新计算下次备份时间
	if cfg, err := s.st.QueryRowMap(`SELECT * FROM backup_config WHERE id=1`); err == nil {
		if t, okv := nextBackupTime(cfg); okv {
			_, _ = s.st.Exec(`UPDATE backup_config SET next_backup_at=? WHERE id=1`, t)
		}
	}
	ok(w, true)
}
