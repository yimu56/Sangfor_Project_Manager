package httpapi

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"sangforpm/internal/store"
)

func (s *Server) hMeta(w http.ResponseWriter, r *http.Request) {
	ok(w, map[string]any{
		"project_types": projectTypes,
		"status_by_type": statusByType,
		"all_statuses":   allStatuses(),
		"mt_statuses":    mtStatuses,
		"priorities":     []string{"高", "中", "低"},
		"product_lines":  []string{"安全产品", "云产品", "基础架构", "桌面云", "其他"},
		"issue_levels":   []string{"高", "中", "低"},
		"issue_statuses": []string{"未关闭", "已关闭"},
		"attachment_types": []string{
			"实施方案", "拓扑图", "配置备份", "授权截图", "验收单", "现场照片", "维保记录", "其他",
		},
		"stage_templates": stageTemplates,
		"net_fields": []map[string]string{
			{"key": "contact", "label": "客户对接人"},
			{"key": "gateway", "label": "出口/网关"},
			{"key": "ip_plan", "label": "IP 地址规划"},
			{"key": "vlan", "label": "VLAN 划分"},
			{"key": "dns", "label": "DNS / NTP"},
			{"key": "account", "label": "网络设备账号密码"},
			{"key": "bandwidth", "label": "线路 / 带宽"},
			{"key": "topology", "label": "拓扑说明"},
			{"key": "remark", "label": "其他说明"},
		},
		"thresholds": map[string]any{
			"warranty_notice": s.st.GetSetting("warranty_notice"),
			"stale_days":      s.st.GetSetting("stale_days"),
			"accept_notice":   s.st.GetSetting("accept_notice"),
			"longrun_days":    s.st.GetSetting("longrun_days"),
		},
	})
}

func (s *Server) settingInt(key string, def int) int {
	v := s.st.GetSetting(key)
	var n int
	if _, err := fmt.Sscanf(v, "%d", &n); err != nil || n <= 0 {
		return def
	}
	return n
}

// ---------- 首页看板 ----------

