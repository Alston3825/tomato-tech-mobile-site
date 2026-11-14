// 番茄科技 · 达人工作台（前端，数据来自后端 /api）
const $ = (s, r = document) => r.querySelector(s);
const esc = s => String(s ?? '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
const bg = d => d.cover ? `background:url(${esc(d.cover)}) center/cover` : `background:linear-gradient(160deg,hsl(${d.hue} 75% 55%),hsl(${(d.hue + 340) % 360} 60% 22%))`;
const money = n => '¥ ' + Number(n || 0).toFixed(2);
const num = n => Number(n || 0).toLocaleString();

const LOGO = `<svg width="38" height="38" viewBox="0 0 48 48"><circle cx="24" cy="27" r="16" fill="#ff6347"/><circle cx="19" cy="22" r="4" fill="#ff9a80" opacity=".6"/><path d="M24 12c-2-4-6-5-9-4 3 1 4 3 4 5-3-1-6 0-7 2 3 0 5 1 7 3l5-2 5 2c2-2 4-3 7-3-1-2-4-3-7-2 0-2 1-4 4-5-3-1-7 0-9 4z" fill="#7bbf4a"/></svg>`;
const ICON = {
  overview: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7"><rect x="3" y="5" width="18" height="14" rx="3"/><path d="M3 10h18"/></svg>',
  query: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7"><path d="M4 20V4M4 20h16M9 16v-5M14 16V8M19 16v-3"/></svg>',
  accounts: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7"><rect x="7" y="3" width="10" height="18" rx="2.5"/><path d="M11 18h2"/></svg>',
  theater: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7"><path d="M4 20V9l8-5 8 5v11M9 20v-6h6v6M8 11h.01M16 11h.01"/></svg>',
  more: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7"><path d="M12 3l2.2 5.8L20 11l-5.8 2.2L12 19l-2.2-5.8L4 11l5.8-2.2z"/></svg>',
};
const TABS = [['overview', '收益概览'], ['query', '收益查询'], ['accounts', '视频账号'], ['theater', '剧场资源'], ['more', '更多']];

// ---------- 接口 ----------
async function api(method, path, body) {
  const res = await fetch('/api' + path, { method, credentials: 'same-origin', headers: body ? { 'Content-Type': 'application/json' } : {}, body: body ? JSON.stringify(body) : undefined });
  const data = await res.json().catch(() => ({}));
  if (res.status === 401 && S.user && !path.startsWith('/auth')) { S.user = null; render(); throw new Error(data.error || '请先登录'); }
  if (!res.ok) throw new Error(data.error || '请求失败');
  return data;
}
const qs = o => { const p = new URLSearchParams(); Object.entries(o).forEach(([k, v]) => v && p.set(k, v)); const s = p.toString(); return s ? '?' + s : ''; };

// ---------- 状态 ----------
const S = {
  user: null, tab: 'overview', authMode: 'login', booting: true,
  accounts: [], apps: [], theaters: [], dramas: [], hot: [], notes: [],
  ov: null, byVideo: [], inc: [], ranks: [],
  range: 'all', from: null, to: null, qTab: 'video', qTheater: '',
  th: { theater: '', q: '', type: '', status: '', sort: 'new', acc: '' },
  acckw: '', appkw: '', appSt: '',
  rank: false, rankTab: 'video', loading: false,
  cal: null, calPick: null,
};

// ---------- 工具 ----------
const fmt = d => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
function bounds() {
  const t = new Date(), day = n => { const d = new Date(t); d.setDate(d.getDate() - n); return fmt(d); };
  if (S.range === 'y') return [day(1), day(1)];
  if (S.range === '7') return [day(6), day(0)];
  if (S.range === '30') return [day(29), day(0)];
  if (S.range === 'c' && S.from) return [S.from, S.to || S.from];
  return ['', ''];
}
const rangeText = () => { const [a, b] = bounds(); return a ? `${a} 至 ${b}` : ''; };
const dateParams = () => { const [from, to] = bounds(); return { from, to }; };
function toast(msg) {
  const el = document.createElement('div'); el.className = 'toast'; el.textContent = msg;
  document.body.appendChild(el); setTimeout(() => el.remove(), 2000);
}
const layer = html => { $('#layer').innerHTML = html ? `<div class="mask" data-act="mask">${html}</div>` : ''; };
const closeLayer = () => { $('#layer').innerHTML = ''; };
const empty = (t, d) => `<div class="empty"><div class="ic">▤</div><b>${t}</b>${d}</div>`;
const sel = (name, opts, val, extra = '') => `<select class="in" data-model="${name}" ${extra}>${opts.map(o => { const [v, l] = Array.isArray(o) ? o : [o, o]; return `<option value="${esc(v)}" ${String(v) === String(val) ? 'selected' : ''}>${esc(l)}</option>`; }).join('')}</select>`;
const unread = () => S.notes.filter(n => !n.is_read).length;

// ---------- 数据加载 ----------
async function loadMe() { try { S.user = await api('GET', '/me'); setTheme(S.user.theme); } catch { S.user = null; } S.booting = false; }
function setTheme(t) { document.documentElement.dataset.theme = t; }
async function refresh() {
  if (!S.user) return render();
  const P = [], dp = dateParams();
  const need = (fn) => P.push(fn().catch(e => toast(e.message)));
  need(async () => { S.notes = await api('GET', '/notifications'); });
  if (['overview', 'query', 'accounts', 'theater'].includes(S.tab)) need(async () => { S.accounts = await api('GET', '/accounts'); });
  if (S.tab === 'overview') {
    need(async () => { S.ov = await api('GET', '/income/overview' + qs(dp)); });
    need(async () => { S.byVideo = await api('GET', '/income/by-video' + qs(dp)); });
    need(async () => { S.apps = await api('GET', '/applications'); });
  }
  if (S.tab === 'query') {
    need(async () => { S.theaters = await api('GET', '/theaters'); });
    need(async () => { S.inc = await api('GET', `/income/${{ video: 'by-video', drama: 'by-drama', detail: 'details' }[S.qTab]}` + qs({ ...dp, theater_id: S.qTheater })); });
  }
  if (S.tab === 'theater') {
    need(async () => { S.theaters = await api('GET', '/theaters'); });
    need(async () => { S.apps = await api('GET', '/applications'); });
    need(async () => { S.dramas = await api('GET', '/dramas' + qs({ theater_id: S.th.theater, q: S.th.q.trim(), type: S.th.type, status: S.th.status, sort: S.th.sort })); });
  }
  if (S.tab === 'more') {
    need(async () => { S.apps = await api('GET', '/applications' + qs({ q: S.appkw.trim(), status: S.appSt })); });
    if (S.rank) need(async () => { S.ranks = await api('GET', '/rank?kind=' + S.rankTab); });
  }
  S.loading = true; await Promise.all(P); S.loading = false; render();
}

// ---------- 登录 / 注册 ----------
function renderAuth() {
  const reg = S.authMode === 'register';
  $('#app').innerHTML = `<div class="auth">
    <div class="hero">${LOGO}<div class="brand">番茄科技<small>达人工作台</small></div><div class="glow">${LOGO.replace('38', '30').replace('38', '30')}</div></div>
    <div class="panel">
      <h2>${reg ? '注册推广账号' : '达人登录'}</h2>
      <p class="muted" style="margin-bottom:14px">${reg ? '填写昵称、账号和密码，注册成功即可使用' : '登录后管理视频号、申请报白并查看收益'}</p>
      <div class="seg" style="margin-bottom:16px"><button data-act="mode" data-v="login" class="${reg ? '' : 'on'}">登录</button><button data-act="mode" data-v="register" class="${reg ? 'on' : ''}">注册</button></div>
      <form id="authForm" autocomplete="off">
        ${reg ? `<div class="field"><label>达人昵称</label><input class="in" name="nick" placeholder="例如：小王 / 某某工作室"><p class="muted tip">2~20 字，填写管理员熟悉的称呼或微信昵称</p></div>` : ''}
        <div class="field"><label>登录账号</label><input class="in" name="acc" placeholder="${reg ? '设置登录账号（至少 4 位）' : '请输入登录账号'}"></div>
        <div class="field"><label>密码</label><input class="in" type="password" name="pwd" placeholder="${reg ? '设置密码（至少 6 位）' : '请输入密码'}">${reg ? '<p class="muted tip">至少 6 位，不强制大小写、数字或特殊符号</p>' : ''}</div>
        ${reg ? `<div class="field"><label>确认密码</label><input class="in" type="password" name="pwd2" placeholder="再次输入密码"></div>` : ''}
        <div class="err" id="authErr"></div>
        <button class="btn primary block" style="height:48px">${reg ? '注册并进入工作台 →' : '登录达人后台 →'}</button>
      </form>
      <p class="muted foot">收益使用报表原始金额，不重新计算分成</p>
    </div></div>`;
  $('#authForm').onsubmit = async e => {
    e.preventDefault();
    const f = Object.fromEntries(new FormData(e.target)), err = m => { $('#authErr').textContent = m; };
    if (reg && f.pwd !== f.pwd2) return err('两次输入的密码不一致');
    try {
      S.user = await api('POST', reg ? '/auth/register' : '/auth/login', { nickname: f.nick, account: f.acc, password: f.pwd });
      setTheme(S.user.theme); S.tab = 'overview'; render(); refresh();
    } catch (ex) { err(ex.message); }
  };
}

// ---------- 页面 ----------
function dateFilter(clearable) {
  const chip = (k, l) => `<button data-act="range" data-v="${k}" class="${S.range === k ? 'on' : ''}">${l}</button>`;
  return `<div class="chips">${chip('y', '昨日')}${chip('7', '近 7 天')}${chip('30', '近 30 天')}${chip('all', '全部日期')}</div>
    <button class="range" data-act="cal"><span>${rangeText() || '自定义日期'}</span><span>🗓</span></button>
    ${clearable ? '<div style="text-align:right;margin-top:10px"><button data-act="clear" style="font-weight:700">清空筛选</button></div>' : ''}`;
}
const emptyAcc = () => empty('先添加自己的视频号', '添加后直接认领，匹配到已导入报表即可查询收益。');
const incRow = r => `<div class="rk"><div><b>${esc(r.name)}</b><span class="muted">${esc(r.sub)} · 视频播放 ${num(r.video_plays)} · 剧集播放 ${num(r.episode_plays)}</span></div><span class="amt">${money(r.income)}</span></div>`;
const noData = '<div class="notice"><span>✦</span><span>当前筛选尚无已入库收益明细，请核对报表入库状态和视频号完整ID。</span></div>';

function pOverview() {
  const ov = S.ov || {}, has = ov.rows > 0, v = x => has ? x : '—';
  return `<h1>收益概览</h1><p class="sub">看收益、管理视频号、查看报白进度</p>
  <div class="acts"><button class="btn" data-act="tab" data-v="accounts">视频账号</button><button class="btn primary" data-act="apply">＋ 申请报白</button></div>
  <div class="card"><b>📅 日期范围</b>${dateFilter(false)}</div>
  ${has ? '' : noData}
  <div class="stats">
    <div class="stat hl"><div class="t"><span>本人原始收益（未分成前）</span></div><div class="v">${v(money(ov.income))}</div><div class="d">分成前的报表金额，不代表达人个人分成收益</div></div>
    <div class="stat"><div class="t"><span>视频播放量</span><span>▷</span></div><div class="v">${v(num(ov.video_plays))}</div><div class="d">按所选报表范围统计</div></div>
    <div class="stat"><div class="t"><span>剧集播放量</span><span>▤</span></div><div class="v">${v(num(ov.episode_plays))}</div><div class="d">按所选报表范围统计</div></div>
    <div class="stat"><div class="t"><span>报白进度</span><span>☐</span></div><div class="v" style="font-size:20px"><span style="color:var(--good)">${ov.approved || 0}</span> <span class="muted">已通过</span> <span style="color:var(--accent)">${ov.pending || 0}</span> <span class="muted">待审核</span></div><div class="d">当前授权状态，不随日期切换</div></div>
  </div>
  <div class="card" style="padding:0"><div class="row" style="align-items:center;padding:14px 14px 0"><b>我的视频号收益</b><button data-act="tab" data-v="query" style="color:var(--primary);text-align:right;font-weight:600">查看明细 →</button></div>
    <p class="muted" style="padding:0 14px 6px">${rangeText() || '全部已导入日期'} · ${S.accounts.length} 个视频号</p>
    ${S.byVideo.length ? S.byVideo.slice(0, 3).map(incRow).join('') : empty('当前范围暂无视频号收益明细', '已添加的视频号可在“我的视频号”中查看。')}</div>
  <div class="card"><div class="row"><b>报白进度</b><button data-act="tab" data-v="more" style="text-align:right;color:var(--muted)">全部记录 →</button></div>
    ${S.apps.length ? appList(S.apps.slice(0, 3)) : empty('还没有报白申请', '选择视频号和剧场，提交后在这里查看审核进度。')}<p class="muted">每个视频号在不同剧场分别审核</p></div>
  <button class="link" data-act="manage"><span class="ic">▯</span><div><b>管理视频号</b><span class="muted">添加或查看名下账号</span></div>→</button>
  <button class="link" data-act="tab" data-v="theater"><span class="ic">▦</span><div><b>挑选推广剧目</b><span class="muted">查看剧场、剧目与素材</span></div>→</button>
  <button class="link" data-act="tab" data-v="query"><span class="ic">▥</span><div><b>查询收益明细</b><span class="muted">按日期、视频号或剧目查询</span></div>→</button>
  <button class="link" data-act="rank"><span class="ic">🏆</span><div><b>漫剧风云榜</b><span class="muted">视频号 / 达人排名</span></div>→</button>
  <p class="ftr">番茄科技 · 达人工作台</p>`;
}

function pQuery() {
  const tabs = [['video', '按视频号'], ['drama', '按剧目'], ['detail', '报表明细']];
  const titles = { video: ['视频号收益汇总', '查看本人全部视频号的收益、播放量和授权状态。'], drama: ['剧目收益汇总', '按剧目查看原始收益与播放量。'], detail: ['报表明细', '按报表日期或原区间查看视频号、剧目及视频收益明细。'] };
  return `<h1>收益查询</h1><p class="sub">按视频号、剧目或日期查看原始收益。分成前的报表金额，不代表达人个人分成收益。切换页面保留筛选条件。</p>
  ${S.accounts.length ? '' : `<div class="promo"><b>▯ 先添加自己的视频号，再查询收益</b><p class="muted" style="margin:6px 0 10px">首次使用请登记视频号；收益还需对应剧场授权且报表已导入后才会显示。</p><button class="btn primary block" data-act="manage">添加视频号</button></div>`}
  <div class="seg" style="margin-bottom:14px">${tabs.map(([k, l]) => `<button data-act="qtab" data-v="${k}" class="${S.qTab === k ? 'on' : ''}">${l}</button>`).join('')}</div>
  <h1 style="font-size:20px">${titles[S.qTab][0]}</h1><p class="sub">${titles[S.qTab][1]}</p>
  <div class="card">${sel('qTheater', [['', '全部剧场'], ...S.theaters.map(t => [t.id, t.name])], S.qTheater)}${dateFilter(true)}</div>
  <div class="card" style="padding:0"><div class="row" style="align-items:center;padding:14px 14px 4px"><b>📊 我的视频号收益</b><span style="text-align:right;color:var(--primary);font-weight:600">${S.inc.length ? '共 ' + S.inc.length + ' 条' : '暂无明细'}</span></div>
    <p class="muted" style="padding:0 14px 6px">${rangeText() || '全部日期'} · 共 ${S.accounts.length} 个视频号</p>
    ${S.inc.length ? S.inc.map(incRow).join('') : noData + empty('当前筛选下暂无视频号明细', '可调整日期或筛选条件；尚未添加的视频号请到“我的视频号”登记。')}</div>
  <p class="ftr">番茄科技 · 达人工作台</p>`;
}

const accRow = a => `<div class="rk"><span class="n">▯</span><div><b>${esc(a.nickname)}</b><span class="muted">${esc(a.channel_id || '未填写 ID')}</span></div><button class="muted" data-act="delacc" data-v="${a.id}">删除</button></div>`;
function pAccounts() {
  const kw = S.acckw.toLowerCase(), list = S.accounts.filter(a => (a.nickname + a.channel_id).toLowerCase().includes(kw));
  return `<h1>视频账号</h1><p class="sub">添加或查看视频号与报白状态</p>
  <button class="btn primary block" data-act="manage" style="margin-bottom:14px">＋ 添加视频号</button>
  <div class="card"><div style="display:flex;justify-content:space-between"><b>名下视频号</b><span class="muted">共 ${S.accounts.length} 个视频号</span></div>
    <input class="in" style="margin:12px 0" data-model="acckw" value="${esc(S.acckw)}" placeholder="🔍 搜索昵称或视频号 ID">
    ${list.length ? list.map(accRow).join('') : emptyAcc()}</div>
  <p class="muted">已有报表自动匹配收益；新剧场需申请报白。 <button data-act="tab" data-v="more" style="color:var(--primary);font-weight:700">查看全部申请记录</button></p>
  <p class="ftr">番茄科技 · 达人工作台</p>`;
}

function dramaCard(d, newest) {
  const date = d.listed_at.replaceAll('-', '/');
  return `<div class="drama">${d.status === '已上架' && d.listed_at === newest ? '<span class="badge">新剧 ✨</span>' : ''}
    <div class="poster" style="${bg(d)}">${d.cover ? '' : esc(d.title)}</div>
    <div class="stars">${'🔥'.repeat(d.heat)} ${d.heat} / 5</div>
    <div style="display:flex;justify-content:space-between;align-items:center"><span class="tag">${esc(d.theater)}</span><span class="tag ${d.status === '已上架' ? 'good' : ''}" style="${d.status === '已上架' ? '' : 'background:var(--surface);color:var(--muted)'}">${d.status}</span></div>
    <h3>${esc(d.title)}</h3><p class="muted">原剧名：${esc(d.original_name || '待补充')}</p>
    <div class="meta"><span>类型<b>${esc(d.type)}</b></span><span>画幅<b>${esc(d.ratio)}</b></span><span>总集数<b>${d.episodes} 集</b></span><span>上架日期<b>${date}</b></span></div>
    <div class="why"><span class="muted">推荐理由</span><br>${esc(d.reason)}</div>
    <button class="btn primary sm" data-act="detail" data-v="${d.id}">详情与素材</button></div>`;
}
function pTheater() {
  const f = S.th, newest = S.dramas.reduce((m, d) => d.listed_at > m ? d.listed_at : m, '');
  const acc = S.accounts.find(a => String(a.id) === String(f.acc)), th = S.theaters.find(t => String(t.id) === String(f.theater));
  let state = '选择视频号后可查看报白状态';
  if (acc) { const a = S.apps.find(x => x.channel_id === acc.channel_id && x.account === acc.nickname && (!th || x.theater === th.name)); state = th ? `${acc.nickname} · ${th.name}：${a ? a.status : '未申请'}` : `${acc.nickname} 已申请 ${S.apps.filter(x => x.account === acc.nickname).length} 个剧场`; }
  return `<h1>剧场资源</h1><p class="sub">选择剧场，查看剧目资料与推广素材。</p>
  <div class="card"><b style="display:block;margin-bottom:8px">选择剧场</b>${sel('th.theater', [['', '全部剧场'], ...S.theaters.map(t => [t.id, `${t.name} · 剩余 ${t.quota_left}`])], f.theater)}
    <div class="row" style="align-items:center;margin:12px 0 4px"><b style="flex:none;font-size:13px">报白视频号</b>${S.accounts.length ? sel('th.acc', [['', '请选择'], ...S.accounts.map(a => [a.id, a.nickname])], f.acc) : sel('x', ['尚未添加视频号'], '', 'disabled')}</div>
    <p style="font-size:13px;font-weight:600;padding:8px 0 12px;border-bottom:1px solid var(--line);margin-bottom:12px">${esc(state)}</p>
    <input class="in" data-model="th.q" value="${esc(f.q)}" placeholder="🔍 搜索剧名或别名" style="margin-bottom:10px">
    <div class="row" style="margin-bottom:10px">${sel('th.type', [['', '全部类型'], 'AI真人剧', 'AI漫剧', '待补充'], f.type)}${sel('th.status', [['', '全部上架剧目'], '已上架', '即将下架', '已下架'], f.status)}</div>
    <div class="row">${sel('th.sort', [['new', '上架时间从新到旧'], ['old', '上架时间从旧到新']], f.sort)}<span></span></div></div>
  <div class="card"><div style="margin-bottom:12px"><b style="font-size:20px">全部剧目</b> <span class="muted">${S.dramas.length} 部剧目</span></div>
    ${S.dramas.length ? `<div class="grid">${S.dramas.map(d => dramaCard(d, newest)).join('')}</div>` : empty('没有符合条件的剧目', '换个筛选条件试试。')}</div>`;
}

const appList = a => a.map(x => `<div class="rk"><span class="n">▤</span><div><b>${esc(x.account)} · ${esc(x.theater)}</b><span class="muted">${esc(x.created_at.slice(0, 10))}${x.note ? ' · ' + esc(x.note) : ''}</span></div><span class="tag ${x.status === '已通过' ? 'good' : ''}">${x.status}</span></div>`).join('');
function pMore() {
  if (S.rank) return pRank();
  const pend = S.apps.filter(a => a.status === '待审核').length;
  return `<h1>报白申请</h1><p class="sub">添加视频号、申请剧场报白、查看审核进度，都在这里。</p>
  <div class="acts"><button class="btn" data-act="manage">▯ 管理视频号</button><button class="btn primary" data-act="apply">＋ 新增申请</button></div>
  <div class="card"><b>申请记录</b><p class="muted" style="margin:4px 0 12px">一个视频号、一个剧场对应一条记录，报表自动确认的授权也会显示。</p>
    <p class="muted" style="margin-bottom:12px">共 ${S.apps.length} 条 · 待审核 ${pend} 条</p>
    <input class="in" data-model="appkw" value="${esc(S.appkw)}" placeholder="🔍 搜索视频号名称、ID 或剧场" style="margin-bottom:10px">
    ${sel('appSt', [['', '全部状态'], '待审核', '已通过', '已驳回'], S.appSt)}
    ${S.apps.length ? `<div style="margin-top:10px">${appList(S.apps)}</div>` : empty('还没有报白申请', '点击右上角“新增申请”，选择或添加视频号后提交。')}</div>
  <button class="link" data-act="rank"><span class="ic">🏆</span><div><b>漫剧风云榜</b><span class="muted">视频号 / 达人排名</span></div>→</button>
  ${S.user.role === 'admin' ? '<a class="link" href="/admin.html" style="color:inherit;text-decoration:none"><span class="ic">⚙</span><div><b>管理后台</b><span class="muted">剧目、剧场、审核与报表导入</span></div>→</a>' : ''}
  <p class="ftr">番茄科技 · 达人工作台</p>`;
}
function pRank() {
  return `<button class="muted" data-act="rankBack" style="margin:4px 0 10px">‹ 返回</button>
  <div class="card"><div style="display:flex;align-items:center;gap:10px;margin-bottom:12px"><span style="width:34px;height:34px;border-radius:10px;background:var(--accent);display:grid;place-items:center">🏆</span><b style="font-size:22px">漫剧风云榜</b></div>
    <div class="seg"><button data-act="rtab" data-v="video" class="${S.rankTab === 'video' ? 'on' : ''}">视频号排名</button><button data-act="rtab" data-v="talent" class="${S.rankTab === 'talent' ? 'on' : ''}">达人排名</button></div>
    <button class="btn block" data-act="hot" style="margin-top:8px;background:none;border:0;color:var(--muted)">今日爆款剧推荐</button></div>
  ${S.loading ? '<div class="spin"><i></i>正在读取数据…</div>' : `<div class="card" style="padding:0"><b style="display:block;padding:14px 14px 4px">前 20 名</b>${S.ranks.length ? S.ranks.map((r, i) => `<div class="rk"><span class="n ${i < 3 ? 'r' + (i + 1) : ''}">${i + 1}</span><div><b>${esc(r.name)}</b><span class="muted">视频播放 ${num(r.video_plays)} · 剧集播放 ${num(r.episode_plays)}</span></div><span class="amt">${money(r.income)}</span></div>`).join('') : empty('暂无榜单数据', '报表导入后会显示排名。')}</div>`}`;
}
const PAGES = { overview: pOverview, query: pQuery, accounts: pAccounts, theater: pTheater, more: pMore };

// ---------- 主框架 ----------
function render() {
  if (S.booting) return;
  if (!S.user) return renderAuth();
  const sc = $('.main'), top = sc ? sc.scrollTop : 0, ae = document.activeElement, am = ae && ae.dataset && ae.dataset.model, pos = ae && ae.selectionStart;
  const t = THEMES.find(x => x.id === document.documentElement.dataset.theme) || THEMES[1], u = unread();
  $('#app').innerHTML = `<div class="shell">
    <div class="top"><button class="chip sq" data-act="bell" style="position:relative">🔔${u ? `<i class="dot">${u}</i>` : ''}</button><button class="chip" data-act="theme">◐ 主题 · ${t.name} ⌄</button><button class="chip sq" data-act="logout">⎋</button></div>
    <div class="main">${PAGES[S.tab]()}</div>
    <div class="nav">${TABS.map(([k, l]) => `<button data-act="tab" data-v="${k}" class="${S.tab === k ? 'on' : ''}">${ICON[k]}${l}</button>`).join('')}</div></div>`;
  if (sc) $('.main').scrollTop = top;
  if (am) { const n = document.querySelector(`[data-model="${am}"]`); if (n && n.tagName === 'INPUT') { n.focus(); n.setSelectionRange(pos, pos); } }
}
function go(tab) { S.tab = tab; if (tab !== 'more') S.rank = false; render(); refresh(); const m = $('.main'); if (m) m.scrollTop = 0; }

// ---------- 弹层 ----------
function modalManage() {
  layer(`<div class="sheet"><button class="x" data-act="close">×</button><h2>管理视频号</h2>
    <p class="muted" style="margin:4px 0 14px">添加本人视频号即可认领，已有报表自动匹配收益；新的报白申请受名额限制。</p>
    <div class="field"><label>视频号 ID</label><input class="in" id="mId" placeholder="请输入完整视频号 ID"></div>
    <div class="field"><label>视频号昵称（必填）</label><input class="in" id="mNick" placeholder="请输入视频号昵称"></div>
    <button class="btn block" data-act="addacc">＋ 添加视频号</button>
    <hr style="border:0;border-top:1px solid var(--line);margin:16px 0">
    ${S.accounts.length ? S.accounts.map(accRow).join('') : empty('暂未添加视频号', '你也可以在新增报白申请时直接添加。')}</div>`);
}
async function modalApply() {
  if (!S.theaters.length) S.theaters = await api('GET', '/theaters').catch(() => []);
  if (!S.accounts.length) S.accounts = await api('GET', '/accounts').catch(() => []);
  const opts = [['', '请选择剧场'], ...S.theaters.map(t => [t.id, `${t.name} · 剩余 ${t.quota_left}`])];
  layer(`<div class="sheet"><button class="x" data-act="close">×</button><h2>新增报白申请</h2><p class="muted" style="margin:4px 0 14px">选择视频号和剧场后提交。每个视频号最多通过 2 个剧场</p>
    <div class="field"><label>申请视频号</label><select class="in" id="aAcc"><option value="">＋ 添加新视频号并申请</option>${S.accounts.map(a => `<option value="${a.id}">${esc(a.nickname)}</option>`).join('')}</select></div>
    <div id="aNew" class="card" style="background:var(--surface2)"><div class="field"><label>视频号 ID</label><input class="in" id="aId" placeholder="请输入完整视频号 ID"></div><div class="field" style="margin:0"><label>视频号昵称（必填）</label><input class="in" id="aNick" placeholder="请输入视频号昵称"></div></div>
    <div class="field"><label>申请剧场</label>${sel('', opts, '').replace('data-model=""', 'id="aTh"')}</div>
    <div class="field"><label>补充说明（选填）</label><textarea class="in" id="aNote" placeholder="填写需要向管理员说明的情况"></textarea></div>
    <div class="btns"><button class="btn sm" data-act="close">取消</button><button class="btn primary sm" data-act="submitapp">提交申请</button></div></div>`);
  $('#aAcc').onchange = e => $('#aNew').classList.toggle('hide', !!e.target.value);
}
async function modalDetail(id) {
  const d = await api('GET', '/dramas/' + id);
  layer(`<div class="sheet"><button class="x" data-act="close">×</button><h2>剧目详情</h2><p class="muted" style="margin:4px 0 14px">剧目资料与推广素材</p>
    <div class="card" style="background:var(--surface2)"><div style="display:flex;gap:12px"><div class="poster" style="width:72px;flex:none;${bg(d)}"></div>
      <div><b style="font-size:16px">${esc(d.title)}</b> <span class="tag good">${d.status}</span><p class="muted">别名：${esc(d.alias || '待补充')}</p><p class="muted">${esc(d.theater)}</p></div></div>
      <div class="row" style="margin-top:12px"><span class="muted">类型<b style="display:block;color:var(--text)">${esc(d.type)}</b></span><span class="muted">画幅<b style="display:block;color:var(--text)">${esc(d.ratio)}</b></span><span class="muted">集数<b style="display:block;color:var(--text)">${d.episodes} 集</b></span></div>
      <p class="muted" style="margin-top:10px">上架日期<b style="display:block;color:var(--text)">${d.listed_at.replaceAll('-', '/')}</b></p></div>
    <div class="card" style="background:var(--surface2)"><b>剧目简介</b><p class="muted" style="margin-top:6px">${esc(d.intro)}</p><p style="margin-top:6px;font-size:12px"><b style="color:var(--primary)">推荐理由</b> ${esc(d.reason)}</p></div>
    <div class="heat">推荐热度 ${'🔥'.repeat(d.heat)} ${d.heat} / 5 <span class="muted">平台推荐 · 重点关注</span></div>
    <div class="card" style="background:var(--surface2)"><b>推广素材</b><div class="card" style="margin:10px 0 0;font-size:12px">${esc(d.material || '素材核验中…')}</div></div></div>`);
}
function modalCal() {
  const c = S.cal || (S.cal = { y: new Date().getFullYear(), m: new Date().getMonth() });
  const start = (new Date(c.y, c.m, 1).getDay() + 6) % 7, cells = [];
  for (let i = 0; i < 42; i++) cells.push(new Date(c.y, c.m, 1 - start + i));
  const a = S.calPick?.a, b = S.calPick?.b || a;
  layer(`<div class="sheet cal"><button class="x" data-act="close">×</button><h2 style="font-size:17px">选择查询日期</h2><p class="muted">单击选择起止日期，支持跨月选择</p>
    <div class="hd"><button class="btn sm" data-act="cm" data-v="-1">‹</button><b>${c.m + 1} 月 ${c.y}</b><button class="btn sm" data-act="cm" data-v="1">›</button></div>
    <div class="g">${'一二三四五六日'.split('').map(x => `<span>${x}</span>`).join('')}${cells.map(d => { const s = fmt(d), cls = [d.getMonth() !== c.m ? 'o' : '', s === a || s === b ? 'ed' : (a && b && s > a && s < b ? 'in-r' : '')].join(' '); return `<button class="${cls}" data-act="pick" data-v="${s}">${d.getDate()}</button>`; }).join('')}</div>
    <div class="btns"><button class="btn sm" data-act="close">取消</button><button class="btn primary sm" data-act="calok">确定</button></div></div>`);
}
function popTheme() {
  const cur = document.documentElement.dataset.theme;
  layer(`<div class="pop" data-act="keep"><p style="text-align:left;font-weight:700;color:var(--text);margin:4px 8px 6px">外观主题</p>${THEMES.map(t => `<button data-act="settheme" data-v="${t.id}" class="${t.id === cur ? 'on' : ''}"><i style="background:${t.color}"></i>${t.name}</button>`).join('')}<p>仅改变外观，自动记住选择</p></div>`);
  $('#layer .mask').style.cssText = 'background:transparent;display:block;padding:0';
}
async function showHot() {
  const list = await api('GET', '/hot');
  layer(`<div class="sheet"><button class="x" data-act="close">×</button><h2>今日爆款剧推荐</h2><p class="muted" style="margin:4px 0 12px">平台今日重点推荐</p>${list.map((d, i) => `<div class="rk"><span class="n r${Math.min(i + 1, 3)}">${i + 1}</span><div><b>${esc(d.title)}</b><span class="muted">${esc(d.theater)} · ${d.episodes} 集</span></div><span class="tag">🔥 ${d.heat}/5</span></div>`).join('') || empty('暂无推荐', '')}</div>`);
}
function showNotes() {
  layer(`<div class="sheet"><button class="x" data-act="close">×</button><h2>消息通知</h2><p class="muted" style="margin:4px 0 12px">审核结果与系统消息</p>${S.notes.map(n => `<div class="rk"><div><b>${esc(n.title)}</b><span class="muted">${esc(n.body)} · ${esc(n.created_at.slice(0, 16))}</span></div></div>`).join('') || empty('暂无通知', '')}</div>`);
  if (unread()) api('POST', '/notifications/read-all').then(() => { S.notes.forEach(n => n.is_read = 1); render(); }).catch(() => {});
}

// ---------- 事件 ----------
const guard = fn => async (...a) => { try { await fn(...a); } catch (e) { toast(e.message); } };
document.addEventListener('click', guard(async e => {
  const el = e.target.closest('[data-act]'); if (!el) return;
  const act = el.dataset.act, v = el.dataset.v;
  if (act === 'mask') { if (e.target === el) closeLayer(); return; }
  if (act === 'keep') return;
  const A = {
    mode() { S.authMode = v; renderAuth(); },
    tab() { closeLayer(); go(v); },
    close: closeLayer, manage: modalManage, apply: modalApply,
    detail: () => modalDetail(v), theme: popTheme, bell: showNotes,
    async settheme() { setTheme(v); closeLayer(); render(); await api('PUT', '/me/theme', { theme: v }); },
    async logout() { await api('POST', '/auth/logout').catch(() => {}); S.user = null; closeLayer(); render(); },
    range() { S.range = v; render(); refresh(); },
    clear() { S.range = 'all'; S.from = S.to = null; S.qTheater = ''; render(); refresh(); },
    cal() { S.calPick = null; modalCal(); },
    cm() { S.cal.m += +v; if (S.cal.m < 0) { S.cal.m = 11; S.cal.y--; } if (S.cal.m > 11) { S.cal.m = 0; S.cal.y++; } modalCal(); },
    pick() { const p = S.calPick; if (!p || p.b) S.calPick = { a: v }; else if (v < p.a) S.calPick = { a: v, b: p.a }; else S.calPick = { a: p.a, b: v }; modalCal(); },
    calok() { const p = S.calPick; if (p) { S.range = 'c'; S.from = p.a; S.to = p.b || p.a; } closeLayer(); render(); refresh(); },
    qtab() { S.qTab = v; S.inc = []; render(); refresh(); },
    async addacc() {
      await api('POST', '/accounts', { channel_id: $('#mId').value, nickname: $('#mNick').value });
      S.accounts = await api('GET', '/accounts'); modalManage(); render(); toast('已添加视频号');
    },
    async delacc() { await api('DELETE', '/accounts/' + v); S.accounts = await api('GET', '/accounts'); if ($('.sheet')) modalManage(); refresh(); },
    async submitapp() {
      const th = $('#aTh').value, acc = $('#aAcc').value;
      if (!th) return toast('请选择剧场');
      await api('POST', '/applications', { video_account_id: +acc || 0, channel_id: $('#aId').value, nickname: $('#aNick').value, theater_id: +th, note: $('#aNote').value });
      S.theaters = []; closeLayer(); toast('申请已提交'); refresh();
    },
    rank() { S.tab = 'more'; S.rank = true; S.ranks = []; render(); refresh(); },
    hot: showHot,
    rankBack() { S.rank = false; render(); refresh(); },
    rtab() { S.rankTab = v; S.ranks = []; render(); refresh(); },
  };
  A[act] && await A[act]();
}));
// 表单控件双向绑定：搜索框防抖请求，其余立即请求
let timer;
document.addEventListener('input', e => {
  const k = e.target.dataset.model; if (!k) return;
  const val = e.target.value;
  if (k.startsWith('th.')) S.th[k.slice(3)] = val; else if (k in S) S[k] = val; else return;
  render();
  if (k === 'acckw') return;
  clearTimeout(timer); timer = setTimeout(refresh, e.target.tagName === 'INPUT' ? 300 : 0);
});

(async () => { await loadMe(); render(); refresh(); })();
