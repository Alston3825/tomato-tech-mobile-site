package main

import (
	"bytes"
	"crypto/rand"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/text/encoding/simplifiedchinese"
)

var statusSet = map[string]bool{"已上架": true, "即将下架": true, "已下架": true}

type dramaIn struct {
	TheaterID    int64  `json:"theater_id"`
	Title        string `json:"title"`
	Alias        string `json:"alias"`
	OriginalName string `json:"original_name"`
	Type         string `json:"type"`
	Ratio        string `json:"ratio"`
	Episodes     int    `json:"episodes"`
	Intro        string `json:"intro"`
	Reason       string `json:"reason"`
	Heat         int    `json:"heat"`
	Status       string `json:"status"`
	ListedAt     string `json:"listed_at"`
	Hue          int    `json:"hue"`
	Material     string `json:"material"`
	Cover        string `json:"cover"`
}

func (s *server) checkDrama(in *dramaIn) string {
	in.Title = strings.TrimSpace(in.Title)
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM theaters WHERE id=?`, in.TheaterID).Scan(&n)
	switch {
	case in.Title == "":
		return "请填写剧名"
	case n == 0:
		return "请选择剧场"
	case !statusSet[in.Status]:
		return "上架状态不合法"
	case in.Heat < 1 || in.Heat > 5:
		return "推荐热度需为 1~5"
	case in.Episodes < 0:
		return "集数不合法"
	}
	if _, err := time.Parse("2006-01-02", in.ListedAt); err != nil {
		return "上架日期格式应为 YYYY-MM-DD"
	}
	if in.Cover != "" && !strings.HasPrefix(in.Cover, "/uploads/") {
		return "封面地址不合法"
	}
	return ""
}

func (s *server) saveDrama(w http.ResponseWriter, r *http.Request, u *user) {
	var in dramaIn
	if !decode(r, &in) {
		fail(w, 400, "参数错误")
		return
	}
	if msg := s.checkDrama(&in); msg != "" {
		fail(w, 400, msg)
		return
	}
	args := []any{in.TheaterID, in.Title, in.Alias, in.OriginalName, in.Type, in.Ratio, in.Episodes, in.Intro, in.Reason, in.Heat, in.Status, in.ListedAt, in.Hue, in.Material, in.Cover}
	if id := r.PathValue("id"); id != "" {
		res, err := s.db.Exec(`UPDATE dramas SET theater_id=?,title=?,alias=?,original_name=?,type=?,ratio=?,episodes=?,intro=?,reason=?,heat=?,status=?,listed_at=?,hue=?,material=?,cover=? WHERE id=?`, append(args, id)...)
		if err != nil {
			fail(w, 500, "保存失败")
			return
		}
		if n, _ := res.RowsAffected(); n == 0 {
			fail(w, 404, "剧目不存在")
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
		return
	}
	res, err := s.db.Exec(`INSERT INTO dramas(theater_id,title,alias,original_name,type,ratio,episodes,intro,reason,heat,status,listed_at,hue,material,cover) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, args...)
	if err != nil {
		fail(w, 500, "保存失败")
		return
	}
	id, _ := res.LastInsertId()
	writeJSON(w, 200, map[string]int64{"id": id})
}

