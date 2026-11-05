/* 进度记录 / 风险问题 */
const PG_REC = {
  /* ---------------- 进度记录（项目内） ---------------- */
  async progressTable(pid, _rows, withActions) {
    const list = await API.get(`/api/projects/${pid}/progress`);
    const stageMap = new Map();
    list.forEach(g => (g.stages || []).forEach(s => { if (s.stage_id && s.name) stageMap.set(s.stage_id, s.name); }));
    const rows = list.map(g => `
      <tr data-sids="${(g.stages || []).map(s => s.stage_id).join(',')}">
        <td class="nowrap">${UI.fmtDate(g.record_date)}</td>
        <td class="nowrap">${(g.stages || []).map(s => `<span class="badge">${UI.esc(s.name)}</span>`).join(' ') || '<span class="muted">—</span>'}</td>
        <td class="pre" style="max-width:320px">${UI.v(g.current_progress)}</td>
        <td class="pre" style="max-width:200px">${UI.v(g.problems)}</td>
        <td class="pre" style="max-width:200px">${UI.v(g.next_plan)}</td>
        <td class="pre" style="max-width:140px">${UI.v(g.info_sync)}</td>
        <td class="nowrap">${(g.attachments || []).map(a => `<a class="badge blue" href="${a.url}" target="_blank" rel="noopener">${UI.esc((a.file_name || '').slice(-12))}</a>`).join(' ') || '—'}</td>
        <td class="nowrap">
          <button class="btn link" onclick="PG_REC.progressForm(${pid},${g.id})">编辑</button>
          <button class="btn link" style="color:var(--danger)" onclick="PG_REC.delProgress(${g.id},${pid})">删除</button>
        </td>
      </tr>`).join('');
    const stageOpts = Array.from(stageMap.entries())
      .map(([id, name]) => `<option value="${id}">${UI.esc(name)}</option>`).join('');
    return UI.tableCard(`进度记录（${list.length} 条）`, `<table class="tbl" id="prog-tbl">
        <tr><th>日期</th><th>阶段</th><th>当前进展</th><th>存在问题</th><th>下一步计划</th><th>信息同步</th><th>附件</th><th>操作</th></tr>
        ${rows || '<tr><td class="empty" colspan="8">暂无进度记录，有进展时记录一条即可</td></tr>'}</table>`,
      `<select id="ps-stage" style="height:26px;border:1px solid var(--border);border-radius:6px;background:var(--panel);color:var(--text);font-size:12px">
         <option value="">按阶段筛选：全部</option>${stageOpts}</select>
       <button class="btn sm primary" onclick="PG_REC.progressForm(${pid})">＋ 新增进度</button>`);
  },

  _afterProgress(pid) {
    const sel = document.getElementById('ps-stage');
    if (!sel) return;
    sel.onchange = () => {
      const v = sel.value;
      document.querySelectorAll('#prog-tbl tbody tr, #prog-tbl tr[data-sids]').forEach(tr => {
        if (!tr.dataset.sids) return;
        const ids = tr.dataset.sids.split(',');
        tr.style.display = (!v || ids.includes(v)) ? '' : 'none';
      });
    };
  },

  async progressForm(pid, id) {
    const [stages, atts, list] = await Promise.all([
      API.get(`/api/projects/${pid}/stages`),
      API.get(`/api/projects/${pid}/attachments`),
      API.get(`/api/projects/${pid}/progress`)
    ]);
    let g = { record_date: UI.today(), stage_ids: [], attachment_ids: [] };
    if (id) {
      const cur = list.find(x => x.id === id);
      if (cur) g = Object.assign({}, cur, {
        stage_ids: (cur.stages || []).map(s => s.stage_id),
        attachment_ids: (cur.attachments || []).map(a => a.id)
      });
    }
    await UI.form({
      title: id ? '编辑进度记录' : '记录项目进展',
      width: 'wide',
      fields: [
        { k: 'record_date', label: '日期', type: 'date', required: true },
        { k: 'stage_ids', label: '关联阶段（可多选）', type: 'checks', full: true, options: stages.map(s => ({ value: s.id, label: s.name })) },
        { k: 'current_progress', label: '当前进展', type: 'textarea', full: true, required: true, rows: 3 },
        { k: 'problems', label: '存在问题', type: 'textarea', full: true },
        { k: 'next_plan', label: '下一步计划', type: 'textarea', full: true, hint: '首页会取最新一条记录的下一步计划，为空显示“未定”' },
        { k: 'info_sync', label: '信息同步（可空）', type: 'textarea', full: true },
        { k: 'attachment_ids', label: '关联附件（从项目附件中选择）', type: 'checks', full: true, options: atts.map(a => ({ value: a.id, label: `[${a.atype}] ${a.file_name}` })) }
      ],
      values: g,
      onSubmit: async d => {
        if (id) await API.put('/api/progress/' + id, d);
        else await API.post(`/api/projects/${pid}/progress`, d);
        UI.toast('已保存', 'ok');
        App.reload();
      }
    });
  },

  async delProgress(id, pid) {
    if (!await UI.confirm('确认删除该进度记录？')) return;
    await API.del('/api/progress/' + id);
    UI.toast('已删除', 'ok'); App.reload();
  },

  /* ---------------- 问题（项目内） ---------------- */
  async issueTable(pid, _rows, withActions) {
    const list = await API.get(`/api/projects/${pid}/issues`);
    const rows = list.map(i => `
      <tr>
        <td>${UI.esc(i.title)}</td>
        <td class="pre" style="max-width:300px">${UI.v(i.description)}</td>
        <td class="nowrap">${UI.levelBadge(i.level)}</td>
        <td class="nowrap">${UI.v(i.owner)}</td>
        <td class="nowrap">${UI.statusBadge(i.status)}</td>
        <td class="nowrap muted">${i.closed_at ? UI.fmtTime(i.closed_at) : '—'}</td>
        <td class="nowrap">
          ${i.status === '未关闭'
        ? `<button class="btn link" onclick="PG_REC.closeIssue(${i.id})">关闭</button>`
        : `<button class="btn link" onclick="PG_REC.reopenIssue(${i.id})">重新打开</button>`}
          <button class="btn link" onclick="PG_REC.issueForm(${pid},${i.id})">编辑</button>
          <button class="btn link" style="color:var(--danger)" onclick="PG_REC.delIssue(${i.id})">删除</button>
        </td>
      </tr>`).join('');
    return UI.tableCard(`风险问题（${list.length} 条）`, `<table class="tbl">
        <tr><th>标题</th><th>描述</th><th>等级</th><th>负责人</th><th>状态</th><th>关闭时间</th><th>操作</th></tr>
        ${rows || '<tr><td class="empty" colspan="7">暂无问题记录</td></tr>'}</table>`,
      `<button class="btn sm primary" onclick="PG_REC.issueForm(${pid})">＋ 新增问题</button>`);
  },

  _afterIssues(pid) { },

  async issueForm(pid, id) {
    let cur = {};
    if (id) {
      const list = await API.get(`/api/projects/${pid}/issues`);
      cur = list.find(x => x.id === id) || {};
    }
    await UI.form({
      title: id ? '编辑风险问题' : '新增风险问题',
      fields: [
        { k: 'title', label: '标题', required: true, full: true },
        { k: 'description', label: '描述', type: 'textarea', full: true },
        { k: 'level', label: '等级', type: 'select', empty: false, options: ['高', '中', '低'] },
        { k: 'owner', label: '负责人' },
        { k: 'status', label: '状态', type: 'select', empty: false, options: ['未关闭', '已关闭'] }
      ],
      values: Object.assign({ level: '中', status: '未关闭' }, cur),
      onSubmit: async d => {
        if (id) await API.put('/api/issues/' + id, d);
        else await API.post(`/api/projects/${pid}/issues`, d);
        UI.toast('已保存', 'ok'); App.reload();
      }
    });
  },

  async closeIssue(id) {
    await API.put('/api/issues/' + id, { status: '已关闭' });
    UI.toast('问题已关闭', 'ok'); App.reload();
  },
  async reopenIssue(id) {
    await API.put('/api/issues/' + id, { status: '未关闭' });
    UI.toast('已重新打开', 'ok'); App.reload();
  },
  async delIssue(id) {
    if (!await UI.confirm('确认删除该问题？')) return;
    await API.del('/api/issues/' + id);
    UI.toast('已删除', 'ok'); App.reload();
  },

  /* ---------------- 全局进度记录页 ---------------- */
  async progressList(params) {
    const view = document.getElementById('view');
    params = params || {};
    const qs = new URLSearchParams();
    ['project_id', 'from', 'to', 'week'].forEach(k => { if (params[k]) qs.set(k, params[k]); });
    const [list, projects] = await Promise.all([
      API.get('/api/progress?' + qs.toString()),
      API.get('/api/projects')
    ]);
    const rows = list.map(g => `
      <tr>
        <td class="nowrap">${UI.fmtDate(g.record_date)}</td>
        <td class="nowrap"><a href="#/projects/${g.project_id}?tab=progress">${UI.esc(g.project_code || '')} ${UI.esc(g.project_name)}</a></td>
        <td class="nowrap">${UI.esc(g.customer_name)}</td>
        <td class="nowrap">${(g.stages || []).map(s => `<span class="badge">${UI.esc(s.name)}</span>`).join(' ') || '—'}</td>
        <td class="pre" style="max-width:300px">${UI.v(g.current_progress)}</td>
        <td class="pre" style="max-width:160px">${UI.v(g.problems)}</td>
        <td class="pre" style="max-width:160px">${UI.v(g.next_plan)}</td>
        <td class="nowrap muted">${UI.v(g.info_sync)}</td>
      </tr>`).join('');
    view.innerHTML = UI.pageHead('进度记录', `共 ${list.length} 条（最多显示 500 条）`,
      `<button class="btn" onclick="API.download('/api/export/excel?type=progress${params.project_id ? '&project_id=' + params.project_id : ''}')">导出 Excel</button>`) +
      `<div class="filters">
        <select id="pf"><option value="">全部项目</option>
          ${projects.map(p => `<option value="${p.id}" ${String(params.project_id) === String(p.id) ? 'selected' : ''}>${UI.esc(p.name)}</option>`).join('')}</select>
        <input type="date" id="pfrom" value="${UI.esc(params.from || '')}" title="开始日期">
        <input type="date" id="pto" value="${UI.esc(params.to || '')}" title="结束日期">
        <button class="btn" id="pgo">查询</button>
        <button class="btn ghost" id="preset">本周</button>
        <button class="btn ghost" id="prst">重置</button>
      </div>
      <div class="card"><div class="body tight"><table class="tbl">
        <tr><th>日期</th><th>项目</th><th>客户</th><th>阶段</th><th>当前进展</th><th>存在问题</th><th>下一步计划</th><th>信息同步</th></tr>
        ${rows || '<tr><td class="empty" colspan="8">暂无进度记录</td></tr>'}
      </table></div></div>`;
    document.getElementById('pgo').onclick = () => {
      const s = new URLSearchParams();
      const v = document.getElementById('pf').value;
      if (v) s.set('project_id', v);
      const f = document.getElementById('pfrom').value, t = document.getElementById('pto').value;
      if (f) s.set('from', f);
      if (t) s.set('to', t);
      location.hash = '#/progress' + (s.toString() ? '?' + s.toString() : '');
    };
    document.getElementById('preset').onclick = () => location.hash = '#/progress?week=' + UI.today();
    document.getElementById('prst').onclick = () => location.hash = '#/progress';
  },

  /* ---------------- 全局问题页 ---------------- */
  async issueList(params) {
    const view = document.getElementById('view');
    params = params || {};
    const qs = new URLSearchParams();
    ['status', 'level', 'project_id', 'q'].forEach(k => { if (params[k]) qs.set(k, params[k]); });
    const [list, projects] = await Promise.all([
      API.get('/api/issues?' + qs.toString()),
      API.get('/api/projects')
    ]);
    const rows = list.map(i => `
      <tr>
        <td class="nowrap">${UI.levelBadge(i.level)}</td>
        <td><a href="#/projects/${i.project_id}?tab=issues">${UI.esc(i.title)}</a></td>
        <td class="pre" style="max-width:320px">${UI.v(i.description)}</td>
        <td class="nowrap">${UI.esc(i.project_code || '')} ${UI.esc(i.project_name)}</td>
        <td class="nowrap">${UI.esc(i.customer_name)}</td>
        <td class="nowrap">${UI.v(i.owner)}</td>
        <td class="nowrap">${UI.statusBadge(i.status)}</td>
        <td class="nowrap muted">${i.closed_at ? UI.fmtTime(i.closed_at) : '—'}</td>
        <td class="nowrap">
          ${i.status === '未关闭' ? `<button class="btn link" onclick="PG_REC.closeIssue(${i.id})">关闭</button>` : ''}
          <button class="btn link" onclick="PG_REC.issueForm(${i.project_id},${i.id})">编辑</button>
        </td>
      </tr>`).join('');
    view.innerHTML = UI.pageHead('风险问题', `共 ${list.length} 条`,
      `<button class="btn" onclick="API.download('/api/export/excel?type=issues${params.status ? '&status=' + encodeURIComponent(params.status) : ''}')">导出 Excel</button>`) +
      `<div class="filters">
        <input id="iq" placeholder="搜索标题/描述/负责人" value="${UI.esc(params.q || '')}">
        <select id="ist"><option value="">全部状态</option>
          ${['未关闭', '已关闭'].map(s => `<option ${params.status === s ? 'selected' : ''}>${s}</option>`).join('')}</select>
        <select id="ilv"><option value="">全部等级</option>
          ${['高', '中', '低'].map(s => `<option ${params.level === s ? 'selected' : ''}>${s}</option>`).join('')}</select>
        <select id="ipj"><option value="">全部项目</option>
          ${projects.map(p => `<option value="${p.id}" ${String(params.project_id) === String(p.id) ? 'selected' : ''}>${UI.esc(p.name)}</option>`).join('')}</select>
        <button class="btn" id="igo">查询</button>
        <button class="btn ghost" id="iroute">仅未关闭</button>
      </div>
      <div class="card"><div class="body tight"><table class="tbl">
        <tr><th>等级</th><th>标题</th><th>描述</th><th>项目</th><th>客户</th><th>负责人</th><th>状态</th><th>关闭时间</th><th>操作</th></tr>
        ${rows || '<tr><td class="empty" colspan="9">暂无问题</td></tr>'}
      </table></div></div>`;
    document.getElementById('igo').onclick = () => {
      const s = new URLSearchParams();
      const q = document.getElementById('iq').value.trim();
      if (q) s.set('q', q);
      const st = document.getElementById('ist').value; if (st) s.set('status', st);
      const lv = document.getElementById('ilv').value; if (lv) s.set('level', lv);
      const pj = document.getElementById('ipj').value; if (pj) s.set('project_id', pj);
      location.hash = '#/issues' + (s.toString() ? '?' + s.toString() : '');
    };
    document.getElementById('iroute').onclick = () => location.hash = '#/issues?status=' + encodeURIComponent('未关闭');
  }
};
