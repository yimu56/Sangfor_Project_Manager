/* 客户档案：客户 / 联系人 / 网络信息版本 / 设备档案 */
const NET_LABELS = {
  contact: '客户对接人', gateway: '出口/网关', ip_plan: 'IP 地址规划', vlan: 'VLAN 划分',
  dns: 'DNS / NTP', account: '网络设备账号密码', bandwidth: '线路 / 带宽',
  topology: '拓扑说明', remark: '其他说明'
};
const NET_KEYS = Object.keys(NET_LABELS);

const PG_CUST = {
  /* ---------------- 客户列表 ---------------- */
  async list() {
    const view = document.getElementById('view');
    const q = (Q.parse().params.q || '');
    const list = await API.get('/api/customers' + (q ? '?q=' + encodeURIComponent(q) : ''));
    const rows = list.map(c => `
      <tr class="row-link" onclick="location.hash='#/customers/${c.id}'">
        <td class="nowrap">${UI.v(c.code)}</td>
        <td><strong>${UI.esc(c.name)}</strong></td>
        <td class="nowrap">${c.contact_count}</td>
        <td class="nowrap">${c.device_count}</td>
        <td class="nowrap">${c.net_version_count}</td>
        <td class="nowrap">${c.project_count}</td>
        <td>${UI.v((c.remark || '').slice(0, 40))}</td>
        <td class="nowrap muted">${UI.fmtDate(c.updated_at)}</td>
      </tr>`).join('');
    view.innerHTML = UI.pageHead('客户档案', `共 ${list.length} 个客户`, 
      `<input class="btn" style="width:200px;cursor:text" id="cq" placeholder="搜索客户名称/编号" value="${UI.esc(q)}">
       <button class="btn primary" onclick="PG_CUST.form()">＋ 新建客户</button>`) +
      `<div class="card"><div class="body tight"><table class="tbl">
        <tr><th>客户编号</th><th>客户名称</th><th>联系人</th><th>设备</th><th>网络版本</th><th>项目数</th><th>备注</th><th>更新时间</th></tr>
        ${rows || `<tr><td class="empty" colspan="8">暂无客户，点击右上角新建</td></tr>`}
      </table></div></div>`;
    const inp = document.getElementById('cq');
    inp.onkeydown = e => {
      if (e.key === 'Enter') location.hash = '#/customers' + (inp.value.trim() ? '?q=' + encodeURIComponent(inp.value.trim()) : '');
    };
  },

  async form(id) {
    let c = {};
    if (id) c = await API.get('/api/customers/' + id);
    await UI.form({
      title: id ? '编辑客户' : '新建客户',
      fields: [
        { k: 'code', label: '客户编号', ph: '如 C001（选填）' },
        { k: 'name', label: '客户名称', required: true },
        { k: 'remark', label: '备注', type: 'textarea', full: true }
      ],
      values: c,
      onSubmit: async d => {
        if (id) { await API.put('/api/customers/' + id, d); UI.toast('已保存', 'ok'); App.reload(); }
        else { const r = await API.post('/api/customers', d); UI.toast('客户已创建', 'ok'); location.hash = '#/customers/' + r.id; }
      }
    });
  },

  /* ---------------- 客户详情 ---------------- */
  async detail(id, tab) {
    const view = document.getElementById('view');
    tab = tab || 'overview';
    const [c, projects] = await Promise.all([
      API.get('/api/customers/' + id),
      API.get('/api/projects?customer_id=' + id)
    ]);
    const head = UI.pageHead(UI.esc(c.name),
      `客户编号 ${UI.v(c.code)} · 创建于 ${UI.fmtDate(c.created_at)}`,
      `<button class="btn" onclick="PG_CUST.form(${id})">编辑客户</button>
       <button class="btn danger" onclick="PG_CUST.remove(${id})">删除</button>
       <a class="btn" href="#/customers">返回列表</a>`);

    const tabNav = UI.tabs([
      { key: 'overview', label: '概览', href: `#/customers/${id}` },
      { key: 'contacts', label: '联系人', href: `#/customers/${id}?tab=contacts` },
      { key: 'network', label: '网络信息版本', href: `#/customers/${id}?tab=network` },
      { key: 'devices', label: '设备档案', href: `#/customers/${id}?tab=devices` },
      { key: 'projects', label: '项目', href: `#/customers/${id}?tab=projects`, count: projects.length }
    ], tab);

    let body = '';
    if (tab === 'overview') body = this.tabOverview(c, projects);
    else if (tab === 'contacts') body = await this.tabContacts(id);
    else if (tab === 'network') body = await this.tabNetwork(id);
    else if (tab === 'devices') body = await this.tabDevices(id);
    else body = this.tabProjects(projects);

    view.innerHTML = head + tabNav + body;
  },

  tabOverview(c, projects) {
    const active = projects.filter(p => !['已结束', '取消', '已验收'].includes(p.status));
    return `<div class="card"><header><h3>基本信息</h3></header><div class="body">
      <div class="kv">
        <div class="item"><div class="k">客户名称</div><div class="v big">${UI.esc(c.name)}</div></div>
        <div class="item"><div class="k">客户编号</div><div class="v">${UI.v(c.code)}</div></div>
        <div class="item"><div class="k">在办项目</div><div class="v">${active.length} / 共 ${projects.length}</div></div>
        <div class="item"><div class="k">更新时间</div><div class="v">${UI.fmtTime(c.updated_at)}</div></div>
        <div class="item" style="grid-column:1/-1"><div class="k">备注</div><div class="v pre">${UI.v(c.remark)}</div></div>
      </div></div></div>
      ${UI.tableCard('项目列表', UI.table([
        { label: '编号', k: 'code', cls: 'nowrap' },
        { label: '项目名称', render: p => `<a href="#/projects/${p.id}">${UI.esc(p.name)}</a>` },
        { label: '类型', render: p => UI.typeBadge(p.ptype) },
        { label: '状态', render: p => UI.statusBadge(p.status) },
        { label: '优先级', render: p => UI.priorityBadge(p.priority) },
        { label: '开始', render: p => UI.fmtDate(p.start_date) }
      ], projects), `<a class="btn sm" href="#/projects/new?customer_id=${c.id}">＋ 新建项目</a>`)}`;
  },

  tabProjects(projects) {
    return UI.tableCard('项目列表', UI.table([
      { label: '编号', k: 'code', cls: 'nowrap' },
      { label: '项目名称', render: p => `<a href="#/projects/${p.id}">${UI.esc(p.name)}</a>` },
      { label: '类型', render: p => UI.typeBadge(p.ptype) },
      { label: '状态', render: p => UI.statusBadge(p.status) },
      { label: '优先级', render: p => UI.priorityBadge(p.priority) },
      { label: '开始', render: p => UI.fmtDate(p.start_date) }
    ], projects));
  },

  /* ---------------- 联系人 ---------------- */
  async tabContacts(cid) {
    const list = await API.get(`/api/customers/${cid}/contacts`);
    return UI.tableCard('联系人', UI.table([
      { label: '姓名', render: c => `<strong>${UI.esc(c.name)}</strong>` },
      { label: '电话', k: 'phone', cls: 'nowrap' },
      { label: '邮箱', k: 'email' },
      { label: '角色', k: 'role', cls: 'nowrap' },
      { label: '备注', k: 'remark' },
      {
        label: '操作', cls: 'nowrap', render: c => `
        <button class="btn link" onclick="PG_CUST.contactForm(${cid},${c.id})">编辑</button>
        <button class="btn link" style="color:var(--danger)" onclick="PG_CUST.removeContact(${c.id},${cid})">删除</button>`
      }
    ], list), `<button class="btn sm primary" onclick="PG_CUST.contactForm(${cid})">＋ 新增联系人</button>`);
  },

  async contactForm(cid, id) {
    let list = await API.get(`/api/customers/${cid}/contacts`);
    const cur = id ? list.find(x => x.id === id) : {};
    await UI.form({
      title: id ? '编辑联系人' : '新增联系人',
      fields: [
        { k: 'name', label: '姓名', required: true },
        { k: 'phone', label: '电话' },
        { k: 'email', label: '邮箱' },
        { k: 'role', label: '角色', ph: '如 网络管理员' },
        { k: 'remark', label: '备注', type: 'textarea', full: true }
      ],
      values: cur,
      onSubmit: async d => {
        if (id) await API.put('/api/contacts/' + id, d);
        else await API.post(`/api/customers/${cid}/contacts`, d);
        UI.toast('已保存', 'ok'); App.reload();
      }
    });
  },

  async removeContact(id, cid) {
    if (!await UI.confirm('确认删除该联系人？')) return;
    await API.del('/api/contacts/' + id);
    UI.toast('已删除', 'ok'); App.reload();
  },

  async remove(id) {
    if (!await UI.confirm('删除客户会同时删除其联系人、网络版本、设备档案，且不可恢复。确认删除？')) return;
    try {
      await API.del('/api/customers/' + id);
      UI.toast('已删除', 'ok');
      location.hash = '#/customers';
    } catch (e) { }
  },

  /* ---------------- 网络信息版本 ---------------- */
  async tabNetwork(cid) {
    const [list, c] = await Promise.all([
      API.get(`/api/customers/${cid}/net-versions`),
      API.get('/api/customers/' + cid)
    ]);
    const rows = list.map(v => {
      const cur = Number(v.is_current) === 1;
      return `<tr>
        <td class="nowrap"><strong>V${v.version_no}</strong> ${cur ? '<span class="badge green">当前版本</span>' : ''}</td>
        <td class="nowrap">${UI.esc((v.content || '').slice(0, 60))}…</td>
        <td class="nowrap">${v.change_count} 项变更</td>
        <td class="nowrap">${UI.v(v.created_by)}</td>
        <td class="nowrap muted">${UI.fmtTime(v.created_at)}</td>
        <td class="nowrap">
          <a class="btn link" href="#/network/${v.id}">查看/变更历史</a>
        </td>
      </tr>`;
    }).join('');
    return `<div class="card"><header><h3>网络信息版本</h3>
      <div class="right">
        <button class="btn sm" onclick="PG_CUST.exportNet(${cid})">导出 Excel</button>
        <button class="btn sm" onclick="API.download('/api/export/pdf?type=network&project_id=0&customer_id=${cid}')">导出 PDF</button>
        <button class="btn sm primary" onclick="PG_CUST.netVersionForm(${cid})">＋ 新建版本</button>
      </div></header>
      <div class="body tight"><table class="tbl">
        <tr><th>版本</th><th>内容摘要</th><th>变更</th><th>创建人</th><th>创建时间</th><th>操作</th></tr>
        ${rows || '<tr><td class="empty" colspan="6">暂无版本</td></tr>'}
      </table></div></div>
      <div class="card"><header><h3>当前版本内容</h3></header><div class="body">
        ${this.netContentView(list.find(v => Number(v.is_current) === 1))}
      </div></div>`;
  },

  netContentView(v) {
    if (!v) return '<div class="muted">暂无网络信息</div>';
    let c = {};
    try { c = JSON.parse(v.content || '{}'); } catch (e) { }
    return `<div class="kv">` + NET_KEYS.map(k => `
      <div class="item"><div class="k">${NET_LABELS[k]}</div>
      <div class="v pre">${c[k] ? UI.esc(c[k]) : '<span class="muted">—</span>'}</div></div>`).join('') + `</div>`;
  },

  async netVersionForm(cid) {
    const list = await API.get(`/api/customers/${cid}/net-versions`);
    const cur = list.find(v => Number(v.is_current) === 1) || {};
    let content = {};
    try { content = JSON.parse(cur.content || '{}'); } catch (e) { }
    await UI.form({
      title: '新建网络信息版本',
      width: 'wide',
      note: '保存后将形成新的客户网络信息版本，并自动记录与上一版本的字段级差异。',
      fields: NET_KEYS.map(k => ({
        k, label: NET_LABELS[k], full: ['ip_plan', 'topology', 'remark', 'account'].includes(k),
        type: ['ip_plan', 'topology', 'remark'].includes(k) ? 'textarea' : 'text'
      })),
      values: content,
      submitText: '生成新版本',
      onSubmit: async d => {
        const r = await API.post(`/api/customers/${cid}/net-versions`, { content: d, note: '' });
        UI.toast('已生成 V' + r.version_no, 'ok');
        App.reload();
      }
    });
  },

  async netVersionDetail(vid) {
    const view = document.getElementById('view');
    const v = await API.get('/api/net-versions/' + vid);
    let content = {};
    try { content = JSON.parse(v.content || '{}'); } catch (e) { }
    const changes = (v.changes || []).map(ch => `
      <tr>
        <td class="nowrap">${UI.esc(NET_LABELS[ch.field_name] || ch.field_name)}</td>
        <td><span class="diff-old pre">${UI.v(ch.old_value)}</span></td>
        <td><span class="diff-new pre">${UI.v(ch.new_value)}</span></td>
        <td class="nowrap">${UI.esc({ add: '新增', update: '修改', delete: '删除' }[ch.action] || ch.action)}</td>
        <td class="nowrap muted">${UI.fmtTime(ch.changed_at)}</td>
      </tr>`).join('');
    view.innerHTML = UI.pageHead(`网络信息 V${v.version_no}`,
      `${UI.esc(v.customer_name || '')} · 创建人 ${UI.v(v.created_by)} · ${UI.fmtTime(v.created_at)}`,
      `<a class="btn" href="#/customers/${v.customer_id}?tab=network">返回</a>`) +
      `<div class="card"><header><h3>版本内容</h3></header><div class="body">${this.netContentView(v)}</div></div>
       <div class="card"><header><h3>变更历史</h3><div class="right muted">共 ${(v.changes || []).length} 项</div></header>
       <div class="body tight"><table class="tbl">
         <tr><th>字段</th><th>修改前</th><th>修改后</th><th>操作类型</th><th>修改时间</th></tr>
         ${changes || '<tr><td class="empty" colspan="5">该版本为初始版本，无变更记录</td></tr>'}
       </table></div></div>`;
  },

  exportNet(cid) {
    API.download('/api/export/excel?type=network&customer_id=' + cid);
  },

  /* ---------------- 设备档案 ---------------- */
  async tabDevices(cid) {
    const list = await API.get(`/api/customers/${cid}/devices`);
    const rows = list.map(d => `
      <tr>
        <td class="nowrap"><strong>${UI.v(d.model)}</strong></td>
        <td class="nowrap mono">${UI.v(d.serial_no)}</td>
        <td class="nowrap mono">${UI.v(d.account)}</td>
        <td class="nowrap">${UI.v(d.contact_name)}</td>
        <td class="pre" style="max-width:280px">${UI.v((d.network_info || '').slice(0, 60))}</td>
        <td class="nowrap">${this.topoChips(d.topology, true)}</td>
        <td class="nowrap">
          <button class="btn link" onclick="PG_CUST.deviceDetail(${d.id})">详情</button>
          <button class="btn link" onclick="PG_CUST.deviceForm(${cid},${d.id})">编辑</button>
          <button class="btn link" style="color:var(--danger)" onclick="PG_CUST.removeDevice(${d.id},${cid})">删除</button>
        </td>
      </tr>`).join('');
    return UI.tableCard('设备档案', `<table class="tbl">
      <tr><th>设备型号</th><th>序列号</th><th>账号密码</th><th>对接人</th><th>网络信息</th><th>拓扑图</th><th>操作</th></tr>
      ${rows || '<tr><td class="empty" colspan="7">暂无设备</td></tr>'}</table>`,
      `<button class="btn sm primary" onclick="PG_CUST.deviceForm(${cid})">＋ 新增设备</button>`);
  },

  topoChips(json, small) {
    let arr = [];
    try { arr = JSON.parse(json || '[]'); } catch (e) { }
    if (!Array.isArray(arr) || !arr.length) return '<span class="muted">—</span>';
    return arr.map(f => `<a class="badge blue" href="${UI.esc(f.url)}" target="_blank" rel="noopener" title="${UI.esc(f.name)}">${UI.esc((f.name || '').slice(-14))}</a>`).join(' ');
  },

  async deviceForm(cid, id) {
    const contacts = await API.get(`/api/customers/${cid}/contacts`);
    let d = { topology: '[]' };
    if (id) d = await API.get('/api/devices/' + id);
    let topo = [];
    try { topo = JSON.parse(d.topology || '[]'); } catch (e) { }
    const renderTopo = () => {
      const box = document.getElementById('topo-box');
      box.innerHTML = (topo || []).map((f, i) =>
        `<span class="badge blue" style="margin:2px 4px 2px 0">${UI.esc(f.name)} <a href="javascript:;" onclick="PG_CUST._delTopo(${i})" style="color:inherit;margin-left:4px">✕</a></span>`
      ).join('') || '<span class="muted">未上传</span>';
    };
    this._delTopo = i => { topo.splice(i, 1); renderTopo(); };
    await UI.form({
      title: id ? '编辑设备' : '新增设备',
      width: 'wide',
      fields: [
        { k: 'model', label: '设备型号', ph: '如 AF-1000-FH1300A' },
        { k: 'serial_no', label: '序列号' },
        {
          k: 'contact_id', label: '客户对接人', type: 'select', numeric: true,
          options: contacts.map(c => ({ value: c.id, label: c.name + (c.role ? '（' + c.role + '）' : '') }))
        },
        { k: 'account', label: '账号密码', ph: 'admin/xxxxxx' },
        { k: 'network_info', label: '网络信息', type: 'textarea', full: true, ph: '管理地址、网段、VLAN 等' },
        { k: 'remark', label: '备注', type: 'textarea', full: true }
      ],
      values: d,
      beforeHTML: `<div class="field full" style="margin-bottom:12px">
        <label>拓扑图（支持图片/文件，可多张）</label>
        <div id="topo-box" class="mb8"></div>
        <input type="file" id="topo-file" multiple style="font-size:12px">
      </div>`,
      onReady: m => {
        renderTopo();
        m.el.querySelector('#topo-file').onchange = async e => {
          for (const f of Array.from(e.target.files)) {
            try {
              const r = await API.upload('/api/upload', f, {});
              topo.push(r);
            } catch (err) { }
          }
          renderTopo();
        };
      },
      onSubmit: async data => {
        data.topology = JSON.stringify(topo);
        if (id) await API.put('/api/devices/' + id, data);
        else await API.post(`/api/customers/${cid}/devices`, data);
        UI.toast('已保存', 'ok');
        App.reload();
      }
    });
  },

  async deviceDetail(id) {
    const [d, hist] = await Promise.all([API.get('/api/devices/' + id), API.get('/api/devices/' + id + '/history')]);
    const changes = hist.map(h => `<tr>
      <td class="nowrap">${h.field_name === '*' ? '新增设备' : UI.esc(h.field_name)}</td>
      <td class="pre"><span class="diff-old">${UI.v(h.old_value)}</span></td>
      <td class="pre"><span class="diff-new">${UI.v(h.new_value)}</span></td>
      <td class="nowrap">${UI.esc({ create: '新增', add: '新增', update: '修改', delete: '删除' }[h.action] || h.action)}</td>
      <td class="nowrap muted">${UI.fmtTime(h.changed_at)}</td>
    </tr>`).join('');
    UI.modal({
      title: '设备详情 · ' + (d.model || ''),
      width: 'wide',
      body: `<div class="kv">
          <div class="item"><div class="k">设备型号</div><div class="v big">${UI.v(d.model)}</div></div>
          <div class="item"><div class="k">序列号</div><div class="v mono">${UI.v(d.serial_no)}</div></div>
          <div class="item"><div class="k">账号密码</div><div class="v mono">${UI.v(d.account)}</div></div>
          <div class="item"><div class="k">对接人</div><div class="v">${UI.v(d.contact_name)}</div></div>
          <div class="item" style="grid-column:1/-1"><div class="k">网络信息</div><div class="v pre">${UI.v(d.network_info)}</div></div>
          <div class="item" style="grid-column:1/-1"><div class="k">备注</div><div class="v pre">${UI.v(d.remark)}</div></div>
          <div class="item" style="grid-column:1/-1"><div class="k">拓扑图</div><div class="v">${this.topoChips(d.topology)}</div></div>
        </div>
        <div class="sep"></div>
        <h3 style="font-size:13px;margin-bottom:8px">变更历史</h3>
        <table class="tbl"><tr><th>字段</th><th>修改前</th><th>修改后</th><th>操作</th><th>时间</th></tr>
        ${changes || '<tr><td class="empty" colspan="5">无变更记录</td></tr>'}</table>`,
      footer: '<button class="btn" data-close>关闭</button>'
    });
  },

  async removeDevice(id, cid) {
    if (!await UI.confirm('确认删除该设备档案？')) return;
    await API.del('/api/devices/' + id);
    UI.toast('已删除', 'ok'); App.reload();
  }
};
