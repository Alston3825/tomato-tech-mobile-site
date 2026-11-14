package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

type server struct {
	db        *sql.DB
	uploadDir string
}

type ctxKey int

type user struct {
	ID       int64  `json:"id"`
	Account  string `json:"account"`
	Nickname string `json:"nickname"`
	Role     string `json:"role"`
	Theme    string `json:"theme"`
}

func now() string { return time.Now().Format("2006-01-02 15:04:05") }

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}
func decode(r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
	return json.NewDecoder(r.Body).Decode(v) == nil
}

// 鉴权中间件：从 Cookie 取会话
func (s *server) auth(admin bool, h func(http.ResponseWriter, *http.Request, *user)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie("tt_session")
		if err != nil {
			fail(w, 401, "请先登录")
			return
		}
		var u user
		err = s.db.QueryRowContext(r.Context(), `SELECT u.id,u.account,u.nickname,u.role,u.theme FROM sessions s JOIN users u ON u.id=s.user_id
			WHERE s.token=? AND s.expires_at>?`, c.Value, now()).Scan(&u.ID, &u.Account, &u.Nickname, &u.Role, &u.Theme)
		if err != nil {
			fail(w, 401, "登录已过期，请重新登录")
			return
		}
		if admin && u.Role != "admin" {
			fail(w, 403, "无权限")
			return
		}
		h(w, r, &u)
	}
}

