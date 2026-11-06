package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Store 封装 SQLite 与固定数据目录。
type Store struct {
	DB      *sql.DB
	DataDir string
	Files   string
	Backups string
}

// DataDir 返回系统固定的数据目录，不向用户开放选择（仅保留环境变量用于测试/运维）。
func DataDir() string {
	if v := os.Getenv("PM_DATA_DIR"); v != "" {
		return v
	}
	if runtime.GOOS == "windows" {
		base := os.Getenv("APPDATA")
		if base == "" {
			base = "."
		}
		return filepath.Join(base, "SangforPM")
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".sangfor-pm")
	}
	// 极端情况：容器 / systemd 里没有 HOME，退回 XDG 或当前目录
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "sangfor-pm")
	}
	return filepath.Join(".", ".sangfor-pm")
}

// Open 使用默认数据目录打开数据库。
func Open() (*Store, error) { return OpenAt("") }

// OpenAt 打开指定目录的数据库；dir 为空时使用 DataDir()。
func OpenAt(dir string) (*Store, error) {
	if strings.TrimSpace(dir) == "" {
		dir = DataDir()
	}
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	s := &Store{
		DataDir: dir,
		Files:   filepath.Join(dir, "files"),
		Backups: filepath.Join(dir, "backups"),
	}
	// 数据库里有明文设备密码，非 Windows 下把数据目录收成仅本人可读。
	dirMode := os.FileMode(0o755)
	if runtime.GOOS != "windows" {
		dirMode = 0o700
	}
	for _, d := range []string{dir, s.Files, s.Backups, filepath.Join(s.Files, "tmp"), filepath.Join(dir, "tmp")} {
		if err := os.MkdirAll(d, dirMode); err != nil {
			return nil, err
		}
	}
	if runtime.GOOS != "windows" {
		_ = os.Chmod(dir, dirMode)
	}
	dsn := "file:" + filepath.ToSlash(filepath.Join(dir, "data.db")) +
		"?_pragma=busy_timeout(8000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schemaSQL); err != nil {
		return nil, fmt.Errorf("初始化数据库失败: %w", err)
	}
	s.DB = db
	if err := s.seed(); err != nil {
		return nil, err
	}
	return s, nil
}

// Close 关闭数据库连接（用于优雅退出）。
func (s *Store) Close() error {
	if s == nil || s.DB == nil {
		return nil
	}
	return s.DB.Close()
}

func (s *Store) seed() error {
	defaults := map[string]string{
		"app_name":         "项目管理",
		"app_subtitle":     "深信服实施交付个人工作台",
		"max_attach_mb":    "300",
		"warranty_notice":  "60",
		"stale_days":       "7",
		"accept_notice":    "3",
		"longrun_days":     "14",
		"issue_high_first": "1",
	}
	for k, v := range defaults {
		if _, err := s.DB.Exec(`INSERT INTO settings(key,value) VALUES(?,?)
			ON CONFLICT(key) DO NOTHING`, k, v); err != nil {
			return err
		}
	}
	_, err := s.DB.Exec(`INSERT INTO backup_config(id) VALUES(1) ON CONFLICT(id) DO NOTHING`)
	return err
}

func Now() string { return time.Now().Format("2006-01-02 15:04:05") }
func Today() string { return time.Now().Format("2006-01-02") }

// ---------- 通用查询 ----------

// QueryMaps 执行查询并把结果转为 map 切片，便于直接 JSON 输出。
func (s *Store) QueryMaps(q string, args ...any) ([]map[string]any, error) {
	rows, err := s.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, _ := rows.Columns()
	out := make([]map[string]any, 0)
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		m := make(map[string]any, len(cols))
		for i, c := range cols {
			switch v := vals[i].(type) {
			case []byte:
				m[c] = string(v)
			default:
				m[c] = v
			}
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) QueryRowMap(q string, args ...any) (map[string]any, error) {
	list, err := s.QueryMaps(q, args...)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, sql.ErrNoRows
	}
	return list[0], nil
}

// ---------- 通用写入（白名单字段，杜绝 SQL 注入） ----------

func (s *Store) Insert(table string, data map[string]any, allowed []string) (int64, error) {
	cols := make([]string, 0, len(allowed))
	phs := make([]string, 0, len(allowed))
	args := make([]any, 0, len(allowed))
	for _, k := range allowed {
		if v, ok := data[k]; ok {
			cols = append(cols, `"`+k+`"`)
			phs = append(phs, "?")
			args = append(args, norm(v))
		}
	}
	if len(cols) == 0 {
		return 0, fmt.Errorf("没有可写入的字段")
	}
	q := fmt.Sprintf(`INSERT INTO %s (%s) VALUES (%s)`, table, strings.Join(cols, ","), strings.Join(phs, ","))
	res, err := s.DB.Exec(q, args...)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) Update(table string, id int64, data map[string]any, allowed []string) error {
	sets := make([]string, 0, len(allowed))
	args := make([]any, 0, len(allowed)+1)
	for _, k := range allowed {
		if v, ok := data[k]; ok {
			sets = append(sets, `"`+k+`"=?`)
			args = append(args, norm(v))
		}
	}
	if len(sets) == 0 {
		return fmt.Errorf("没有可更新的字段")
	}
	args = append(args, id)
	_, err := s.DB.Exec(fmt.Sprintf(`UPDATE %s SET %s WHERE id=?`, table, strings.Join(sets, ",")), args...)
	return err
}

func (s *Store) Delete(table string, id int64) error {
	_, err := s.DB.Exec(fmt.Sprintf(`DELETE FROM %s WHERE id=?`, table), id)
	return err
}

func (s *Store) Exec(q string, args ...any) (sql.Result, error) { return s.DB.Exec(q, args...) }

func (s *Store) GetSetting(key string) string {
	m, err := s.QueryRowMap(`SELECT value FROM settings WHERE key=?`, key)
	if err != nil {
		return ""
	}
	if v, ok := m["value"].(string); ok {
		return v
	}
	return ""
}

func (s *Store) SetSetting(key, value string) error {
	_, err := s.DB.Exec(`INSERT INTO settings(key,value) VALUES(?,?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

// Pick 从 map 中取字符串
func Pick(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case []byte:
		return string(t)
	default:
		return fmt.Sprint(t)
	}
}

func PickInt(m map[string]any, key string) int {
	v := Pick(m, key)
	if v == "" {
		return 0
	}
	var n int
	fmt.Sscanf(v, "%d", &n)
	return n
}

func norm(v any) any {
	switch t := v.(type) {
	case bool:
		if t {
			return 1
		}
		return 0
	case nil:
		return nil
	default:
		return v
	}
}
