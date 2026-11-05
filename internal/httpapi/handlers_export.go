package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/signintech/gopdf"
	"github.com/xuri/excelize/v2"

	"sangforpm/internal/store"
)

// ---------- Excel ----------

type sheetDef struct {
	name string
	cols []string
	rows [][]string
}

func (s *Server) hExportExcel(w http.ResponseWriter, r *http.Request) {
	typ := r.URL.Query().Get("type")
	if typ == "" {
		typ = "all"
	}
	pidStr := r.URL.Query().Get("project_id")
	filters := r.URL.Query().Get("filters")

	f := excelize.NewFile()
	defer f.Close()
	first := true
	addSheet := func(def sheetDef) error {
		name := def.name
		if first {
			if err := f.SetSheetName("Sheet1", name); err != nil {
				return err
			}
			first = false
		} else {
			if _, err := f.NewSheet(name); err != nil {
				return err
			}
		}
		head := make([]any, len(def.cols))
		for i, c := range def.cols {
			head[i] = c
		}
		if err := f.SetSheetRow(name, "A1", &head); err != nil {
			return err
		}
		for i, row := range def.rows {
			cells := make([]any, len(row))
			for j, v := range row {
				cells[j] = v
			}
			ref := fmt.Sprintf("A%d", i+2)
			if err := f.SetSheetRow(name, ref, &cells); err != nil {
				return err
			}
		}
		// 表头样式 + 列宽 + 冻结
		style, _ := f.NewStyle(&excelize.Style{
			Font:      &excelize.Font{Bold: true, Color: "FFFFFF"},
			Fill:      excelize.Fill{Type: "pattern", Color: []string{"2F5597"}, Pattern: 1},
			Alignment: &excelize.Alignment{Vertical: "center", Horizontal: "center", WrapText: true},
		})
		lastCol, _ := excelize.ColumnNumberToName(len(def.cols))
		_ = f.SetCellStyle(name, "A1", lastCol+"1", style)
		_ = f.SetColWidth(name, "A", lastCol, 18)
		_ = f.SetPanes(name, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"})
		_ = f.AutoFilter(name, "A1:"+lastCol+"1", nil)
		return nil
	}

	var err error
	if typ == "all" || typ == "projects" {
		err = addSheet(sheetProjects(s.st, r))
	}
	if err == nil && (typ == "all" || typ == "progress") {
		err = addSheet(sheetProgress(s.st, r, pidStr))
	}
	if err == nil && (typ == "all" || typ == "network") {
		err = addSheet(sheetNetwork(s.st, r))
	}
	if err == nil && (typ == "all" || typ == "devices") {
		err = addSheet(sheetDevices(s.st, r))
	}
	if err == nil && (typ == "all" || typ == "issues") {
		err = addSheet(sheetIssues(s.st, r))
	}
	if err == nil && (typ == "all" || typ == "maintenance") {
		err = addSheet(sheetMaintenance(s.st))
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	nameMap := map[string]string{
		"all": "项目管理全量清单", "projects": "项目清单", "progress": "进度记录",
		"network": "客户网络信息", "devices": "设备清单", "issues": "问题清单",
		"maintenance": "维保到期清单",
	}
	fname := nameMap[typ] + "_" + time.Now().Format("20060102_1504") + ".xlsx"
	_, _ = s.st.Insert("export_logs", map[string]any{
		"export_type": typ, "format": "xlsx", "filters": filters,
		"file_path": fname, "created_at": store.Now(),
	}, []string{"export_type", "format", "filters", "file_path", "created_at"})
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(fname))
	_ = f.Write(w)
}

func (s *Server) filteredProjects(r *http.Request) []map[string]any {
	q := r.URL.Query()
	sqlStr := `SELECT p.*, c.name AS customer_name,
		(SELECT group_concat(DISTINCT product_line) FROM project_products x WHERE x.project_id=p.id) AS product_lines,
		(SELECT group_concat(product_name,'、') FROM project_products x WHERE x.project_id=p.id AND product_name<>'') AS product_names,
		(SELECT COUNT(*) FROM issues x WHERE x.project_id=p.id AND x.status='未关闭') AS open_issues,
		(SELECT MAX(record_date) FROM progress x WHERE x.project_id=p.id) AS last_progress_date
		FROM projects p JOIN customers c ON c.id=p.customer_id WHERE 1=1`
	args := []any{}
	if v := q.Get("q"); v != "" {
		sqlStr += ` AND (p.name LIKE ? OR p.code LIKE ? OR c.name LIKE ?)`
		args = append(args, "%"+v+"%", "%"+v+"%", "%"+v+"%")
	}
	for _, k := range []string{"ptype", "status", "priority"} {
		if v := q.Get(k); v != "" && v != "全部" {
			sqlStr += " AND p." + k + "=?"
			args = append(args, v)
		}
	}
	if v := q.Get("customer_id"); v != "" && v != "0" {
		sqlStr += ` AND p.customer_id=?`
		args = append(args, v)
	}
	if v := q.Get("product_line"); v != "" && v != "全部" {
		sqlStr += ` AND EXISTS(SELECT 1 FROM project_products x WHERE x.project_id=p.id AND x.product_line=?)`
		args = append(args, v)
	}
	if v := q.Get("project_id"); v != "" && v != "0" {
		sqlStr += ` AND p.id=?`
		args = append(args, v)
	}
	if v := q.Get("ids"); v != "" {
		parts := strings.Split(v, ",")
		ph := make([]string, 0, len(parts))
		for _, p := range parts {
			if n, err := strconv.Atoi(strings.TrimSpace(p)); err == nil && n > 0 {
				ph = append(ph, "?")
				args = append(args, n)
			}
		}
		if len(ph) > 0 {
			sqlStr += ` AND p.id IN (` + strings.Join(ph, ",") + `)`
		}
	}
	sqlStr += ` ORDER BY p.updated_at DESC`
	list, err := s.st.QueryMaps(sqlStr, args...)
	if err != nil {
		return nil
	}
	return list
}

func sheetProjects(st *store.Store, r *http.Request) sheetDef {
	s := &Server{st: st}
	list := s.filteredProjects(r)
	rows := make([][]string, 0, len(list))
	for _, m := range list {
		rows = append(rows, []string{
			store.Pick(m, "code"), store.Pick(m, "name"), store.Pick(m, "customer_name"),
			store.Pick(m, "ptype"), store.Pick(m, "status"), store.Pick(m, "priority"),
			store.Pick(m, "product_lines"), store.Pick(m, "product_names"),
			store.Pick(m, "start_date"), store.Pick(m, "expect_accept_date"),
			store.Pick(m, "accept_date"), store.Pick(m, "acceptor"),
			store.Pick(m, "legacy_issues"), store.Pick(m, "remark"),
			strconv.Itoa(store.PickInt(m, "open_issues")), store.Pick(m, "last_progress_date"),
		})
	}
	return sheetDef{
		name: "项目清单",
		cols: []string{"项目编号", "项目名称", "客户", "项目类型", "项目状态", "优先级", "产品线",
			"具体产品", "开始日期", "预计验收日期", "验收日期", "验收人", "遗留问题", "备注",
			"未关闭问题数", "最后进度日期"},
		rows: rows,
	}
}

func sheetProgress(st *store.Store, r *http.Request, pid string) sheetDef {
	sqlStr := `SELECT g.*, p.name AS project_name, p.code AS project_code, c.name AS customer_name
		FROM progress g JOIN projects p ON p.id=g.project_id JOIN customers c ON c.id=p.customer_id WHERE 1=1`
	args := []any{}
	q := r.URL.Query()
	if v := q.Get("project_id"); v != "" && v != "0" {
		sqlStr += ` AND g.project_id=?`
		args = append(args, v)
	} else if pid != "" && pid != "0" {
		sqlStr += ` AND g.project_id=?`
		args = append(args, pid)
	}
	if v := q.Get("from"); v != "" {
		sqlStr += ` AND g.record_date>=?`
		args = append(args, v)
	}
	if v := q.Get("to"); v != "" {
		sqlStr += ` AND g.record_date<=?`
		args = append(args, v)
	}
	if v := q.Get("week"); v != "" {
		a, b := weekRange(v)
		sqlStr += ` AND g.record_date>=? AND g.record_date<=?`
		args = append(args, a, b)
	}
	sqlStr += ` ORDER BY g.record_date DESC, g.id DESC`
	list, _ := st.QueryMaps(sqlStr, args...)
	s := &Server{st: st}
	s.hydrateProgress(list)
	rows := make([][]string, 0, len(list))
	for _, m := range list {
		rows = append(rows, []string{
			store.Pick(m, "record_date"), store.Pick(m, "project_code"), store.Pick(m, "project_name"),
			store.Pick(m, "customer_name"), store.Pick(m, "stage_names"),
			store.Pick(m, "current_progress"), store.Pick(m, "problems"),
			store.Pick(m, "next_plan"), store.Pick(m, "info_sync"),
		})
	}
	return sheetDef{
		name: "进度记录",
		cols: []string{"日期", "项目编号", "项目名称", "客户", "阶段", "当前进展", "存在问题", "下一步计划", "信息同步"},
		rows: rows,
	}
}

func netContent(m map[string]any) map[string]any {
	out := map[string]any{}
	_ = json.Unmarshal([]byte(store.Pick(m, "content")), &out)
	return out
}

func sheetNetwork(st *store.Store, r *http.Request) sheetDef {
	sqlStr := `SELECT v.*, c.name AS customer_name FROM net_versions v
		JOIN customers c ON c.id=v.customer_id WHERE 1=1`
	args := []any{}
	if v := r.URL.Query().Get("customer_id"); v != "" && v != "0" {
		sqlStr += ` AND v.customer_id=?`
		args = append(args, v)
	}
	sqlStr += ` ORDER BY c.name, v.version_no DESC`
	list, _ := st.QueryMaps(sqlStr, args...)
	rows := make([][]string, 0, len(list))
	for _, m := range list {
		c := netContent(m)
		cur := "否"
		if store.PickInt(m, "is_current") == 1 {
			cur = "是"
		}
		rows = append(rows, []string{
			store.Pick(m, "customer_name"), strconv.Itoa(store.PickInt(m, "version_no")), cur,
			store.Pick(c, "contact"), store.Pick(c, "gateway"), store.Pick(c, "ip_plan"),
			store.Pick(c, "vlan"), store.Pick(c, "dns"), store.Pick(c, "account"),
			store.Pick(c, "bandwidth"), store.Pick(c, "topology"), store.Pick(c, "remark"),
			store.Pick(m, "created_by"), store.Pick(m, "created_at"),
		})
	}
	return sheetDef{
		name: "客户网络信息",
		cols: []string{"客户", "版本号", "当前版本", "客户对接人", "出口/网关", "IP 地址规划", "VLAN 划分",
			"DNS / NTP", "网络设备账号密码", "线路 / 带宽", "拓扑说明", "其他说明", "创建人", "创建时间"},
		rows: rows,
	}
}

func sheetDevices(st *store.Store, r *http.Request) sheetDef {
	sqlStr := `SELECT d.*, c.name AS customer_name, ct.name AS contact_name FROM devices d
		JOIN customers c ON c.id=d.customer_id LEFT JOIN contacts ct ON ct.id=d.contact_id WHERE 1=1`
	args := []any{}
	if v := r.URL.Query().Get("customer_id"); v != "" && v != "0" {
		sqlStr += ` AND d.customer_id=?`
		args = append(args, v)
	}
	sqlStr += ` ORDER BY c.name, d.id`
	list, _ := st.QueryMaps(sqlStr, args...)
	rows := make([][]string, 0, len(list))
	for _, m := range list {
		rows = append(rows, []string{
			store.Pick(m, "customer_name"), store.Pick(m, "contact_name"), store.Pick(m, "model"),
			store.Pick(m, "serial_no"), store.Pick(m, "account"), store.Pick(m, "network_info"),
			store.Pick(m, "remark"), store.Pick(m, "updated_at"),
		})
	}
	return sheetDef{
		name: "设备清单",
		cols: []string{"客户", "对接人", "设备型号", "序列号", "账号密码", "网络信息", "备注", "更新时间"},
		rows: rows,
	}
}

func sheetIssues(st *store.Store, r *http.Request) sheetDef {
	sqlStr := `SELECT i.*, p.name AS project_name, p.code AS project_code, c.name AS customer_name
		FROM issues i JOIN projects p ON p.id=i.project_id JOIN customers c ON c.id=p.customer_id WHERE 1=1`
	args := []any{}
	if v := r.URL.Query().Get("status"); v != "" && v != "全部" {
		sqlStr += ` AND i.status=?`
		args = append(args, v)
	}
	if v := r.URL.Query().Get("project_id"); v != "" && v != "0" {
		sqlStr += ` AND i.project_id=?`
		args = append(args, v)
	}
	sqlStr += ` ORDER BY (i.status='未关闭') DESC, i.id DESC`
	list, _ := st.QueryMaps(sqlStr, args...)
	rows := make([][]string, 0, len(list))
	for _, m := range list {
		rows = append(rows, []string{
			store.Pick(m, "project_code"), store.Pick(m, "project_name"), store.Pick(m, "customer_name"),
			store.Pick(m, "title"), store.Pick(m, "description"), store.Pick(m, "level"),
			store.Pick(m, "owner"), store.Pick(m, "status"), store.Pick(m, "closed_at"),
			store.Pick(m, "created_at"),
		})
	}
	return sheetDef{
		name: "问题清单",
		cols: []string{"项目编号", "项目名称", "客户", "标题", "描述", "等级", "负责人", "状态", "关闭时间", "创建时间"},
		rows: rows,
	}
}

func sheetMaintenance(st *store.Store) sheetDef {
	list, _ := st.QueryMaps(`SELECT m.*, p.name AS project_name, p.code AS project_code, c.name AS customer_name,
		CAST(julianday(m.expire_date) - julianday(date('now','localtime')) AS INTEGER) AS days_left
		FROM maintenance m JOIN projects p ON p.id=m.project_id JOIN customers c ON c.id=p.customer_id
		ORDER BY m.expire_date`)
	rows := make([][]string, 0, len(list))
	for _, m := range list {
		ov := "否"
		if store.PickInt(m, "manual_override") == 1 {
			ov = "是"
		}
		rows = append(rows, []string{
			store.Pick(m, "project_code"), store.Pick(m, "project_name"), store.Pick(m, "customer_name"),
			store.Pick(m, "start_date"), store.Pick(m, "expire_date"),
			strconv.Itoa(store.PickInt(m, "days_left")), store.Pick(m, "status"), ov,
			strconv.Itoa(store.PickInt(m, "cycle_months")), store.Pick(m, "maint_record"),
		})
	}
	return sheetDef{
		name: "维保到期清单",
		cols: []string{"项目编号", "项目名称", "客户", "维保起始日", "到期日", "剩余天数", "维保状态", "手动覆盖", "到期周期(月)", "维保记录"},
		rows: rows,
	}
}

func (s *Server) hExportLogs(w http.ResponseWriter, r *http.Request) {
	list, err := s.st.QueryMaps(`SELECT * FROM export_logs ORDER BY id DESC LIMIT 100`)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ok(w, list)
}

// ---------- PDF ----------

func findCJKFont() string {
	cands := []string{
		os.Getenv("PM_PDF_FONT"),
		`C:\Windows\Fonts\simhei.ttf`,
		`C:\Windows\Fonts\msyh.ttf`,
		`C:\Windows\Fonts\simkai.ttf`,
		"/usr/share/fonts/truetype/arphic/uming.ttc",
		"/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc",
		"/usr/share/fonts/truetype/noto/NotoSansCJK-Regular.ttc",
		"/usr/share/fonts/truetype/wqy/wqy-zenhei.ttc",
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
	}
	for _, c := range cands {
		if c != "" && exists(c) {
			return c
		}
	}
	return ""
}

type pdfDoc struct {
	*gopdf.GoPdf
	fontPath string
	pageW    float64
	margin   float64
	y        float64
}

func newPDF(font string) (*pdfDoc, error) {
	p := &gopdf.GoPdf{}
	p.Start(gopdf.Config{PageSize: *gopdf.PageSizeA4})
	if err := p.AddTTFFont("cjk", font); err != nil {
		return nil, err
	}
	p.SetMargins(40, 40, 40, 40)
	p.AddPage()
	d := &pdfDoc{GoPdf: p, fontPath: font, pageW: gopdf.PageSizeA4.W, margin: 40, y: 50}
	_ = d.SetFont("cjk", "", 10)
	return d, nil
}

func (d *pdfDoc) text(size float64, s string) {
	_ = d.SetFont("cjk", "", size)
	d.SetX(d.margin)
	d.SetY(d.y)
	d.CellWithOption(&gopdf.Rect{W: d.pageW - 2*d.margin, H: size + 6}, s, gopdf.CellOption{Align: gopdf.Left | gopdf.Top})
	d.y += size + 6
}

func (d *pdfDoc) title(s string) {
	_ = d.SetFont("cjk", "", 18)
	d.SetX(d.margin)
	d.SetY(d.y)
	d.CellWithOption(&gopdf.Rect{W: d.pageW - 2*d.margin, H: 28}, s, gopdf.CellOption{Align: gopdf.Center | gopdf.Middle})
	d.y += 34
}

func (d *pdfDoc) hr() {
	d.SetLineWidth(0.5)
	d.SetStrokeColor(200, 200, 200)
	d.Line(d.margin, d.y, d.pageW-d.margin, d.y)
	d.y += 6
}

func (d *pdfDoc) table(head []string, rows [][]string, widths []float64) {
	if d.y > 740 {
		d.AddPage()
		d.y = 50
	}
	_ = d.SetFont("cjk", "", 9)
	d.SetX(d.margin)
	d.SetY(d.y)
	x := d.margin
	total := 0.0
	for i, h := range head {
		w := widths[i]
		total += w
		d.SetXY(x, d.y)
		d.CellWithOption(&gopdf.Rect{W: w, H: 20}, h, gopdf.CellOption{Align: gopdf.Center | gopdf.Middle, Border: gopdf.Bottom | gopdf.Top | gopdf.Left | gopdf.Right})
		x += w
	}
	d.y += 20
	for _, row := range rows {
		if d.y > 780 {
			d.AddPage()
			d.y = 50
		}
		x = d.margin
		h := 18.0
		for i, cell := range row {
			if i >= len(widths) {
				break
			}
			w := widths[i]
			d.SetXY(x, d.y)
			d.CellWithOption(&gopdf.Rect{W: w, H: h}, truncateRunes(cell, int(w/5)), gopdf.CellOption{Align: gopdf.Left | gopdf.Middle, Border: gopdf.Bottom | gopdf.Left | gopdf.Right})
			x += w
		}
		d.y += h
	}
	d.y += 8
	_ = total
}

func truncateRunes(s string, n int) string {
	if n <= 1 {
		return s
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func (s *Server) hExportPDF(w http.ResponseWriter, r *http.Request) {
	typ := r.URL.Query().Get("type")
	pid, _ := strconv.ParseInt(r.URL.Query().Get("project_id"), 10, 64)

	// 验收报告：直接导出用户上传的验收单附件
	if typ == "acceptance" {
		s.exportAcceptancePDF(w, r, pid)
		return
	}
	font := findCJKFont()
	if font == "" {
		writeErr(w, 501, "未找到可用中文字体，请设置环境变量 PM_PDF_FONT 指向 ttf 字体文件")
		return
	}
	d, err := newPDF(font)
	if err != nil {
		writeErr(w, 500, "PDF 字体加载失败: "+err.Error())
		return
	}
	switch typ {
	case "weekly":
		s.buildWeekly(d, pid)
	case "network":
		s.buildNetwork(d, pid, r)
	case "maintenance":
		s.buildMaintenance(d, pid)
	default:
		writeErr(w, 400, "PDF 类型无效")
		return
	}
	fname := map[string]string{"weekly": "项目周报", "network": "客户网络信息表", "maintenance": "维保报告"}[typ]
	fname += "_" + time.Now().Format("20060102_1504") + ".pdf"
	_, _ = s.st.Insert("export_logs", map[string]any{
		"export_type": typ, "format": "pdf", "filters": fmt.Sprintf("project_id=%d", pid),
		"file_path": fname, "created_at": store.Now(),
	}, []string{"export_type", "format", "filters", "file_path", "created_at"})
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(fname))
	_ = d.Write(w)
}

func (s *Server) exportAcceptancePDF(w http.ResponseWriter, r *http.Request, pid int64) {
	var att map[string]any
	var err error
	if aid, _ := strconv.ParseInt(r.URL.Query().Get("attachment_id"), 10, 64); aid > 0 {
		att, err = s.st.QueryRowMap(`SELECT a.*, v.file_path FROM attachments a
			LEFT JOIN attachment_versions v ON v.id=a.current_version_id WHERE a.id=?`, aid)
	} else {
		att, err = s.st.QueryRowMap(`SELECT a.*, v.file_path FROM attachments a
			LEFT JOIN attachment_versions v ON v.id=a.current_version_id
			WHERE a.project_id=? AND a.atype IN ('验收单','验收报告') ORDER BY a.id DESC LIMIT 1`, pid)
	}
	if err != nil {
		writeErr(w, 404, "未找到验收单附件，请先在项目附件中上传")
		return
	}
	p := filepath.Join(s.st.Files, filepath.FromSlash(store.Pick(att, "file_path")))
	if !exists(p) {
		writeErr(w, 404, "验收单文件已丢失")
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(store.Pick(att, "file_name")))
	http.ServeFile(w, r, p)
}

func (s *Server) buildWeekly(d *pdfDoc, pid int64) {
	p, err := s.st.QueryRowMap(`SELECT p.*, c.name AS customer_name FROM projects p
		JOIN customers c ON c.id=p.customer_id WHERE p.id=?`, pid)
	if err != nil {
		d.title("项目不存在")
		return
	}
	weekStart, weekEnd := weekRange(store.Today())
	d.title("项目周报")
	d.text(9, fmt.Sprintf("统计周期：%s ~ %s    生成时间：%s", weekStart, weekEnd, time.Now().Format("2006-01-02 15:04")))
	d.hr()
	d.text(11, "一、项目基础信息")
	d.text(10, fmt.Sprintf("项目编号：%s      项目名称：%s", store.Pick(p, "code"), store.Pick(p, "name")))
	d.text(10, fmt.Sprintf("客户：%s      项目类型：%s      项目状态：%s      优先级：%s",
		store.Pick(p, "customer_name"), store.Pick(p, "ptype"), store.Pick(p, "status"), store.Pick(p, "priority")))
	d.text(10, fmt.Sprintf("开始日期：%s      预计验收：%s      验收日期：%s      验收人：%s",
		store.Pick(p, "start_date"), store.Pick(p, "expect_accept_date"), store.Pick(p, "accept_date"), store.Pick(p, "acceptor")))
	d.y += 4
	d.text(11, "二、本周进度记录")
	rows, _ := s.st.QueryMaps(`SELECT * FROM progress WHERE project_id=? AND record_date>=? AND record_date<=?
		ORDER BY record_date`, pid, weekStart, weekEnd)
	s.hydrateProgress(rows)
	if len(rows) == 0 {
		d.text(10, "（本周无进度记录）")
	}
	trows := make([][]string, 0, len(rows))
	for _, m := range rows {
		trows = append(trows, []string{
			store.Pick(m, "record_date"), store.Pick(m, "stage_names"),
			store.Pick(m, "current_progress"), store.Pick(m, "problems"), store.Pick(m, "next_plan"),
		})
	}
	if len(trows) > 0 {
		d.table([]string{"日期", "阶段", "当前进展", "存在问题", "下一步计划"}, trows,
			[]float64{56, 70, 160, 120, 110})
	}
	d.text(11, "三、未关闭问题")
	issues, _ := s.st.QueryMaps(`SELECT * FROM issues WHERE project_id=? AND status='未关闭' ORDER BY id DESC`, pid)
	if len(issues) == 0 {
		d.text(10, "（无未关闭问题）")
	} else {
		irows := make([][]string, 0, len(issues))
		for _, m := range issues {
			irows = append(irows, []string{store.Pick(m, "title"), store.Pick(m, "level"), store.Pick(m, "owner"), store.Pick(m, "description")})
		}
		d.table([]string{"标题", "等级", "负责人", "描述"}, irows, []float64{140, 46, 70, 260})
	}
	d.text(11, "四、下一步计划")
	if len(rows) > 0 {
		np := strings.TrimSpace(store.Pick(rows[0], "next_plan"))
		if np == "" {
			np = "未定"
		}
		d.text(10, np)
	} else {
		last, _ := s.st.QueryRowMap(`SELECT next_plan FROM progress WHERE project_id=? ORDER BY record_date DESC, id DESC LIMIT 1`, pid)
		np := strings.TrimSpace(store.Pick(last, "next_plan"))
		if np == "" {
			np = "未定"
		}
		d.text(10, np)
	}
}

func (s *Server) buildNetwork(d *pdfDoc, pid int64, r *http.Request) {
	var cid int64
	if pid > 0 {
		if p, err := s.st.QueryRowMap(`SELECT customer_id FROM projects WHERE id=?`, pid); err == nil {
			cid = int64(store.PickInt(p, "customer_id"))
		}
	}
	if cid == 0 {
		cid, _ = strconv.ParseInt(r.URL.Query().Get("customer_id"), 10, 64)
	}
	if v := s.currentNetVersion(cid, pid); v != nil {
		c := netContent(v)
		d.title("客户网络信息表")
		d.text(9, fmt.Sprintf("客户：%s    版本：V%d    生成时间：%s",
			store.Pick(v, "customer_name"), store.PickInt(v, "version_no"), time.Now().Format("2006-01-02 15:04")))
		d.hr()
		labels := map[string]string{
			"contact": "客户对接人", "gateway": "出口/网关", "ip_plan": "IP 地址规划",
			"vlan": "VLAN 划分", "dns": "DNS / NTP", "account": "网络设备账号密码",
			"bandwidth": "线路 / 带宽", "topology": "拓扑说明", "remark": "其他说明",
		}
		for _, k := range netFieldOrder {
			d.text(10, fmt.Sprintf("%s：%s", labels[k], store.Pick(c, k)))
			d.y += 1
		}
		return
	}
	d.title("客户网络信息表")
	d.text(10, "未找到网络信息版本")
}

func (s *Server) currentNetVersion(cid, pid int64) map[string]any {
	if pid > 0 {
		if m, err := s.st.QueryRowMap(`SELECT v.*, c.name AS customer_name FROM project_net_refs r
			JOIN net_versions v ON v.id=r.net_version_id JOIN customers c ON c.id=v.customer_id
			WHERE r.project_id=?`, pid); err == nil {
			return m
		}
	}
	if m, err := s.st.QueryRowMap(`SELECT v.*, c.name AS customer_name FROM net_versions v
		JOIN customers c ON c.id=v.customer_id WHERE v.customer_id=? ORDER BY v.version_no DESC LIMIT 1`, cid); err == nil {
		return m
	}
	return nil
}

func (s *Server) buildMaintenance(d *pdfDoc, pid int64) {
	m, err := s.st.QueryRowMap(`SELECT m.*, p.name AS project_name, p.code AS project_code, c.name AS customer_name,
		p.accept_date, p.acceptor, p.legacy_issues,
		CAST(julianday(m.expire_date) - julianday(date('now','localtime')) AS INTEGER) AS days_left
		FROM maintenance m JOIN projects p ON p.id=m.project_id JOIN customers c ON c.id=p.customer_id
		WHERE m.project_id=?`, pid)
	d.title("维保报告")
	if err != nil {
		d.text(10, "未找到维保信息")
		return
	}
	d.text(9, "生成时间："+time.Now().Format("2006-01-02 15:04"))
	d.hr()
	d.text(10, fmt.Sprintf("项目编号：%s      项目名称：%s", store.Pick(m, "project_code"), store.Pick(m, "project_name")))
	d.text(10, fmt.Sprintf("客户：%s", store.Pick(m, "customer_name")))
	d.text(10, fmt.Sprintf("维保起始日：%s      到期日：%s      到期周期：%d 个月",
		store.Pick(m, "start_date"), store.Pick(m, "expire_date"), store.PickInt(m, "cycle_months")))
	d.text(10, fmt.Sprintf("当前维保状态：%s      剩余天数：%d", store.Pick(m, "status"), store.PickInt(m, "days_left")))
	d.text(10, fmt.Sprintf("验收日期：%s      验收人：%s", store.Pick(m, "accept_date"), store.Pick(m, "acceptor")))
	d.y += 4
	d.text(11, "遗留问题")
	d.text(10, orDash(store.Pick(m, "legacy_issues")))
	d.y += 4
	d.text(11, "维保记录")
	d.text(10, orDash(store.Pick(m, "maint_record")))
	d.y += 4
	d.text(11, "授权截图 / 验收报告")
	atts, _ := s.st.QueryMaps(`SELECT atype, file_name FROM attachments WHERE project_id=? AND atype IN ('授权截图','验收单','验收报告','维保记录')`, pid)
	if len(atts) == 0 {
		d.text(10, "（无）")
	}
	for _, a := range atts {
		d.text(10, fmt.Sprintf("· [%s] %s", store.Pick(a, "atype"), store.Pick(a, "file_name")))
	}
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

var _ = json.Marshal
