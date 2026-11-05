package httpapi

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"sangforpm/internal/store"
)

// ---------- 客户 ----------

func (s *Server) hCustomerList(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	sqlStr := `SELECT c.*,
		(SELECT COUNT(*) FROM contacts x WHERE x.customer_id=c.id) AS contact_count,
		(SELECT COUNT(*) FROM devices x WHERE x.customer_id=c.id) AS device_count,
		(SELECT COUNT(*) FROM projects x WHERE x.customer_id=c.id) AS project_count,
		(SELECT COUNT(*) FROM net_versions x WHERE x.customer_id=c.id) AS net_version_count
		FROM customers c WHERE 1=1`
	args := []any{}
	if q != "" {
		sqlStr += ` AND (c.name LIKE ? OR c.code LIKE ? OR c.remark LIKE ?)`
		args = append(args, "%"+q+"%", "%"+q+"%", "%"+q+"%")
	}
	sqlStr += ` ORDER BY c.updated_at DESC`
	list, err := s.st.QueryMaps(sqlStr, args...)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ok(w, list)
}

var customerFields = []string{"code", "name", "remark"}

func (s *Server) hCustomerCreate(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	name := strings.TrimSpace(store.Pick(body, "name"))
	if name == "" {
		writeErr(w, 400, "客户名称必填")
		return
	}
	now := store.Now()
	body["created_at"], body["updated_at"] = now, now
	id, err := s.st.Insert("customers", body, append(customerFields, "created_at", "updated_at"))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	// 新建客户自动建立 v1 网络信息版本
	cid, _ := s.st.Insert("net_versions", map[string]any{
		"customer_id": id, "version_no": 1, "content": "{}", "is_current": 1,
		"created_by": s.currentUser(r), "created_at": now,
	}, []string{"customer_id", "version_no", "content", "is_current", "created_by", "created_at"})
	_ = cid
	ok(w, map[string]any{"id": id})
}

func (s *Server) hCustomerGet(w http.ResponseWriter, r *http.Request) {
	id, okid := reqID(w, r, "id")
	if !okid {
		return
	}
	m, err := s.st.QueryRowMap(`SELECT * FROM customers WHERE id=?`, id)
	if err != nil {
		writeErr(w, 404, "客户不存在")
		return
	}
	ok(w, m)
}

func (s *Server) hCustomerUpdate(w http.ResponseWriter, r *http.Request) {
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
	if err := s.st.Update("customers", id, body, append(customerFields, "updated_at")); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ok(w, true)
}