func (s *Server) hDashboard(w http.ResponseWriter, r *http.Request) {
	longrunDays := s.settingInt("longrun_days", 14)
	staleDaysCfg := s.settingInt("stale_days", 7)
	warnDays := s.settingInt("warranty_notice", 60)
	acceptDays := s.settingInt("accept_notice", 3)

	base := `SELECT p.*, c.name AS customer_name,
		(SELECT group_concat(DISTINCT product_line) FROM project_products x WHERE x.project_id=p.id) AS product_lines,
		(SELECT MAX(record_date) FROM progress x WHERE x.project_id=p.id) AS last_progress_date,
		(SELECT COUNT(*) FROM issues x WHERE x.project_id=p.id AND x.status='未关闭') AS open_issues
		FROM projects p JOIN customers c ON c.id=p.customer_id WHERE 1=1`

	// 1. 长时间未结束项目
	active := base + ` AND p.status NOT IN ('已结束','取消','已验收') ORDER BY p.start_date`
	rows, _ := s.st.QueryMaps(active)
	longList := make([]map[string]any, 0)
	nextPlans := make([]map[string]any, 0)
	staleList := make([]map[string]any, 0)
	for _, m := range rows {
		start := strings.TrimSpace(store.Pick(m, "start_date"))
		if start == "" {
			start = strings.TrimSpace(store.Pick(m, "created_at"))
		}
		days := 0
		if t, err := parseDate(start); err == nil {
			days = int(time.Since(t).Hours() / 24)
		}
		m["running_days"] = days
		sd := staleDays(m)
		m["stale_days"] = sd
		if days > longrunDays {
			longList = append(longList, m)
		}
		if sd > staleDaysCfg {
			staleList = append(staleList, m)
		}
		np, _ := s.st.QueryRowMap(`SELECT next_plan FROM progress WHERE project_id=? ORDER BY record_date DESC, id DESC LIMIT 1`,
			store.PickInt(m, "id"))
		plan := strings.TrimSpace(store.Pick(np, "next_plan"))
		if plan == "" {
			plan = "未定"
		}
		m["next_plan_show"] = plan
		nextPlans = append(nextPlans, m)
	}

	// 2. 本周进度记录
	now := time.Now()
	wd := int(now.Weekday())
	if wd == 0 {
		wd = 7
	}
	monday := now.AddDate(0, 0, -(wd - 1))
	weekStart := monday.Format("2006-01-02")
	weekEnd := monday.AddDate(0, 0, 6).Format("2006-01-02")
	weekRows, _ := s.st.QueryMaps(`SELECT g.*, p.name AS project_name, p.code AS project_code, c.name AS customer_name
		FROM progress g JOIN projects p ON p.id=g.project_id JOIN customers c ON c.id=p.customer_id
		WHERE g.record_date>=? AND g.record_date<=? ORDER BY g.record_date DESC, g.id DESC`, weekStart, weekEnd)
	s.hydrateProgress(weekRows)

	// 3. 即将到期 / 已过保维保
	mtRows, _ := s.st.QueryMaps(`SELECT m.*, p.name AS project_name, p.id AS project_id, c.name AS customer_name,
		CAST(julianday(m.expire_date) - julianday(date('now','localtime')) AS INTEGER) AS days_left
		FROM maintenance m JOIN projects p ON p.id=m.project_id JOIN customers c ON c.id=p.customer_id
		WHERE m.expire_date IS NOT NULL AND m.expire_date<>''
		ORDER BY m.expire_date`)
	expiring := make([]map[string]any, 0)
	for _, m := range mtRows {
		s.refreshMaintStatus(int64(store.PickInt(m, "project_id")))
	}
	mtRows, _ = s.st.QueryMaps(`SELECT m.*, p.name AS project_name, p.id AS project_id, c.name AS customer_name,
		CAST(julianday(m.expire_date) - julianday(date('now','localtime')) AS INTEGER) AS days_left
		FROM maintenance m JOIN projects p ON p.id=m.project_id JOIN customers c ON c.id=p.customer_id
		WHERE m.expire_date IS NOT NULL AND m.expire_date<>''
		ORDER BY m.expire_date`)
	for _, m := range mtRows {
		dl := store.PickInt(m, "days_left")
		if dl <= warnDays {
			expiring = append(expiring, m)
		}
	}

	// 4. 未关闭问题
	issues, _ := s.st.QueryMaps(`SELECT i.*, p.name AS project_name, p.code AS project_code, c.name AS customer_name
		FROM issues i JOIN projects p ON p.id=i.project_id JOIN customers c ON c.id=p.customer_id
		WHERE i.status='未关闭'
		ORDER BY CASE i.level WHEN '高' THEN 0 WHEN '中' THEN 1 ELSE 2 END, i.id DESC LIMIT 100`)

	// 5. 验收前提醒候选
	acceptRows, _ := s.st.QueryMaps(`SELECT p.*, c.name AS customer_name,
		CAST(julianday(p.expect_accept_date) - julianday(date('now','localtime')) AS INTEGER) AS days_left
		FROM projects p JOIN customers c ON c.id=p.customer_id
		WHERE p.expect_accept_date IS NOT NULL AND p.expect_accept_date<>''
		  AND p.status NOT IN ('已验收','已结束','取消')
		ORDER BY p.expect_accept_date`)
	acceptSoon := make([]map[string]any, 0)
	for _, m := range acceptRows {
		if store.PickInt(m, "days_left") <= acceptDays {
			acceptSoon = append(acceptSoon, m)
		}
	}

	// 6. 未读提醒数
	cnt, _ := s.st.QueryRowMap(`SELECT COUNT(*) AS n FROM reminders WHERE is_read=0 AND is_done=0`)
	waiting, _ := s.st.QueryRowMap(`SELECT COUNT(*) AS n FROM projects WHERE status NOT IN ('已结束','已验收','取消')`)
	openIssueCnt, _ := s.st.QueryRowMap(`SELECT COUNT(*) AS n FROM issues WHERE status='未关闭'`)

	ok(w, map[string]any{
		"long_running":  longList,
		"next_plans":    nextPlans,
		"week_progress": weekRows,
		"week_range":    map[string]string{"start": weekStart, "end": weekEnd},
		"week_count":    len(weekRows),
		"expiring":      expiring,
		"open_issues":   issues,
		"accept_soon":   acceptSoon,
		"stale":         staleList,
		"stats": map[string]any{
			"unread_reminders": store.PickInt(cnt, "n"),
			"active_projects":  store.PickInt(waiting, "n"),
			"open_issues":      store.PickInt(openIssueCnt, "n"),
			"expiring_count":   len(expiring),
			"longrun_count":    len(longList),
		},
		"thresholds": map[string]int{
			"longrun_days": longrunDays, "stale_days": staleDaysCfg,
			"warranty_notice": warnDays, "accept_notice": acceptDays,
		},
	})
}

