// 番茄科技 · 管理后台（仅 admin 可用）
const $ = (s, r = document) => r.querySelector(s);
const esc = s => String(s ?? '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
const toast = m => { const e = document.createElement('div'); e.className = 'toast'; e.textContent = m; document.body.appendChild(e); setTimeout(() => e.remove(), 2200); };
async function api(method, path, body) {
  const isForm = body instanceof FormData;
  const res = await fetch('/api' + path, { method, credentials: 'same-origin', headers: body && !isForm ? { 'Content-Type': 'application/json' } : {}, body: body ? (isForm ? body : JSON.stringify(body)) : undefined });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) { const e = new Error(data.error || '请求失败'); e.errors = data.errors; throw e; }
  return data;
}
const S = { user: null, tab: 'dramas', theaters: [], dramas: [], apps: [], users: [], result: null };
const TABS = [['dramas', '剧目'], ['theaters', '剧场'], ['apps', '报白审核'], ['import', '报表导入'], ['users', '用户']];
const layer = h => { $('#layer').innerHTML = h ? `<div class="mask" data-act="mask">${h}</div>` : ''; };
const field = (label, input) => `<div class="field"><label>${label}</label>${input}</div>`;
const opt = (list, val) => list.map(([v, l]) => `<option value="${esc(v)}" ${String(v) === String(val) ? 'selected' : ''}>${esc(l)}</option>`).join('');
const cover = d => d.cover ? `background-image:url(${esc(d.cover)})` : `background:linear-gradient(160deg,hsl(${d.hue} 75% 55%),hsl(${(d.hue + 340) % 360} 60% 22%))`;

async function load() {
  S.theaters = await api('GET', '/theaters');
  if (S.tab === 'dramas') S.dramas = await api('GET', '/dramas');
  if (S.tab === 'apps') S.apps = await api('GET', '/admin/applications');
  if (S.tab === 'users') S.users = await api('GET', '/admin/users');
  render();
}

function login() {
  $('#app').innerHTML = `<div class="panel" style="margin:60px 0;background:var(--surface);border:1px solid var(--line);border-radius:20px;padding:22px"><h2>管理后台登录</h2><p class="muted" style="margin:4px 0 16px">仅管理员账号可进入</p>
    <form id="f">${field('账号', '<input class="in" name="a" value="admin">')}${field('密码', '<input class="in" type="password" name="p">')}<div class="err" id="e"></div><button class="btn primary block">登录</button></form></div>`;
  $('#f').onsubmit = async e => {
    e.preventDefault();
    try {
      const u = await api('POST', '/auth/login', { account: e.target.a.value, password: e.target.p.value });
      if (u.role !== 'admin') { await api('POST', '/auth/logout'); throw new Error('该账号不是管理员'); }
      S.user = u; load();
    } catch (x) { $('#e').textContent = x.message; }
  };
}

function render() {
  const body = { dramas: vDramas, theaters: vTheaters, apps: vApps, import: vImport, users: vUsers }[S.tab]();
  $('#app').innerHTML = `<div style="display:flex;justify-content:space-between;align-items:center;padding-top:calc(env(safe-area-inset-top) + 12px)"><b style="font-size:20px">番茄科技 · 管理后台</b><span><a href="/" class="muted">返回前台</a> <button class="muted" data-act="logout">退出</button></span></div>
    <div class="tabs">${TABS.map(([k, l]) => `<button data-act="tab" data-v="${k}" class="${S.tab === k ? 'on' : ''}">${l}</button>`).join('')}</div>${body}`;
}

const vDramas = () => `<div class="row" style="margin-bottom:12px"><button class="btn primary" data-act="editDrama">＋ 新增剧目</button><button class="btn" data-act="batchDrama">⇪ 批量导入</button></div>
  <div class="card" style="padding:0">${S.dramas.map(d => `<div class="rk"><div class="thumb" style="${cover(d)}"></div><div><b>${esc(d.title)}</b><span class="muted">${esc(d.theater)} · ${d.episodes} 集 · ${esc(d.type || '—')} · ${d.status}</span></div><div class="ops"><button class="btn sm" data-act="editDrama" data-v="${d.id}">编辑</button><button class="btn sm" data-act="delDrama" data-v="${d.id}">删除</button></div></div>`).join('') || '<p class="muted" style="padding:16px">暂无剧目</p>'}</div>`;

