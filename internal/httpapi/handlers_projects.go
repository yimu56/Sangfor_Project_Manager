package httpapi

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"sangforpm/internal/store"
)

var stageTemplates = map[string][]string{
	"新实施": {"需求沟通", "上架", "配置", "验收"},
	"加点":  {"需求确认", "加点实施", "配置", "验收"},
	"测试":  {"需求沟通", "测试实施", "测试验证", "测试结束"},
	"维保":  {"维保交接", "维保服务", "维保结束"},
}

var projectTypes = []string{"维保", "加点", "新实施", "测试"}

var statusByType = map[string][]string{
	"维保": {"未开始", "维保中", "已结束", "暂停", "取消"},
	"新实施": {"未开始", "进行中", "待验收", "已验收", "已结束", "暂停", "取消"},
	"加点":  {"未开始", "进行中", "待验收", "已验收", "已结束", "暂停", "取消"},
	"测试":  {"未开始", "进行中", "待验收", "已验收", "已结束", "暂停", "取消"},
}

var mtStatuses = []string{"在保", "即将过保", "过保"}

func allStatuses() []string {
	return []string{"未开始", "进行中", "待验收", "已验收", "已结束", "暂停", "取消", "维保中", "在保", "即将过保", "过保"}
}

// statusOptions 返回项目状态选项（维保项目状态 + 维保到期状态合并展示用）
func statusOptions(ptype string) []string {
	base := statusByType[ptype]
	if ptype == "维保" {
		out := []string{"未开始", "维保中"}
		out = append(out, mtStatuses...)
		out = append(out, "已结束", "暂停", "取消")
		return out
	}
	return base
}

// ---------- 列表 / 详情 ----------

func (s *Server) hProjectList(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	custID := r.URL.Query().Get("customer_id")
	ptype := r.URL.Query().Get("ptype")
	status := r.URL.Query().Get("status")
	priority := r.URL.Query().Get("priority")
	pline := r.URL.Query().Get("product_line")
	issueOpen := r.URL.Query().Get("issue_open")
	stale := r.URL.Query().Get("stale_days")

	sqlStr := `SELECT p.*, c.name AS customer_name,
		(SELECT group_concat(DISTINCT product_line) FROM project_products x WHERE x.project_id=p.id) AS product_lines,
		(SELECT group_concat(product_name, '、') FROM project_products x WHERE x.project_id=p.id AND product_name<>'') AS product_names,
		(SELECT COUNT(*) FROM issues x WHERE x.project_id=p.id AND x.status='未关闭') AS open_issues,
		(SELECT MAX(record_date) FROM progress x WHERE x.project_id=p.id) AS last_progress_date,
		(SELECT next_plan FROM progress x WHERE x.project_id=p.id ORDER BY record_date DESC, id DESC LIMIT 1) AS next_plan,
		(SELECT expire_date FROM maintenance m WHERE m.project_id=p.id) AS expire_date,
		(SELECT status FROM maintenance m WHERE m.project_id=p.id) AS mt_status,
		(SELECT manual_override FROM maintenance m WHERE m.project_id=p.id) AS mt_manual
		FROM projects p JOIN customers c ON c.id=p.customer_id WHERE 1=1`
	args := []any{}
	if q != "" {
		sqlStr += ` AND (p.name LIKE ? OR p.code LIKE ? OR c.name LIKE ? OR p.remark LIKE ?)`
		args = append(args, "%"+q+"%", "%"+q+"%", "%"+q+"%", "%"+q+"%")
	}
	if custID != "" && custID != "0" {
		sqlStr += ` AND p.customer_id=?`
		args = append(args, custID)
	}
	if ptype != "" && ptype != "全部" {
		sqlStr += ` AND p.ptype=?`
		args = append(args, ptype)
	}
	if status != "" && status != "全部" {
		sqlStr += ` AND p.status=?`
		args = append(args, status)
	}
	if priority != "" && priority != "全部" {
		sqlStr += ` AND p.priority=?`
		args = append(args, priority)
	}
	if pline != "" && pline != "全部" {
		sqlStr += ` AND EXISTS(SELECT 1 FROM project_products x WHERE x.project_id=p.id AND x.product_line=?)`
		args = append(args, pline)
	}
	if issueOpen == "1" {
		sqlStr += ` AND EXISTS(SELECT 1 FROM issues x WHERE x.project_id=p.id AND x.status='未关闭')`
	}
	sqlStr += ` ORDER BY p.updated_at DESC`

	list, err := s.st.QueryMaps(sqlStr, args...)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	// 维保状态实时刷新
	for _, m := range list {
		if store.Pick(m, "ptype") == "维保" {
			s.refreshMaintStatus(int64(store.PickInt(m, "id")))
		}
	}
	list, _ = s.st.QueryMaps(sqlStr, args...)
	for _, m := range list {
		nxt := strings.TrimSpace(store.Pick(m, "next_plan"))
		if nxt == "" {
			nxt = "未定"
		}
		m["next_plan_show"] = nxt
		m["stale_days"] = staleDays(m)
	}
	if stale != "" {
		days, _ := strconv.Atoi(stale)
		filtered := make([]map[string]any, 0, len(list))
		for _, m := range list {
			if staleDays(m) > days {
				filtered = append(filtered, m)
			}
		}
		list = filtered
	}
	ok(w, list)
}