// ---------- 提醒 ----------

func (s *Server) hReminderList(w http.ResponseWriter, r *http.Request) {
	onlyUnread := r.URL.Query().Get("unread")
	sqlStr := `SELECT * FROM reminders WHERE is_done=0`
	if onlyUnread == "1" {
		sqlStr += ` AND is_read=0`
	}
	sqlStr += ` ORDER BY is_read, remind_at DESC, id DESC LIMIT 300`
	list, err := s.st.QueryMaps(sqlStr)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	// 附带项目名
	for _, m := range list {
		if store.Pick(m, "obj_type") == "project" {
			if p, err := s.st.QueryRowMap(`SELECT name, code FROM projects WHERE id=?`, store.PickInt(m, "obj_id")); err == nil {
				m["project_name"] = store.Pick(p, "name")
				m["project_code"] = store.Pick(p, "code")
			}
		}
	}
	ok(w, list)
}

func (s *Server) hReminderUpdate(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("all") == "1" {
		_, _ = s.st.Exec(`UPDATE reminders SET is_read=1 WHERE is_done=0`)
		ok(w, true)
		return
	}
	id, okid := reqID(w, r, "id")
	if !okid {
		return
	}
	var body map[string]any
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	if err := s.st.Update("reminders", id, body, []string{"is_read", "is_done"}); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ok(w, true)
}

func (s *Server) addReminder(rtype, objType string, objID int64, title, content, dedupe string, at time.Time) {
	_, _ = s.st.Exec(`INSERT INTO reminders(rtype,obj_type,obj_id,title,content,remind_at,is_read,is_done,dedupe_key)
		VALUES(?,?,?,?,?,?,0,0,?) ON CONFLICT(dedupe_key) DO NOTHING`,
		rtype, objType, objID, title, content, at.Format("2006-01-02 15:04:05"), dedupe)
}

// hReminderScan 扫描并生成提醒（幂等）
func (s *Server) hReminderScan(w http.ResponseWriter, r *http.Request) {
	warnDays := s.settingInt("warranty_notice", 60)
	staleDaysCfg := s.settingInt("stale_days", 7)
	acceptDays := s.settingInt("accept_notice", 3)
	longrunDays := s.settingInt("longrun_days", 14)
	now := time.Now()
	iyear, iweek := now.ISOWeek()
	week := fmt.Sprintf("%d-W%02d", iyear, iweek)
	created := 0

	// 维保到期
	mtRows, _ := s.st.QueryMaps(`SELECT m.*, p.name AS project_name, c.name AS customer_name,
		CAST(julianday(m.expire_date) - julianday(date('now','localtime')) AS INTEGER) AS days_left
		FROM maintenance m JOIN projects p ON p.id=m.project_id JOIN customers c ON c.id=p.customer_id
		WHERE m.expire_date IS NOT NULL AND m.expire_date<>''`)
	for _, m := range mtRows {
		dl := store.PickInt(m, "days_left")
		if dl > warnDays {
			continue
		}
		content := fmt.Sprintf("客户「%s」项目「%s」维保将于 %s 到期（剩余 %d 天）",
			store.Pick(m, "customer_name"), store.Pick(m, "project_name"), store.Pick(m, "expire_date"), dl)
		if dl < 0 {
			content = fmt.Sprintf("客户「%s」项目「%s」维保已于 %s 过期 %d 天",
				store.Pick(m, "customer_name"), store.Pick(m, "project_name"), store.Pick(m, "expire_date"), -dl)
		}
		s.addReminder("维保到期", "project", int64(store.PickInt(m, "project_id")),
			"维保到期提醒", content, fmt.Sprintf("mt:%d:%s", store.PickInt(m, "project_id"), store.Pick(m, "expire_date")), now)
		created++
	}

	// 长期未更新 / 长时间未结束 / 验收前
	projRows, _ := s.st.QueryMaps(`SELECT p.*, c.name AS customer_name,
		(SELECT MAX(record_date) FROM progress x WHERE x.project_id=p.id) AS last_progress_date,
		CAST(julianday(p.expect_accept_date) - julianday(date('now','localtime')) AS INTEGER) AS days_to_accept
		FROM projects p JOIN customers c ON c.id=p.customer_id`)
	for _, p := range projRows {
		pid := int64(store.PickInt(p, "id"))
		st := store.Pick(p, "status")
		ended := st == "已结束" || st == "取消" || st == "已验收"

		if !ended && staleDays(p) > staleDaysCfg {
			s.addReminder("长期未更新", "project", pid, "项目长期未更新",
				fmt.Sprintf("项目「%s」已 %d 天没有新的进度记录", store.Pick(p, "name"), staleDays(p)),
				fmt.Sprintf("stale:%d:%s", pid, week), now)
			created++
		}
		if !ended && store.Pick(p, "ptype") != "维保" {
			start := strings.TrimSpace(store.Pick(p, "start_date"))
			if start == "" {
				start = strings.TrimSpace(store.Pick(p, "created_at"))
			}
			if t, err := parseDate(start); err == nil {
				if days := int(time.Since(t).Hours() / 24); days > longrunDays {
					s.addReminder("长时间未结束", "project", pid, "项目长时间未结束",
						fmt.Sprintf("项目「%s」已进行 %d 天（阈值 %d 天）", store.Pick(p, "name"), days, longrunDays),
						fmt.Sprintf("longrun:%d:%s", pid, week), now)
					created++
				}
			}
		}
		if !ended {
			if ed := strings.TrimSpace(store.Pick(p, "expect_accept_date")); ed != "" {
				if dl := store.PickInt(p, "days_to_accept"); dl <= acceptDays {
					msg := fmt.Sprintf("项目「%s」预计 %s 验收，剩余 %d 天", store.Pick(p, "name"), ed, dl)
					if dl < 0 {
						msg = fmt.Sprintf("项目「%s」预计验收日 %s 已逾期 %d 天", store.Pick(p, "name"), ed, -dl)
					}
					s.addReminder("验收提醒", "project", pid, "验收前提醒", msg,
						fmt.Sprintf("accept:%d:%s", pid, ed), now)
					created++
				}
			}
		}
	}
	ok(w, map[string]any{"scanned": created})
}

