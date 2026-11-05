/* 维保到期 / 提醒中心 / 导出中心 / 系统与备份设置 */
window.PG_MISC = window.PG_MISC || {};

/* ---------------- 维保到期清单 ---------------- */
PG_MISC.maintenance = async function () {
  const view = document.getElementById('view');
  const list = await API.get('/api/maintenance');
  const stat = { ok: 0, soon: 0, over: 0 };
  list.forEach(m => {
    const dl = Number(m.days_left);
    if (dl < 0) stat.over++;
    else if (dl <= 60) stat.soon++;
    else stat.ok++;
  });
  const rows = list.map(m => {
    const dl = Number(m.days_left);
    const cls = dl < 0 ? 'red' : dl <= 30 ? 'orange' : dl <= 60 ? 'orange' : 'gray';
    return `<tr>
      <td class="nowrap">${UI.v(m.project_code)}</td>
      <td><a href="#/projects/${m.project_id}?tab=maintenance">${UI.esc(m.project_name)}</a></td>
      <td class="nowrap">${UI.esc(m.customer_name)}</td>
      <td class="nowrap">${UI.fmtDate(m.start_date)}</td>
      <td class="nowrap">${UI.fmtDate(m.expire_date)}</td>
      <td class="nowrap"><span class="badge ${cls}">${isNaN(dl) ? '—' : (dl < 0 ? '已过保 ' + (-dl) + ' 天' : '剩 ' + dl + ' 天')}</span></td>
      <td class="nowrap">${UI.statusBadge(m.status)}${Number(m.manual_override) ? ' <span class="badge gray">手动</span>' : ''}</td>
      <td class="nowrap">${UI.v(m.cycle_months)} 月</td>
      <td class="pre" style="max-width:220px">${UI.v((m.maint_record || '').slice(0, 40))}</td>
      <td class="nowrap"><button class="btn link" onclick="location.hash='#/projects/${m.project_id}?tab=maintenance'">维护</button></td>
    </tr>`;
  }).join('');
  view.innerHTML = UI.pageHead('维保到期清单', `共 ${list.length} 个维保项目`, 
    `<button class="btn" onclick="API.download('/api/export/excel?type=maintenance')">导出 Excel</button>
     <button class="btn" onclick="PG_MISC.scanReminders()">扫描到期提醒</button>`) +
    `<div class="stat-grid">
      <div class="stat green"><div class="n">${stat.ok}</div><div class="l">在保（60 天以上）</div></div>
      <div class="stat orange"><div class="n">${stat.soon}</div><div class="l">即将过保（60 天内）</div></div>
      <div class="stat red"><div class="n">${stat.over}</div><div class="l">已过保</div></div>
    </div>
    <div class="card"><div class="body tight"><table class="tbl">
      <tr><th>编号</th><th>项目</th><th>客户</th><th>起始日</th><th>到期日</th><th>剩余</th><th>状态</th><th>周期</th><th>维保记录</th><th>操作</th></tr>
      ${rows || '<tr><td class="empty" colspan="10">暂无维保项目</td></tr>'}
    </table></div></div>`;
};

/* ---------------- 提醒中心 ---------------- */
PG_MISC.reminders = async function () {
  const view = document.getElementById('view');
  const list = await API.get('/api/reminders');
  const typeCls = { '维保到期': 'red', '长期未更新': 'orange', '验收提醒': 'blue', '长时间未结束': 'orange' };
  const rows = list.map(r => `
    <tr style="${Number(r.is_read) ? 'opacity:.62' : ''}">
      <td class="nowrap"><span class="badge ${typeCls[r.rtype] || 'gray'}">${UI.esc(r.rtype)}</span></td>
      <td><strong>${UI.esc(r.title)}</strong></td>
      <td class="pre">${UI.esc(r.content)}</td>
      <td class="nowrap">${r.obj_type === 'project' ? `<a href="#/projects/${r.obj_id}">${UI.esc(r.project_name || '')}</a>` : '—'}</td>
      <td class="nowrap muted">${UI.fmtTime(r.remind_at)}</td>
      <td class="nowrap">${Number(r.is_read) ? '<span class="badge gray">已读</span>' : '<span class="badge blue">未读</span>'}</td>
      <td class="nowrap">
        ${Number(r.is_read) ? '' : `<button class="btn link" onclick="PG_MISC.readReminder(${r.id})">标记已读</button>`}
        <button class="btn link" onclick="PG_MISC.doneReminder(${r.id})">忽略</button>
      </td>
    </tr>`).join('');
  view.innerHTML = UI.pageHead('提醒中心', '软件内提醒 · 维保到期 60 天 / 长期未更新 7 天 / 验收前 3 天 / 超 14 天未结束',
    `<button class="btn" id="allread">全部已读</button>
     <button class="btn primary" onclick="PG_MISC.scanReminders()">立即扫描</button>`) +
    `<div class="card"><div class="body tight"><table class="tbl">
      <tr><th>类型</th><th>标题</th><th>内容</th><th>关联项目</th><th>提醒时间</th><th>状态</th><th>操作</th></tr>
      ${rows || '<tr><td class="empty" colspan="7">暂无提醒，点击「立即扫描」按规则生成</td></tr>'}
    </table></div></div>`;
  document.getElementById('allread').onclick = async () => {
    await API.put('/api/reminders/0?all=1', {});
    App.refreshUnread();
    App.reload();
  };
};

