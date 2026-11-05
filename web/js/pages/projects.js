/* 项目：列表、新建/编辑、详情（阶段/快照/网络/附件/进度/问题/维保/关联） */
const AT_TYPES = ['实施方案', '拓扑图', '配置备份', '授权截图', '验收单', '现场照片', '维保记录', '其他'];

const PG_PROJ = {
  /* ---------------- 列表 ---------------- */
  async list(params) {
    const view = document.getElementById('view');
    params = params || {};
    const qs = new URLSearchParams();
    ['q', 'customer_id', 'ptype', 'status', 'priority', 'product_line', 'issue_open'].forEach(k => {
      if (params[k]) qs.set(k, params[k]);
    });
    const [list, customers, meta] = await Promise.all([
      API.get('/api/projects?' + qs.toString()),
      API.get('/api/customers'),
      App.state.meta || API.get('/api/meta')
    ]);
    this._list = list;

    const f = (name, label, options, cur) => `<select id="f-${name}"><option value="">${label}</option>` +
      options.map(o => `<option value="${UI.esc(o)}" ${cur === o ? 'selected' : ''}>${UI.esc(o)}</option>`).join('') + `</select>`;

    const rows = list.map(p => `
      <tr>
        <td class="nowrap"><input type="checkbox" class="pk" value="${p.id}"></td>
        <td class="nowrap"><a href="#/projects/${p.id}" class="mono">${UI.v(p.code)}</a></td>
        <td><a href="#/projects/${p.id}"><strong>${UI.esc(p.name)}</strong></a>
            ${p.open_issues > 0 ? `<span class="badge red" style="margin-left:6px">${p.open_issues} 问题</span>` : ''}</td>
        <td class="nowrap"><a href="#/customers/${p.customer_id}">${UI.esc(p.customer_name)}</a></td>
        <td class="nowrap">${UI.typeBadge(p.ptype)}</td>
        <td class="nowrap">${UI.statusBadge(p.status)}</td>
        <td class="nowrap">${UI.priorityBadge(p.priority)}</td>
        <td class="nowrap">${UI.v(p.product_lines)}</td>
        <td class="nowrap">${UI.fmtDate(p.start_date)}</td>
        <td class="nowrap">${p.ptype === '维保' ? UI.statusBadge(p.mt_status) + (p.expire_date ? `<div class="muted" style="font-size:11px">${UI.fmtDate(p.expire_date)}</div>` : '') : '—'}</td>
        <td class="nowrap muted">${p.stale_days} 天</td>
        <td class="nowrap"><span class="pre" style="max-width:220px;display:inline-block">${UI.esc(p.next_plan_show || '未定')}</span></td>
      </tr>`).join('');

    view.innerHTML = UI.pageHead('项目管理', `共 ${list.length} 个项目`,
      `<button class="btn" onclick="PG_PROJ.exportExcel()">导出 Excel</button>
       <button class="btn primary" onclick="location.hash='#/projects/new'">＋ 新建项目</button>`) +
      `<div class="filters">
        <input id="f-q" placeholder="搜索项目名称/编号/客户" value="${UI.esc(params.q || '')}">
        <select id="f-customer_id"><option value="">全部客户</option>
          ${customers.map(c => `<option value="${c.id}" ${String(params.customer_id) === String(c.id) ? 'selected' : ''}>${UI.esc(c.name)}</option>`).join('')}</select>
        ${f('ptype', '全部类型', (meta.project_types || []), params.ptype)}
        ${f('status', '全部状态', (meta.all_statuses || []), params.status)}
        ${f('priority', '全部优先级', ['高', '中', '低'], params.priority)}
        ${f('product_line', '全部产品线', (meta.product_lines || []), params.product_line)}
        <label style="font-size:12.5px;display:inline-flex;align-items:center;gap:5px;cursor:pointer">
          <input type="checkbox" id="f-issue_open" style="width:14px;height:14px;accent-color:var(--primary)" ${params.issue_open ? 'checked' : ''}> 仅有未关闭问题</label>
        <button class="btn" id="apply">查询</button>
        <button class="btn ghost" id="reset">重置</button>
      </div>
      <div class="flex" style="margin:-4px 0 10px">
        <span class="muted" id="selinfo" style="font-size:12px">未选择</span>
        <div style="flex:1"></div>
        <button class="btn sm" id="batch-status">批量改状态</button>
        <button class="btn sm" id="batch-export">批量导出所选</button>
      </div>
      <div class="card"><div class="body tight"><table class="tbl">
        <tr><th style="width:28px"><input type="checkbox" id="ckall"></th><th>编号</th><th>项目名称</th><th>客户</th>
          <th>类型</th><th>状态</th><th>优先级</th><th>产品线</th><th>开始</th><th>维保</th><th>未更新</th><th>下一步计划</th></tr>
        ${rows || '<tr><td class="empty" colspan="12">暂无项目</td></tr>'}
      </table></div></div>`;

    const goto = () => {
      const p = {};
      ['q', 'customer_id', 'ptype', 'status', 'priority', 'product_line'].forEach(k => {
        const el = document.getElementById('f-' + k);
        if (el && el.value) p[k] = el.value;
      });
      if (document.getElementById('f-issue_open').checked) p.issue_open = '1';
      const s = new URLSearchParams(p).toString();
      location.hash = '#/projects' + (s ? '?' + s : '');
    };
    document.getElementById('apply').onclick = goto;
    document.getElementById('f-q').onkeydown = e => { if (e.key === 'Enter') goto(); };
    document.getElementById('reset').onclick = () => location.hash = '#/projects';

    const sel = () => Array.from(document.querySelectorAll('.pk:checked')).map(x => Number(x.value));
    const upd = () => {
      const n = sel().length;
      document.getElementById('selinfo').textContent = n ? `已选 ${n} 个项目` : '未选择';
    };
    document.getElementById('ckall').onchange = e => {
      document.querySelectorAll('.pk').forEach(x => x.checked = e.target.checked);
      upd();
    };
    document.querySelectorAll('.pk').forEach(x => x.onchange = upd);
    document.getElementById('batch-export').onclick = () => {
      const ids = sel();
      if (!ids.length) return UI.toast('请先勾选项目', 'err');
      API.download('/api/export/excel?type=all&ids=' + ids.join(','));
    };
    document.getElementById('batch-status').onclick = () => {
      const ids = sel();
      if (!ids.length) return UI.toast('请先勾选项目', 'err');
      this.batchStatus(ids);
    };
  },

  async batchStatus(ids) {
    await UI.form({
      title: `批量修改状态（${ids.length} 个项目）`,
      width: 'narrow',
      fields: [{ k: 'status', label: '目标状态', type: 'select', required: true, empty: false, options: ['未开始', '进行中', '待验收', '已验收', '已结束', '暂停', '取消', '维保中'] }],
      submitText: '确定修改',
      onSubmit: async d => {
        const r = await API.post('/api/projects/batch/status', { ids, status: d.status });
        UI.toast(`已更新 ${r.updated} 个项目`, 'ok');
        App.reload();
      }
    });
  },

  exportExcel() {
    const { params } = Q.parse();
    const qs = new URLSearchParams(params).toString();
    API.download('/api/export/excel?type=projects' + (qs ? '&' + qs : ''));
  },

  /* ---------------- 新建 / 编辑 ---------------- */
  async form(id, params) {
    const view = document.getElementById('view');
    params = params || {};
    const [customers, meta, allProjects] = await Promise.all([API.get('/api/customers'), API.get('/api/meta'), API.get('/api/projects')]);
    let p = { ptype: params.ptype || '新实施', status: '未开始', priority: '中', customer_id: params.customer_id || '' };
    let products = [{ product_line: '安全产品', product_name: '' }];
    if (id) {
      p = await API.get('/api/projects/' + id);
      products = (p.products && p.products.length) ? p.products : [{ product_line: '', product_name: '' }];
    }
    const relType = params.mode || (p.ptype === '维保' ? 'accept2maint' : p.ptype === '加点' ? 'addon_source' : '');
    if (!id && params.source_id) p.related_project_id = params.source_id;

    const statusOpts = ['未开始', '进行中', '待验收', '已验收', '已结束', '暂停', '取消', '维保中'];
    const prodHTML = () => `<div class="field full"><label>产品线 / 具体产品</label>
      <table class="tbl" id="prod-tbl" style="border:1px solid var(--border);border-radius:8px;overflow:hidden">
        <tr><th style="width:40%">产品线</th><th>具体产品名称</th><th style="width:60px"></th></tr>
        ${products.map((x, i) => `<tr>
          <td><select data-pl="${i}" style="width:100%;border:none;background:transparent">
            ${(meta.product_lines || []).map(l => `<option ${x.product_line === l ? 'selected' : ''}>${UI.esc(l)}</option>`).join('')}
            </select></td>
          <td><input data-pn="${i}" value="${UI.esc(x.product_name || '')}" placeholder="如 AF-1000、HCI、aTrust" style="width:100%;border:none;background:transparent"></td>
          <td><button type="button" class="btn link" style="color:var(--danger)" onclick="PG_PROJ._delProd(${i})">删除</button></td>
        </tr>`).join('')}
      </table>
      <button type="button" class="btn sm mt8" onclick="PG_PROJ._addProd()">＋ 添加产品</button></div>`;

    this._products = products;
    this._renderProd = null;

    view.innerHTML = UI.pageHead(id ? '编辑项目' : '新建项目', id ? UI.esc(p.name) : '填写项目基础信息',
      `<a class="btn" href="${id ? '#/projects/' + id : '#/projects'}">取消</a>
       <button class="btn primary" id="save">保存</button>`) +
      `<div class="card"><div class="body"><div class="form-grid" id="pform">
        ${UI.fieldHTML({ k: 'code', label: '项目编号', ph: '手填，如 SJ-2026-001' }, p.code)}
        ${UI.fieldHTML({ k: 'name', label: '项目名称', required: true }, p.name)}
        ${UI.fieldHTML({
          k: 'customer_id', label: '客户', type: 'select', required: true, numeric: true, empty: false,
          options: customers.map(c => ({ value: c.id, label: (c.code ? c.code + ' ' : '') + c.name }))
        }, p.customer_id)}
        ${UI.fieldHTML({ k: 'ptype', label: '项目类型', type: 'select', required: true, empty: false, options: meta.project_types }, p.ptype)}
        ${UI.fieldHTML({ k: 'status', label: '项目状态', type: 'select', empty: false, options: statusOpts }, p.status)}
        ${UI.fieldHTML({ k: 'priority', label: '优先级', type: 'select', empty: false, options: ['高', '中', '低'] }, p.priority)}
        ${UI.fieldHTML({ k: 'start_date', label: '开始日期', type: 'date' }, p.start_date)}
        ${UI.fieldHTML({ k: 'expect_accept_date', label: '预计验收日期', type: 'date', hint: '用于验收前 3 天提醒，留空不提醒' }, p.expect_accept_date)}
        ${UI.fieldHTML({ k: 'accept_date', label: '验收日期', type: 'date' }, p.accept_date)}
        ${UI.fieldHTML({ k: 'acceptor', label: '验收人' }, p.acceptor)}
        ${UI.fieldHTML({
          k: 'related_project_id', label: '关联原项目', type: 'select', numeric: true,
          hint: id ? '已有项目一般不修改' : '加点项目必须关联原项目；测试转实施 / 验收转维保建议走「关联与转化」页',
          options: allProjects.filter(x => String(x.id) !== String(id || 0)).map(x => ({ value: x.id, label: `${x.code ? x.code + ' ' : ''}${x.name}（${x.ptype}）` }))
        }, p.related_project_id)}
        ${UI.fieldHTML({ k: 'legacy_issues', label: '遗留问题', type: 'textarea', full: true }, p.legacy_issues)}
        ${UI.fieldHTML({ k: 'remark', label: '备注', type: 'textarea', full: true }, p.remark)}
      </div>
      <div class="sep"></div>
      <div id="prod-wrap">${prodHTML()}</div>
      </div></div>`;

    this._renderProd = () => {
      document.getElementById('prod-wrap').innerHTML = prodHTML();
    };
    this._addProd = () => { products.push({ product_line: meta.product_lines[0], product_name: '' }); this._renderProd(); };
    this._delProd = i => { products.splice(i, 1); this._renderProd(); };

    document.getElementById('save').onclick = async () => {
      const scope = document.getElementById('pform');
      const fields = [
        { k: 'code' }, { k: 'name' }, { k: 'customer_id', numeric: true }, { k: 'ptype' }, { k: 'status' },
        { k: 'priority' }, { k: 'start_date' }, { k: 'expect_accept_date' }, { k: 'accept_date' },
        { k: 'acceptor' }, { k: 'related_project_id', numeric: true }, { k: 'legacy_issues' }, { k: 'remark' }
      ];
      const data = UI.collect(scope, fields);
      if (!data.name) return UI.toast('请填写项目名称', 'err');
      if (!data.customer_id) return UI.toast('请选择客户', 'err');
      const prods = [];
      document.querySelectorAll('#prod-tbl tr').forEach(tr => {
        const pl = tr.querySelector('select[data-pl]');
        const pn = tr.querySelector('input[data-pn]');
        if (pl && pn && (pn.value.trim() || pl.value)) prods.push({ product_line: pl.value, product_name: pn.value.trim() });
      });
      data.products = prods;
      if (!id) data.rel_type = relType || undefined;
      try {
        if (id) { await API.put('/api/projects/' + id, data); UI.toast('已保存', 'ok'); location.hash = '#/projects/' + id; }
        else {
          const r = await API.post('/api/projects', data);
          UI.toast('项目已创建', 'ok');
          location.hash = '#/projects/' + r.id;
        }
      } catch (e) { }
    };
  },

  /* ---------------- 详情 ---------------- */
  async detail(id, tab) {
    const view = document.getElementById('view');
    tab = tab || 'overview';
    const p = await API.get('/api/projects/' + id);
    const head = UI.pageHead(`${UI.esc(p.name)} ${UI.typeBadge(p.ptype)}`,
      `${UI.esc(p.customer_name)} · 编号 ${UI.v(p.code)} · 更新 ${UI.fmtTime(p.updated_at)}`,
      `<a class="btn" href="#/customers/${p.customer_id}">客户档案</a>
       <button class="btn" onclick="PG_PROJ.exportPdf(${id})">导出 PDF</button>
       <a class="btn" href="#/projects/${id}/edit">编辑项目</a>
       <a class="btn" href="#/projects">返回列表</a>`);

    const tabs = [
      { key: 'overview', label: '概览', href: `#/projects/${id}` },
      { key: 'stages', label: '阶段', href: `#/projects/${id}?tab=stages`, count: p.stages.length },
      { key: 'devices', label: '设备快照', href: `#/projects/${id}?tab=devices`, count: p.snapshots.length },
      { key: 'network', label: '网络信息', href: `#/projects/${id}?tab=network` },
      { key: 'attachments', label: '附件', href: `#/projects/${id}?tab=attachments`, count: p.attachments.length },
      { key: 'progress', label: '进度记录', href: `#/projects/${id}?tab=progress` },
      { key: 'issues', label: '风险问题', href: `#/projects/${id}?tab=issues`, count: p.issues.length },
      { key: 'maintenance', label: '维保信息', href: `#/projects/${id}?tab=maintenance` },
      { key: 'relations', label: '关联与转化', href: `#/projects/${id}?tab=relations` }
    ];

    let body = '';
    if (tab === 'overview') body = this.tabOverview(p);
    else if (tab === 'stages') body = await this.tabStages(p);
    else if (tab === 'devices') body = await this.tabSnapshots(p);
    else if (tab === 'network') body = await this.tabNetwork(p);
    else if (tab === 'attachments') body = await this.tabAttachments(p);
    else if (tab === 'progress') body = await PG_REC.progressTable(p.id, p.progress_recent, true);
    else if (tab === 'issues') body = await PG_REC.issueTable(p.id, p.issues, true);
    else if (tab === 'maintenance') body = await this.tabMaintenance(p);
    else body = this.tabRelations(p);

    view.innerHTML = head + `<div class="filters">${UI.tabs(tabs, tab)}</div>` + body;
    if (tab === 'progress') PG_REC._afterProgress(p.id);
    if (tab === 'issues') PG_REC._afterIssues(p.id);
    if (tab === 'maintenance' && p.ptype === '维保') this._wireMaint(p);
  },

  _wireMaint(p) {
    const btn = document.getElementById('msave');
    if (!btn) return;
    btn.onclick = async () => {
      const scope = document.getElementById('view');
      const fields = [
        { k: 'start_date' }, { k: 'cycle_months', numeric: true }, { k: 'expire_date' },
        { k: 'auto_calc' }, { k: 'manual_override' }, { k: 'status' },
        { k: 'auth_attachment_id', numeric: true }, { k: 'accept_attachment_id', numeric: true },
        { k: 'maint_record' }
      ];
      const data = UI.collect(scope, fields);
      try {
        await API.put(`/api/projects/${p.id}/maintenance`, data);
        UI.toast('维保信息已保存', 'ok');
        App.reload();
      } catch (e) { }
    };
  },

  tabOverview(p) {
    const mt = p.maintenance;
    const plan = (p.progress_recent[0] && p.progress_recent[0].next_plan) || '未定';
    const openIssues = (p.issues || []).filter(i => i.status === '未关闭');
    return `<div class="card"><header><h3>项目信息</h3></header><div class="body">
      <div class="kv">
        <div class="item"><div class="k">项目编号</div><div class="v">${UI.v(p.code)}</div></div>
        <div class="item"><div class="k">客户</div><div class="v">${UI.esc(p.customer_name)}</div></div>
        <div class="item"><div class="k">项目类型</div><div class="v">${UI.typeBadge(p.ptype)}</div></div>
        <div class="item"><div class="k">项目状态</div><div class="v">${UI.statusBadge(p.status)}</div></div>
        <div class="item"><div class="k">优先级</div><div class="v">${UI.priorityBadge(p.priority)}</div></div>
        <div class="item"><div class="k">产品线</div><div class="v">${(p.products || []).map(x => `<span class="badge blue">${UI.esc(x.product_line)}</span>`).join(' ') || '—'}</div></div>
        <div class="item"><div class="k">具体产品</div><div class="v">${UI.v((p.products || []).map(x => x.product_name).filter(Boolean).join('、'))}</div></div>
        <div class="item"><div class="k">开始日期</div><div class="v">${UI.fmtDate(p.start_date)}</div></div>
        <div class="item"><div class="k">预计验收</div><div class="v">${UI.fmtDate(p.expect_accept_date)}</div></div>
        <div class="item"><div class="k">验收日期</div><div class="v">${UI.fmtDate(p.accept_date)}</div></div>
        <div class="item"><div class="k">验收人</div><div class="v">${UI.v(p.acceptor)}</div></div>
        <div class="item"><div class="k">距上次进度</div><div class="v">${p.stale_days} 天 ${p.stale_days > 7 ? '<span class="badge orange">需关注</span>' : ''}</div></div>
        <div class="item" style="grid-column:1/-1"><div class="k">下一步计划</div><div class="v">${UI.esc(plan)}</div></div>
        <div class="item" style="grid-column:1/-1"><div class="k">遗留问题</div><div class="v pre">${UI.v(p.legacy_issues)}</div></div>
        <div class="item" style="grid-column:1/-1"><div class="k">备注</div><div class="v pre">${UI.v(p.remark)}</div></div>
      </div></div></div>
      ${p.ptype === '维保' && mt ? `<div class="card"><header><h3>维保概览</h3>
        <div class="right"><a class="btn sm" href="#/projects/${p.id}?tab=maintenance">编辑维保信息</a></div></header>
        <div class="body"><div class="kv">
          <div class="item"><div class="k">维保状态</div><div class="v">${UI.statusBadge(mt.status)} ${Number(mt.manual_override) ? '<span class="badge gray">手动覆盖</span>' : '<span class="badge gray">自动计算</span>'}</div></div>
          <div class="item"><div class="k">起始日</div><div class="v">${UI.fmtDate(mt.start_date)}</div></div>
          <div class="item"><div class="k">到期日</div><div class="v">${UI.fmtDate(mt.expire_date)}</div></div>
          <div class="item"><div class="k">周期</div><div class="v">${UI.v(mt.cycle_months, '—')} 个月</div></div>
        </div></div></div>` : ''}
      <div class="dash-grid">
        ${UI.tableCard('未关闭问题', UI.table([
      { label: '标题', render: i => `<a href="#/projects/${p.id}?tab=issues">${UI.esc(i.title)}</a>` },
      { label: '等级', render: i => UI.levelBadge(i.level), cls: 'nowrap' },
      { label: '负责人', k: 'owner', cls: 'nowrap' }
    ], openIssues, { empty: '无未关闭问题' }))}
        ${UI.tableCard('最近进度', UI.table([
      { label: '日期', render: g => UI.fmtDate(g.record_date), cls: 'nowrap' },
      { label: '进展', render: g => UI.esc((g.current_progress || '').slice(0, 50)) },
      { label: '下一步', render: g => UI.esc((g.next_plan || '').slice(0, 30)) }
    ], p.progress_recent, { empty: '暂无进度记录' }), `<a class="btn sm ghost" href="#/projects/${p.id}?tab=progress">全部</a>`)}
      </div>`;
  },

  /* ---------------- 阶段 ---------------- */
  async tabStages(p) {
    const stages = await API.get(`/api/projects/${p.id}/stages`);
    const opts = ['未开始', '进行中', '已完成'];
    const rows = stages.map(s => `
      <tr>
        <td class="nowrap">${s.seq}</td>
        <td><strong>${UI.esc(s.name)}</strong></td>
        <td class="nowrap">
          <select onchange="PG_PROJ.setStage(${s.id},this.value)" style="height:26px;border:1px solid var(--border);border-radius:6px;background:var(--panel);color:var(--text)">
            ${opts.map(o => `<option ${s.status === o ? 'selected' : ''}>${o}</option>`).join('')}
          </select>
        </td>
        <td class="nowrap muted">${UI.fmtDate(s.start_at)}</td>
        <td class="nowrap muted">${UI.fmtDate(s.done_at)}</td>
        <td class="nowrap">${s.record_count} 条记录</td>
        <td class="nowrap"><button class="btn link" style="color:var(--danger)" onclick="PG_PROJ.delStage(${s.id},${p.id})">删除</button></td>
      </tr>`).join('');
    return UI.tableCard(`项目阶段（${UI.esc(p.ptype)}）`, `<table class="tbl">
        <tr><th>顺序</th><th>阶段名称</th><th>阶段状态</th><th>开始</th><th>完成</th><th>进度记录</th><th>操作</th></tr>
        ${rows || '<tr><td class="empty" colspan="7">暂无阶段</td></tr>'}</table>`,
      `<button class="btn sm" onclick="PG_PROJ.addStage(${p.id})">＋ 添加阶段</button>
       <button class="btn sm" onclick="PG_PROJ.initStages(${p.id})">补充模板阶段</button>`);
  },

  async setStage(id, status) {
    await API.put('/api/stages/' + id, { status });
    UI.toast('阶段已更新', 'ok');
  },
  async initStages(pid) {
    await API.post(`/api/projects/${pid}/stages/init`);
    UI.toast('已补充模板阶段', 'ok'); App.reload();
  },
  async delStage(id, pid) {
    if (!await UI.confirm('删除阶段不会删除进度记录，但会解除阶段关联。确认删除？')) return;
    await API.del('/api/stages/' + id);
    App.reload();
  },
  async addStage(pid) {
    await UI.form({
      title: '添加阶段', width: 'narrow',
      fields: [{ k: 'name', label: '阶段名称', required: true }, { k: 'status', label: '状态', type: 'select', empty: false, options: ['未开始', '进行中', '已完成'] }],
      values: { status: '未开始' },
      onSubmit: async d => { await API.post(`/api/projects/${pid}/stages`, d); UI.toast('已添加', 'ok'); App.reload(); }
    });
  },

  /* ---------------- 设备快照 ---------------- */
  async tabSnapshots(p) {
    const list = await API.get(`/api/projects/${p.id}/devices`);
    const rows = list.map(d => `
      <tr>
        <td class="nowrap">${UI.v(d.model)}</td>
        <td class="nowrap mono">${UI.v(d.serial_no)}</td>
        <td class="nowrap mono">${UI.v(d.account)}</td>
        <td class="nowrap">${UI.v(d.contact_name)}</td>
        <td class="pre" style="max-width:260px">${UI.v((d.network_info || '').slice(0, 50))}</td>
        <td class="nowrap">${PG_CUST.topoChips(d.topology, true)}</td>
        <td class="nowrap">v${d.snapshot_version}</td>
        <td class="nowrap">
          <button class="btn link" onclick="PG_PROJ.snapshotForm(${p.id},${d.id})">编辑</button>
          <button class="btn link" onclick="PG_PROJ.snapshotHistory(${d.id})">历史</button>
          <button class="btn link" style="color:var(--danger)" onclick="PG_PROJ.delSnapshot(${d.id},${p.id})">删除</button>
        </td>
      </tr>`).join('');
    return UI.tableCard('项目设备快照', `<table class="tbl">
        <tr><th>型号</th><th>序列号</th><th>账号密码</th><th>对接人</th><th>网络信息</th><th>拓扑</th><th>版本</th><th>操作</th></tr>
        ${rows || '<tr><td class="empty" colspan="8">暂无设备快照</td></tr>'}</table>`,
      `<button class="btn sm" onclick="PG_PROJ.inheritDevices(${p.id})">从项目继承</button>
       <button class="btn sm primary" onclick="PG_PROJ.pickCustomerDevice(${p.id},${p.customer_id})">＋ 从客户档案添加</button>`);
  },

  async pickCustomerDevice(pid, cid) {
    const list = await API.get(`/api/customers/${cid}/devices`);
    if (!list.length) {
      UI.toast('该客户暂无设备档案，请先到客户档案新增', 'err');
      return;
    }
    const m = UI.modal({
      title: '从客户设备档案添加到项目快照',
      body: `<div class="muted mb8">选择后会复制为客户档案的设备快照副本，后续修改不影响客户档案。</div>` +
        UI.table([
          { label: '', cls: 'nowrap', render: d => `<input type="checkbox" class="dv" value="${d.id}">` },
          { label: '型号', k: 'model' }, { label: '序列号', k: 'serial_no' }, { label: '账号密码', k: 'account' }
        ], list),
      footer: `<button class="btn" data-close>取消</button><button class="btn primary" id="ok">添加所选</button>`
    });
    m.el.querySelector('#ok').onclick = async () => {
      const ids = Array.from(m.el.querySelectorAll('.dv:checked')).map(x => Number(x.value));
      if (!ids.length) return UI.toast('请选择设备', 'err');
      for (const sid of ids) await API.post(`/api/projects/${pid}/devices`, { source_device_id: sid });
      UI.toast(`已添加 ${ids.length} 台设备`, 'ok');
      m.close(); App.reload();
    };
  },

  async inheritDevices(pid) {
    const all = await API.get('/api/projects');
    const others = all.filter(x => x.id !== pid);
    await UI.form({
      title: '从其他项目继承设备快照',
      fields: [{
        k: 'from_project_id', label: '来源项目', type: 'select', required: true, numeric: true, empty: false,
        options: others.map(x => ({ value: x.id, label: `${x.code || ''} ${x.name}（${x.ptype}）` }))
      }],
      submitText: '继承',
      onSubmit: async d => {
        const r = await API.post(`/api/projects/${pid}/devices/inherit`, { from_project_id: d.from_project_id });
        UI.toast(`已继承 ${r.copied} 台设备快照`, 'ok'); App.reload();
      }
    });
  },

  async snapshotForm(pid, id) {
    const proj = await API.get('/api/projects/' + pid);
    const contacts = await API.get(`/api/customers/${proj.customer_id}/contacts`).catch(() => []);
    let d = { topology: '[]' };
    if (id) d = await API.get('/api/projects/' + pid + '/devices').then(l => l.find(x => x.id === id));
    let topo = [];
    try { topo = JSON.parse(d.topology || '[]'); } catch (e) { }
    const renderTopo = () => {
      document.getElementById('tp2').innerHTML = topo.map((f, i) =>
        `<span class="badge blue" style="margin:2px 4px 2px 0">${UI.esc(f.name)} <a href="javascript:;" onclick="PG_PROJ._dl(${i})" style="color:inherit;margin-left:4px">✕</a></span>`
      ).join('') || '<span class="muted">未上传</span>';
    };
    this._dl = i => { topo.splice(i, 1); renderTopo(); };
    await UI.form({
      title: id ? '编辑设备快照' : '新增设备快照',
      width: 'wide',
      note: '修改仅作用于项目内设备快照，并自动记录变更历史，不影响客户设备档案。',
      fields: [
        { k: 'model', label: '设备型号' },
        { k: 'serial_no', label: '序列号' },
        { k: 'contact_id', label: '客户对接人', type: 'select', numeric: true, options: contacts.map(c => ({ value: c.id, label: c.name })) },
        { k: 'account', label: '账号密码' },
        { k: 'network_info', label: '网络信息', type: 'textarea', full: true },
        { k: 'remark', label: '备注', type: 'textarea', full: true }
      ],
      values: d,
      beforeHTML: `<div class="field full" style="margin-bottom:12px"><label>拓扑图</label>
        <div id="tp2" class="mb8"></div><input type="file" id="tpf2" multiple style="font-size:12px"></div>`,
      onReady: m => {
        renderTopo();
        m.el.querySelector('#tpf2').onchange = async e => {
          for (const f of Array.from(e.target.files)) {
            try { topo.push(await API.upload('/api/upload', f, {})); } catch (err) { }
          }
          renderTopo();
        };
      },
      onSubmit: async data => {
        data.topology = JSON.stringify(topo);
        if (id) await API.put('/api/snapshots/' + id, data);
        else await API.post(`/api/projects/${pid}/devices`, data);
        UI.toast('已保存', 'ok'); App.reload();
      }
    });
  },

  async snapshotHistory(id) {
    const list = await API.get(`/api/snapshots/${id}/history`);
    UI.modal({
      title: '设备快照变更历史',
      width: 'wide',
      body: UI.table([
        { label: '字段', render: h => h.field_name === '*' ? '新增快照' : UI.esc(h.field_name) },
        { label: '修改前', render: h => `<span class="diff-old pre">${UI.v(h.old_value)}</span>` },
        { label: '修改后', render: h => `<span class="diff-new pre">${UI.v(h.new_value)}</span>` },
        { label: '操作', render: h => UI.esc({ create: '新增', add: '新增', update: '修改', delete: '删除' }[h.action] || h.action), cls: 'nowrap' },
        { label: '时间', render: h => UI.fmtTime(h.changed_at), cls: 'nowrap' }
      ], list, { empty: '暂无变更记录' }),
      footer: '<button class="btn" data-close>关闭</button>'
    });
  },

  async delSnapshot(id, pid) {
    if (!await UI.confirm('确认删除该设备快照？')) return;
    await API.del('/api/snapshots/' + id);
    UI.toast('已删除', 'ok'); App.reload();
  },

  /* ---------------- 网络信息引用 ---------------- */
  async tabNetwork(p) {
    const [ref, versions] = await Promise.all([
      API.get(`/api/projects/${p.id}/net-ref`),
      API.get(`/api/customers/${p.customer_id}/net-versions`)
    ]);
    let content = {};
    if (ref) { try { content = JSON.parse(ref.content || '{}'); } catch (e) { } }
    const rows = versions.map(v => `
      <tr>
        <td class="nowrap"><strong>V${v.version_no}</strong> ${Number(v.is_current) ? '<span class="badge green">当前</span>' : ''}</td>
        <td class="nowrap muted">${UI.fmtTime(v.created_at)}</td>
        <td class="nowrap">${ref && ref.net_version_id === v.id ? '<span class="badge blue">本项目引用</span>' : ''}</td>
        <td class="nowrap">
          <button class="btn link" onclick="location.hash='#/network/${v.id}'">查看</button>
          <button class="btn link" onclick="PG_PROJ.setNetRef(${p.id},${v.id})">引用此版本</button>
        </td>
      </tr>`).join('');
    return `<div class="card"><header><h3>项目引用的网络信息版本</h3>
      <div class="right"><button class="btn sm" onclick="location.hash='#/customers/${p.customer_id}?tab=network'">管理客户网络版本</button>
      <button class="btn sm" onclick="API.download('/api/export/pdf?type=network&project_id=${p.id}')">导出 PDF</button></div></header>
      <div class="body">
        ${ref ? `<div class="mb8"><span class="badge blue">V${ref.version_no}</span> 引用时间 ${UI.fmtTime(ref.ref_at)}</div>
          <div class="kv">` + NET_KEYS.map(k => `<div class="item"><div class="k">${NET_LABELS[k]}</div><div class="v pre">${content[k] ? UI.esc(content[k]) : '<span class="muted">—</span>'}</div></div>`).join('') + `</div>`
        : '<div class="muted">本项目尚未引用网络信息版本</div>'}
      </div></div>
      <div class="card"><header><h3>可选版本</h3></header><div class="body tight"><table class="tbl">
        <tr><th>版本</th><th>创建时间</th><th>状态</th><th>操作</th></tr>${rows}</table></div></div>`;
  },

  async setNetRef(pid, vid) {
    await API.post(`/api/projects/${pid}/net-ref`, { net_version_id: vid });
    UI.toast('已切换引用版本', 'ok'); App.reload();
  },

  /* ---------------- 附件 ---------------- */
  async tabAttachments(p) {
    const list = await API.get(`/api/projects/${p.id}/attachments`);
    this._atts = list;
    const rows = list.map(a => `
      <tr>
        <td class="nowrap"><span class="badge">${UI.esc(a.atype)}</span></td>
        <td><a href="javascript:;" onclick="PG_PROJ.previewAtt(${a.id})">${UI.esc(a.file_name)}</a></td>
        <td class="nowrap">${UI.fmtSize(a.file_size)}</td>
        <td class="nowrap">v${a.version_no} / 共 ${a.version_count} 版</td>
        <td class="nowrap muted">${UI.fmtTime(a.created_at)}</td>
        <td class="nowrap">
          <button class="btn link" onclick="PG_PROJ.previewAtt(${a.id})">预览</button>
          <button class="btn link" onclick="PG_PROJ.attachVersions(${a.id})">版本</button>
          <button class="btn link" onclick="PG_PROJ.uploadVersion(${a.id},'${UI.esc(a.atype)}')">上传新版本</button>
          <button class="btn link" style="color:var(--danger)" onclick="PG_PROJ.delAttach(${a.id})">删除</button>
        </td>
      </tr>`).join('');
    return UI.tableCard('项目附件', `<table class="tbl">
        <tr><th>类型</th><th>文件名</th><th>大小</th><th>版本</th><th>创建</th><th>操作</th></tr>
        ${rows || '<tr><td class="empty" colspan="6">暂无附件</td></tr>'}</table>`,
      `<button class="btn sm primary" onclick="PG_PROJ.uploadAttach(${p.id})">＋ 上传附件</button>`);
  },

  previewAtt(id) {
    const a = (this._atts || []).find(x => x.id === id);
    if (!a) return;
    this.preview(a.current_version_id, a.file_name);
  },

  uploadAttach(pid) {
    const m = UI.modal({
      title: '上传项目附件',
      body: `<div class="form-grid">
        <div class="field full"><label>附件类型<span class="req">*</span></label>
          <select id="a-type">${AT_TYPES.map(t => `<option>${t}</option>`).join('')}</select></div>
        <div class="field full"><label>文件（单个不超过 300MB）</label><input type="file" id="a-file"></div>
        <div class="field full"><label>版本说明</label><input id="a-note" placeholder="选填"></div>
      </div>`,
      footer: `<button class="btn" data-close>取消</button><button class="btn primary" id="ok">上传</button>`
    });
    m.el.querySelector('#ok').onclick = async () => {
      const f = m.el.querySelector('#a-file').files[0];
      if (!f) return UI.toast('请选择文件', 'err');
      const btn = m.el.querySelector('#ok');
      btn.disabled = true; btn.textContent = '上传中…';
      try {
        await API.upload(`/api/projects/${pid}/attachments`, f, {
          atype: m.el.querySelector('#a-type').value, note: m.el.querySelector('#a-note').value
        });
        UI.toast('上传成功', 'ok'); m.close(); App.reload();
      } catch (e) { btn.disabled = false; btn.textContent = '上传'; }
    };
  },

  uploadVersion(aid, atype) {
    const m = UI.modal({
      title: '上传新版本',
      body: `<div class="muted mb8">将生成新版本，旧版本保留可追溯。</div>
        <div class="field"><label>文件</label><input type="file" id="v-file"></div>
        <div class="field mt8"><label>版本说明</label><input id="v-note" placeholder="本次修改内容"></div>`,
      footer: `<button class="btn" data-close>取消</button><button class="btn primary" id="ok">上传</button>`
    });
    m.el.querySelector('#ok').onclick = async () => {
      const f = m.el.querySelector('#v-file').files[0];
      if (!f) return UI.toast('请选择文件', 'err');
      try {
        const r = await API.upload(`/api/attachments/${aid}/versions`, f, {
          atype: atype, note: m.el.querySelector('#v-note').value
        });
        UI.toast('已上传 V' + r.version_no, 'ok'); m.close(); App.reload();
      } catch (e) { }
    };
  },

  async attachVersions(aid) {
    const r = await API.get(`/api/attachments/${aid}/versions`);
    UI.modal({
      title: '附件版本 · ' + r.attachment.file_name,
      body: UI.table([
        { label: '版本', render: v => `<strong>v${v.version_no}</strong>`, cls: 'nowrap' },
        { label: '文件名', k: 'file_name' },
        { label: '大小', render: v => UI.fmtSize(v.file_size), cls: 'nowrap' },
        { label: '说明', k: 'note' },
        { label: '上传时间', render: v => UI.fmtTime(v.created_at), cls: 'nowrap' },
        { label: '', render: v => `<a class="btn link" href="${v.url}" target="_blank" rel="noopener">下载/预览</a>`, cls: 'nowrap' }
      ], r.versions),
      footer: '<button class="btn" data-close>关闭</button>'
    });
  },

  preview(vid, name) {
    if (!vid) return UI.toast('该附件没有文件', 'err');
    const url = '/api/files/' + vid;
    const lower = (name || '').toLowerCase();
    const isImg = /\.(png|jpe?g|gif|webp|bmp)$/.test(lower);
    const isPdf = lower.endsWith('.pdf');
    const body = isImg ? `<img src="${url}" style="max-width:100%">`
      : isPdf ? `<iframe src="${url}" style="width:100%;height:68vh;border:none"></iframe>`
        : `<div class="muted">该类型不支持在线预览，请下载后查看。</div>`;
    UI.modal({
      title: '预览 · ' + (name || ''),
      width: 'wide',
      body,
      footer: `<a class="btn" href="${url}" target="_blank" rel="noopener">新窗口打开</a><button class="btn" data-close>关闭</button>`
    });
  },

  async delAttach(id) {
    if (!await UI.confirm('删除附件及其全部版本文件？此操作不可恢复。')) return;
    await API.del('/api/attachments/' + id);
    UI.toast('已删除', 'ok'); App.reload();
  },

  /* ---------------- 维保 ---------------- */
  async tabMaintenance(p) {
    if (p.ptype !== '维保') {
      return `<div class="card"><div class="body">${UI.empty('该项目不是维保项目', '维保信息仅在「维保」类型项目中维护')}
        <div class="flex" style="justify-content:center"><a class="btn" href="#/projects/${p.id}/convert?mode=accept2maint">由本项目新建维保项目</a></div></div></div>`;
    }
    const mt = (await API.get(`/api/projects/${p.id}/maintenance`)) || {};
    const atts = await API.get(`/api/projects/${p.id}/attachments`);
    const attOpts = atts.map(a => ({ value: a.id, label: `[${a.atype}] ${a.file_name}` }));
    return `<div class="card"><header><h3>维保信息</h3><div class="right">
        <button class="btn sm" onclick="API.download('/api/export/pdf?type=maintenance&project_id=${p.id}')">导出维保报告 PDF</button>
      </div></header><div class="body">
      <div class="form-grid">
        ${UI.fieldHTML({ k: 'start_date', label: '维保起始日', type: 'date' }, mt.start_date)}
        ${UI.fieldHTML({ k: 'cycle_months', label: '到期周期（月）', type: 'number' }, mt.cycle_months)}
        ${UI.fieldHTML({ k: 'expire_date', label: '到期日', type: 'date', hint: '勾选自动计算则按起始日+周期生成' }, mt.expire_date)}
        ${UI.fieldHTML({ k: 'auto_calc', label: '到期日计算方式', type: 'checkbox', checkLabel: '按周期自动计算（取消勾选则手填生效）' }, mt.auto_calc)}
        ${UI.fieldHTML({ k: 'manual_override', label: '维保状态来源', type: 'checkbox', checkLabel: '手动覆盖状态（不勾选则按到期日自动计算）' }, mt.manual_override)}
        ${UI.fieldHTML({ k: 'status', label: '维保状态', type: 'select', empty: false, options: ['在保', '即将过保', '过保'] }, mt.status)}
        ${UI.fieldHTML({
          k: 'auth_attachment_id', label: '授权截图（附件）', type: 'select', numeric: true,
          options: attOpts.filter(a => atts.find(x => x.id === a.value).atype === '授权截图')
        }, mt.auth_attachment_id)}
        ${UI.fieldHTML({
          k: 'accept_attachment_id', label: '验收报告（附件）', type: 'select', numeric: true,
          options: attOpts.filter(a => ['验收单', '其他'].includes(atts.find(x => x.id === a.value).atype))
        }, mt.accept_attachment_id)}
        ${UI.fieldHTML({ k: 'maint_record', label: '维保记录', type: 'textarea', full: true, rows: 5 }, mt.maint_record)}
      </div>
      <div class="flex mt12"><button class="btn primary" id="msave">保存维保信息</button>
      <span class="muted">提示：授权截图/验收报告请先在「附件」页上传对应类型文件</span></div>
      </div></div>`;
  },

  /* ---------------- 关联与转化 ---------------- */
  tabRelations(p) {
    const rels = p.relations || [];
    const relLabel = { addon_source: '加点关联原项目', test2impl: '测试转实施', accept2maint: '验收转维保' };
    const rows = rels.map(r => `<tr>
      <td class="nowrap"><span class="badge blue">${UI.esc(relLabel[r.rel_type] || r.rel_type)}</span></td>
      <td><a href="#/projects/${r.related_project_id}">${UI.esc(r.related_name || ('#' + r.related_project_id))}</a></td>
      <td class="nowrap">${UI.esc(r.related_type || '')}</td>
      <td class="nowrap">${UI.fmtTime(r.created_at)}</td>
    </tr>`).join('');
    const canAddon = p.ptype === '新实施';
    return `<div class="card"><header><h3>关联项目</h3>
      <div class="right"><button class="btn sm" onclick="PG_PROJ.addRelation(${p.id})">关联项目</button></div></header>
      <div class="body tight"><table class="tbl"><tr><th>关联类型</th><th>关联项目</th><th>类型</th><th>建立时间</th></tr>
      ${rows || '<tr><td class="empty" colspan="4">暂无关联</td></tr>'}</table></div></div>
      <div class="card"><header><h3>项目转化</h3></header><div class="body">
        <div class="muted mb8">转化时会自动继承客户、产品、设备快照、网络信息版本、附件与进度记录（可勾选），并建立关联关系。</div>
        <div class="flex wrap">
          ${canAddon ? `<a class="btn" href="#/projects/${p.id}/convert?mode=addon">新建加点项目（继承本项目）</a>` : ''}
          ${p.ptype === '测试' ? `<a class="btn" href="#/projects/${p.id}/convert?mode=test2impl">测试转实施</a>` : ''}
          ${p.ptype !== '维保' ? `<a class="btn" href="#/projects/${p.id}/convert?mode=accept2maint">验收后新建维保项目</a>` : ''}
        </div>
      </div></div>`;
  },

  async addRelation(id) {
    const all = await API.get('/api/projects');
    const others = all.filter(x => x.id !== id);
    await UI.form({
      title: '关联项目',
      fields: [
        {
          k: 'related_project_id', label: '关联项目', type: 'select', required: true, numeric: true, empty: false,
          options: others.map(x => ({ value: x.id, label: `${x.code || ''} ${x.name}（${x.ptype}）` }))
        },
        { k: 'rel_type', label: '关联类型', type: 'select', empty: false, options: [{ value: 'addon_source', label: '加点关联原项目' }, { value: 'test2impl', label: '测试转实施' }, { value: 'accept2maint', label: '验收转维保' }] }
      ],
      onSubmit: async d => { await API.post(`/api/projects/${id}/relation`, d); UI.toast('已关联', 'ok'); App.reload(); }
    });
  },

  /* ---------------- 转化表单 ---------------- */
  async convertForm(srcId, mode) {
    const view = document.getElementById('view');
    const src = await API.get('/api/projects/' + srcId);
    mode = mode || 'accept2maint';
    const config = {
      addon: { title: '新建加点项目', type: '加点' },
      test2impl: { title: '测试转实施项目', type: '新实施' },
      accept2maint: { title: '验收后新建维保项目', type: '维保' }
    }[mode];
    const fields = [
      { k: 'code', label: '新项目编号', ph: '手填' },
      { k: 'name', label: '新项目名称', required: true, def: src.name + (mode === 'addon' ? '-加点' : mode === 'test2impl' ? '-实施' : '-维保') },
      { k: 'priority', label: '优先级', type: 'select', empty: false, options: ['高', '中', '低'], def: src.priority },
      { k: 'status', label: '项目状态', type: 'select', empty: false, options: mode === 'accept2maint' ? ['未开始', '维保中'] : ['未开始', '进行中'] }
    ];
    if (mode === 'accept2maint') {
      fields.push(
        { k: 'start_date', label: '维保起始日', type: 'date', required: true, def: UI.today() },
        { k: 'cycle_months', label: '到期周期（月）', type: 'number', def: 12 },
        { k: 'expire_date', label: '到期日（留空自动计算）', type: 'date' },
        { k: 'accept_date', label: '验收日期', type: 'date', def: src.accept_date },
        { k: 'acceptor', label: '验收人', def: src.acceptor },
        { k: 'legacy_issues', label: '遗留问题', type: 'textarea', full: true, def: src.legacy_issues }
      );
    } else {
      fields.push({ k: 'start_date', label: '开始日期', type: 'date', def: UI.today() });
    }
    fields.push(
      { k: 'copy_devices', label: '继承设备快照', type: 'checkbox', checkLabel: '复制来源项目设备快照', def: 1 },
      { k: 'copy_attachments', label: '继承附件', type: 'checkbox', checkLabel: '复制附件为新版本（不影响原项目）', def: 1 },
      { k: 'copy_progress', label: '继承进度记录', type: 'checkbox', checkLabel: '复制来源项目进度记录', def: mode === 'test2impl' ? 1 : 0 }
    );

    view.innerHTML = UI.pageHead(config.title, `来源项目：${UI.esc(src.name)}（${src.ptype}）· ${UI.esc(src.customer_name)}`,
      `<a class="btn" href="#/projects/${srcId}">取消</a><button class="btn primary" id="go">创建</button>`) +
      `<div class="card"><div class="body"><div class="form-grid" id="cform">
        ${fields.map(f => UI.fieldHTML(f, f.def !== undefined ? f.def : undefined)).join('')}
      </div>
      <div class="sep"></div>
      <div class="muted">将自动继承：客户、产品线与具体产品、项目网络信息引用版本${mode === 'accept2maint' ? '、验收信息' : ''}，并建立「${mode === 'addon' ? '加点关联原项目' : mode === 'test2impl' ? '测试转实施' : '验收转维保'}」关联。</div>
      </div></div>`;

    document.getElementById('go').onclick = async () => {
      const flds = fields.map(f => ({ k: f.k, numeric: ['cycle_months'].includes(f.k) }));
      const data = UI.collect(document.getElementById('cform'), flds);
      data.source_project_id = srcId;
      data.mode = mode;
      if (!data.name) return UI.toast('请填写项目名称', 'err');
      if (mode === 'accept2maint' && !data.start_date) return UI.toast('请填写维保起始日', 'err');
      try {
        const r = await API.post('/api/projects/convert', data);
        UI.toast('已创建：' + r.name, 'ok');
        location.hash = '#/projects/' + r.id;
      } catch (e) { }
    };
  },

  exportPdf(id) {
    const m = UI.modal({
      title: '导出 PDF',
      width: 'narrow',
      body: `<div class="flex" style="flex-direction:column;align-items:stretch;gap:8px">
        <button class="btn" onclick="API.download('/api/export/pdf?type=weekly&project_id=${id}')">项目周报（本周进度汇总）</button>
        <button class="btn" onclick="API.download('/api/export/pdf?type=network&project_id=${id}')">客户网络信息表</button>
        <button class="btn" onclick="API.download('/api/export/pdf?type=maintenance&project_id=${id}')">维保报告</button>
        <button class="btn" onclick="API.download('/api/export/pdf?type=acceptance&project_id=${id}')">验收报告（上传的验收单）</button>
      </div>`,
      footer: '<button class="btn" data-close>关闭</button>'
    });
  }
};
