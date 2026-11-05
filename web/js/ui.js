/* UI 基础组件：提示、弹窗、表单、表格、徽标 */
const UI = {
  esc(s) {
    if (s === null || s === undefined) return '';
    return String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;').replace(/'/g, '&#39;');
  },
  v(s, def) {
    if (s === null || s === undefined || s === '') return def === undefined ? '—' : def;
    return this.esc(s);
  },
  today() {
    const d = new Date();
    return d.getFullYear() + '-' + String(d.getMonth() + 1).padStart(2, '0') + '-' + String(d.getDate()).padStart(2, '0');
  },
  fmtDate(s) {
    if (!s) return '—';
    return this.esc(String(s).slice(0, 10));
  },
  fmtTime(s) {
    if (!s) return '—';
    return this.esc(String(s).slice(0, 16));
  },
  fmtSize(n) {
    n = Number(n || 0);
    if (n < 1024) return n + ' B';
    if (n < 1024 * 1024) return (n / 1024).toFixed(1) + ' KB';
    if (n < 1024 * 1024 * 1024) return (n / 1048576).toFixed(1) + ' MB';
    return (n / 1073741824).toFixed(2) + ' GB';
  },
  daysUntil(dateStr) {
    if (!dateStr) return null;
    const d = new Date(String(dateStr).slice(0, 10) + 'T00:00:00');
    if (isNaN(d)) return null;
    const t = new Date(); t.setHours(0, 0, 0, 0);
    return Math.round((d - t) / 86400000);
  },

  toast(msg, type) {
    const el = document.createElement('div');
    el.className = 'toast ' + (type === 'err' ? 'err' : type === 'ok' ? 'ok' : '');
    el.textContent = msg;
    document.getElementById('toasts').appendChild(el);
    setTimeout(() => { el.style.opacity = '0'; el.style.transition = 'opacity .25s'; }, 2600);
    setTimeout(() => el.remove(), 2950);
  },

  modal(opts) {
    const root = document.getElementById('modal-root');
    const mask = document.createElement('div');
    mask.className = 'mask';
    const width = opts.width ? ' ' + opts.width : '';
    mask.innerHTML = `<div class="modal${width}">
      <header><h3>${this.esc(opts.title || '')}</h3><span class="x">✕</span></header>
      <div class="mbody">${opts.body || ''}</div>
      ${opts.footer === null ? '' : `<footer>${opts.footer || '<button class="btn" data-close>关闭</button>'}</footer>`}
    </div>`;
    root.appendChild(mask);
    const close = () => { mask.remove(); document.removeEventListener('keydown', onKey); };
    const onKey = e => { if (e.key === 'Escape') close(); };
    document.addEventListener('keydown', onKey);
    mask.querySelector('.x').onclick = close;
    mask.querySelectorAll('[data-close]').forEach(b => b.onclick = () => { if (opts.onClose) opts.onClose(); close(); });
    mask.addEventListener('click', e => { if (e.target === mask) { if (opts.onClose) opts.onClose(); close(); } });
    return { el: mask.querySelector('.modal'), close };
  },

  confirm(msg, title) {
    return new Promise(resolve => {
      const m = this.modal({
        title: title || '确认操作',
        width: 'narrow',
        body: `<div class="pre">${this.esc(msg)}</div>`,
        footer: `<button class="btn" data-close>取消</button><button class="btn primary" data-yes>确定</button>`
      });
      let done = false;
      m.el.querySelector('[data-yes]').onclick = () => { done = true; m.close(); resolve(true); };
      m.el.querySelector('[data-close]').onclick = () => { if (!done) resolve(false); m.close(); };
      m.el.querySelector('.x').onclick = () => { if (!done) resolve(false); m.close(); };
    });
  },

  /* 表单字段 HTML */
  fieldHTML(f, val) {
    const id = 'f_' + f.k;
    const v = val === undefined || val === null ? (f.def !== undefined ? f.def : '') : val;
    const req = f.required ? '<span class="req">*</span>' : '';
    let input = '';
    if (f.type === 'textarea') {
      input = `<textarea id="${id}" data-k="${f.k}" placeholder="${this.esc(f.ph || '')}" ${f.rows ? `style="min-height:${f.rows * 22}px"` : ''}>${this.esc(v)}</textarea>`;
    } else if (f.type === 'select') {
      const opts = (f.options || []).map(o => {
        const ov = typeof o === 'object' ? o.value : o;
        const ol = typeof o === 'object' ? o.label : o;
        return `<option value="${this.esc(ov)}" ${String(ov) === String(v) ? 'selected' : ''}>${this.esc(ol)}</option>`;
      }).join('');
      input = `<select id="${id}" data-k="${f.k}" data-type="select">${f.empty !== false ? '<option value="">请选择</option>' : ''}${opts}</select>`;
    } else if (f.type === 'checkbox') {
      input = `<div class="check-row"><label><input type="checkbox" id="${id}" data-k="${f.k}" data-type="check" ${v ? 'checked' : ''}> ${this.esc(f.checkLabel || '是')}</label></div>`;
    } else if (f.type === 'checks') {
      const arr = Array.isArray(v) ? v.map(String) : (v ? String(v).split(',') : []);
      input = `<div class="check-row" id="${id}" data-k="${f.k}" data-type="checks">` +
        (f.options || []).map(o => {
          const ov = typeof o === 'object' ? o.value : o;
          const ol = typeof o === 'object' ? o.label : o;
          return `<label><input type="checkbox" value="${this.esc(ov)}" ${arr.includes(String(ov)) ? 'checked' : ''}> ${this.esc(ol)}</label>`;
        }).join('') + '</div>';
    } else if (f.type === 'static') {
      input = `<div class="pre" style="font-size:13px;padding:4px 0">${v}</div>`;
    } else {
      const type = f.type === 'number' ? 'number' : f.type === 'date' ? 'date' : f.type === 'password' ? 'password' : 'text';
      input = `<input type="${type}" id="${id}" data-k="${f.k}" value="${this.esc(v)}" placeholder="${this.esc(f.ph || '')}" ${f.readonly ? 'readonly' : ''} ${f.type === 'number' ? 'step="any"' : ''}>`;
    }
    return `<div class="field ${f.full ? 'full' : ''}" ${f.hide ? 'style="display:none"' : ''}>
      <label for="${id}">${this.esc(f.label)}${req}</label>${input}
      ${f.hint ? `<div class="hint">${this.esc(f.hint)}</div>` : ''}
    </div>`;
  },

  formHTML(fields, values) {
    return `<div class="form-grid">` + fields.map(f => this.fieldHTML(f, values ? values[f.k] : undefined)).join('') + `</div>`;
  },

  collect(scope, fields) {
    const out = {};
    fields.forEach(f => {
      const el = scope.querySelector('[data-k="' + f.k + '"]');
      if (!el) return;
      const t = el.getAttribute('data-type');
      if (t === 'checks') {
        out[f.k] = Array.from(el.querySelectorAll('input:checked')).map(i => i.value);
      } else if (t === 'check') {
        out[f.k] = el.checked ? 1 : 0;
      } else if (t === 'select') {
        out[f.k] = el.value === '' ? null : (f.numeric ? Number(el.value) : el.value);
      } else {
        out[f.k] = el.value.trim() === '' ? (f.numeric ? null : '') : el.value;
      }
    });
    return out;
  },

  /* 打开表单弹窗，成功提交后 resolve(data)，取消 resolve(null) */
  form(opts) {
    return new Promise(resolve => {
      const fields = opts.fields || [];
      const body = `<div id="form-scope">${opts.beforeHTML || ''}${this.formHTML(fields, opts.values || {})}${opts.afterHTML || ''}</div>
        ${opts.note ? `<div class="hint" style="margin-top:10px">${this.esc(opts.note)}</div>` : ''}`;
      const m = this.modal({
        title: opts.title,
        width: opts.width,
        body,
        footer: `<button class="btn" data-close>取消</button>
                 <button class="btn primary" data-ok>${this.esc(opts.submitText || '保存')}</button>`,
        onClose: () => resolve(null)
      });
      const scope = m.el.querySelector('#form-scope');
      const submit = async () => {
        const data = this.collect(scope, fields);
        for (const f of fields) {
          if (f.required && (data[f.k] === '' || data[f.k] === null || data[f.k] === undefined)) {
            this.toast('请填写：' + f.label, 'err');
            const el = scope.querySelector('[data-k="' + f.k + '"]');
            if (el && el.focus) el.focus();
            return;
          }
        }
        const btn = m.el.querySelector('[data-ok]');
        btn.disabled = true;
        try {
          await opts.onSubmit(data, m);
          m.close();
          resolve(data);
        } catch (e) {
          btn.disabled = false;
        }
      };
      m.el.querySelector('[data-ok]').onclick = submit;
      if (opts.onReady) opts.onReady(m);
      const first = m.el.querySelector('input,textarea,select');
      if (first) setTimeout(() => first.focus(), 60);
    });
  },

  table(cols, rows, opts) {
    opts = opts || {};
    const head = `<tr>${cols.map(c => `<th class="${c.cls || ''}" ${c.w ? `style="width:${c.w}"` : ''}>${this.esc(c.label)}</th>`).join('')}</tr>`;
    let body = '';
    if (!rows || rows.length === 0) {
      body = `<tr><td class="empty" colspan="${cols.length}">${this.esc(opts.empty || '暂无数据')}</td></tr>`;
    } else {
      body = rows.map((r, i) => `<tr data-i="${i}" class="${opts.rowClass ? opts.rowClass(r) : ''}">` +
        cols.map(c => `<td class="${c.cls || ''}">${c.render ? c.render(r, i) : this.v(r[c.k])}</td>`).join('') + `</tr>`).join('');
    }
    return `<table class="tbl">${head}${body}</table>`;
  },

  empty(text, sub) {
    return `<div class="empty-state"><div class="big">◌</div><div>${this.esc(text)}</div>${sub ? `<div class="muted" style="font-size:12px;margin-top:4px">${this.esc(sub)}</div>` : ''}</div>`;
  },

  statusBadge(s) {
    if (!s) return '<span class="badge gray">—</span>';
    const map = {
      '未开始': 'gray', '进行中': 'blue', '待验收': 'orange', '已验收': 'green', '已结束': 'gray',
      '暂停': 'orange', '取消': 'gray', '维保中': 'blue', '在保': 'green', '即将过保': 'orange', '过保': 'red',
      '未关闭': 'red', '已关闭': 'green'
    };
    return `<span class="badge ${map[s] || 'gray'}">${this.esc(s)}</span>`;
  },
  levelBadge(l) {
    const map = { '高': 'red', '中': 'orange', '低': 'gray' };
    return `<span class="badge ${map[l] || 'gray'}">${this.esc(l || '—')}</span>`;
  },
  priorityBadge(p) {
    const map = { '高': 'red', '中': 'blue', '低': 'gray' };
    return `<span class="badge ${map[p] || 'gray'}">${this.esc(p || '—')}</span>`;
  },
  typeBadge(t) {
    const map = { '维保': 'green', '加点': 'orange', '新实施': 'blue', '测试': 'gray' };
    return `<span class="badge ${map[t] || 'gray'}">${this.esc(t || '—')}</span>`;
  },

  tableCard(title, tableHTML, rightHTML) {
    return `<div class="card"><header><h3>${this.esc(title)}</h3><div class="right">${rightHTML || ''}</div></header>
      <div class="body tight">${tableHTML}</div></div>`;
  },

  pageHead(title, sub, rightHTML) {
    return `<div class="page-head"><div><h2>${title}</h2>${sub ? `<div class="sub">${this.esc(sub)}</div>` : ''}</div>
      <div class="right">${rightHTML || ''}</div></div>`;
  },

  tabs(items, active) {
    return `<div class="tabs">` + items.map(t =>
      `<a href="${t.href}" class="${t.key === active ? 'on' : ''}">${this.esc(t.label)}${t.count !== undefined ? ` <span class="muted">${t.count}</span>` : ''}</a>`
    ).join('') + `</div>`;
  },

  download(path) { API.download(path); }
};