PG_MISC.readReminder = async function (id) {
  await API.put('/api/reminders/' + id, { is_read: 1 });
  App.refreshUnread(); App.reload();
};
PG_MISC.doneReminder = async function (id) {
  await API.put('/api/reminders/' + id, { is_done: 1, is_read: 1 });
  App.refreshUnread(); App.reload();
};

/* ---------------- 导出中心 ---------------- */
PG_MISC.exportCenter = async function () {
  const view = document.getElementById('view');
  const [projects, customers, logs] = await Promise.all([
    API.get('/api/projects'), API.get('/api/customers'), API.get('/api/export/logs')
  ]);
  const projOpts = projects.map(p => `<option value="${p.id}">${UI.esc((p.code ? p.code + ' ' : '') + p.name)}</option>`).join('');
  const custOpts = customers.map(c => `<option value="${c.id}">${UI.esc(c.name)}</option>`).join('');
  const xl = [
    ['projects', '项目清单'], ['progress', '进度记录'], ['network', '客户网络信息'],
    ['devices', '设备清单'], ['issues', '问题清单'], ['maintenance', '维保到期清单']
  ];
  const logRows = logs.map(l => `<tr>
    <td class="nowrap">${UI.fmtTime(l.created_at)}</td>
    <td class="nowrap">${UI.esc(l.export_type)}</td>
    <td class="nowrap">${UI.esc(l.format)}</td>
    <td class="nowrap">${UI.esc(l.file_path)}</td>
    <td>${UI.esc(l.filters || '')}</td>
  </tr>`).join('');

  view.innerHTML = UI.pageHead('导出中心', 'Excel 六类清单 + PDF 四类模板') +
    `<div class="dash-grid">
      <div class="card"><header><h3>Excel 导出</h3><div class="right">
        <button class="btn sm primary" onclick="API.download('/api/export/excel?type=all')">一键导出全部 6 张表</button></div></header>
        <div class="body">
          <div class="flex wrap">${xl.map(([k, label]) =>
    `<button class="btn" onclick="API.download('/api/export/excel?type=${k}')">${label}</button>`).join('')}</div>
          <div class="sep"></div>
          <div class="muted">导出的网络信息、设备清单中账号密码为明文（按需求要求）。</div>
        </div></div>
      <div class="card"><header><h3>PDF 导出</h3></header><div class="body">
        <div class="field mb8"><label>项目周报（按周汇总本周进度）</label>
          <div class="flex"><select id="d-week" style="flex:1">${projOpts}</select>
          <button class="btn" onclick="API.download('/api/export/pdf?type=weekly&project_id='+document.getElementById('d-week').value)">导出</button></div></div>
        <div class="field mb8"><label>验收报告（导出已上传的验收单附件）</label>
          <div class="flex"><select id="d-acc" style="flex:1">${projOpts}</select>
          <button class="btn" onclick="API.download('/api/export/pdf?type=acceptance&project_id='+document.getElementById('d-acc').value)">导出</button></div></div>
        <div class="field mb8"><label>客户网络信息表</label>
          <div class="flex"><select id="d-netp" style="flex:1"><option value="0">按客户导出</option>${projOpts}</select>
          <button class="btn" onclick="API.download('/api/export/pdf?type=network&project_id='+document.getElementById('d-netp').value)">导出</button></div></div>
        <div class="field mb8"><label>维保报告</label>
          <div class="flex"><select id="d-mt" style="flex:1">${projOpts}</select>
          <button class="btn" onclick="API.download('/api/export/pdf?type=maintenance&project_id='+document.getElementById('d-mt').value)">导出</button></div></div>
      </div></div>
    </div>
    <div class="card"><header><h3>导出记录</h3></header><div class="body tight"><table class="tbl">
      <tr><th>时间</th><th>类型</th><th>格式</th><th>文件名</th><th>筛选条件</th></tr>
      ${logRows || '<tr><td class="empty" colspan="5">暂无导出记录</td></tr>'}
    </table></div></div>`;
};

