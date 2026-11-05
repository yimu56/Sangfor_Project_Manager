package httpapi

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"net/http"

	"sangforpm/internal/store"
)

// ---------- 进度记录 ----------

func (s *Server) hProgressList(w http.ResponseWriter, r *http.Request) {
	id, okid := reqID(w, r, "id")
	if !okid {
		return
	}
	rows, err := s.st.QueryMaps(`SELECT * FROM progress WHERE project_id=? ORDER BY record_date DESC, id DESC`, id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.hydrateProgress(rows)
	ok(w, rows)
}

func (s *Server) hydrateProgress(rows []map[string]any) {
	for _, m := range rows {
		pid := store.PickInt(m, "id")
		stages, _ := s.st.QueryMaps(`SELECT ps.stage_id, s.name, s.seq FROM progress_stages ps
			LEFT JOIN project_stages s ON s.id=ps.stage_id WHERE ps.progress_id=? ORDER BY s.seq`, pid)
		names := make([]string, 0, len(stages))
		for _, st := range stages {
			names = append(names, store.Pick(st, "name"))
		}
		m["stages"] = stages
		m["stage_names"] = strings.Join(names, "、")
		atts, _ := s.st.QueryMaps(`SELECT a.id, a.file_name, a.atype, a.current_version_id,
			(SELECT file_path FROM attachment_versions v WHERE v.id=a.current_version_id) AS file_path
			FROM progress_attachments pa JOIN attachments a ON a.id=pa.attachment_id WHERE pa.progress_id=?`, pid)
		for _, a := range atts {
			if v := store.Pick(a, "current_version_id"); v != "" {
				a["url"] = "/api/files/" + v
			}
		}
		m["attachments"] = atts
	}
}

var progressFields = []string{"record_date", "current_progress", "problems", "next_plan", "info_sync"}

func (s *Server) hProgressCreate(w http.ResponseWriter, r *http.Request) {
	pid, okid := reqID(w, r, "id")
	if !okid {
		return
	}
	var body map[string]any
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	if strings.TrimSpace(store.Pick(body, "record_date")) == "" {
		body["record_date"] = store.Today()
	}
	if strings.TrimSpace(store.Pick(body, "current_progress")) == "" {
		writeErr(w, 400, "当前进展不能为空")
		return
	}
	now := store.Now()
	body["project_id"] = pid
	body["created_at"], body["updated_at"] = now, now
	nid, err := s.st.Insert("progress", body, append(progressFields, "project_id", "created_at", "updated_at"))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.saveProgressLinks(nid, body)
	_, _ = s.st.Exec(`UPDATE projects SET updated_at=? WHERE id=?`, now, pid)
	ok(w, map[string]any{"id": nid})
}

func (s *Server) saveProgressLinks(progressID int64, body map[string]any) {
	if raw, okv := body["stage_ids"]; okv {
		if arr, isArr := raw.([]any); isArr {
			_, _ = s.st.Exec(`DELETE FROM progress_stages WHERE progress_id=?`, progressID)
			for _, it := range arr {
				var sid int64
				switch t := it.(type) {
				case float64:
					sid = int64(t)
				case string:
					sid, _ = strconv.ParseInt(t, 10, 64)
				}
				if sid > 0 {
					_, _ = s.st.Insert("progress_stages", map[string]any{
						"progress_id": progressID, "stage_id": sid,
					}, []string{"progress_id", "stage_id"})
				}
			}
		}
	}
	if raw, okv := body["attachment_ids"]; okv {
		if arr, isArr := raw.([]any); isArr {
			_, _ = s.st.Exec(`DELETE FROM progress_attachments WHERE progress_id=?`, progressID)
			for _, it := range arr {
				var aid int64
				switch t := it.(type) {
				case float64:
					aid = int64(t)
				case string:
					aid, _ = strconv.ParseInt(t, 10, 64)
				}
				if aid > 0 {
					_, _ = s.st.Insert("progress_attachments", map[string]any{
						"progress_id": progressID, "attachment_id": aid,
					}, []string{"progress_id", "attachment_id"})
				}
			}
		}
	}
}

func (s *Server) hProgressUpdate(w http.ResponseWriter, r *http.Request) {
	id, okid := reqID(w, r, "id")
	if !okid {
		return
	}
	var body map[string]any
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	body["updated_at"] = store.Now()
	if err := s.st.Update("progress", id, body, append(progressFields, "updated_at")); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.saveProgressLinks(id, body)
	ok(w, true)
}

func (s *Server) hProgressDelete(w http.ResponseWriter, r *http.Request) {
	id, okid := reqID(w, r, "id")
	if !okid {
		return
	}
	if err := s.st.Delete("progress", id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ok(w, true)
}

// hProgressAll 全局进度记录（支持项目/阶段/日期范围筛选）
func (s *Server) hProgressAll(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	sqlStr := `SELECT g.*, p.name AS project_name, p.code AS project_code, c.name AS customer_name
		FROM progress g JOIN projects p ON p.id=g.project_id JOIN customers c ON c.id=p.customer_id WHERE 1=1`
	args := []any{}
	if v := q.Get("project_id"); v != "" && v != "0" {
		sqlStr += ` AND g.project_id=?`
		args = append(args, v)
	}
	if v := q.Get("from"); v != "" {
		sqlStr += ` AND g.record_date>=?`
		args = append(args, v)
	}
	if v := q.Get("to"); v != "" {
		sqlStr += ` AND g.record_date<=?`
		args = append(args, v)
	}
	if v := q.Get("stage_id"); v != "" && v != "0" {
		sqlStr += ` AND EXISTS(SELECT 1 FROM progress_stages ps WHERE ps.progress_id=g.id AND ps.stage_id=?)`
		args = append(args, v)
	}
	if v := q.Get("week"); v != "" {
		start, end := weekRange(v)
		sqlStr += ` AND g.record_date>=? AND g.record_date<=?`
		args = append(args, start, end)
	}
	sqlStr += ` ORDER BY g.record_date DESC, g.id DESC LIMIT 500`
	rows, err := s.st.QueryMaps(sqlStr, args...)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.hydrateProgress(rows)
	ok(w, rows)
}

// weekRange 支持 yyyy-MM-dd（返回该日期所在周一~周日）或 yyyy-Www
func weekRange(v string) (string, string) {
	t, err := time.ParseInLocation("2006-01-02", v, time.Local)
	if err != nil {
		now := time.Now()
		t = now
	}
	wd := int(t.Weekday())
	if wd == 0 {
		wd = 7
	}
	monday := t.AddDate(0, 0, -(wd - 1))
	sunday := monday.AddDate(0, 0, 6)
	return monday.Format("2006-01-02"), sunday.Format("2006-01-02")
}

// ---------- 风险问题 ----------

func (s *Server) hIssueList(w http.ResponseWriter, r *http.Request) {
	id, okid := reqID(w, r, "id")
	if !okid {
		return
	}
	rows, err := s.st.QueryMaps(`SELECT * FROM issues WHERE project_id=? ORDER BY (status='未关闭') DESC, id DESC`, id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ok(w, rows)
}

var issueFields = []string{"title", "description", "level", "owner", "status", "closed_at"}

func (s *Server) hIssueCreate(w http.ResponseWriter, r *http.Request) {
	pid, okid := reqID(w, r, "id")
	if !okid {
		return
	}
	var body map[string]any
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	if strings.TrimSpace(store.Pick(body, "title")) == "" {
		writeErr(w, 400, "问题标题必填")
		return
	}
	if store.Pick(body, "status") == "" {
		body["status"] = "未关闭"
	}
	if store.Pick(body, "level") == "" {
		body["level"] = "中"
	}
	now := store.Now()
	body["project_id"] = pid
	body["created_at"], body["updated_at"] = now, now
	nid, err := s.st.Insert("issues", body, append(issueFields, "project_id", "created_at", "updated_at"))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ok(w, map[string]any{"id": nid})
}

func (s *Server) hIssueUpdate(w http.ResponseWriter, r *http.Request) {
	id, okid := reqID(w, r, "id")
	if !okid {
		return
	}
	var body map[string]any
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	if store.Pick(body, "status") == "已关闭" && strings.TrimSpace(store.Pick(body, "closed_at")) == "" {
		body["closed_at"] = store.Now()
	}
	if store.Pick(body, "status") == "未关闭" {
		body["closed_at"] = nil
	}
	body["updated_at"] = store.Now()
	if err := s.st.Update("issues", id, body, append(issueFields, "updated_at")); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ok(w, true)
}

func (s *Server) hIssueDelete(w http.ResponseWriter, r *http.Request) {
	id, okid := reqID(w, r, "id")
	if !okid {
		return
	}
	if err := s.st.Delete("issues", id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ok(w, true)
}

func (s *Server) hIssueAll(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	sqlStr := `SELECT i.*, p.name AS project_name, p.code AS project_code, p.ptype AS project_type,
		c.name AS customer_name FROM issues i JOIN projects p ON p.id=i.project_id
		JOIN customers c ON c.id=p.customer_id WHERE 1=1`
	args := []any{}
	if v := q.Get("status"); v != "" && v != "全部" {
		sqlStr += ` AND i.status=?`
		args = append(args, v)
	}
	if v := q.Get("level"); v != "" && v != "全部" {
		sqlStr += ` AND i.level=?`
		args = append(args, v)
	}
	if v := q.Get("project_id"); v != "" && v != "0" {
		sqlStr += ` AND i.project_id=?`
		args = append(args, v)
	}
	if v := q.Get("q"); v != "" {
		sqlStr += ` AND (i.title LIKE ? OR i.description LIKE ? OR i.owner LIKE ?)`
		args = append(args, "%"+v+"%", "%"+v+"%", "%"+v+"%")
	}
	sqlStr += ` ORDER BY (i.status='未关闭') DESC, CASE i.level WHEN '高' THEN 0 WHEN '中' THEN 1 ELSE 2 END, i.id DESC LIMIT 500`
	rows, err := s.st.QueryMaps(sqlStr, args...)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ok(w, rows)
}

// ---------- 项目转化 / 继承 ----------

// hProjectConvert 支持：加点关联原项目、测试转实施、验收转维保
func (s *Server) hProjectConvert(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SourceProjectID int64          `json:"source_project_id"`
		Mode            string         `json:"mode"`
		Code            string         `json:"code"`
		Name            string         `json:"name"`
		Status          string         `json:"status"`
		Priority        string         `json:"priority"`
		StartDate       string         `json:"start_date"`
		CycleMonths     int            `json:"cycle_months"`
		ExpireDate      string         `json:"expire_date"`
		CopyDevices     bool           `json:"copy_devices"`
		CopyAttachments bool           `json:"copy_attachments"`
		CopyProgress    bool           `json:"copy_progress"`
		AcceptDate      string         `json:"accept_date"`
		Acceptor        string         `json:"acceptor"`
		LegacyIssues    string         `json:"legacy_issues"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	src, err := s.st.QueryRowMap(`SELECT * FROM projects WHERE id=?`, body.SourceProjectID)
	if err != nil {
		writeErr(w, 404, "来源项目不存在")
		return
	}
	var ptype, relType string
	switch body.Mode {
	case "addon":
		ptype, relType = "加点", "addon_source"
	case "test2impl":
		ptype, relType = "新实施", "test2impl"
	case "accept2maint":
		ptype, relType = "维保", "accept2maint"
	default:
		writeErr(w, 400, "转化类型无效")
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		suffix := map[string]string{"addon": "-加点", "test2impl": "-实施", "accept2maint": "-维保"}[body.Mode]
		name = store.Pick(src, "name") + suffix
	}
	now := store.Now()
	status := strings.TrimSpace(body.Status)
	if status == "" {
		status = "未开始"
	}
	priority := strings.TrimSpace(body.Priority)
	if priority == "" {
		priority = store.Pick(src, "priority")
	}
	newProj := map[string]any{
		"code": body.Code, "name": name, "customer_id": store.PickInt(src, "customer_id"),
		"ptype": ptype, "status": status, "priority": priority,
		"start_date": nullIfEmpty(body.StartDate),
		"accept_date": nullIfEmpty(body.AcceptDate),
		"acceptor": body.Acceptor, "legacy_issues": body.LegacyIssues,
		"remark": "由项目「" + store.Pick(src, "name") + "」转化",
		"created_at": now, "updated_at": now,
	}
	if newProj["accept_date"] == nil && body.Mode == "accept2maint" {
		newProj["accept_date"] = nullIfEmpty(store.Pick(src, "accept_date"))
	}
	if strings.TrimSpace(body.Acceptor) == "" && body.Mode == "accept2maint" {
		newProj["acceptor"] = store.Pick(src, "acceptor")
	}
	if strings.TrimSpace(body.LegacyIssues) == "" && body.Mode == "accept2maint" {
		newProj["legacy_issues"] = store.Pick(src, "legacy_issues")
	}
	nid, err := s.st.Insert("projects", newProj, append(append([]string{}, projectFields...), "created_at", "updated_at"))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	// 阶段模板
	for i, nm := range stageTemplates[ptype] {
		_, _ = s.st.Insert("project_stages", map[string]any{
			"project_id": nid, "name": nm, "seq": i + 1, "status": "未开始",
		}, []string{"project_id", "name", "seq", "status"})
	}
	// 产品线复制
	if rows, err := s.st.QueryMaps(`SELECT product_line, product_name FROM project_products WHERE project_id=?`, body.SourceProjectID); err == nil {
		for _, rw := range rows {
			_, _ = s.st.Insert("project_products", map[string]any{
				"project_id": nid, "product_line": store.Pick(rw, "product_line"),
				"product_name": store.Pick(rw, "product_name"),
			}, []string{"project_id", "product_line", "product_name"})
		}
	}
	// 关联
	_, _ = s.st.Insert("project_relations", map[string]any{
		"project_id": nid, "related_project_id": body.SourceProjectID,
		"rel_type": relType, "created_at": now,
	}, []string{"project_id", "related_project_id", "rel_type", "created_at"})
	// 网络信息引用（沿用来源项目引用的版本，无则用客户当前版本）
	var netVer int64
	if m, err := s.st.QueryRowMap(`SELECT net_version_id FROM project_net_refs WHERE project_id=?`, body.SourceProjectID); err == nil {
		netVer = int64(store.PickInt(m, "net_version_id"))
	}
	if netVer == 0 {
		if m, err := s.st.QueryRowMap(`SELECT id FROM net_versions WHERE customer_id=? ORDER BY version_no DESC LIMIT 1`,
			store.PickInt(src, "customer_id")); err == nil {
			netVer = int64(store.PickInt(m, "id"))
		}
	}
	if netVer > 0 {
		_, _ = s.st.Insert("project_net_refs", map[string]any{
			"project_id": nid, "net_version_id": netVer, "ref_at": now,
		}, []string{"project_id", "net_version_id", "ref_at"})
	}
	// 设备快照复制（默认复制）
	if body.CopyDevices || body.Mode != "accept2maint" {
		_, _ = s.copySnapshots(body.SourceProjectID, nid)
	}
	// 附件复制（物理复制文件，形成独立版本）
	if body.CopyAttachments || body.Mode != "accept2maint" {
		s.copyAttachments(body.SourceProjectID, nid, now)
	}
	// 进度记录复制
	if body.CopyProgress || body.Mode == "test2impl" {
		s.copyProgress(body.SourceProjectID, nid, now)
	}
	// 维保信息
	if ptype == "维保" {
		cycle := body.CycleMonths
		if cycle <= 0 {
			cycle = 12
		}
		start := strings.TrimSpace(body.StartDate)
		if start == "" {
			start = store.Today()
		}
		expire := strings.TrimSpace(body.ExpireDate)
		if expire == "" {
			if sd, err := parseDate(start); err == nil {
				expire = sd.AddDate(0, cycle, 0).Format("2006-01-02")
			}
		}
		_, _ = s.st.Insert("maintenance", map[string]any{
			"project_id": nid, "start_date": start, "expire_date": expire,
			"cycle_months": cycle, "auto_calc": 1, "status": "在保",
			"manual_override": 0, "updated_at": now,
		}, []string{"project_id", "start_date", "expire_date", "cycle_months", "auto_calc", "status", "manual_override", "updated_at"})
		s.refreshMaintStatus(nid)
	}
	// 测试转实施：原测试项目结束
	if body.Mode == "test2impl" {
		_, _ = s.st.Exec(`UPDATE projects SET status='已结束',updated_at=? WHERE id=?`, now, body.SourceProjectID)
	}
	ok(w, map[string]any{"id": nid, "name": name, "ptype": ptype})
}

func nullIfEmpty(v string) any {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return v
}

func (s *Server) copyAttachments(from, to int64, now string) {
	rows, err := s.st.QueryMaps(`SELECT * FROM attachments WHERE project_id=?`, from)
	if err != nil {
		return
	}
	for _, a := range rows {
		newAttID, err := s.st.Insert("attachments", map[string]any{
			"project_id": to, "atype": store.Pick(a, "atype"), "file_name": store.Pick(a, "file_name"),
			"file_size": store.PickInt(a, "file_size"), "version_no": 1,
			"source_attachment_id": store.PickInt(a, "id"), "created_at": now, "updated_at": now,
		}, []string{"project_id", "atype", "file_name", "file_size", "version_no", "source_attachment_id", "created_at", "updated_at"})
		if err != nil {
			continue
		}
		cur := store.PickInt(a, "current_version_id")
		if cur == 0 {
			continue
		}
		v, err := s.st.QueryRowMap(`SELECT * FROM attachment_versions WHERE id=?`, cur)
		if err != nil {
			continue
		}
		src := filepath.Join(s.st.Files, filepath.FromSlash(store.Pick(v, "file_path")))
		if !exists(src) {
			continue
		}
		dstName := filepath.Base(store.Pick(v, "file_path"))
		dst := filepath.Join(s.st.Files, "att", strconv.FormatInt(newAttID, 10)+"_"+dstName)
		_ = os.MkdirAll(filepath.Dir(dst), 0o755)
		if err := copyFile(src, dst); err != nil {
			continue
		}
		vid, err := s.st.Insert("attachment_versions", map[string]any{
			"attachment_id": newAttID, "version_no": 1,
			"file_path": filepath.ToSlash(filepath.Join("att", filepath.Base(dst))),
			"file_name": store.Pick(v, "file_name"), "file_size": store.PickInt(v, "file_size"),
			"note": "继承自项目附件", "created_at": now,
		}, []string{"attachment_id", "version_no", "file_path", "file_name", "file_size", "note", "created_at"})
		if err == nil {
			_, _ = s.st.Exec(`UPDATE attachments SET current_version_id=? WHERE id=?`, vid, newAttID)
		}
	}
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = out.ReadFrom(in)
	return err
}

func (s *Server) copyProgress(from, to int64, now string) {
	rows, err := s.st.QueryMaps(`SELECT * FROM progress WHERE project_id=? ORDER BY record_date`, from)
	if err != nil {
		return
	}
	for _, p := range rows {
		nid, err := s.st.Insert("progress", map[string]any{
			"project_id": to, "record_date": store.Pick(p, "record_date"),
			"current_progress": store.Pick(p, "current_progress"),
			"problems":         store.Pick(p, "problems"),
			"next_plan":        store.Pick(p, "next_plan"),
			"info_sync":        store.Pick(p, "info_sync"),
			"created_at":       now, "updated_at": now,
		}, []string{"project_id", "record_date", "current_progress", "problems", "next_plan", "info_sync", "created_at", "updated_at"})
		if err != nil {
			continue
		}
		_, _ = s.st.Exec(`INSERT INTO progress_stages(progress_id,stage_id)
			SELECT ?, ps.stage_id FROM progress_stages ps WHERE ps.progress_id=?`, nid, store.PickInt(p, "id"))
	}
}
