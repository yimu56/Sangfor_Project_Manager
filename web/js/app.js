/* 应用外壳：登录、导航、路由 */
const NAV = [
  { group: '工作台' },
  { key: '', label: '首页看板', ico: '◈', href: '#/' },
  { key: 'projects', label: '项目管理', ico: '▤', href: '#/projects' },
  { key: 'customers', label: '客户档案', ico: '☰', href: '#/customers' },
  { group: '记录与跟踪' },
  { key: 'progress', label: '进度记录', ico: '✎', href: '#/progress' },
  { key: 'issues', label: '风险问题', ico: '⚠', href: '#/issues' },
  { key: 'maintenance', label: '维保到期', ico: '⏱', href: '#/maintenance' },
  { key: 'reminders', label: '提醒中心', ico: '🔔', href: '#/reminders', badge: true },
  { group: '数据与设置' },
  { key: 'export', label: '导出中心', ico: '⤓', href: '#/export' },
  { key: 'settings', label: '系统 / 备份', ico: '⚙', href: '#/settings' }
];

const App = {
  state: { user: '', meta: null, appName: '项目管理', subtitle: '', unread: 0 },
  current: '',

  async boot() {
    this.applyTheme();
    let st = {};
    try { st = await API.get('/api/state'); } catch (e) { return; }
    this.state.appName = st.app_name || '项目管理';
    this.state.subtitle = st.subtitle || '';
    document.title = this.state.appName;
    if (!st.initialized) { this.renderSetup(); return; }
    try {
      const me = await API.get('/api/me');
      this.state.user = me.username;
      await this.loadMeta();
      this.renderShell();
      window.addEventListener('hashchange', () => this.route());
      this.route();
      this.refreshUnread();
    } catch (e) {
      this.showLogin();
    }
  },

  async loadMeta() {
    try { this.state.meta = await API.get('/api/meta'); } catch (e) { this.state.meta = {}; }
  },

  applyTheme() {
    const t = localStorage.getItem('pm-theme') || 'light';
    document.documentElement.setAttribute('data-theme', t);
  },
  toggleTheme() {
    const cur = document.documentElement.getAttribute('data-theme') === 'dark' ? 'light' : 'dark';
    document.documentElement.setAttribute('data-theme', cur);
    localStorage.setItem('pm-theme', cur);
  },

  /* ---------- 认证界面 ---------- */
  renderSetup() {
    document.getElementById('root').innerHTML = `<div class="auth-wrap"><div class="auth-card">
      <div class="logo-lg">PM</div>
      <h2>初始化系统</h2>
      <p class="sub">首次使用，请设置本机的管理员账号与密码</p>
      <div class="field"><label>用户名</label><input id="u" value="admin"></div>
      <div class="field"><label>密码（至少 6 位）</label><input id="p" type="password" placeholder="设置登录密码"></div>
      <div class="field"><label>确认密码</label><input id="p2" type="password" placeholder="再次输入"></div>
      <button class="btn primary" id="go">创建并进入</button>
      <div class="foot">数据保存在本机固定目录，不联网上传</div>
    </div></div>`;
    document.getElementById('go').onclick = async () => {
      const u = document.getElementById('u').value.trim();
      const p = document.getElementById('p').value;
      const p2 = document.getElementById('p2').value;
      if (p !== p2) return UI.toast('两次密码不一致', 'err');
      try {
        await API.post('/api/setup', { username: u, password: p });
        UI.toast('初始化完成', 'ok');
        this.boot();
      } catch (e) { }
    };
  },

  showLogin() {
    document.getElementById('root').innerHTML = `<div class="auth-wrap"><div class="auth-card">
      <div class="logo-lg">PM</div>
      <h2>${UI.esc(this.state.appName)}</h2>
      <p class="sub">${UI.esc(this.state.subtitle || '深信服实施交付个人工作台')}</p>
      <div class="field"><label>用户名</label><input id="u" value="admin"></div>
      <div class="field"><label>密码</label><input id="p" type="password" placeholder="请输入密码"></div>
      <button class="btn primary" id="go">登录</button>
      <div class="foot"><a href="javascript:;" id="forgot">忘记密码？</a></div>
    </div></div>`;
    const doLogin = async () => {
      try {
        await API.post('/api/login', { username: document.getElementById('u').value.trim(), password: document.getElementById('p').value });
        UI.toast('登录成功', 'ok');
        this.boot();
      } catch (e) { }
    };
    document.getElementById('go').onclick = doLogin;
    document.getElementById('p').onkeydown = e => { if (e.key === 'Enter') doLogin(); };
    document.getElementById('forgot').onclick = () => this.showReset();
  },

  /* 本地重置密码：生成一次性令牌 → 校验令牌 → 设置新密码 */
  showReset() {
    document.getElementById('root').innerHTML = `<div class="auth-wrap"><div class="auth-card">
      <div class="logo-lg">PM</div>
      <h2>重置密码</h2>
      <p class="sub">本机个人使用，无需邮箱；系统会生成一次性令牌</p>
      <div id="step1">
        <button class="btn" id="gen">① 生成重置令牌</button>
        <div id="tokenBox" class="hidden">
          <div class="field mt12"><label>重置令牌（30 分钟内有效）</label>
            <input id="tk" readonly style="font-family:monospace;letter-spacing:1px"></div>
          <div class="field"><label>新密码（至少 6 位）</label><input id="np" type="password"></div>
          <div class="field"><label>确认新密码</label><input id="np2" type="password"></div>
          <button class="btn primary" id="do">② 重置密码</button>
        </div>
      </div>
      <div class="foot"><a href="javascript:;" id="back">返回登录</a></div>
    </div></div>`;
    document.getElementById('back').onclick = () => this.showLogin();
    document.getElementById('gen').onclick = async () => {
      try {
        const r = await API.post('/api/password/reset-token');
        document.getElementById('tk').value = r.token;
        document.getElementById('tokenBox').classList.remove('hidden');
        UI.toast('令牌已生成，同时写入 ' + r.file, 'ok');
      } catch (e) { }
    };
    document.getElementById('do').onclick = async () => {
      const tk = document.getElementById('tk').value.trim();
      const np = document.getElementById('np').value;
      const np2 = document.getElementById('np2').value;
      if (np !== np2) return UI.toast('两次密码不一致', 'err');
      if (np.length < 6) return UI.toast('密码至少 6 位', 'err');
      try {
        await API.post('/api/password/reset', { token: tk, new: np });
        UI.toast('密码已重置，请重新登录', 'ok');
        this.showLogin();
      } catch (e) { }
    };
  },

  /* ---------- 主框架 ---------- */
  renderShell() {
    document.getElementById('root').innerHTML = `
      <div id="app">
        <aside class="sidebar">
          <div class="brand">
            <h1><span class="logo">PM</span>${UI.esc(this.state.appName)}</h1>
            <small>深信服实施交付工作台</small>
          </div>
          <nav class="nav" id="nav"></nav>
          <div style="padding:10px 14px;border-top:1px solid rgba(255,255,255,.07);font-size:11px;color:#55637a">
            v1.0.0 · 本地版
          </div>
        </aside>
        <div class="main">
          <header class="topbar">
            <div class="search-box">
              <span class="mag">⌕</span>
              <input id="gsearch" placeholder="搜索项目 / 客户 / 进度 / 问题…">
              <div id="sp" class="search-panel hidden"></div>
            </div>
            <div class="spacer"></div>
            <button class="icon-btn" id="themeBtn" title="切换深色/浅色">◐</button>
            <button class="icon-btn" id="bell" title="提醒中心">🔔<span class="dot hidden" id="bellDot">0</span></button>
            <div class="user-chip"><span class="avatar">${UI.esc((this.state.user || 'A').slice(0, 1).toUpperCase())}</span>
              <span>${UI.esc(this.state.user)}</span></div>
            <button class="icon-btn" id="userBtn" title="账号">⋯</button>
          </header>
          <div class="view" id="view"></div>
        </div>
      </div>`;
    this.renderNav();
    document.getElementById('themeBtn').onclick = () => this.toggleTheme();
    document.getElementById('bell').onclick = () => location.hash = '#/reminders';
    document.getElementById('userBtn').onclick = () => this.userMenu();
    this.bindSearch();
  },

  renderNav(activeKey) {
    const key = activeKey === undefined ? this.current : activeKey;
    document.getElementById('nav').innerHTML = NAV.map(n => {
      if (n.group) return `<div class="nav-group">${UI.esc(n.group)}</div>`;
      const on = n.key === key ? ' on' : '';
      const badge = n.badge && this.state.unread > 0 ? `<span class="cnt">${this.state.unread}</span>` : '';
      return `<a href="${n.href}" class="${on}">${n.ico ? `<span class="ico">${n.ico}</span>` : ''}${UI.esc(n.label)}${badge}</a>`;
    }).join('');
  },

  setActive(key) {
    this.current = key;
    this.renderNav(key);
  },

  async refreshUnread() {
    try {
      const list = await API.get('/api/reminders?unread=1');
      this.state.unread = (list || []).length;
      const dot = document.getElementById('bellDot');
      if (dot) {
        dot.classList.toggle('hidden', this.state.unread === 0);
        dot.textContent = this.state.unread > 99 ? '99+' : this.state.unread;
      }
      this.renderNav();
    } catch (e) { }
  },

  userMenu() {
    const m = UI.modal({
      title: '账号',
      width: 'narrow',
      body: `<div class="kv"><div class="item"><div class="k">当前用户</div><div class="v">${UI.esc(this.state.user)}</div></div></div>
        <div class="sep"></div>
        <div class="flex" style="flex-direction:column;align-items:stretch;gap:8px">
          <button class="btn" id="chpwd">修改密码</button>
          <button class="btn danger" id="logout">退出登录</button>
        </div>`,
      footer: '<button class="btn" data-close>关闭</button>'
    });
    m.el.querySelector('#chpwd').onclick = () => { m.close(); this.changePassword(); };
    m.el.querySelector('#logout').onclick = async () => {
      await API.post('/api/logout');
      location.hash = '';
      this.boot();
    };
  },

  changePassword() {
    UI.form({
      title: '修改密码',
      width: 'narrow',
      fields: [
        { k: 'old', label: '原密码', type: 'password', required: true, full: true },
        { k: 'new', label: '新密码（至少 6 位）', type: 'password', required: true, full: true }
      ],
      submitText: '保存',
      onSubmit: async d => { await API.post('/api/password/change', d); UI.toast('密码已更新', 'ok'); }
    });
  },

  bindSearch() {
    const input = document.getElementById('gsearch');
    const panel = document.getElementById('sp');
    let timer = null;
    input.addEventListener('input', () => {
      clearTimeout(timer);
      const q = input.value.trim();
      if (!q) { panel.classList.add('hidden'); return; }
      timer = setTimeout(() => this.doSearch(q, panel), 260);
    });
    input.addEventListener('focus', () => { if (input.value.trim()) panel.classList.remove('hidden'); });
    document.addEventListener('click', e => {
      if (!panel.contains(e.target) && e.target !== input) panel.classList.add('hidden');
    });
  },

  async doSearch(q, panel) {
    let r;
    try { r = await API.get('/api/search?q=' + encodeURIComponent(q)); } catch (e) { return; }
    if (!r || !r.total) {
      panel.innerHTML = `<div class="sp-group">无匹配结果</div>`;
      panel.classList.remove('hidden');
      return;
    }
    const sec = (title, arr, fn) => {
      if (!arr || !arr.length) return '';
      return `<div class="sp-group">${title}</div>` + arr.map(x => `<a href="${fn(x)}" onclick="document.getElementById('sp').classList.add('hidden')">${UI.esc(this.searchLabel(title, x))}</a>`).join('');
    };
    panel.innerHTML =
      sec('项目', r.projects, x => '#/projects/' + x.id) +
      sec('客户', r.customers, x => '#/customers/' + x.id) +
      sec('进度记录', r.progress, x => '#/projects/' + x.project_id + '?tab=progress') +
      sec('风险问题', r.issues, x => '#/projects/' + x.project_id + '?tab=issues') +
      sec('附件', r.attachments, x => '#/projects/' + x.project_id + '?tab=attachments') +
      sec('设备', r.devices, x => '#/customers/' + x.customer_id + '?tab=devices');
    panel.classList.remove('hidden');
  },

  searchLabel(group, x) {
    if (group === '项目') return `${x.code || ''} ${x.name} · ${x.customer_name || ''} · ${x.status || ''}`;
    if (group === '客户') return `${x.code || ''} ${x.name}`;
    if (group === '进度记录') return `${x.record_date} ${x.project_name} · ${(x.current_progress || '').slice(0, 40)}`;
    if (group === '风险问题') return `${x.project_name} · ${x.title} [${x.status}]`;
    if (group === '附件') return `${x.project_name} · [${x.atype}] ${x.file_name}`;
    if (group === '设备') return `${x.customer_name} · ${x.model || ''} ${x.serial_no || ''}`;
    return '';
  },

  /* ---------- 路由 ---------- */
  route() {
    if (!document.getElementById('view')) return;
    const { seg, params } = Q.parse();
    const view = document.getElementById('view');
    view.innerHTML = '<div class="empty-state">加载中…</div>';
    const k = seg[0] || '';
    this.setActive(k);
    const tab = params.tab || '';
    const run = fn => { try { fn(); } catch (e) { console.error(e); view.innerHTML = `<div class="card"><div class="body">页面渲染失败：${UI.esc(e.message)}</div></div>`; } };
    switch (k) {
      case '': run(() => PG_HOME.dashboard()); break;
      case 'customers':
        if (seg[1] === 'new') run(() => PG_CUST.form());
        else if (seg[1]) run(() => PG_CUST.detail(Number(seg[1]), tab));
        else run(() => PG_CUST.list());
        break;
      case 'network': run(() => PG_CUST.netVersionDetail(Number(seg[1]))); break;
      case 'projects':
        if (seg[1] === 'new') run(() => PG_PROJ.form(null, params));
        else if (seg[1] && seg[2] === 'edit') run(() => PG_PROJ.form(Number(seg[1]), params));
        else if (seg[1] && seg[2] === 'convert') run(() => PG_PROJ.convertForm(Number(seg[1]), params.mode));
        else if (seg[1]) run(() => PG_PROJ.detail(Number(seg[1]), tab));
        else run(() => PG_PROJ.list(params));
        break;
      case 'progress': run(() => PG_REC.progressList(params)); break;
      case 'issues': run(() => PG_REC.issueList(params)); break;
      case 'maintenance': run(() => PG_MISC.maintenance()); break;
      case 'reminders': run(() => PG_MISC.reminders()); break;
      case 'export': run(() => PG_MISC.exportCenter()); break;
      case 'settings': run(() => PG_MISC.settings(params.tab)); break;
      default: view.innerHTML = UI.empty('页面不存在');
    }
    window.scrollTo(0, 0);
  },

  go(hash) { location.hash = hash; },
  reload() { this.route(); }
};
