/* 后端接口封装 */
const API = {
  async req(method, path, body, opts) {
    const init = { method, headers: {}, credentials: 'same-origin' };
    if (body !== undefined && body !== null) {
      init.headers['Content-Type'] = 'application/json';
      init.body = JSON.stringify(body);
    }
    if (opts && opts.raw) return fetch(path, init);
    let res;
    try {
      res = await fetch(path, init);
    } catch (e) {
      UI.toast('网络错误：' + e.message, 'err');
      throw e;
    }
    if (res.status === 401) {
      App.showLogin();
      throw new Error('未登录');
    }
    const ct = res.headers.get('Content-Type') || '';
    if (ct.includes('application/json')) {
      const json = await res.json();
      if (!res.ok) {
        UI.toast(json.error || '请求失败', 'err');
        throw new Error(json.error || 'request failed');
      }
      return json.data;
    }
    if (!res.ok) {
      UI.toast('请求失败 (' + res.status + ')', 'err');
      throw new Error('request failed');
    }
    return res;
  },
  get(p) { return this.req('GET', p); },
  post(p, b) { return this.req('POST', p, b === undefined ? {} : b); },
  put(p, b) { return this.req('PUT', p, b); },
  del(p) { return this.req('DELETE', p); },

  /* 文件上传 */
  async upload(path, file, fields) {
    const fd = new FormData();
    fd.append('file', file);
    Object.keys(fields || {}).forEach(k => fd.append(k, fields[k]));
    const res = await fetch(path, { method: 'POST', body: fd, credentials: 'same-origin' });
    if (res.status === 401) { App.showLogin(); throw new Error('未登录'); }
    const json = await res.json().catch(() => ({}));
    if (!res.ok) { UI.toast(json.error || '上传失败', 'err'); throw new Error(json.error || 'upload failed'); }
    return json.data;
  },

  /* 带 cookies 的下载 */
  download(path) {
    const a = document.createElement('a');
    a.href = path;
    a.rel = 'noopener';
    document.body.appendChild(a);
    a.click();
    a.remove();
  }
};

const Q = {
  parse() {
    const h = location.hash.replace(/^#\/?/, '');
    const [path, qs] = h.split('?');
    const params = {};
    new URLSearchParams(qs || '').forEach((v, k) => params[k] = v);
    return { seg: path.split('/').filter(Boolean), params };
  }
};