func (s *server) startSession(w http.ResponseWriter, uid int64) error {
	b := make([]byte, 24)
	rand.Read(b)
	tok := hex.EncodeToString(b)
	exp := time.Now().Add(30 * 24 * time.Hour)
	if _, err := s.db.Exec(`INSERT INTO sessions(token,user_id,expires_at) VALUES(?,?,?)`, tok, uid, exp.Format("2006-01-02 15:04:05")); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{Name: "tt_session", Value: tok, Path: "/", Expires: exp, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	return nil
}

// ---------- 认证 ----------
func (s *server) register(w http.ResponseWriter, r *http.Request) {
	var in struct{ Nickname, Account, Password string }
	if !decode(r, &in) {
		fail(w, 400, "参数错误")
		return
	}
	in.Nickname, in.Account = strings.TrimSpace(in.Nickname), strings.TrimSpace(in.Account)
	if n := utf8.RuneCountInString(in.Nickname); n < 2 || n > 20 {
		fail(w, 400, "昵称需为 2~20 字")
		return
	}
	if utf8.RuneCountInString(in.Account) < 4 {
		fail(w, 400, "登录账号至少 4 位")
		return
	}
	if len(in.Password) < 6 {
		fail(w, 400, "密码至少 6 位")
		return
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	res, err := s.db.Exec(`INSERT INTO users(account,nickname,password_hash,created_at) VALUES(?,?,?,?)`, in.Account, in.Nickname, hash, now())
	if err != nil {
		fail(w, 409, "该账号已被注册")
		return
	}
	id, _ := res.LastInsertId()
	s.db.Exec(`INSERT INTO notifications(user_id,title,body,created_at) VALUES(?,?,?,?)`, id, "欢迎来到番茄科技", "先添加自己的视频号，再申请剧场报白。", now())
	if s.startSession(w, id) != nil {
		fail(w, 500, "服务器错误")
		return
	}
	writeJSON(w, 200, user{id, in.Account, in.Nickname, "talent", "caramel"})
}

func (s *server) login(w http.ResponseWriter, r *http.Request) {
	var in struct{ Account, Password string }
	if !decode(r, &in) {
		fail(w, 400, "参数错误")
		return
	}
	var u user
	var hash string
	err := s.db.QueryRow(`SELECT id,account,nickname,role,theme,password_hash FROM users WHERE account=?`, strings.TrimSpace(in.Account)).
		Scan(&u.ID, &u.Account, &u.Nickname, &u.Role, &u.Theme, &hash)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(in.Password)) != nil {
		fail(w, 401, "账号或密码不正确")
		return
	}
	if s.startSession(w, u.ID) != nil {
		fail(w, 500, "服务器错误")
		return
	}
	writeJSON(w, 200, u)
}

func (s *server) logout(w http.ResponseWriter, r *http.Request, u *user) {
	if c, err := r.Cookie("tt_session"); err == nil {
		s.db.Exec(`DELETE FROM sessions WHERE token=?`, c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: "tt_session", Path: "/", MaxAge: -1})
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *server) me(w http.ResponseWriter, r *http.Request, u *user) { writeJSON(w, 200, u) }

func (s *server) setTheme(w http.ResponseWriter, r *http.Request, u *user) {
	var in struct{ Theme string }
	valid := map[string]bool{"cream": true, "caramel": true, "tomato": true, "amber": true, "peach": true}
	if !decode(r, &in) || !valid[in.Theme] {
		fail(w, 400, "主题不存在")
		return
	}
	s.db.Exec(`UPDATE users SET theme=? WHERE id=?`, in.Theme, u.ID)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// ---------- 通用查询 ----------
func (s *server) query(w http.ResponseWriter, q string, args ...any) []map[string]any {
	rows, err := s.db.Query(q, args...)
	if err != nil {
		fail(w, 500, "查询失败")
		return nil
	}
	defer rows.Close()
	cols, _ := rows.Columns()
	out := []map[string]any{}
	for rows.Next() {
		vals := make([]any, len(cols))
		ptr := make([]any, len(cols))
		for i := range vals {
			ptr[i] = &vals[i]
		}
		rows.Scan(ptr...)
		m := map[string]any{}
		for i, c := range cols {
			if b, ok := vals[i].([]byte); ok {
				vals[i] = string(b)
			}
			m[c] = vals[i]
		}
		out = append(out, m)
	}
	return out
}

// ---------- 视频号 ----------
func (s *server) listAccounts(w http.ResponseWriter, r *http.Request, u *user) {
	rows := s.query(w, `SELECT id,channel_id,nickname,created_at FROM video_accounts WHERE user_id=? ORDER BY id DESC`, u.ID)
	if rows != nil {
		writeJSON(w, 200, rows)
	}
}

func (s *server) addAccountTx(ctx context.Context, uid int64, channelID, nick string) (int64, string) {
	nick, channelID = strings.TrimSpace(nick), strings.TrimSpace(channelID)
	if nick == "" {
		return 0, "请填写视频号昵称"
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO video_accounts(user_id,channel_id,nickname,created_at) VALUES(?,?,?,?)`, uid, channelID, nick, now())
	if err != nil {
		return 0, "该视频号 ID 已添加"
	}
	id, _ := res.LastInsertId()
	return id, ""
}

func (s *server) addAccount(w http.ResponseWriter, r *http.Request, u *user) {
	var in struct {
		ChannelID string `json:"channel_id"`
		Nickname  string `json:"nickname"`
	}
	if !decode(r, &in) {
		fail(w, 400, "参数错误")
		return
	}
	id, msg := s.addAccountTx(r.Context(), u.ID, in.ChannelID, in.Nickname)
	if msg != "" {
		fail(w, 400, msg)
		return
	}
	writeJSON(w, 200, map[string]int64{"id": id})
}

func (s *server) delAccount(w http.ResponseWriter, r *http.Request, u *user) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	s.db.Exec(`DELETE FROM video_accounts WHERE id=? AND user_id=?`, id, u.ID)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// ---------- 剧场 / 剧目 ----------

// 报白名额规则：每个剧场每年最多 200 个、每月最多 100 个（待审核+已通过占用，已驳回释放）
const (
	yearQuota  = 200
	monthQuota = 100
)

// 剧场（别名 t）本月/本年已占用名额及剩余名额的 SQL 列，时间前缀由服务端生成
func quotaCols() string {
	used := func(prefix string) string {
		return fmt.Sprintf(`(SELECT COUNT(*) FROM applications a WHERE a.theater_id=t.id AND a.status<>'已驳回' AND a.created_at LIKE '%s%%')`, prefix)
	}
	n := time.Now()
	m, y := used(n.Format("2006-01")), used(n.Format("2006"))
	return fmt.Sprintf(`%s AS month_used,%s AS year_used,MAX(0,MIN(%d-%s,%d-%s)) AS quota_left`, m, y, yearQuota, y, monthQuota, m)
}

func (s *server) listTheaters(w http.ResponseWriter, r *http.Request, u *user) {
	if rows := s.query(w, `SELECT t.id,t.name,`+quotaCols()+` FROM theaters t ORDER BY quota_left DESC,t.id`); rows != nil {
		writeJSON(w, 200, rows)
	}
}

const dramaCols = `d.id,d.title,d.alias,d.original_name,d.type,d.ratio,d.episodes,d.intro,d.reason,d.heat,d.status,d.listed_at,d.hue,d.cover,t.name AS theater,d.theater_id`

func (s *server) listDramas(w http.ResponseWriter, r *http.Request, u *user) {
	q := r.URL.Query()
	sqlStr, args := `SELECT `+dramaCols+` FROM dramas d JOIN theaters t ON t.id=d.theater_id WHERE 1=1`, []any{}
	if v := q.Get("theater_id"); v != "" {
		sqlStr += ` AND d.theater_id=?`
		args = append(args, v)
	}
	if v := q.Get("type"); v != "" {
		sqlStr += ` AND d.type=?`
		args = append(args, v)
	}
	if v := q.Get("status"); v != "" {
		sqlStr += ` AND d.status=?`
		args = append(args, v)
	}
	if v := strings.TrimSpace(q.Get("q")); v != "" {
		sqlStr += ` AND (d.title LIKE ? OR d.alias LIKE ?)`
		args = append(args, "%"+v+"%", "%"+v+"%")
	}
	if q.Get("sort") == "old" {
		sqlStr += ` ORDER BY d.listed_at ASC,d.id`
	} else {
		sqlStr += ` ORDER BY d.listed_at DESC,d.id`
	}
	if rows := s.query(w, sqlStr, args...); rows != nil {
		writeJSON(w, 200, rows)
	}
}

func (s *server) getDrama(w http.ResponseWriter, r *http.Request, u *user) {
	rows := s.query(w, `SELECT `+dramaCols+`,d.material FROM dramas d JOIN theaters t ON t.id=d.theater_id WHERE d.id=?`, r.PathValue("id"))
	if rows == nil {
		return
	}
	if len(rows) == 0 {
		fail(w, 404, "剧目不存在")
		return
	}
	writeJSON(w, 200, rows[0])
}

func (s *server) hot(w http.ResponseWriter, r *http.Request, u *user) {
	if rows := s.query(w, `SELECT `+dramaCols+` FROM dramas d JOIN theaters t ON t.id=d.theater_id WHERE d.status='已上架' ORDER BY d.heat DESC,d.listed_at DESC LIMIT 4`); rows != nil {
		writeJSON(w, 200, rows)
	}
}

// ---------- 报白申请 ----------
func (s *server) listApps(w http.ResponseWriter, r *http.Request, u *user) {
	sqlStr, args := `SELECT a.id,a.status,a.note,a.created_at,v.nickname AS account,v.channel_id,t.name AS theater FROM applications a
		JOIN video_accounts v ON v.id=a.video_account_id JOIN theaters t ON t.id=a.theater_id WHERE v.user_id=?`, []any{u.ID}
	if v := r.URL.Query().Get("status"); v != "" {
		sqlStr += ` AND a.status=?`
		args = append(args, v)
	}
	if v := strings.TrimSpace(r.URL.Query().Get("q")); v != "" {
		sqlStr += ` AND (v.nickname LIKE ? OR v.channel_id LIKE ? OR t.name LIKE ?)`
		args = append(args, "%"+v+"%", "%"+v+"%", "%"+v+"%")
	}
	if rows := s.query(w, sqlStr+` ORDER BY a.id DESC`, args...); rows != nil {
		writeJSON(w, 200, rows)
	}
}

func (s *server) createApp(w http.ResponseWriter, r *http.Request, u *user) {
	var in struct {
		VideoAccountID int64  `json:"video_account_id"`
		ChannelID      string `json:"channel_id"` // 无已有视频号时直接新增
		Nickname       string `json:"nickname"`
		TheaterID      int64  `json:"theater_id"`
		Note           string `json:"note"`
	}
	if !decode(r, &in) || in.TheaterID == 0 {
		fail(w, 400, "请选择剧场")
		return
	}
	var left int
	if s.db.QueryRow(`SELECT `+quotaCols()+` FROM theaters t WHERE t.id=?`, in.TheaterID).Scan(new(int), new(int), &left) != nil {
		fail(w, 400, "剧场不存在")
		return
	}
	if left <= 0 {
		fail(w, 400, "该剧场本月或本年报白名额已满")
		return
	}
	vid := in.VideoAccountID
	if vid == 0 {
		var msg string
		if vid, msg = s.addAccountTx(r.Context(), u.ID, in.ChannelID, in.Nickname); msg != "" {
			fail(w, 400, msg)
			return
		}
	} else {
		var own int
		s.db.QueryRow(`SELECT COUNT(*) FROM video_accounts WHERE id=? AND user_id=?`, vid, u.ID).Scan(&own)
		if own == 0 {
			fail(w, 400, "视频号不存在")
			return
		}
	}
	var dup, cnt int
	s.db.QueryRow(`SELECT COUNT(*) FROM applications WHERE video_account_id=? AND theater_id=?`, vid, in.TheaterID).Scan(&dup)
	if dup > 0 {
		fail(w, 400, "该视频号已申请过此剧场")
		return
	}
	s.db.QueryRow(`SELECT COUNT(*) FROM applications WHERE video_account_id=? AND status<>'已驳回'`, vid).Scan(&cnt)
	if cnt >= 2 {
		fail(w, 400, "每个视频号最多申请 2 个剧场")
		return
	}
	if _, err := s.db.Exec(`INSERT INTO applications(video_account_id,theater_id,note,created_at) VALUES(?,?,?,?)`, vid, in.TheaterID, strings.TrimSpace(in.Note), now()); err != nil {
		fail(w, 500, "提交失败")
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// 管理员：查看全部待审核 / 审核
func (s *server) adminApps(w http.ResponseWriter, r *http.Request, u *user) {
	if rows := s.query(w, `SELECT a.id,a.status,a.note,a.created_at,v.nickname AS account,v.channel_id,t.name AS theater,us.nickname AS talent FROM applications a
		JOIN video_accounts v ON v.id=a.video_account_id JOIN users us ON us.id=v.user_id JOIN theaters t ON t.id=a.theater_id ORDER BY a.status='待审核' DESC,a.id DESC`); rows != nil {
		writeJSON(w, 200, rows)
	}
}

func (s *server) review(w http.ResponseWriter, r *http.Request, u *user) {
	var in struct{ Status string }
	if !decode(r, &in) || (in.Status != "已通过" && in.Status != "已驳回") {
		fail(w, 400, "状态不合法")
		return
	}
	id := r.PathValue("id")
	// 状态和通知一起提交；条件更新避免同一申请被重复审核。
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		fail(w, 500, "审核失败")
		return
	}
	defer tx.Rollback()
	var uid int64
	var old, th string
	err = tx.QueryRowContext(r.Context(), `SELECT v.user_id,a.status,t.name FROM applications a JOIN video_accounts v ON v.id=a.video_account_id JOIN theaters t ON t.id=a.theater_id WHERE a.id=?`, id).Scan(&uid, &old, &th)
	if err == sql.ErrNoRows {
		fail(w, 404, "申请不存在")
		return
	}
	if err != nil {
		fail(w, 500, "审核失败")
		return
	}
	if old != "待审核" {
		fail(w, 400, "该申请已处理")
		return
	}
	res, err := tx.ExecContext(r.Context(), `UPDATE applications SET status=?,reviewed_at=? WHERE id=? AND status='待审核'`, in.Status, now(), id)
	if err != nil {
		fail(w, 500, "审核失败")
		return
	}
	changed, err := res.RowsAffected()
	if err != nil || changed != 1 {
		fail(w, 500, "审核失败")
		return
	}
	if _, err := tx.ExecContext(r.Context(), `INSERT INTO notifications(user_id,title,body,created_at) VALUES(?,?,?,?)`, uid, "报白申请"+in.Status, "剧场：“"+th+"”", now()); err != nil {
		fail(w, 500, "审核失败")
		return
	}
	if err := tx.Commit(); err != nil {
		fail(w, 500, "审核失败")
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// ---------- 收益（仅统计当前用户视频号匹配到的报表）----------
func incomeFilter(r *http.Request, uid int64) (string, []any) {
	q := r.URL.Query()
	where, args := ` FROM income_reports i JOIN video_accounts v ON v.channel_id=i.channel_id AND v.user_id=? JOIN theaters t ON t.id=i.theater_id JOIN dramas d ON d.id=i.drama_id WHERE 1=1`, []any{uid}
	if v := q.Get("from"); v != "" {
		where += ` AND i.report_date>=?`
		args = append(args, v)
	}
	if v := q.Get("to"); v != "" {
		where += ` AND i.report_date<=?`
		args = append(args, v)
	}
	if v := q.Get("theater_id"); v != "" {
		where += ` AND i.theater_id=?`
		args = append(args, v)
	}
	return where, args
}

func (s *server) overview(w http.ResponseWriter, r *http.Request, u *user) {
	where, args := incomeFilter(r, u.ID)
	var inc float64
	var vp, ep, rows int64
	s.db.QueryRow(`SELECT COALESCE(SUM(i.income),0),COALESCE(SUM(i.video_plays),0),COALESCE(SUM(i.episode_plays),0),COUNT(*)`+where, args...).Scan(&inc, &vp, &ep, &rows)
	var ok, pend, accs int
	s.db.QueryRow(`SELECT COUNT(*) FROM video_accounts WHERE user_id=?`, u.ID).Scan(&accs)
	s.db.QueryRow(`SELECT COALESCE(SUM(a.status='已通过'),0),COALESCE(SUM(a.status='待审核'),0) FROM applications a JOIN video_accounts v ON v.id=a.video_account_id WHERE v.user_id=?`, u.ID).Scan(&ok, &pend)
	writeJSON(w, 200, map[string]any{"income": inc, "video_plays": vp, "episode_plays": ep, "rows": rows, "approved": ok, "pending": pend, "accounts": accs})
}

func (s *server) incomeBy(group string) func(http.ResponseWriter, *http.Request, *user) {
	return func(w http.ResponseWriter, r *http.Request, u *user) {
		where, args := incomeFilter(r, u.ID)
		var sel, grp string
		switch group {
		case "video":
			sel, grp = `v.nickname AS name,v.channel_id AS sub`, `v.id`
		case "drama":
			sel, grp = `d.title AS name,t.name AS sub`, `d.id`
		default:
			sel, grp = `d.title AS name,v.nickname||' · '||i.report_date AS sub,i.income AS income,i.video_plays AS video_plays,i.episode_plays AS episode_plays`, ``
		}
		var q string
		if grp != "" {
			q = `SELECT ` + sel + `,ROUND(SUM(i.income),2) AS income,SUM(i.video_plays) AS video_plays,SUM(i.episode_plays) AS episode_plays` + where + ` GROUP BY ` + grp + ` ORDER BY income DESC LIMIT 100`
		} else {
			q = `SELECT ` + sel + where + ` ORDER BY i.report_date DESC,i.id DESC LIMIT 100`
		}
		if rows := s.query(w, q, args...); rows != nil {
			writeJSON(w, 200, rows)
		}
	}
}

// ---------- 榜单 ----------
func (s *server) rank(w http.ResponseWriter, r *http.Request, u *user) {
	sel := `i.channel_id`
	join := ``
	if r.URL.Query().Get("kind") == "talent" {
		sel = `COALESCE(us.nickname,'达人'||substr(i.channel_id,-4))`
		join = ` LEFT JOIN video_accounts v ON v.channel_id=i.channel_id LEFT JOIN users us ON us.id=v.user_id`
	}
	if rows := s.query(w, `SELECT `+sel+` AS name,ROUND(SUM(i.income),2) AS income,SUM(i.video_plays) AS video_plays,SUM(i.episode_plays) AS episode_plays
		FROM income_reports i`+join+` GROUP BY name ORDER BY income DESC LIMIT 20`); rows != nil {
		writeJSON(w, 200, rows)
	}
}

// ---------- 通知 ----------
func (s *server) notes(w http.ResponseWriter, r *http.Request, u *user) {
	if rows := s.query(w, `SELECT id,title,body,is_read,created_at FROM notifications WHERE user_id=? ORDER BY id DESC LIMIT 30`, u.ID); rows != nil {
		writeJSON(w, 200, rows)
	}
}
func (s *server) readAll(w http.ResponseWriter, r *http.Request, u *user) {
	s.db.Exec(`UPDATE notifications SET is_read=1 WHERE user_id=?`, u.ID)
	writeJSON(w, 200, map[string]bool{"ok": true})
}
