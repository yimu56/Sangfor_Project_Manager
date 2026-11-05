/* 首页看板 */
const PG_HOME = {
  async dashboard() {
    const view = document.getElementById('view');
    let d;
    try { d = await API.get('/api/dashboard'); } catch (e) { return; }
    const s = d.stats || {};
    const th = d.thresholds || {};

    const stats = `<div class="stat-grid">
      <div class="stat blue" onclick="location.hash='#/projects'"><div class="n">${s.active_projects || 0}</div><div class="l">在办项目</div></div>
      <div class="stat green" onclick="location.hash='#/progress'"><div class="n">${d.week_count || 0}</div><div class="l">本周进度记录</div></div>
      <div class="stat orange" onclick="location.hash='#/maintenance'"><div class="n">${s.expiring_count || 0}</div><div class="l">${th.warranty_notice} 天内到期维保</div></div>
      <div class="stat red" onclick="location.hash='#/issues'"><div class="n">${s.open_issues || 0}</div><div class="l">未关闭问题</div></div>
      <div class="stat" onclick="location.hash='#/reminders'"><div class="n">${s.unread_reminders || 0}</div><div class="l">未读提醒</div></div>
    </div>`;

    // 下一步计划
    const plans = (d.next_plans || []).map(p => `
      <div class="mini-item plan-list-row">
        <div class="p">
          <div class="t"><a href="#/projects/${p.id}">${UI.esc(p.name)}</a> ${UI.typeBadge(p.ptype)}</div>
          <div class="m">${UI.esc(p.customer_name)} · 下一步：<span style="color:${p.next_plan_show === '未定' ? 'var(--text-3)' : 'var(--text)'}">${UI.esc(p.next_plan_show)}</span></div>
        </div>
        <div class="r">${UI.statusBadge(p.status)}<div class="m">${p.stale_days !== undefined ? p.stale_days + ' 天未更新' : ''}</div></div>
      </div>`).join('');

    // 本周进度
    const week = (d.week_progress || []).map(g => `
      <div class="mini-item">
        <div style="flex:1;min-width:0">
          <div class="t">${UI.esc((g.current_progress || '').slice(0, 80))}</div>
          <div class="m"><a href="#/projects/${g.project_id}?tab=progress">${UI.esc(g.project_name)}</a> · ${UI.esc(g.customer_name)}${g.stage_names ? ' · ' + UI.esc(g.stage_names) : ''}</div>
        </div>
        <div class="r"><span class="badge gray">${UI.fmtDate(g.record_date)}</span></div>
      </div>`).join('');

    // 长时间未结束
    const longrun = (d.long_running || []).map(p => `
      <div class="mini-item">
        <div style="flex:1;min-width:0">
          <div class="t"><a href="#/projects/${p.id}">${UI.esc(p.name)}</a></div>
          <div class="m">${UI.esc(p.customer_name)} · ${UI.esc(p.ptype)} · 开始于 ${UI.fmtDate(p.start_date || p.created_at)}</div>
        </div>
        <div class="r"><span class="badge orange">${p.running_days} 天</span></div>
      </div>`).join('');

    // 即将到期维保
    const expiring = (d.expiring || []).map(m => {
      const dl = m.days_left;
      const cls = dl < 0 ? 'red' : dl <= 30 ? 'orange' : 'gray';
      return `<div class="mini-item">
        <div style="flex:1;min-width:0">
          <div class="t"><a href="#/projects/${m.project_id}?tab=maintenance">${UI.esc(m.project_name)}</a></div>
          <div class="m">${UI.esc(m.customer_name)} · 到期 ${UI.fmtDate(m.expire_date)}</div>
        </div>
        <div class="r"><span class="badge ${cls}">${dl < 0 ? '已过保 ' + (-dl) + ' 天' : '剩 ' + dl + ' 天'}</span></div>
      </div>`;
    }).join('');

    // 未关闭问题
    const issues = (d.open_issues || []).map(i => `
      <div class="mini-item">
        <div style="flex:1;min-width:0">
          <div class="t">${UI.esc(i.title)}</div>
          <div class="m"><a href="#/projects/${i.project_id}?tab=issues">${UI.esc(i.project_name)}</a> · ${UI.esc(i.customer_name)}${i.owner ? ' · ' + UI.esc(i.owner) : ''}</div>
        </div>
        <div class="r">${UI.levelBadge(i.level)}</div>
      </div>`).join('');

    // 长期未更新
    const stale = (d.stale || []).map(p => `
      <div class="mini-item">
        <div style="flex:1;min-width:0">
          <div class="t"><a href="#/projects/${p.id}?tab=progress">${UI.esc(p.name)}</a></div>
          <div class="m">${UI.esc(p.customer_name)} · 最后进度 ${UI.fmtDate(p.last_progress_date) || '无记录'}</div>
        </div>
        <div class="r"><span class="badge orange">${p.stale_days} 天未更新</span></div>
      </div>`).join('');

    const card = (title, body, right) => `<div class="card"><header><h3>${title}</h3><div class="right">${right || ''}</div></header>
      <div class="mini-list">${body || `<div class="empty-state" style="padding:26px">暂无</div>`}</div></div>`;

    view.innerHTML = UI.pageHead('首页看板',
      `今天 ${UI.today()} · 本周 ${d.week_range ? d.week_range.start + ' ~ ' + d.week_range.end : ''}`,
      `<button class="btn" onclick="PG_MISC.scanReminders()">扫描提醒</button>
       <button class="btn primary" onclick="location.hash='#/projects/new'">＋ 新建项目</button>`) +
      stats +
      `<div class="dash-grid">
        <div>
          ${card('每个项目的下一步计划', plans)}
          ${card('本周进度记录 <span class="muted" style="font-weight:400">(' + (d.week_count || 0) + ' 条)</span>', week,
        `<a href="#/progress" class="btn sm ghost">全部记录</a>`)}
        </div>
        <div>
          ${card('长时间未结束项目 <span class="muted" style="font-weight:400">(超过 ' + th.longrun_days + ' 天)</span>', longrun)}
          ${card('即将到期维保 <span class="muted" style="font-weight:400">(' + th.warranty_notice + ' 天内)</span>', expiring,
          `<a href="#/maintenance" class="btn sm ghost">到期清单</a>`)}
          ${card('未关闭问题', issues, `<a href="#/issues" class="btn sm ghost">问题清单</a>`)}
          ${stale ? card('长期未更新项目 <span class="muted" style="font-weight:400">(超过 ' + th.stale_days + ' 天)</span>', stale) : ''}
        </div>
      </div>`;
  }
};

window.PG_MISC = window.PG_MISC || {};
PG_MISC.scanReminders = async function () {
  const r = await API.post('/api/reminders/scan');
  UI.toast(`扫描完成，处理 ${r.scanned} 条提醒条件`, 'ok');
  App.refreshUnread();
  App.reload();
};