const vTheaters = () => `<div class="card"><b>新增剧场</b><div class="row" style="margin-top:10px"><input class="in" id="tn" placeholder="剧场名称" style="flex:2"><button class="btn primary" data-act="addTheater" style="flex:none">添加</button></div></div>
  <div class="card" style="padding:0">${S.theaters.map(t => `<div class="rk" data-tid="${t.id}"><div><input class="in" data-f="name" value="${esc(t.name)}"></div><span class="muted" style="flex:none;font-size:11px;text-align:right;white-space:nowrap">本月 ${t.month_used}/100<br>全年 ${t.year_used}/200</span><div class="ops"><button class="btn sm" data-act="saveTheater" data-v="${t.id}">保存</button><button class="btn sm" data-act="delTheater" data-v="${t.id}">删除</button></div></div>`).join('')}</div>`;

const vApps = () => `<div class="card" style="padding:0">${S.apps.map(a => `<div class="rk"><div><b>${esc(a.talent)} · ${esc(a.account)}</b><span class="muted">${esc(a.theater)} · ${esc(a.channel_id || '无 ID')} · ${esc(a.created_at.slice(0, 16))}${a.note ? ' · ' + esc(a.note) : ''}</span></div>${a.status === '待审核' ? `<div class="ops"><button class="btn primary sm" data-act="review" data-v="${a.id}" data-s="已通过">通过</button><button class="btn sm" data-act="review" data-v="${a.id}" data-s="已驳回">驳回</button></div>` : `<span class="tag ${a.status === '已通过' ? 'good' : ''}">${a.status}</span>`}</div>`).join('') || '<p class="muted" style="padding:16px">暂无申请</p>'}</div>`;

const vImport = () => `<div class="card"><b>导入收益报表（CSV）</b><p class="muted" style="margin:6px 0 12px">列顺序：日期(YYYY-MM-DD), 视频号ID, 剧目名称, 收益, 视频播放量, 剧集播放量。首行表头可有可无；有任何错误行则整体不导入。</p>
  <input type="file" id="csv" accept=".csv,text/csv" class="in" style="padding-top:10px"><button class="btn primary block" data-act="doImport" style="margin-top:12px">开始导入</button>
  <a class="muted" style="display:inline-block;margin-top:10px" download="收益报表模板.csv" href="data:text/csv;charset=utf-8,%EF%BB%BF%E6%97%A5%E6%9C%9F,%E8%A7%86%E9%A2%91%E5%8F%B7ID,%E5%89%A7%E7%9B%AE,%E6%94%B6%E7%9B%8A,%E8%A7%86%E9%A2%91%E6%92%AD%E6%94%BE,%E5%89%A7%E9%9B%86%E6%92%AD%E6%94%BE%0A2026-11-04,demo001,%E5%A2%99%E5%86%85%E5%AE%88%E5%85%B0%E8%AE%B0,12.5,1000,2000">下载模板</a>
  ${S.result ? `<pre class="err" style="color:${S.result.ok ? 'var(--good)' : 'var(--primary)'}">${esc(S.result.text)}</pre>` : ''}</div>`;

const vUsers = () => `<div class="card" style="padding:0">${S.users.map(u => `<div class="rk"><div><b>${esc(u.nickname)} <span class="muted">${esc(u.account)}</span></b><span class="muted">${u.role === 'admin' ? '管理员' : u.accounts + ' 个视频号'} · ${esc(u.created_at.slice(0, 10))}</span></div>${u.role === 'admin' ? '' : `<button class="btn sm" data-act="resetPwd" data-v="${u.id}">重置密码</button>`}</div>`).join('')}</div>`;