func (s *Server) hCustomerDelete(w http.ResponseWriter, r *http.Request) {
	id, okid := reqID(w, r, "id")
	if !okid {
		return
	}
	if m, err := s.st.QueryRowMap(`SELECT COUNT(*) AS n FROM projects WHERE customer_id=?`, id); err == nil {
		if store.PickInt(m, "n") > 0 {
			writeErr(w, 400, "该客户下仍有项目，无法删除")
			return
		}
	}
	if err := s.st.Delete("customers", id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ok(w, true)
}

// ---------- 联系人 ----------

func (s *Server) hContactList(w http.ResponseWriter, r *http.Request) {
	id, okid := reqID(w, r, "id")
	if !okid {
		return
	}
	list, err := s.st.QueryMaps(`SELECT * FROM contacts WHERE customer_id=? ORDER BY id`, id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ok(w, list)
}

var contactFields = []string{"customer_id", "name", "phone", "email", "role", "remark"}

func (s *Server) hContactCreate(w http.ResponseWriter, r *http.Request) {
	id, okid := reqID(w, r, "id")
	if !okid {
		return
	}
	var body map[string]any
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	if strings.TrimSpace(store.Pick(body, "name")) == "" {
		writeErr(w, 400, "联系人姓名必填")
		return
	}
	now := store.Now()
	body["customer_id"] = id
	body["created_at"], body["updated_at"] = now, now
	nid, err := s.st.Insert("contacts", body, append(contactFields, "created_at", "updated_at"))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ok(w, map[string]any{"id": nid})
}

func (s *Server) hContactUpdate(w http.ResponseWriter, r *http.Request) {
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
	if err := s.st.Update("contacts", id, body, []string{"name", "phone", "email", "role", "remark", "updated_at"}); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ok(w, true)
}

func (s *Server) hContactDelete(w http.ResponseWriter, r *http.Request) {
	id, okid := reqID(w, r, "id")
	if !okid {
		return
	}
	if err := s.st.Delete("contacts", id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ok(w, true)
}

// ---------- 客户网络信息版本 ----------

func (s *Server) hNetVersionList(w http.ResponseWriter, r *http.Request) {
	id, okid := reqID(w, r, "id")
	if !okid {
		return
	}
	list, err := s.st.QueryMaps(`SELECT * FROM net_versions WHERE customer_id=? ORDER BY version_no DESC`, id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	for _, m := range list {
		var cnt int
		_ = s.st.DB.QueryRow(`SELECT COUNT(*) FROM net_changes WHERE version_id=?`, store.PickInt(m, "id")).Scan(&cnt)
		m["change_count"] = cnt
	}
	ok(w, list)
}

func (s *Server) hNetVersionGet(w http.ResponseWriter, r *http.Request) {
	id, okid := reqID(w, r, "id")
	if !okid {
		return
	}
	m, err := s.st.QueryRowMap(`SELECT * FROM net_versions WHERE id=?`, id)
	if err != nil {
		writeErr(w, 404, "版本不存在")
		return
	}
	changes, _ := s.st.QueryMaps(`SELECT * FROM net_changes WHERE version_id=? ORDER BY id`, id)
	m["changes"] = changes
	// 客户名
	if c, err := s.st.QueryRowMap(`SELECT name FROM customers WHERE id=?`, store.PickInt(m, "customer_id")); err == nil {
		m["customer_name"] = store.Pick(c, "name")
	}
	ok(w, m)
}

var netFieldOrder = []string{"contact", "gateway", "ip_plan", "vlan", "dns", "account", "bandwidth", "topology", "remark"}

func (s *Server) hNetVersionCreate(w http.ResponseWriter, r *http.Request) {
	cid, okid := reqID(w, r, "id")
	if !okid {
		return
	}
	var body struct {
		Content map[string]any `json:"content"`
		Note    string         `json:"note"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	if body.Content == nil {
		body.Content = map[string]any{}
	}
	now := store.Now()
	prev, err := s.st.QueryRowMap(`SELECT * FROM net_versions WHERE customer_id=? ORDER BY version_no DESC LIMIT 1`, cid)
	prevContent := map[string]any{}
	prevNo := 0
	prevID := int64(0)
	prevIsJSON := true
	if err == nil {
		prevNo = store.PickInt(prev, "version_no")
		prevID = int64(store.PickInt(prev, "id"))
		if jerr := json.Unmarshal([]byte(store.Pick(prev, "content")), &prevContent); jerr != nil {
			prevIsJSON = false
		}
	}
	if !prevIsJSON {
		prevContent = map[string]any{"remark": store.Pick(prev, "content")}
	}
	buf, _ := json.Marshal(body.Content)
	// 先取消所有当前版本标记
	_, _ = s.st.Exec(`UPDATE net_versions SET is_current=0 WHERE customer_id=?`, cid)
	vid, err := s.st.Insert("net_versions", map[string]any{
		"customer_id": cid, "version_no": prevNo + 1, "content": string(buf),
		"is_current": 1, "created_by": s.currentUser(r), "created_at": now,
	}, []string{"customer_id", "version_no", "content", "is_current", "created_by", "created_at"})
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	// 生成字段级变更历史
	if prevID > 0 {
		for _, k := range netFieldOrder {
			oldV := strings.TrimSpace(store.Pick(prevContent, k))
			newV := strings.TrimSpace(store.Pick(body.Content, k))
			if oldV == newV {
				continue
			}
			act := "update"
			if oldV == "" && newV != "" {
				act = "add"
			} else if oldV != "" && newV == "" {
				act = "delete"
			}
			_, _ = s.st.Insert("net_changes", map[string]any{
				"customer_id": cid, "version_id": vid, "field_name": k,
				"old_value": oldV, "new_value": newV, "action": act, "changed_at": now,
			}, []string{"customer_id", "version_id", "field_name", "old_value", "new_value", "action", "changed_at"})
		}
	}
	ok(w, map[string]any{"id": vid, "version_no": prevNo + 1})
}

func (s *Server) hNetChanges(w http.ResponseWriter, r *http.Request) {
	id, okid := reqID(w, r, "id")
	if !okid {
		return
	}
	list, err := s.st.QueryMaps(`SELECT * FROM net_changes WHERE version_id=? ORDER BY id`, id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ok(w, list)
}

// ---------- 客户设备档案 ----------

func (s *Server) hDeviceList(w http.ResponseWriter, r *http.Request) {
	cid, okid := reqID(w, r, "id")
	if !okid {
		return
	}
	list, err := s.st.QueryMaps(`SELECT d.*, c.name AS contact_name FROM devices d
		LEFT JOIN contacts c ON c.id=d.contact_id
		WHERE d.customer_id=? ORDER BY d.id DESC`, cid)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ok(w, list)
}

var deviceFields = []string{"contact_id", "model", "serial_no", "account", "topology", "network_info", "remark"}

func (s *Server) hDeviceCreate(w http.ResponseWriter, r *http.Request) {
	cid, okid := reqID(w, r, "id")
	if !okid {
		return
	}
	var body map[string]any
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	now := store.Now()
	body["customer_id"] = cid
	body["created_at"], body["updated_at"] = now, now
	id, err := s.st.Insert("devices", body, append(deviceFields, "customer_id", "created_at", "updated_at"))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	_, _ = s.st.Insert("device_changes", map[string]any{
		"device_id": id, "field_name": "*", "old_value": "", "new_value": "新增设备",
		"action": "create", "changed_at": now,
	}, []string{"device_id", "field_name", "old_value", "new_value", "action", "changed_at"})
	ok(w, map[string]any{"id": id})
}

func (s *Server) hDeviceGet(w http.ResponseWriter, r *http.Request) {
	id, okid := reqID(w, r, "id")
	if !okid {
		return
	}
	m, err := s.st.QueryRowMap(`SELECT d.*, c.name AS contact_name FROM devices d
		LEFT JOIN contacts c ON c.id=d.contact_id WHERE d.id=?`, id)
	if err != nil {
		writeErr(w, 404, "设备不存在")
		return
	}
	ok(w, m)
}

func (s *Server) hDeviceUpdate(w http.ResponseWriter, r *http.Request) {
	id, okid := reqID(w, r, "id")
	if !okid {
		return
	}
	var body map[string]any
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	old, err := s.st.QueryRowMap(`SELECT * FROM devices WHERE id=?`, id)
	if err != nil {
		writeErr(w, 404, "设备不存在")
		return
	}
	now := store.Now()
	body["updated_at"] = now
	if err := s.st.Update("devices", id, body, append(deviceFields, "updated_at")); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	logDiff(s.st, "device_changes", "device_id", id, old, body, deviceFields, now)
	ok(w, true)
}

func (s *Server) hDeviceDelete(w http.ResponseWriter, r *http.Request) {
	id, okid := reqID(w, r, "id")
	if !okid {
		return
	}
	if err := s.st.Delete("devices", id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ok(w, true)
}

func (s *Server) hDeviceHistory(w http.ResponseWriter, r *http.Request) {
	id, okid := reqID(w, r, "id")
	if !okid {
		return
	}
	list, err := s.st.QueryMaps(`SELECT * FROM device_changes WHERE device_id=? ORDER BY id DESC`, id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ok(w, list)
}

// logDiff 写入字段级变更历史
func logDiff(st *store.Store, table, fk string, id int64, old, body map[string]any, fields []string, now string) {
	for _, f := range fields {
		nv, provided := body[f]
		if !provided {
			continue
		}
		oldV := strings.TrimSpace(store.Pick(old, f))
		newV := strings.TrimSpace(store.Pick(map[string]any{"v": nv}, "v"))
		if oldV == newV {
			continue
		}
		act := "update"
		if oldV == "" && newV != "" {
			act = "add"
		} else if oldV != "" && newV == "" {
			act = "delete"
		}
		_, _ = st.Insert(table, map[string]any{
			fk: id, "field_name": f, "old_value": oldV, "new_value": newV,
			"action": act, "changed_at": now,
		}, []string{fk, "field_name", "old_value", "new_value", "action", "changed_at"})
	}
}

// ---------- 通用文件上传（拓扑图等） ----------

func (s *Server) hUpload(w http.ResponseWriter, r *http.Request) {
	f, hdr, err := r.FormFile("file")
	if err != nil {
		writeErr(w, 400, "缺少文件")
		return
	}
	defer f.Close()
	maxMB := 300
	if v := s.st.GetSetting("max_attach_mb"); v != "" {
		fmt.Sscanf(v, "%d", &maxMB)
	}
	if hdr.Size > int64(maxMB)<<20 {
		writeErr(w, 400, fmt.Sprintf("文件超过 %dMB 限制", maxMB))
		return
	}
	dir := filepath.Join(s.st.Files, "misc")
	_ = os.MkdirAll(dir, 0o755)
	name := fmt.Sprintf("%d_%s", nowTime().UnixNano(), safeName(hdr.Filename))
	dst := filepath.Join(dir, name)
	out, err := os.Create(dst)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer out.Close()
	size, err := copyWithLimit(out, f, int64(maxMB)<<20)
	if err != nil {
		os.Remove(dst)
		writeErr(w, 500, err.Error())
		return
	}
	ok(w, map[string]any{
		"name": hdr.Filename, "size": size,
		"path": "misc/" + name, "url": "/api/blob/misc/" + name,
	})
}

func (s *Server) hBlob(w http.ResponseWriter, r *http.Request) {
	sub := r.PathValue("name")
	sub = strings.ReplaceAll(sub, "..", "")
	p := filepath.Join(s.st.Files, filepath.FromSlash(sub))
	if !exists(p) {
		writeErr(w, 404, "文件不存在")
		return
	}
	serveInline(w, r, p)
}

func safeName(n string) string {
	n = filepath.Base(n)
	n = strings.ReplaceAll(n, " ", "_")
	bad := []string{"\\", "/", ":", "*", "?", "\"", "<", ">", "|"}
	for _, b := range bad {
		n = strings.ReplaceAll(n, b, "_")
	}
	if len(n) > 120 {
		n = n[len(n)-120:]
	}
	if n == "" {
		n = "file"
	}
	return n
}

func copyWithLimit(dst *os.File, src io.Reader, limit int64) (int64, error) {
	buf := make([]byte, 256<<10)
	var total int64
	for {
		n, err := src.Read(buf)
		if n > 0 {
			total += int64(n)
			if total > limit {
				return total, fmt.Errorf("文件超过大小限制")
			}
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return total, werr
			}
		}
		if err == io.EOF {
			return total, nil
		}
		if err != nil {
			return total, err
		}
	}
}

func serveInline(w http.ResponseWriter, r *http.Request, p string) {
	w.Header().Set("Content-Disposition", "inline")
	http.ServeFile(w, r, p)
}

var _ = sql.ErrNoRows