// staleDays 计算距上次进度记录（无记录则按创建时间）的天数
func staleDays(p map[string]any) int {
	base := strings.TrimSpace(store.Pick(p, "last_progress_date"))
	if base == "" {
		base = strings.TrimSpace(store.Pick(p, "created_at"))
	}
	t, err := parseDate(base)
	if err != nil {
		return 0
	}
	return int(time.Since(t).Hours() / 24)
}

func (s *Server) hProjectGet(w http.ResponseWriter, r *http.Request) {
	id, okid := reqID(w, r, "id")
	if !okid {
		return
	}
	s.refreshMaintStatus(id)
	p, err := s.st.QueryRowMap(`SELECT p.*, c.name AS customer_name FROM projects p
		JOIN customers c ON c.id=p.customer_id WHERE p.id=?`, id)
	if err != nil {
		writeErr(w, 404, "项目不存在")
		return
	}
	products, _ := s.st.QueryMaps(`SELECT * FROM project_products WHERE project_id=? ORDER BY id`, id)
	stages, _ := s.st.QueryMaps(`SELECT * FROM project_stages WHERE project_id=? ORDER BY seq,id`, id)
	related, _ := s.st.QueryMaps(`SELECT r.*, p.name AS related_name, p.code AS related_code, p.ptype AS related_type
		FROM project_relations r LEFT JOIN projects p ON p.id=r.related_project_id WHERE r.project_id=?`, id)
	netRef, _ := s.st.QueryRowMap(`SELECT r.*, v.version_no, v.content, v.created_at AS version_created
		FROM project_net_refs r JOIN net_versions v ON v.id=r.net_version_id WHERE r.project_id=?`, id)
	snaps, _ := s.st.QueryMaps(`SELECT * FROM project_devices WHERE project_id=? ORDER BY id`, id)
	atts, _ := s.st.QueryMaps(`SELECT * FROM attachments WHERE project_id=? ORDER BY id`, id)
	prog, _ := s.st.QueryMaps(`SELECT * FROM progress WHERE project_id=? ORDER BY record_date DESC, id DESC LIMIT 5`, id)
	issues, _ := s.st.QueryMaps(`SELECT * FROM issues WHERE project_id=? ORDER BY status, id DESC`, id)
	mt, _ := s.st.QueryRowMap(`SELECT * FROM maintenance WHERE project_id=?`, id)

	p["products"] = products
	p["stages"] = stages
	p["relations"] = related
	p["net_ref"] = netRef
	p["snapshots"] = snaps
	p["attachments"] = atts
	p["progress_recent"] = prog
	p["issues"] = issues
	p["maintenance"] = mt
	p["status_options"] = statusOptions(store.Pick(p, "ptype"))
	p["stale_days"] = staleDays(p)
	ok(w, p)
}

// ---------- 创建 / 更新 / 删除 ----------

var projectFields = []string{
	"code", "name", "customer_id", "ptype", "status", "priority",
	"start_date", "expect_accept_date", "accept_date", "acceptor",
	"legacy_issues", "remark",
}