function formDrama(d) {
  const t = [['', '请选择'], ...S.theaters.map(x => [x.id, x.name])];
  layer(`<div class="sheet"><button class="x" data-act="close">×</button><h2>${d.id ? '编辑剧目' : '新增剧目'}</h2><form id="df" style="margin-top:12px">
    ${field('封面图（jpg/png/webp ≤5MB）', `<div style="display:flex;gap:10px;align-items:center"><div class="thumb" id="pv" style="${cover(d)}"></div><input type="file" id="cf" accept="image/*"></div>`)}
    ${field('剧名', `<input class="in" name="title" value="${esc(d.title)}">`)}
    <div class="grid2">${field('剧场', `<select class="in" name="theater_id">${opt(t, d.theater_id)}</select>`)}${field('状态', `<select class="in" name="status">${opt(['已上架', '即将下架', '已下架'].map(x => [x, x]), d.status)}</select>`)}</div>
    <div class="grid2">${field('类型', `<input class="in" name="type" value="${esc(d.type)}" placeholder="AI真人剧 / AI漫剧">`)}${field('画幅', `<input class="in" name="ratio" value="${esc(d.ratio)}" placeholder="竖屏 / 横屏">`)}</div>
    <div class="grid2">${field('总集数', `<input class="in" type="number" min="0" name="episodes" value="${d.episodes}">`)}${field('上架日期', `<input class="in" type="date" name="listed_at" value="${esc(d.listed_at)}">`)}</div>
    <div class="grid2">${field('别名', `<input class="in" name="alias" value="${esc(d.alias)}">`)}${field('原剧名', `<input class="in" name="original_name" value="${esc(d.original_name)}">`)}</div>
    ${field('推荐热度（1~5）', `<input class="in" type="number" min="1" max="5" name="heat" value="${d.heat}">`)}
    ${field('推荐理由', `<input class="in" name="reason" value="${esc(d.reason)}">`)}
    ${field('剧目简介', `<textarea class="in" name="intro">${esc(d.intro)}</textarea>`)}
    ${field('推广素材（网盘分享文本 / 链接）', `<textarea class="in" name="material">${esc(d.material)}</textarea>`)}
    <input type="hidden" name="cover" value="${esc(d.cover)}"><input type="hidden" name="hue" value="${d.hue}">
    <div class="err" id="de"></div><div class="btns"><button type="button" class="btn sm" data-act="close">取消</button><button class="btn primary sm">保存</button></div></form></div>`);
  $('#cf').onchange = async e => {
    const fd = new FormData(); fd.append('file', e.target.files[0]);
    try { const r = await api('POST', '/admin/upload', fd); document.querySelector('[name=cover]').value = r.url; $('#pv').style.cssText = `background-image:url(${r.url})`; toast('封面已上传'); } catch (x) { toast(x.message); }
  };
  $('#df').onsubmit = async e => {
    e.preventDefault();
    const f = Object.fromEntries(new FormData(e.target));
    const body = { ...f, theater_id: +f.theater_id, episodes: +f.episodes, heat: +f.heat, hue: +f.hue };
    try { await api(d.id ? 'PUT' : 'POST', '/admin/dramas' + (d.id ? '/' + d.id : ''), body); layer(''); toast('已保存'); load(); } catch (x) { $('#de').textContent = x.message; }
  };
}

function batchModal() {
  layer(`<div class="sheet"><button class="x" data-act="close">×</button><h2>批量导入剧目</h2>
    <p class="muted" style="margin:6px 0 12px">1. 下载模板并按格式填写（可用 Excel / WPS 打开，保存为 CSV）<br>2. 上传填好的文件。<b>剧名 + 剧场</b>相同的剧目会更新，不同的会新增；封面请导入后在“编辑”里单独上传。</p>
    <a class="btn block" href="/drama-template.csv" download="剧目导入模板.csv">⇩ 下载标准模板</a>
    <div class="card" style="margin:12px 0;background:var(--surface2);font-size:12px;line-height:1.7"><b>填写说明</b><br>· 带 * 的列必填：剧名、剧场（剧场必须已在“剧场”里存在）<br>· 状态：已上架 / 即将下架 / 已下架（留空=已上架）<br>· 推荐热度 1~5（留空=5）；画幅留空=竖屏<br>· 上架日期如 2026-11-05（留空=今天）<br>· 示例行请删除后再上传；有任何错误行则整批不导入</div>
    <input type="file" id="dcsv" accept=".csv,text/csv" class="in" style="padding-top:10px">
    <button class="btn primary block" data-act="doBatch" style="margin-top:12px">开始导入</button><pre class="err" id="dres" style="margin-top:10px"></pre></div>`);
}
const blank = () => ({ title: '', theater_id: '', status: '已上架', type: '', ratio: '竖屏', episodes: 0, listed_at: new Date().toISOString().slice(0, 10), alias: '', original_name: '', heat: 5, reason: '', intro: '', material: '', cover: '', hue: Math.floor(Math.random() * 360) });