// ---------- 全局搜索 ----------

func (s *Server) hSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		ok(w, map[string]any{})
		return
	}
	like := "%" + q + "%"
	customers, _ := s.st.QueryMaps(`SELECT id,name,code,'customer' AS kind FROM customers
		WHERE name LIKE ? OR code LIKE ? OR remark LIKE ? LIMIT 20`, like, like, like)
	projects, _ := s.st.QueryMaps(`SELECT p.id,p.name,p.code,p.ptype,p.status,'project' AS kind,c.name AS customer_name
		FROM projects p JOIN customers c ON c.id=p.customer_id
		WHERE p.name LIKE ? OR p.code LIKE ? OR c.name LIKE ? OR p.remark LIKE ? LIMIT 30`, like, like, like, like)
	progress, _ := s.st.QueryMaps(`SELECT g.id,g.record_date,g.current_progress,g.project_id,'progress' AS kind,p.name AS project_name
		FROM progress g JOIN projects p ON p.id=g.project_id
		WHERE g.current_progress LIKE ? OR g.next_plan LIKE ? OR g.problems LIKE ? LIMIT 30`, like, like, like)
	issues, _ := s.st.QueryMaps(`SELECT i.id,i.title,i.status,i.level,'issue' AS kind,p.name AS project_name,i.project_id
		FROM issues i JOIN projects p ON p.id=i.project_id
		WHERE i.title LIKE ? OR i.description LIKE ? LIMIT 30`, like, like)
	atts, _ := s.st.QueryMaps(`SELECT a.id,a.file_name,a.atype,'attachment' AS kind,p.name AS project_name,a.project_id
		FROM attachments a JOIN projects p ON p.id=a.project_id WHERE a.file_name LIKE ? LIMIT 30`, like)
	devices, _ := s.st.QueryMaps(`SELECT d.id,d.model,d.serial_no,'device' AS kind,c.name AS customer_name,d.customer_id
		FROM devices d JOIN customers c ON c.id=d.customer_id
		WHERE d.model LIKE ? OR d.serial_no LIKE ? OR d.network_info LIKE ? LIMIT 20`, like, like, like)
	ok(w, map[string]any{
		"customers": customers, "projects": projects, "progress": progress,
		"issues": issues, "attachments": atts, "devices": devices,
		"total": len(customers) + len(projects) + len(progress) + len(issues) + len(atts) + len(devices),
	})
}