/* ---------------- 系统设置 ---------------- */
PG_MISC.settings = async function (tab) {
  tab = tab || 'system';
  const view = document.getElementById('view');
  const [st, bc, bl] = await Promise.all([
    API.get('/api/settings'), API.get('/api/backup/config'), API.get('/api/backup/list')
  ]);
  const tabs = [
    { key: 'system', label: '系统设置', href: '#/settings' },
    { key: 'backup', label: '备份设置', href: '#/settings?tab=backup' },
    { key: 'account', label: '账号安全', href: '#/settings?tab=account' }
  ];
  let body = '';
  if (tab === 'system') {
    body = `<div class="card"><header><h3>系统参数</h3></header><div class="body">
      <div class="form-grid">
        ${UI.fieldHTML({ k: 'app_name', label: '软件名称' }, st.app_name)}
        ${UI.fieldHTML({ k: 'app_subtitle', label: '副标题' }, st.app_subtitle)}
        ${UI.fieldHTML({ k: 'max_attach_mb', label: '单个附件上限（MB）', type: 'number', hint: '默认 300' }, st.max_attach_mb)}
        ${UI.fieldHTML({ k: 'warranty_notice', label: '维保到期提前提醒（天）', type: 'number' }, st.warranty_notice)}
        ${UI.fieldHTML({ k: 'stale_days', label: '长期未更新阈值（天）', type: 'number' }, st.stale_days)}
        ${UI.fieldHTML({ k: 'accept_notice', label: '验收前提醒（天）', type: 'number' }, st.accept_notice)}
        ${UI.fieldHTML({ k: 'longrun_days', label: '长时间未结束阈值（天）', type: 'number' }, st.longrun_days)}
        ${UI.fieldHTML({ k: 'data_dir', label: '数据目录（系统固定，不可修改）', readonly: true }, st.data_dir)}
      </div>
      <div class="sep"></div>
      <button class="btn primary" id="ssave">保存设置</button>
    </div></div>`;
  } else if (tab === 'backup') {
    body = `<div class="card"><header><h3>自动备份</h3><div class="right">
        <button class="btn sm primary" id="runbackup">立即备份</button></div></header><div class="body">
      <div class="form-grid">
        ${UI.fieldHTML({
      k: 'enabled', label: '启用自动备份', type: 'checkbox', checkLabel: '按频率自动执行'
    }, bc.enabled === undefined ? 1 : bc.enabled)}
        ${UI.fieldHTML({
      k: 'target_type', label: '备份目标', type: 'select', empty: false,
      options: [{ value: 'local', label: '本地目录' }, { value: 's3', label: 'S3 对象存储（后续版本）' }]
    }, bc.target_type || 'local')}
        ${UI.fieldHTML({ k: 'local_path', label: '本地备份目录', hint: '留空使用默认目录：' + bl.dir }, bc.local_path)}
        ${UI.fieldHTML({
      k: 'frequency', label: '备份频率', type: 'select', empty: false,
      options: [{ value: 'hourly', label: '每 N 小时' }, { value: 'daily', label: '每 N 天' }, { value: 'weekly', label: '每 N 周' }, { value: 'monthly', label: '每 N 月' }]
    }, bc.frequency || 'daily')}
        ${UI.fieldHTML({ k: 'frequency_value', label: '频率数值', type: 'number' }, bc.frequency_value || 1)}
        ${UI.fieldHTML({ k: 'keep_count', label: '保留份数', type: 'number' }, bc.keep_count || 10)}
        ${UI.fieldHTML({ k: 'include_attachments', label: '备份内容', type: 'checkbox', checkLabel: '包含附件文件（数据库始终备份）' }, bc.include_attachments)}
        ${UI.fieldHTML({ k: 'next_backup_at', label: '下次备份时间', readonly: true }, bc.next_backup_at)}
      </div>
      <div class="sep"></div>
      <div class="muted mb8">S3 配置（第一版仅保留参数，云端备份在后续迭代启用）：</div>
      <div class="form-grid">
        ${UI.fieldHTML({ k: 's3_endpoint', label: 'Endpoint' }, bc.s3_endpoint)}
        ${UI.fieldHTML({ k: 's3_bucket', label: 'Bucket' }, bc.s3_bucket)}
        ${UI.fieldHTML({ k: 's3_region', label: 'Region' }, bc.s3_region)}
        ${UI.fieldHTML({ k: 's3_prefix', label: '前缀目录' }, bc.s3_prefix)}
        ${UI.fieldHTML({ k: 's3_access_key', label: 'Access Key' }, bc.s3_access_key)}
        ${UI.fieldHTML({ k: 's3_secret_key', label: 'Secret Key', type: 'password' }, bc.s3_secret_key)}
      </div>
      <div class="sep"></div>
      <button class="btn primary" id="bsave">保存备份配置</button>
    </div></div>
    <div class="card"><header><h3>备份文件</h3><div class="right muted">${UI.esc(bl.dir)}</div></header>
      <div class="body tight"><table class="tbl"><tr><th>文件名</th><th>大小</th><th>时间</th><th>操作</th></tr>
      ${(bl.backups || []).map(b => `<tr><td class="mono">${UI.esc(b.name)}</td><td class="nowrap">${UI.fmtSize(b.size)}</td>
        <td class="nowrap">${UI.esc(b.created_at)}</td>
        <td><a class="btn link" href="/api/backup/download?name=${encodeURIComponent(b.name)}">下载</a></td></tr>`).join('')
      || '<tr><td class="empty" colspan="4">暂无备份文件</td></tr>'}
      </table></div></div>`;
  } else {
    body = `<div class="card"><header><h3>账号安全</h3></header><div class="body">
      <div class="kv"><div class="item"><div class="k">当前用户</div><div class="v big">${UI.esc(App.state.user)}</div></div></div>
      <div class="sep"></div>
      <div class="flex"><button class="btn" onclick="App.changePassword()">修改密码</button>
      <button class="btn danger" id="out">退出登录</button></div>
      <div class="sep"></div>
      <div class="muted">忘记密码时：退出后在登录页点击「忘记密码」，系统会生成一次性重置令牌（同时写入数据目录 RESET_TOKEN.txt）。</div>
    </div></div>`;
  }

  view.innerHTML = UI.pageHead('系统 / 备份', '软件名称、提醒阈值、自动备份与账号', '') +
    UI.tabs(tabs, tab) + body;

  if (tab === 'system') {
    document.getElementById('ssave').onclick = async () => {
      const scope = document.getElementById('view');
      const keys = ['app_name', 'app_subtitle', 'max_attach_mb', 'warranty_notice', 'stale_days', 'accept_notice', 'longrun_days'];
      const data = UI.collect(scope, keys.map(k => ({ k })));
      await API.put('/api/settings', data);
      UI.toast('设置已保存', 'ok');
      App.state.appName = data.app_name;
      App.loadMeta();
    };
  } else if (tab === 'backup') {
    document.getElementById('bsave').onclick = async () => {
      const scope = document.getElementById('view');
      const keys = ['enabled', 'target_type', 'local_path', 'frequency', 'frequency_value', 'keep_count', 'include_attachments',
        's3_endpoint', 's3_bucket', 's3_region', 's3_prefix', 's3_access_key', 's3_secret_key'];
      const data = UI.collect(scope, keys.map(k => ({ k, numeric: ['frequency_value', 'keep_count'].includes(k) })));
      await API.put('/api/backup/config', data);
      UI.toast('备份配置已保存', 'ok');
      App.reload();
    };
    document.getElementById('runbackup').onclick = async e => {
      const btn = e.target;
      btn.disabled = true; btn.textContent = '备份中…';
      try {
        const r = await API.post('/api/backup/run');
        UI.toast('备份完成：' + r.file.split(/[\\/]/).pop(), 'ok');
        App.reload();
      } catch (err) { btn.disabled = false; btn.textContent = '立即备份'; }
    };
  } else {
    document.getElementById('out').onclick = async () => {
      await API.post('/api/logout');
      location.hash = '';
      App.boot();
    };
  }
};