func (s *Server) hProjectCreate(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	name := strings.TrimSpace(store.Pick(body, "name"))
	if name == "" {
		writeErr(w, 400, "项目名称必填")
		return
	}
	ptype := store.Pick(body, "ptype")
	if _, okp := stageTemplates[ptype]; !okp {
		writeErr(w, 400, "项目类型无效")
		return
	}
	if store.PickInt(body, "customer_id") == 0 {
		writeErr(w, 400, "请选择客户")
		return
	}
	if ptype == "加点" && store.PickInt(body, "related_project_id") == 0 {
		writeErr(w, 400, "加点项目必须关联原项目")
		return
	}
	if strings.TrimSpace(store.Pick(body, "status")) == "" {
		body["status"] = "未开始"
	}
	if strings.TrimSpace(store.Pick(body, "priority")) == "" {
		body["priority"] = "中"
	}
	now := store.Now()
	body["created_at"], body["updated_at"] = now, now
	id, err := s.st.Insert("projects", body, append(projectFields, "created_at", "updated_at"))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.applyProjectExtras(r, id, body, now, true)
	ok(w, map[string]any{"id": id})
}

// applyProjectExtras 处理产品行、阶段模板、维保信息、关联
func (s *Server) applyProjectExtras(r *http.Request, id int64, body map[string]any, now string, create bool) {
	if raw, okv := body["products"]; okv {
		if arr, isArr := raw.([]any); isArr {
			_, _ = s.st.Exec(`DELETE FROM project_products WHERE project_id=?`, id)
			for _, it := range arr {
				m, _ := it.(map[string]any)
				line := strings.TrimSpace(store.Pick(m, "product_line"))
				pname := strings.TrimSpace(store.Pick(m, "product_name"))
				if line == "" && pname == "" {
					continue
				}
				_, _ = s.st.Insert("project_products", map[string]any{
					"project_id": id, "product_line": line, "product_name": pname,
				}, []string{"project_id", "product_line", "product_name"})
			}
		}
	}
	if create {
		ptype := store.Pick(body, "ptype")
		tpl := stageTemplates[ptype]
		for i, nm := range tpl {
			_, _ = s.st.Insert("project_stages", map[string]any{
				"project_id": id, "name": nm, "seq": i + 1, "status": "未开始",
			}, []string{"project_id", "name", "seq", "status"})
		}
		// 新建项目默认引用客户当前网络信息版本
		cid := int64(store.PickInt(body, "customer_id"))
		if v, err := s.st.QueryRowMap(`SELECT id FROM net_versions WHERE customer_id=? ORDER BY version_no DESC LIMIT 1`, cid); err == nil {
			_, _ = s.st.Insert("project_net_refs", map[string]any{
				"project_id": id, "net_version_id": store.PickInt(v, "id"), "ref_at": now,
			}, []string{"project_id", "net_version_id", "ref_at"})
		}
		if ptype == "维保" {
			_, _ = s.st.Insert("maintenance", map[string]any{
				"project_id": id, "status": "在保", "cycle_months": 12,
				"auto_calc": 1, "manual_override": 0, "updated_at": now,
			}, []string{"project_id", "status", "cycle_months", "auto_calc", "manual_override", "updated_at"})
		}
	}
	if raw, okv := body["related_project_id"]; okv {
		rid := 0
		fmt.Sscanf(store.Pick(map[string]any{"v": raw}, "v"), "%d", &rid)
		if rid > 0 {
			rel := strings.TrimSpace(store.Pick(body, "rel_type"))
			if rel == "" {
				rel = defaultRelType(store.Pick(body, "ptype"))
			}
			_, _ = s.st.Insert("project_relations", map[string]any{
				"project_id": id, "related_project_id": rid, "rel_type": rel, "created_at": now,
			}, []string{"project_id", "related_project_id", "rel_type", "created_at"})
		}
	}
}

func defaultRelType(ptype string) string {
	switch ptype {
	case "加点":
		return "addon_source"
	case "维保":
		return "accept2maint"
	default:
		return "test2impl"
	}
}

func (s *Server) hProjectUpdate(w http.ResponseWriter, r *http.Request) {
	id, okid := reqID(w, r, "id")
	if !okid {
		return
	}
	var body map[string]any
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	now := store.Now()
	body["updated_at"] = now
	if err := s.st.Update("projects", id, body, append(projectFields, "updated_at")); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.applyProjectExtras(r, id, body, now, false)
	s.refreshMaintStatus(id)
	ok(w, true)
}

