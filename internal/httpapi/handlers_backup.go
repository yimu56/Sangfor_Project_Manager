package httpapi

import (
	"archive/zip"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"sangforpm/internal/store"
)

// ---------- 批量改状态 ----------

func (s *Server) hBatchStatus(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs    []int64 `json:"ids"`
		Status string  `json:"status"`
	}
	if err := readJSON(r, &body); err != nil || len(body.IDs) == 0 || strings.TrimSpace(body.Status) == "" {
		writeErr(w, 400, "请选择项目并指定状态")
		return
	}
	now := store.Now()
	n := 0
	for _, id := range body.IDs {
		if _, err := s.st.Exec(`UPDATE projects SET status=?,updated_at=? WHERE id=?`, body.Status, now, id); err != nil {
			continue
		}
		n++
	}
	ok(w, map[string]any{"updated": n})
}

// ---------- 本地备份 ----------

func (s *Server) hBackupRun(w http.ResponseWriter, r *http.Request) {
	p, err := s.runBackup()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ok(w, map[string]any{"file": p})
}

func (s *Server) hBackupList(w http.ResponseWriter, r *http.Request) {
	list := make([]map[string]any, 0)
	entries, err := os.ReadDir(s.st.Backups)
	if err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".zip") {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			list = append(list, map[string]any{
				"name": e.Name(), "size": info.Size(),
				"created_at": info.ModTime().Format("2006-01-02 15:04:05"),
			})
		}
	}
	sort.Slice(list, func(i, j int) bool {
		return store.Pick(list[i], "created_at") > store.Pick(list[j], "created_at")
	})
	cfg, _ := s.st.QueryRowMap(`SELECT * FROM backup_config WHERE id=1`)
	ok(w, map[string]any{"backups": list, "config": cfg, "dir": s.st.Backups})
}

func (s *Server) hBackupDownload(w http.ResponseWriter, r *http.Request) {
	name := filepath.Base(r.URL.Query().Get("name"))
	if name == "" || !strings.HasSuffix(name, ".zip") {
		writeErr(w, 400, "参数错误")
		return
	}
	p := filepath.Join(s.st.Backups, name)
	if !exists(p) {
		writeErr(w, 404, "备份不存在")
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename="+name)
	http.ServeFile(w, r, p)
}

// runBackup 生成 zip 备份（数据库快照 + 附件文件）
func (s *Server) runBackup() (string, error) {
	cfg, err := s.st.QueryRowMap(`SELECT * FROM backup_config WHERE id=1`)
	if err != nil {
		cfg = map[string]any{}
	}
	ts := time.Now().Format("20060102-150405")
	zipPath := filepath.Join(s.st.Backups, fmt.Sprintf("pm-backup-%s.zip", ts))
	out, err := os.Create(zipPath)
	if err != nil {
		return "", err
	}
	defer out.Close()
	zw := zip.NewWriter(out)

	// 1) 数据库一致性快照
	tmpDB := filepath.Join(s.st.Backups, ".snapshot.db")
	_ = os.Remove(tmpDB)
	if _, err := s.st.DB.Exec(`VACUUM INTO ?`, tmpDB); err != nil {
		// 回退方案：直接复制
		if err2 := copyFile(filepath.Join(s.st.DataDir, "data.db"), tmpDB); err2 != nil {
			zw.Close()
			os.Remove(zipPath)
			return "", fmt.Errorf("数据库快照失败: %v / %v", err, err2)
		}
	}
	if err := addFileToZip(zw, tmpDB, "data.db"); err != nil {
		zw.Close()
		os.Remove(zipPath)
		return "", err
	}
	_ = os.Remove(tmpDB)

	// 2) 附件文件
	if store.PickInt(cfg, "include_attachments") != 0 {
		_ = filepath.Walk(s.st.Files, func(p string, info os.FileInfo, err error) error {
			if err != nil || info == nil {
				return nil
			}
			if info.IsDir() {
				if info.Name() == "tmp" {
					return filepath.SkipDir
				}
				return nil
			}
			rel, _ := filepath.Rel(s.st.Files, p)
			_ = addFileToZip(zw, p, "files/"+filepath.ToSlash(rel))
			return nil
		})
	}
	// 3) 元信息
	if w, err := zw.Create("backup-info.txt"); err == nil {
		_, _ = fmt.Fprintf(w, "软件: %s\n备份时间: %s\n包含附件: %v\n数据目录: %s\n",
			s.st.GetSetting("app_name"), store.Now(),
			store.PickInt(cfg, "include_attachments") != 0, s.st.DataDir)
	}
	if err := zw.Close(); err != nil {
		os.Remove(zipPath)
		return "", err
	}
	// 更新上次/下次备份时间
	_, _ = s.st.Exec(`UPDATE backup_config SET last_backup_at=? WHERE id=1`, store.Now())
	if t, okv := nextBackupTime(cfg); okv {
		_, _ = s.st.Exec(`UPDATE backup_config SET next_backup_at=? WHERE id=1`, t)
	}
	s.pruneBackups(store.PickInt(cfg, "keep_count"))
	return zipPath, nil
}

func addFileToZip(zw *zip.Writer, path, name string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = io.Copy(w, f)
	return err
}

func (s *Server) pruneBackups(keep int) {
	if keep <= 0 {
		keep = 10
	}
	entries, err := os.ReadDir(s.st.Backups)
	if err != nil {
		return
	}
	names := make([]string, 0)
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".zip") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for len(names) > keep {
		_ = os.Remove(filepath.Join(s.st.Backups, names[0]))
		names = names[1:]
	}
}

// nextBackupTime 依据频率计算下次备份时间
func nextBackupTime(cfg map[string]any) (string, bool) {
	if store.PickInt(cfg, "enabled") == 0 {
		return "", false
	}
	freq := store.Pick(cfg, "frequency")
	val := store.PickInt(cfg, "frequency_value")
	if val <= 0 {
		val = 1
	}
	now := time.Now()
	var next time.Time
	switch freq {
	case "hourly":
		next = now.Add(time.Duration(val) * time.Hour)
	case "weekly":
		next = now.AddDate(0, 0, 7*val)
	case "monthly":
		next = now.AddDate(0, val, 0)
	default: // daily
		next = now.AddDate(0, 0, val)
	}
	return next.Format("2006-01-02 15:04:05"), true
}

// StartBackupScheduler 每分钟检查一次是否到达下次备份时间
func (s *Server) StartBackupScheduler() {
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			cfg, err := s.st.QueryRowMap(`SELECT * FROM backup_config WHERE id=1`)
			if err != nil || store.PickInt(cfg, "enabled") == 0 {
				continue
			}
			next := strings.TrimSpace(store.Pick(cfg, "next_backup_at"))
			if next == "" {
				if t, okv := nextBackupTime(cfg); okv {
					_, _ = s.st.Exec(`UPDATE backup_config SET next_backup_at=? WHERE id=1`, t)
				}
				continue
			}
			t, err := time.ParseInLocation("2006-01-02 15:04:05", next, time.Local)
			if err != nil || time.Now().Before(t) {
				continue
			}
			if _, err := s.runBackup(); err == nil {
				// 备份成功后 runBackup 已更新 next_backup_at
			} else {
				_, _ = s.st.Exec(`UPDATE backup_config SET next_backup_at=? WHERE id=1`,
					time.Now().Add(10*time.Minute).Format("2006-01-02 15:04:05"))
			}
		}
	}()
}

var _ = sql.ErrNoRows