document.addEventListener('click', async e => {
  const el = e.target.closest('[data-act]'); if (!el) return;
  const act = el.dataset.act, v = el.dataset.v;
  if (act === 'mask') return; // 点击空白处不关闭弹窗，避免误触丢失已填内容，只能点 × 或保存
  try {
    if (act === 'close') layer('');
    else if (act === 'tab') { S.tab = v; S.result = null; render(); load(); }
    else if (act === 'logout') { await api('POST', '/auth/logout'); S.user = null; login(); }
    else if (act === 'editDrama') formDrama(v ? await api('GET', '/dramas/' + v) : blank());
    else if (act === 'batchDrama') batchModal();
    else if (act === 'doBatch') {
      const f = $('#dcsv').files[0]; if (!f) return toast('请选择 CSV 文件');
      const fd = new FormData(); fd.append('file', f); const res = $('#dres');
      try { const r = await api('POST', '/admin/import-dramas', fd); res.style.color = 'var(--good)'; res.textContent = `导入成功：新增 ${r.added} 部，更新 ${r.updated} 部`; S.dramas = await api('GET', '/dramas'); render(); }
      catch (x) { res.style.color = 'var(--primary)'; res.textContent = x.message + '\n' + (x.errors || []).join('\n'); }
    }
    else if (act === 'delDrama') { if (confirm('确定删除该剧目？\n若已有收益报表，这些报表也会一并删除，且无法恢复。')) { await api('DELETE', '/admin/dramas/' + v + '?force=1'); load(); } }
    else if (act === 'addTheater') { await api('POST', '/admin/theaters', { name: $('#tn').value }); load(); }
    else if (act === 'saveTheater') { const r = document.querySelector(`[data-tid="${v}"]`); await api('PUT', '/admin/theaters/' + v, { name: r.querySelector('[data-f=name]').value }); toast('已保存'); load(); }
    else if (act === 'delTheater') { if (confirm('确定删除该剧场？\n该剧场下的所有剧目、报白申请和收益报表也会一并删除，且无法恢复。')) { await api('DELETE', '/admin/theaters/' + v + '?force=1'); load(); } }
    else if (act === 'review') { await api('PATCH', '/admin/applications/' + v, { status: el.dataset.s }); toast('已' + el.dataset.s.slice(1)); load(); }
    else if (act === 'resetPwd') { if (confirm('重置后原密码立即失效，确定？')) { const r = await api('POST', `/admin/users/${v}/reset-password`); layer(`<div class="sheet"><h2>新密码</h2><p class="muted" style="margin:8px 0">请转告该达人，关闭后无法再次查看：</p><div class="card" style="font-size:22px;text-align:center;font-weight:700">${esc(r.password)}</div><div class="btns"><button class="btn primary sm" data-act="close">知道了</button></div></div>`); } }
    else if (act === 'doImport') {
      const f = $('#csv').files[0]; if (!f) return toast('请选择 CSV 文件');
      const fd = new FormData(); fd.append('file', f);
      try { const r = await api('POST', '/admin/import-income', fd); S.result = { ok: true, text: `导入成功：${r.imported} 行` }; }
      catch (x) { S.result = { ok: false, text: x.message + '\n' + (x.errors || []).join('\n') }; }
      render();
    }
  } catch (x) { toast(x.message); }
});

(async () => { try { S.user = await api('GET', '/me'); if (S.user.role !== 'admin') throw 0; setTheme(S.user.theme); await load(); } catch { login(); } })();
function setTheme(t) { document.documentElement.dataset.theme = t; }
