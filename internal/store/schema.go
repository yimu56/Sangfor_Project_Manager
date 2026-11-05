package store

// schemaSQL 为第一版完整数据模型（对应需求"数据模型初稿"1~25 项）。
// 所有表使用 IF NOT EXISTS，可在已有库上重复执行。
const schemaSQL = `
CREATE TABLE IF NOT EXISTS users (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	username     TEXT NOT NULL UNIQUE,
	password_hash TEXT NOT NULL,
	reset_token  TEXT,
	reset_expire TEXT,
	created_at   TEXT NOT NULL,
	updated_at   TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS settings (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS customers (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	code       TEXT DEFAULT '',
	name       TEXT NOT NULL,
	remark     TEXT DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS contacts (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	customer_id INTEGER NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
	name        TEXT NOT NULL,
	phone       TEXT DEFAULT '',
	email       TEXT DEFAULT '',
	role        TEXT DEFAULT '',
	remark      TEXT DEFAULT '',
	created_at  TEXT NOT NULL,
	updated_at  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_contacts_cust ON contacts(customer_id);

CREATE TABLE IF NOT EXISTS net_versions (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	customer_id INTEGER NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
	version_no  INTEGER NOT NULL,
	content     TEXT DEFAULT '',
	is_current  INTEGER DEFAULT 0,
	created_by  TEXT DEFAULT '',
	created_at  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_netver_cust ON net_versions(customer_id);

CREATE TABLE IF NOT EXISTS net_changes (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	customer_id INTEGER NOT NULL,
	version_id  INTEGER NOT NULL,
	field_name  TEXT NOT NULL,
	old_value   TEXT DEFAULT '',
	new_value   TEXT DEFAULT '',
	action      TEXT DEFAULT 'update',
	changed_at  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_netchg_ver ON net_changes(version_id);

CREATE TABLE IF NOT EXISTS devices (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	customer_id   INTEGER NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
	contact_id    INTEGER,
	model         TEXT DEFAULT '',
	serial_no     TEXT DEFAULT '',
	account       TEXT DEFAULT '',
	topology      TEXT DEFAULT '[]',
	network_info  TEXT DEFAULT '',
	remark        TEXT DEFAULT '',
	created_at    TEXT NOT NULL,
	updated_at    TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_dev_cust ON devices(customer_id);

CREATE TABLE IF NOT EXISTS device_changes (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	device_id  INTEGER NOT NULL,
	field_name TEXT NOT NULL,
	old_value  TEXT DEFAULT '',
	new_value  TEXT DEFAULT '',
	action     TEXT DEFAULT 'update',
	changed_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_devchg_dev ON device_changes(device_id);

CREATE TABLE IF NOT EXISTS projects (
	id                 INTEGER PRIMARY KEY AUTOINCREMENT,
	code               TEXT DEFAULT '',
	name               TEXT NOT NULL,
	customer_id        INTEGER NOT NULL REFERENCES customers(id),
	ptype              TEXT NOT NULL,
	status             TEXT NOT NULL DEFAULT '未开始',
	priority           TEXT NOT NULL DEFAULT '中',
	start_date         TEXT,
	expect_accept_date TEXT,
	accept_date        TEXT,
	acceptor           TEXT DEFAULT '',
	legacy_issues      TEXT DEFAULT '',
	remark             TEXT DEFAULT '',
	created_at         TEXT NOT NULL,
	updated_at         TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_proj_cust ON projects(customer_id);
CREATE INDEX IF NOT EXISTS idx_proj_type ON projects(ptype);

CREATE TABLE IF NOT EXISTS project_products (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	project_id   INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
	product_line TEXT NOT NULL,
	product_name TEXT DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_prod_proj ON project_products(project_id);

CREATE TABLE IF NOT EXISTS project_stages (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
	name       TEXT NOT NULL,
	seq        INTEGER NOT NULL DEFAULT 0,
	status     TEXT NOT NULL DEFAULT '未开始',
	start_at   TEXT,
	done_at    TEXT
);
CREATE INDEX IF NOT EXISTS idx_stage_proj ON project_stages(project_id);

CREATE TABLE IF NOT EXISTS project_relations (
	id                 INTEGER PRIMARY KEY AUTOINCREMENT,
	project_id         INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
	related_project_id INTEGER NOT NULL,
	rel_type           TEXT NOT NULL,
	created_at         TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_rel_proj ON project_relations(project_id);

CREATE TABLE IF NOT EXISTS project_devices (
	id             INTEGER PRIMARY KEY AUTOINCREMENT,
	project_id     INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
	source_device_id INTEGER,
	contact_id     INTEGER,
	model          TEXT DEFAULT '',
	serial_no      TEXT DEFAULT '',
	account        TEXT DEFAULT '',
	topology       TEXT DEFAULT '[]',
	network_info   TEXT DEFAULT '',
	remark         TEXT DEFAULT '',
	snapshot_version INTEGER DEFAULT 1,
	created_at     TEXT NOT NULL,
	updated_at     TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_pdev_proj ON project_devices(project_id);

CREATE TABLE IF NOT EXISTS project_device_changes (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	snapshot_id INTEGER NOT NULL,
	field_name TEXT NOT NULL,
	old_value  TEXT DEFAULT '',
	new_value  TEXT DEFAULT '',
	action     TEXT DEFAULT 'update',
	changed_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_pdevchg ON project_device_changes(snapshot_id);

CREATE TABLE IF NOT EXISTS project_net_refs (
	id             INTEGER PRIMARY KEY AUTOINCREMENT,
	project_id     INTEGER NOT NULL UNIQUE REFERENCES projects(id) ON DELETE CASCADE,
	net_version_id INTEGER NOT NULL,
	ref_at         TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS attachments (
	id                   INTEGER PRIMARY KEY AUTOINCREMENT,
	project_id           INTEGER REFERENCES projects(id) ON DELETE CASCADE,
	atype                TEXT NOT NULL DEFAULT '其他',
	file_name            TEXT NOT NULL,
	file_size            INTEGER DEFAULT 0,
	version_no           INTEGER DEFAULT 1,
	source_attachment_id INTEGER,
	current_version_id   INTEGER,
	created_at           TEXT NOT NULL,
	updated_at           TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_att_proj ON attachments(project_id);

CREATE TABLE IF NOT EXISTS attachment_versions (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	attachment_id INTEGER NOT NULL REFERENCES attachments(id) ON DELETE CASCADE,
	version_no   INTEGER NOT NULL,
	file_path    TEXT NOT NULL,
	file_name    TEXT DEFAULT '',
	file_size    INTEGER DEFAULT 0,
	note         TEXT DEFAULT '',
	created_at   TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_attver_att ON attachment_versions(attachment_id);

CREATE TABLE IF NOT EXISTS progress (
	id              INTEGER PRIMARY KEY AUTOINCREMENT,
	project_id      INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
	record_date     TEXT NOT NULL,
	current_progress TEXT DEFAULT '',
	problems        TEXT DEFAULT '',
	next_plan       TEXT DEFAULT '',
	info_sync       TEXT DEFAULT '',
	created_at      TEXT NOT NULL,
	updated_at      TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_prog_proj ON progress(project_id);

CREATE TABLE IF NOT EXISTS progress_stages (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	progress_id INTEGER NOT NULL REFERENCES progress(id) ON DELETE CASCADE,
	stage_id   INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_pstage_prog ON progress_stages(progress_id);

CREATE TABLE IF NOT EXISTS progress_attachments (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	progress_id   INTEGER NOT NULL REFERENCES progress(id) ON DELETE CASCADE,
	attachment_id INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_patt_prog ON progress_attachments(progress_id);

CREATE TABLE IF NOT EXISTS issues (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	project_id  INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
	title       TEXT NOT NULL,
	description TEXT DEFAULT '',
	level       TEXT NOT NULL DEFAULT '中',
	owner       TEXT DEFAULT '',
	status      TEXT NOT NULL DEFAULT '未关闭',
	closed_at   TEXT,
	created_at  TEXT NOT NULL,
	updated_at  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_issue_proj ON issues(project_id);

CREATE TABLE IF NOT EXISTS reminders (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	rtype      TEXT NOT NULL,
	obj_type   TEXT NOT NULL,
	obj_id     INTEGER NOT NULL,
	title      TEXT DEFAULT '',
	content    TEXT DEFAULT '',
	remind_at  TEXT NOT NULL,
	is_read    INTEGER DEFAULT 0,
	is_done    INTEGER DEFAULT 0,
	dedupe_key TEXT NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS maintenance (
	id                     INTEGER PRIMARY KEY AUTOINCREMENT,
	project_id             INTEGER NOT NULL UNIQUE REFERENCES projects(id) ON DELETE CASCADE,
	start_date             TEXT,
	expire_date            TEXT,
	cycle_months           INTEGER DEFAULT 12,
	auto_calc              INTEGER DEFAULT 1,
	status                 TEXT DEFAULT '在保',
	auth_attachment_id     INTEGER,
	maint_record           TEXT DEFAULT '',
	accept_attachment_id   INTEGER,
	manual_override        INTEGER DEFAULT 0,
	updated_at             TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS backup_config (
	id                  INTEGER PRIMARY KEY CHECK (id = 1),
	target_type         TEXT NOT NULL DEFAULT 'local',
	local_path          TEXT DEFAULT '',
	s3_endpoint         TEXT DEFAULT '',
	s3_region           TEXT DEFAULT '',
	s3_bucket           TEXT DEFAULT '',
	s3_access_key       TEXT DEFAULT '',
	s3_secret_key       TEXT DEFAULT '',
	s3_prefix           TEXT DEFAULT 'backup/',
	frequency           TEXT NOT NULL DEFAULT 'daily',
	frequency_value     INTEGER DEFAULT 1,
	keep_count          INTEGER DEFAULT 10,
	include_attachments INTEGER DEFAULT 1,
	enabled             INTEGER DEFAULT 1,
	last_backup_at      TEXT,
	next_backup_at      TEXT
);

CREATE TABLE IF NOT EXISTS export_logs (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	export_type TEXT NOT NULL,
	format      TEXT NOT NULL,
	filters     TEXT DEFAULT '',
	file_path   TEXT DEFAULT '',
	created_at  TEXT NOT NULL
);
`