func (s *server) delDrama(w http.ResponseWriter, r *http.Request, u *user) {
	id := r.PathValue("id")
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM income_reports WHERE drama_id=?`, id).Scan(&n)
	if n > 0 && r.URL.Query().Get("force") != "1" {
		fail(w, 400, "该剧目已有收益报表，不能直接删除")
		return
	}
	tx, _ := s.db.Begin()
	defer tx.Rollback()
	tx.Exec(`DELETE FROM income_reports WHERE drama_id=?`, id) // 强制删除时连带报表
	tx.Exec(`DELETE FROM dramas WHERE id=?`, id)
	tx.Commit()
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// ---------- 剧场 ----------
func (s *server) saveTheater(w http.ResponseWriter, r *http.Request, u *user) {
	var in struct {
		Name string `json:"name"`
	}
	if !decode(r, &in) || strings.TrimSpace(in.Name) == "" {
		fail(w, 400, "请填写剧场名称")
		return
	}
	name := strings.TrimSpace(in.Name)
	var err error
	if id := r.PathValue("id"); id != "" {
		_, err = s.db.Exec(`UPDATE theaters SET name=? WHERE id=?`, name, id)
	} else {
		_, err = s.db.Exec(`INSERT INTO theaters(name) VALUES(?)`, name)
	}
	if err != nil {
		fail(w, 400, "剧场名称已存在")
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *server) delTheater(w http.ResponseWriter, r *http.Request, u *user) {
	id := r.PathValue("id")
	var n int
	s.db.QueryRow(`SELECT (SELECT COUNT(*) FROM dramas WHERE theater_id=?)+(SELECT COUNT(*) FROM applications WHERE theater_id=?)+(SELECT COUNT(*) FROM income_reports WHERE theater_id=?)`, id, id, id).Scan(&n)
	if n > 0 && r.URL.Query().Get("force") != "1" {
		fail(w, 400, "该剧场下已有剧目、申请或报表，不能直接删除")
		return
	}
	tx, _ := s.db.Begin()
	defer tx.Rollback()
	for _, q := range []string{`DELETE FROM income_reports WHERE theater_id=?`, `DELETE FROM applications WHERE theater_id=?`, `DELETE FROM dramas WHERE theater_id=?`, `DELETE FROM theaters WHERE id=?`} {
		tx.Exec(q, id) // 强制删除时连带剧目、申请与报表
	}
	tx.Commit()
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// ---------- 封面上传 ----------
func (s *server) upload(w http.ResponseWriter, r *http.Request, u *user) {
	r.Body = http.MaxBytesReader(w, r.Body, 6<<20)
	f, hdr, err := r.FormFile("file")
	if err != nil {
		fail(w, 400, "请选择图片（不超过 5MB）")
		return
	}
	defer f.Close()
	head := make([]byte, 512)
	n, _ := f.Read(head)
	ext := map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp"}[http.DetectContentType(head[:n])]
	if ext == "" || hdr.Size > 5<<20 {
		fail(w, 400, "仅支持 jpg / png / webp，且不超过 5MB")
		return
	}
	f.Seek(0, io.SeekStart)
	b := make([]byte, 8)
	rand.Read(b)
	name := hex.EncodeToString(b) + ext
	os.MkdirAll(s.uploadDir, 0o755)
	out, err := os.Create(filepath.Join(s.uploadDir, name))
	if err != nil {
		fail(w, 500, "保存失败")
		return
	}
	defer out.Close()
	io.Copy(out, f)
	writeJSON(w, 200, map[string]string{"url": "/uploads/" + name})
}

// ---------- 收益报表 CSV 导入 ----------
// 列：日期,视频号ID,剧目,收益,视频播放,剧集播放（首行表头可有可无）
func (s *server) importIncome(w http.ResponseWriter, r *http.Request, u *user) {
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	f, _, err := r.FormFile("file")
	if err != nil {
		fail(w, 400, "请选择 CSV 文件")
		return
	}
	defer f.Close()
	recs, err := readCSV(f)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	dramas := map[string][2]int64{}
	rows, _ := s.db.Query(`SELECT title,id,theater_id FROM dramas`)
	for rows.Next() {
		var t string
		var id, th int64
		rows.Scan(&t, &id, &th)
		dramas[t] = [2]int64{id, th}
	}
	rows.Close()
	tx, _ := s.db.Begin()
	defer tx.Rollback()
	ok, errs := 0, []string{}
	for i, rec := range recs {
		rec[0] = strings.TrimPrefix(rec[0], "\xef\xbb\xbf")
		if _, e := time.Parse("2006-01-02", strings.TrimSpace(rec[0])); e != nil && i == 0 {
			continue // 表头
		}
		if len(rec) < 6 {
			errs = append(errs, fmt.Sprintf("第 %d 行：需要 6 列", i+1))
			continue
		}
		d, e := time.Parse("2006-01-02", strings.TrimSpace(rec[0]))
		inc, e2 := strconv.ParseFloat(strings.TrimSpace(rec[3]), 64)
		vp, e3 := strconv.ParseInt(strings.TrimSpace(rec[4]), 10, 64)
		ep, e4 := strconv.ParseInt(strings.TrimSpace(rec[5]), 10, 64)
		dr, found := dramas[strings.TrimSpace(rec[2])]
		ch := strings.TrimSpace(rec[1])
		switch {
		case e != nil:
			errs = append(errs, fmt.Sprintf("第 %d 行：日期格式应为 YYYY-MM-DD", i+1))
		case ch == "":
			errs = append(errs, fmt.Sprintf("第 %d 行：视频号 ID 为空", i+1))
		case !found:
			errs = append(errs, fmt.Sprintf("第 %d 行：找不到剧目“%s”", i+1, strings.TrimSpace(rec[2])))
		case e2 != nil || e3 != nil || e4 != nil || inc < 0 || vp < 0 || ep < 0:
			errs = append(errs, fmt.Sprintf("第 %d 行：收益或播放量不是有效数字", i+1))
		default:
			tx.Exec(`INSERT INTO income_reports(report_date,channel_id,theater_id,drama_id,income,video_plays,episode_plays) VALUES(?,?,?,?,?,?,?)`,
				d.Format("2006-01-02"), ch, dr[1], dr[0], inc, vp, ep)
			ok++
		}
	}
	if len(errs) > 0 { // 有错误整体不入库，避免半截数据
		if len(errs) > 20 {
			errs = append(errs[:20], fmt.Sprintf("…另有 %d 处错误", len(errs)-20))
		}
		writeJSON(w, 400, map[string]any{"error": "存在错误行，未导入任何数据", "errors": errs})
		return
	}
	if err := tx.Commit(); err != nil {
		fail(w, 500, "导入失败")
		return
	}
	writeJSON(w, 200, map[string]int{"imported": ok})
}

// ---------- 用户 ----------
func (s *server) adminUsers(w http.ResponseWriter, r *http.Request, u *user) {
	if rows := s.query(w, `SELECT u.id,u.account,u.nickname,u.role,u.created_at,(SELECT COUNT(*) FROM video_accounts v WHERE v.user_id=u.id) AS accounts FROM users u ORDER BY u.id DESC`); rows != nil {
		writeJSON(w, 200, rows)
	}
}

func (s *server) resetPassword(w http.ResponseWriter, r *http.Request, u *user) {
	b := make([]byte, 4)
	rand.Read(b)
	pwd := hex.EncodeToString(b)
	hash, _ := bcrypt.GenerateFromPassword([]byte(pwd), bcrypt.DefaultCost)
	res, _ := s.db.Exec(`UPDATE users SET password_hash=? WHERE id=? AND role='talent'`, hash, r.PathValue("id"))
	if n, _ := res.RowsAffected(); n == 0 {
		fail(w, 404, "用户不存在或不可重置")
		return
	}
	s.db.Exec(`DELETE FROM sessions WHERE user_id=?`, r.PathValue("id"))
	writeJSON(w, 200, map[string]string{"password": pwd})
}

// 读取 CSV：去掉 BOM；拒绝 xlsx；非 UTF-8 时按 GBK 解码（Excel 中文版默认另存格式）
func readCSV(f io.Reader) ([][]string, error) {
	raw, err := io.ReadAll(f)
	if err != nil {
		return nil, errors.New("读取文件失败")
	}
	if bytes.HasPrefix(raw, []byte("PK")) {
		return nil, errors.New("这是 Excel 文件，请另存为“CSV UTF-8（逗号分隔）”后再上传")
	}
	raw = bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))
	if !utf8.Valid(raw) {
		if raw, err = simplifiedchinese.GBK.NewDecoder().Bytes(raw); err != nil {
			return nil, errors.New("文件编码无法识别，请另存为 CSV UTF-8")
		}
	}
	cr := csv.NewReader(bytes.NewReader(raw))
	cr.FieldsPerRecord = -1
	cr.LazyQuotes = true
	cr.TrimLeadingSpace = true
	recs, err := cr.ReadAll()
	if err != nil {
		return nil, errors.New("CSV 解析失败：" + err.Error())
	}
	return recs, nil
}

// 兼容 2026-11-04 / 2026/11/4 等 Excel 常见写法
func parseDate(s string) (string, bool) {
	s = strings.ReplaceAll(strings.TrimSpace(s), "/", "-")
	for _, layout := range []string{"2006-01-02", "2006-1-2"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Format("2006-01-02"), true
		}
	}
	return "", false
}

// 批量导入剧目：按表头名取列；“剧名+剧场”已存在则更新（保留封面），否则新增；有错误整体不入库
func (s *server) importDramas(w http.ResponseWriter, r *http.Request, u *user) {
	r.Body = http.MaxBytesReader(w, r.Body, 5<<20)
	f, _, err := r.FormFile("file")
	if err != nil {
		fail(w, 400, "请选择 CSV 文件")
		return
	}
	defer f.Close()
	recs, err := readCSV(f)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	if len(recs) < 2 {
		fail(w, 400, "文件里没有数据行，请按模板填写")
		return
	}
	if len(recs) > 1001 {
		fail(w, 400, "一次最多导入 1000 行")
		return
	}
	col := map[string]int{}
	for i, h := range recs[0] {
		col[strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(h), "*"))] = i
	}
	for _, need := range []string{"剧名", "剧场"} {
		if _, ok := col[need]; !ok {
			fail(w, 400, "表头缺少“"+need+"”列，请使用标准模板")
			return
		}
	}
	theaters := map[string]int64{}
	rows, _ := s.db.Query(`SELECT name,id FROM theaters`)
	for rows.Next() {
		var n string
		var id int64
		rows.Scan(&n, &id)
		theaters[n] = id
	}
	rows.Close()

	tx, _ := s.db.Begin()
	defer tx.Rollback()
	added, updated, errs := 0, 0, []string{}
	for i, rec := range recs[1:] {
		line := i + 2
		get := func(name string) string {
			if j, ok := col[name]; ok && j < len(rec) {
				return strings.TrimSpace(rec[j])
			}
			return ""
		}
		if strings.Join(rec, "") == "" {
			continue
		}
		bad := func(msg string) { errs = append(errs, fmt.Sprintf("第 %d 行：%s", line, msg)) }
		title, tname := get("剧名"), get("剧场")
		tid, found := theaters[tname]
		status, heat, eps, date := get("状态"), 5, 0, time.Now().Format("2006-01-02")
		if status == "" {
			status = "已上架"
		}
		if v := get("推荐热度"); v != "" {
			if heat, err = strconv.Atoi(v); err != nil || heat < 1 || heat > 5 {
				bad("推荐热度需为 1~5")
				continue
			}
		}
		if v := get("总集数"); v != "" {
			if eps, err = strconv.Atoi(v); err != nil || eps < 0 {
				bad("总集数需为非负整数")
				continue
			}
		}
		if v := get("上架日期"); v != "" {
			var ok bool
			if date, ok = parseDate(v); !ok {
				bad("上架日期格式应为 2026-11-04")
				continue
			}
		}
		switch {
		case title == "":
			bad("剧名为空")
			continue
		case !found:
			bad("剧场“" + tname + "”不存在，请先在“剧场”里添加")
			continue
		case !statusSet[status]:
			bad("状态只能是 已上架 / 即将下架 / 已下架")
			continue
		}
		ratio := get("画幅")
		if ratio == "" {
			ratio = "竖屏"
		}
		var id int64
		err := tx.QueryRow(`SELECT id FROM dramas WHERE title=? AND theater_id=?`, title, tid).Scan(&id)
		if err == nil {
			tx.Exec(`UPDATE dramas SET alias=?,original_name=?,type=?,ratio=?,episodes=?,intro=?,reason=?,heat=?,status=?,listed_at=?,material=? WHERE id=?`,
				get("别名"), get("原剧名"), get("类型"), ratio, eps, get("剧目简介"), get("推荐理由"), heat, status, date, get("推广素材"), id)
			updated++
			continue
		}
		hue := 0
		for _, c := range title {
			hue += int(c)
		}
		tx.Exec(`INSERT INTO dramas(theater_id,title,alias,original_name,type,ratio,episodes,intro,reason,heat,status,listed_at,hue,material) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			tid, title, get("别名"), get("原剧名"), get("类型"), ratio, eps, get("剧目简介"), get("推荐理由"), heat, status, date, hue%360, get("推广素材"))
		added++
	}
	if len(errs) > 0 {
		if len(errs) > 20 {
			errs = append(errs[:20], fmt.Sprintf("…另有 %d 处错误", len(errs)-20))
		}
		writeJSON(w, 400, map[string]any{"error": "存在错误行，未导入任何数据", "errors": errs})
		return
	}
	if err := tx.Commit(); err != nil {
		fail(w, 500, "导入失败")
		return
	}
	writeJSON(w, 200, map[string]int{"added": added, "updated": updated})
}