func (s *Server) hProjectDelete(w http.ResponseWriter, r *http.Request) {
	id, okid := reqID(w, r, "id")
	if !okid {
		return
	}
	// 先清理该项目的附件物理文件
	vers, _ := s.st.QueryMaps(`SELECT v.file_path FROM attachment_versions v
		JOIN attachments a ON a.id=v.attachment_id WHERE a.project_id=?`, id)
	for _, v := range vers {
		_ = os.Remove(filepath.Join(s.st.Files, filepath.FromSlash(store.Pick(v, "file_path"))))
	}
	if err := s.st.Delete("projects", id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ok(w, true)
}

func (s *Server) hProjectRelated(w http.ResponseWriter, r *http.Request) {
	id, okid := reqID(w, r, "id")
	if !okid {
		return
	}
	list, err := s.st.QueryMaps(`SELECT r.*, p.name AS related_name, p.code AS related_code, p.ptype AS related_type, p.status AS related_status
		FROM project_relations r LEFT JOIN projects p ON p.id=r.related_project_id WHERE r.project_id=?`, id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	rev, _ := s.st.QueryMaps(`SELECT r.*, p.name AS related_name, p.code AS related_code, p.ptype AS related_type, p.status AS related_status
		FROM project_relations r LEFT JOIN projects p ON p.id=r.project_id WHERE r.related_project_id=?`, id)
	ok(w, map[string]any{"out": list, "in": rev})
}

func (s *Server) hProjectRelationAdd(w http.ResponseWriter, r *http.Request) {
	id, okid := reqID(w, r, "id")
	if !okid {
		return
	}
	var body struct {
		RelatedID int64  `json:"related_project_id"`
		RelType   string `json:"rel_type"`
	}
	if err := readJSON(r, &body); err != nil || body.RelatedID == 0 {
		writeErr(w, 400, "请选择关联项目")
		return
	}
	_, err := s.st.Insert("project_relations", map[string]any{
		"project_id": id, "related_project_id": body.RelatedID,
		"rel_type": body.RelType, "created_at": store.Now(),
	}, []string{"project_id", "related_project_id", "rel_type", "created_at"})
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ok(w, true)
}

// ---------- 阶段 ----------

func (s *Server) hStageList(w http.ResponseWriter, r *http.Request) {
	id, okid := reqID(w, r, "id")
	if !okid {
		return
	}
	list, err := s.st.QueryMaps(`SELECT s.*,
		(SELECT COUNT(*) FROM progress_stages ps WHERE ps.stage_id=s.id) AS record_count
		FROM project_stages s WHERE s.project_id=? ORDER BY s.seq, s.id`, id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ok(w, list)
}

func (s *Server) hStageInit(w http.ResponseWriter, r *http.Request) {
	id, okid := reqID(w, r, "id")
	if !okid {
		return
	}
	m, err := s.st.QueryRowMap(`SELECT ptype FROM projects WHERE id=?`, id)
	if err != nil {
		writeErr(w, 404, "项目不存在")
		return
	}
	tpl := stageTemplates[store.Pick(m, "ptype")]
	// 只补充缺失阶段，不重复创建
	exist := map[string]bool{}
	if rows, err := s.st.QueryMaps(`SELECT name FROM project_stages WHERE project_id=?`, id); err == nil {
		for _, rw := range rows {
			exist[store.Pick(rw, "name")] = true
		}
	}
	seq := len(exist)
	for _, nm := range tpl {
		if exist[nm] {
			continue
		}
		seq++
		_, _ = s.st.Insert("project_stages", map[string]any{
			"project_id": id, "name": nm, "seq": seq, "status": "未开始",
		}, []string{"project_id", "name", "seq", "status"})
	}
	ok(w, true)
}

func (s *Server) hStageCreate(w http.ResponseWriter, r *http.Request) {
	id, okid := reqID(w, r, "id")
	if !okid {
		return
	}
	var body map[string]any
	if err := readJSON(r, &body); err != nil || strings.TrimSpace(store.Pick(body, "name")) == "" {
		writeErr(w, 400, "阶段名称必填")
		return
	}
	body["project_id"] = id
	if _, okv := body["seq"]; !okv {
		m, _ := s.st.QueryRowMap(`SELECT COALESCE(MAX(seq),0)+1 AS n FROM project_stages WHERE project_id=?`, id)
		body["seq"] = store.PickInt(m, "n")
	}
	nid, err := s.st.Insert("project_stages", body, []string{"project_id", "name", "seq", "status", "start_at", "done_at"})
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ok(w, map[string]any{"id": nid})
}

func (s *Server) hStageUpdate(w http.ResponseWriter, r *http.Request) {
	id, okid := reqID(w, r, "id")
	if !okid {
		return
	}
	var body map[string]any
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	if st := store.Pick(body, "status"); st == "已完成" {
		if strings.TrimSpace(store.Pick(body, "done_at")) == "" {
			body["done_at"] = store.Today()
		}
	}
	if err := s.st.Update("project_stages", id, body, []string{"name", "seq", "status", "start_at", "done_at"}); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	_, _ = s.st.Exec(`UPDATE projects SET updated_at=? WHERE id=(SELECT project_id FROM project_stages WHERE id=?)`, store.Now(), id)
	ok(w, true)
}

func (s *Server) hStageDelete(w http.ResponseWriter, r *http.Request) {
	id, okid := reqID(w, r, "id")
	if !okid {
		return
	}
	_, _ = s.st.Exec(`DELETE FROM progress_stages WHERE stage_id=?`, id)
	if err := s.st.Delete("project_stages", id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ok(w, true)
}

// ---------- 项目设备快照 ----------

func (s *Server) hSnapshotList(w http.ResponseWriter, r *http.Request) {
	id, okid := reqID(w, r, "id")
	if !okid {
		return
	}
	list, err := s.st.QueryMaps(`SELECT d.*, c.name AS contact_name FROM project_devices d
		LEFT JOIN contacts c ON c.id=d.contact_id WHERE d.project_id=? ORDER BY d.id`, id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ok(w, list)
}

var snapshotFields = []string{"contact_id", "model", "serial_no", "account", "topology", "network_info", "remark"}

func (s *Server) hSnapshotCreate(w http.ResponseWriter, r *http.Request) {
	id, okid := reqID(w, r, "id")
	if !okid {
		return
	}
	var body map[string]any
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	now := store.Now()
	// 从客户设备档案复制
	if src := store.PickInt(body, "source_device_id"); src > 0 {
		if d, err := s.st.QueryRowMap(`SELECT * FROM devices WHERE id=?`, src); err == nil {
			for _, f := range snapshotFields {
				body[f] = store.Pick(d, f)
			}
			body["source_device_id"] = src
		}
	}
	body["project_id"] = id
	body["snapshot_version"] = 1
	body["created_at"], body["updated_at"] = now, now
	nid, err := s.st.Insert("project_devices", body, append(snapshotFields,
		"project_id", "source_device_id", "snapshot_version", "created_at", "updated_at"))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	_, _ = s.st.Insert("project_device_changes", map[string]any{
		"snapshot_id": nid, "field_name": "*", "old_value": "", "new_value": "新增设备快照",
		"action": "create", "changed_at": now,
	}, []string{"snapshot_id", "field_name", "old_value", "new_value", "action", "changed_at"})
	_, _ = s.st.Exec(`UPDATE projects SET updated_at=? WHERE id=?`, now, id)
	ok(w, map[string]any{"id": nid})
}

func (s *Server) hSnapshotUpdate(w http.ResponseWriter, r *http.Request) {
	id, okid := reqID(w, r, "id")
	if !okid {
		return
	}
	var body map[string]any
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	old, err := s.st.QueryRowMap(`SELECT * FROM project_devices WHERE id=?`, id)
	if err != nil {
		writeErr(w, 404, "设备快照不存在")
		return
	}
	now := store.Now()
	body["updated_at"] = now
	body["snapshot_version"] = store.PickInt(old, "snapshot_version") + 1
	if err := s.st.Update("project_devices", id, body, append(snapshotFields, "snapshot_version", "updated_at")); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	logDiff(s.st, "project_device_changes", "snapshot_id", id, old, body, snapshotFields, now)
	_, _ = s.st.Exec(`UPDATE projects SET updated_at=? WHERE id=?`, now, store.PickInt(old, "project_id"))
	ok(w, true)
}

func (s *Server) hSnapshotDelete(w http.ResponseWriter, r *http.Request) {
	id, okid := reqID(w, r, "id")
	if !okid {
		return
	}
	if err := s.st.Delete("project_devices", id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ok(w, true)
}

func (s *Server) hSnapshotHistory(w http.ResponseWriter, r *http.Request) {
	id, okid := reqID(w, r, "id")
	if !okid {
		return
	}
	list, err := s.st.QueryMaps(`SELECT * FROM project_device_changes WHERE snapshot_id=? ORDER BY id DESC`, id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ok(w, list)
}

func (s *Server) hSnapshotInherit(w http.ResponseWriter, r *http.Request) {
	id, okid := reqID(w, r, "id")
	if !okid {
		return
	}
	var body struct {
		FromProjectID int64 `json:"from_project_id"`
	}
	if err := readJSON(r, &body); err != nil || body.FromProjectID == 0 {
		writeErr(w, 400, "请选择来源项目")
		return
	}
	n, err := s.copySnapshots(body.FromProjectID, id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ok(w, map[string]any{"copied": n})
}

func (s *Server) copySnapshots(from, to int64) (int, error) {
	rows, err := s.st.QueryMaps(`SELECT * FROM project_devices WHERE project_id=?`, from)
	if err != nil {
		return 0, err
	}
	now := store.Now()
	for _, d := range rows {
		m := map[string]any{"project_id": to, "snapshot_version": 1, "created_at": now, "updated_at": now}
		for _, f := range snapshotFields {
			m[f] = store.Pick(d, f)
		}
		if v := store.PickInt(d, "source_device_id"); v > 0 {
			m["source_device_id"] = v
		}
		_, _ = s.st.Insert("project_devices", m, append(snapshotFields,
			"project_id", "source_device_id", "snapshot_version", "created_at", "updated_at"))
	}
	return len(rows), nil
}

// ---------- 项目网络信息引用 ----------

func (s *Server) hNetRefGet(w http.ResponseWriter, r *http.Request) {
	id, okid := reqID(w, r, "id")
	if !okid {
		return
	}
	m, err := s.st.QueryRowMap(`SELECT r.*, v.version_no, v.content, v.created_at AS version_created
		FROM project_net_refs r JOIN net_versions v ON v.id=r.net_version_id WHERE r.project_id=?`, id)
	if err != nil {
		ok(w, nil)
		return
	}
	ok(w, m)
}

func (s *Server) hNetRefSet(w http.ResponseWriter, r *http.Request) {
	id, okid := reqID(w, r, "id")
	if !okid {
		return
	}
	var body struct {
		NetVersionID int64 `json:"net_version_id"`
	}
	if err := readJSON(r, &body); err != nil || body.NetVersionID == 0 {
		writeErr(w, 400, "请选择网络信息版本")
		return
	}
	now := store.Now()
	_, _ = s.st.Exec(`INSERT INTO project_net_refs(project_id,net_version_id,ref_at) VALUES(?,?,?)
		ON CONFLICT(project_id) DO UPDATE SET net_version_id=excluded.net_version_id, ref_at=excluded.ref_at`,
		id, body.NetVersionID, now)
	_, _ = s.st.Exec(`UPDATE projects SET updated_at=? WHERE id=?`, now, id)
	ok(w, true)
}

// ---------- 附件 ----------

func (s *Server) hAttachList(w http.ResponseWriter, r *http.Request) {
	id, okid := reqID(w, r, "id")
	if !okid {
		return
	}
	list, err := s.st.QueryMaps(`SELECT a.*, v.file_path, v.file_name AS current_file, v.version_no AS current_version,
		(SELECT COUNT(*) FROM attachment_versions x WHERE x.attachment_id=a.id) AS version_count
		FROM attachments a LEFT JOIN attachment_versions v ON v.id=a.current_version_id
		WHERE a.project_id=? ORDER BY a.atype, a.id DESC`, id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	for _, m := range list {
		if v := store.Pick(m, "current_version_id"); v != "" {
			m["url"] = "/api/files/" + v
		}
	}
	ok(w, list)
}

func (s *Server) hAttachUpload(w http.ResponseWriter, r *http.Request) {
	id, okid := reqID(w, r, "id")
	if !okid {
		return
	}
	s.uploadAttachment(w, r, id, 0)
}

func (s *Server) hAttachUploadVersion(w http.ResponseWriter, r *http.Request) {
	id, okid := reqID(w, r, "id")
	if !okid {
		return
	}
	att, err := s.st.QueryRowMap(`SELECT * FROM attachments WHERE id=?`, id)
	if err != nil {
		writeErr(w, 404, "附件不存在")
		return
	}
	s.uploadAttachment(w, r, int64(store.PickInt(att, "project_id")), id)
}

func (s *Server) uploadAttachment(w http.ResponseWriter, r *http.Request, projectID, attachID int64) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeErr(w, 400, "解析上传失败: "+err.Error())
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		writeErr(w, 400, "缺少文件")
		return
	}
	defer file.Close()
	maxMB := 300
	fmt.Sscanf(s.st.GetSetting("max_attach_mb"), "%d", &maxMB)
	if maxMB <= 0 {
		maxMB = 300
	}
	limit := int64(maxMB) << 20
	if hdr.Size > limit {
		writeErr(w, 400, fmt.Sprintf("单个附件不能超过 %dMB", maxMB))
		return
	}
	now := store.Now()
	atype := strings.TrimSpace(r.FormValue("atype"))
	note := strings.TrimSpace(r.FormValue("note"))
	if atype == "" {
		atype = "其他"
	}
	if attachID == 0 {
		nid, err := s.st.Insert("attachments", map[string]any{
			"project_id": projectID, "atype": atype, "file_name": hdr.Filename,
			"file_size": hdr.Size, "version_no": 1, "created_at": now, "updated_at": now,
		}, []string{"project_id", "atype", "file_name", "file_size", "version_no", "created_at", "updated_at"})
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		attachID = nid
	}
	att, err := s.st.QueryRowMap(`SELECT * FROM attachments WHERE id=?`, attachID)
	if err != nil {
		writeErr(w, 404, "附件不存在")
		return
	}
	verNo := 1
	if m, err := s.st.QueryRowMap(`SELECT COALESCE(MAX(version_no),0)+1 AS n FROM attachment_versions WHERE attachment_id=?`, attachID); err == nil {
		verNo = store.PickInt(m, "n")
	}
	_ = att
	absDir := filepath.Join(s.st.Files, "att")
	_ = os.MkdirAll(absDir, 0o755)
	dst := filepath.Join(absDir, fmt.Sprintf("%d_v%d_%s", attachID, verNo, safeName(hdr.Filename)))
	out, err := os.Create(dst)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	size, cerr := copyWithLimit(out, file, limit)
	out.Close()
	if cerr != nil {
		os.Remove(dst)
		writeErr(w, 500, cerr.Error())
		return
	}
	vid, err := s.st.Insert("attachment_versions", map[string]any{
		"attachment_id": attachID, "version_no": verNo,
		"file_path": filepath.ToSlash(filepath.Join("att", filepath.Base(dst))),
		"file_name": hdr.Filename, "file_size": size, "note": note, "created_at": now,
	}, []string{"attachment_id", "version_no", "file_path", "file_name", "file_size", "note", "created_at"})
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	_, _ = s.st.Exec(`UPDATE attachments SET current_version_id=?,version_no=?,file_name=?,file_size=?,atype=?,updated_at=? WHERE id=?`,
		vid, verNo, hdr.Filename, size, atype, now, attachID)
	_, _ = s.st.Exec(`UPDATE projects SET updated_at=? WHERE id=?`, now, projectID)
	ok(w, map[string]any{"id": attachID, "version_id": vid, "version_no": verNo})
}

func (s *Server) hAttachVersions(w http.ResponseWriter, r *http.Request) {
	id, okid := reqID(w, r, "id")
	if !okid {
		return
	}
	att, err := s.st.QueryRowMap(`SELECT * FROM attachments WHERE id=?`, id)
	if err != nil {
		writeErr(w, 404, "附件不存在")
		return
	}
	list, err := s.st.QueryMaps(`SELECT * FROM attachment_versions WHERE attachment_id=? ORDER BY version_no DESC`, id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	for _, m := range list {
		m["url"] = "/api/files/" + store.Pick(m, "id")
	}
	ok(w, map[string]any{"attachment": att, "versions": list})
}

func (s *Server) hAttachDelete(w http.ResponseWriter, r *http.Request) {
	id, okid := reqID(w, r, "id")
	if !okid {
		return
	}
	vers, _ := s.st.QueryMaps(`SELECT file_path FROM attachment_versions WHERE attachment_id=?`, id)
	for _, v := range vers {
		_ = os.Remove(filepath.Join(s.st.Files, filepath.FromSlash(store.Pick(v, "file_path"))))
	}
	if err := s.st.Delete("attachments", id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ok(w, true)
}

func (s *Server) hFileServe(w http.ResponseWriter, r *http.Request) {
	vid, okid := reqID(w, r, "vid")
	if !okid {
		return
	}
	v, err := s.st.QueryRowMap(`SELECT * FROM attachment_versions WHERE id=?`, vid)
	if err != nil {
		writeErr(w, 404, "文件不存在")
		return
	}
	p := filepath.Join(s.st.Files, filepath.FromSlash(store.Pick(v, "file_path")))
	if !exists(p) {
		writeErr(w, 404, "文件已丢失")
		return
	}
	serveInline(w, r, p)
}

// ---------- 维保 ----------

// refreshMaintStatus 按到期日自动计算维保状态（未手动覆盖时）
func (s *Server) refreshMaintStatus(projectID int64) {
	m, err := s.st.QueryRowMap(`SELECT * FROM maintenance WHERE project_id=?`, projectID)
	if err != nil {
		return
	}
	if store.PickInt(m, "manual_override") == 1 {
		return
	}
	exp := strings.TrimSpace(store.Pick(m, "expire_date"))
	if exp == "" {
		return
	}
	ed, err := parseDate(exp)
	if err != nil {
		return
	}
	warn := 60
	fmt.Sscanf(s.st.GetSetting("warranty_notice"), "%d", &warn)
	days := int(time.Until(ed).Hours() / 24)
	st := "在保"
	if days < 0 {
		st = "过保"
	} else if days <= warn {
		st = "即将过保"
	}
	if st != store.Pick(m, "status") {
		_, _ = s.st.Exec(`UPDATE maintenance SET status=?,updated_at=? WHERE project_id=?`, st, store.Now(), projectID)
	}
}

func (s *Server) hMaintGet(w http.ResponseWriter, r *http.Request) {
	id, okid := reqID(w, r, "id")
	if !okid {
		return
	}
	s.refreshMaintStatus(id)
	m, err := s.st.QueryRowMap(`SELECT * FROM maintenance WHERE project_id=?`, id)
	if err != nil {
		ok(w, nil)
		return
	}
	ok(w, m)
}

var maintFields = []string{
	"start_date", "expire_date", "cycle_months", "auto_calc", "status",
	"auth_attachment_id", "maint_record", "accept_attachment_id", "manual_override",
}

func (s *Server) hMaintUpdate(w http.ResponseWriter, r *http.Request) {
	id, okid := reqID(w, r, "id")
	if !okid {
		return
	}
	var body map[string]any
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	// 自动计算到期日：起始日 + 周期月数
	if store.PickInt(body, "auto_calc") == 1 {
		start := strings.TrimSpace(store.Pick(body, "start_date"))
		months := store.PickInt(body, "cycle_months")
		if start != "" && months > 0 {
			if sd, err := parseDate(start); err == nil {
				body["expire_date"] = sd.AddDate(0, months, 0).Format("2006-01-02")
			}
		}
	}
	body["updated_at"] = store.Now()
	exists := true
	if _, err := s.st.QueryRowMap(`SELECT id FROM maintenance WHERE project_id=?`, id); err != nil {
		exists = false
	}
	if !exists {
		body["project_id"] = id
		if _, err := s.st.Insert("maintenance", body, append(maintFields, "project_id", "updated_at")); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	} else {
		sets := []string{}
		args := []any{}
		for _, f := range maintFields {
			if v, okv := body[f]; okv {
				sets = append(sets, f+"=?")
				args = append(args, v)
			}
		}
		sets = append(sets, "updated_at=?")
		args = append(args, body["updated_at"], id)
		if _, err := s.st.Exec(`UPDATE maintenance SET `+strings.Join(sets, ",")+` WHERE project_id=?`, args...); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	}
	s.refreshMaintStatus(id)
	ok(w, true)
}

func (s *Server) hMaintList(w http.ResponseWriter, r *http.Request) {
	// 先刷新所有维保项目状态
	rows, _ := s.st.QueryMaps(`SELECT project_id FROM maintenance`)
	for _, rw := range rows {
		s.refreshMaintStatus(int64(store.PickInt(rw, "project_id")))
	}
	list, err := s.st.QueryMaps(`SELECT m.*, p.name AS project_name, p.code AS project_code, p.customer_id,
		c.name AS customer_name,
		CAST(julianday(m.expire_date) - julianday(date('now','localtime')) AS INTEGER) AS days_left
		FROM maintenance m JOIN projects p ON p.id=m.project_id JOIN customers c ON c.id=p.customer_id
		ORDER BY CASE WHEN m.expire_date IS NULL OR m.expire_date='' THEN 1 ELSE 0 END, m.expire_date`)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ok(w, list)
}

func parseDate(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, fmt.Errorf("empty")
	}
	if len(s) > 10 {
		s = s[:10]
	}
	return time.ParseInLocation("2006-01-02", s, time.Local)
}

var _ = io.EOF
