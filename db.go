package main

import (
	"database/sql"
	"fmt"
	"math/rand"
	"time"

	_ "modernc.org/sqlite"
)

// 表结构：用户/会话/视频号/剧场/剧目/报白申请/收益报表/通知
const schema = `
CREATE TABLE IF NOT EXISTS users (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  account TEXT NOT NULL UNIQUE,
  nickname TEXT NOT NULL,
  password_hash TEXT NOT NULL,
  role TEXT NOT NULL DEFAULT 'talent',          -- talent 达人 / admin 管理员
  theme TEXT NOT NULL DEFAULT 'caramel',
  created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS sessions (
  token TEXT PRIMARY KEY,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS video_accounts (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  channel_id TEXT NOT NULL DEFAULT '',          -- 视频号完整 ID，用于匹配报表
  nickname TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_va_user_channel ON video_accounts(user_id, channel_id) WHERE channel_id <> '';
CREATE TABLE IF NOT EXISTS theaters (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL UNIQUE,
  quota_left INTEGER NOT NULL DEFAULT 0         -- 剩余报白名额
);
CREATE TABLE IF NOT EXISTS dramas (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  theater_id INTEGER NOT NULL REFERENCES theaters(id),
  title TEXT NOT NULL,
  alias TEXT NOT NULL DEFAULT '',
  original_name TEXT NOT NULL DEFAULT '',
  type TEXT NOT NULL DEFAULT '',                -- AI真人剧 / AI漫剧
  ratio TEXT NOT NULL DEFAULT '',               -- 竖屏 / 横屏
  episodes INTEGER NOT NULL DEFAULT 0,
  intro TEXT NOT NULL DEFAULT '',
  reason TEXT NOT NULL DEFAULT '',              -- 推荐理由
  heat INTEGER NOT NULL DEFAULT 5,              -- 推荐热度 1~5
  status TEXT NOT NULL DEFAULT '已上架',         -- 已上架 / 即将下架 / 已下架
  listed_at TEXT NOT NULL,
  hue INTEGER NOT NULL DEFAULT 10,              -- 封面渐变色相
  material TEXT NOT NULL DEFAULT ''             -- 推广素材（网盘分享文本）
);
CREATE TABLE IF NOT EXISTS applications (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  video_account_id INTEGER NOT NULL REFERENCES video_accounts(id) ON DELETE CASCADE,
  theater_id INTEGER NOT NULL REFERENCES theaters(id),
  note TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT '待审核',         -- 待审核 / 已通过 / 已驳回
  created_at TEXT NOT NULL,
  reviewed_at TEXT,
  UNIQUE(video_account_id, theater_id)
);
CREATE TABLE IF NOT EXISTS income_reports (     -- 分成前的原始报表明细
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  report_date TEXT NOT NULL,
  channel_id TEXT NOT NULL,
  theater_id INTEGER NOT NULL REFERENCES theaters(id),
  drama_id INTEGER NOT NULL REFERENCES dramas(id),
  income REAL NOT NULL DEFAULT 0,
  video_plays INTEGER NOT NULL DEFAULT 0,
  episode_plays INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS ix_ir_channel_date ON income_reports(channel_id, report_date);
CREATE TABLE IF NOT EXISTS notifications (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  title TEXT NOT NULL,
  body TEXT NOT NULL DEFAULT '',
  is_read INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL
);
`

func openDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		return nil, err
	}
	if err := migrate(db); err != nil {
		return nil, err
	}
	return db, seed(db)
}

type seedDrama struct {
	theater, title, typ, ratio, listed, reason, intro, material string
	eps, hue                                                    int
	status                                                      string
}

// 首次启动写入示例剧场、剧目与报表数据
func seed(db *sql.DB) error {
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM theaters`).Scan(&n)
	if n > 0 {
		return nil
	}
	theaters := []struct {
		name string
		left int
	}{{"番茄炒鸡蛋漫剧社", 16}, {"亿点点漫剧场", 77}, {"极好剧场", 5}, {"大番茄AI漫剧社", 0}, {"帝乡剧场", 0}, {"火栖爱看漫", 0}, {"格格语漫剧社", 0}}
	ids := map[string]int64{}
	for _, t := range theaters {
		r, err := db.Exec(`INSERT INTO theaters(name,quota_left) VALUES(?,?)`, t.name, t.left)
		if err != nil {
			return err
		}
		ids[t.name], _ = r.LastInsertId()
	}
	dramas := []seedDrama{
		{"极好剧场", "墙内守兰记", "待补充", "待补充", "2026-11-04", "爆剧", "一座老宅、一株兰花，守墙人的半生秘密在雨夜被揭开。", "墙内守兰记 · 示例提取码 6666", 77, 12, "已上架"},
		{"大番茄AI漫剧社", "寿宴不请我，账单别找我", "AI真人剧", "竖屏", "2026-11-02", "五分钟让人上瘾的短剧，剧情紧凑，太好看了！", "寿宴当天，被刻意遗忘的女儿亮出了账本。", "寿宴不请我 · 示例提取码 8888", 84, 28, "已上架"},
		{"亿点点漫剧场", "婚礼当天他接了个电话", "AI真人剧", "竖屏", "2026-11-02", "反转密集", "红毯尽头，新郎的一通电话让全场安静。", "婚礼当天 · 示例提取码 1688", 70, 350, "已上架"},
		{"极好剧场", "婆婆，别做得太绝", "AI真人剧", "竖屏", "2026-11-02", "情绪拉满", "忍到极限的儿媳，决定不再退让。", "婆婆别做得太绝 · 示例提取码 2026", 51, 40, "已上架"},
		{"番茄炒鸡蛋漫剧社", "抄家后我靠赶海养瘸腿相公", "AI真人剧", "竖屏", "2026-11-02", "剧情精彩，播放量高", "家道中落，她在海边挖出了新的天地。", "赶海养相公 · 示例提取码 5201", 265, 200, "已上架"},
		{"火栖爱看漫", "奶奶的身份与婚礼风波", "AI真人剧", "竖屏", "2026-11-02", "新剧上架", "六十八岁的盛远集团董事长隐去身份，回国参加孙子的婚礼，却被嫌弃衣着朴素。她看清了谁才是真心。", "奶奶的身份 · 示例提取码 AIGC", 48, 20, "已上架"},
		{"帝乡剧场", "重生后我撕了联姻书", "AI漫剧", "竖屏", "2026-10-28", "口碑稳定", "睁眼回到订婚前夜，这一次她要自己选。", "撕联姻书 · 示例提取码 0707", 60, 5, "即将下架"},
		{"格格语漫剧社", "山河信笺", "AI漫剧", "横屏", "2026-10-20", "经典回顾", "一封迟到三十年的信，改写两代人的命运。", "山河信笺 · 示例提取码 3636", 36, 45, "已下架"},
		{"番茄炒鸡蛋漫剧社", "总裁的番茄小铺", "AI漫剧", "竖屏", "2026-10-18", "治愈甜宠", "街角小铺里，一颗番茄牵出一段缘分。", "番茄小铺 · 示例提取码 9999", 42, 8, "已上架"},
	}
	var dramaIDs []int64
	var dramaTheater []int64
	for _, d := range dramas {
		r, err := db.Exec(`INSERT INTO dramas(theater_id,title,original_name,type,ratio,episodes,intro,reason,status,listed_at,hue,material)
			VALUES(?,?,'待补充',?,?,?,?,?,?,?,?,?)`, ids[d.theater], d.title, d.typ, d.ratio, d.eps, d.intro, d.reason, d.status, d.listed, d.hue, d.material)
		if err != nil {
			return err
		}
		id, _ := r.LastInsertId()
		dramaIDs = append(dramaIDs, id)
		dramaTheater = append(dramaTheater, ids[d.theater])
	}
	// 示例报表：演示视频号 demo001/demo002 可直接用于添加后查看收益，其余用于榜单
	rnd := rand.New(rand.NewSource(7))
	channels := []string{"demo001", "demo002", "sphpcJalp3q7edO", "sphT0bqdyn7ADR0", "sphbnyrrFOtFXOm", "sph5ThUr7JghMt6", "sphAWAxNZNBFNs2", "sphORguPpmlqvSb", "sphUiCjuV6VlgQY", "sph0XROd4ZVS0Vt"}
	today := time.Now()
	tx, _ := db.Begin()
	for _, ch := range channels {
		for day := 1; day <= 40; day++ {
			k := rnd.Intn(len(dramaIDs))
			plays := 200 + rnd.Intn(4000)
			tx.Exec(`INSERT INTO income_reports(report_date,channel_id,theater_id,drama_id,income,video_plays,episode_plays) VALUES(?,?,?,?,?,?,?)`,
				today.AddDate(0, 0, -day).Format("2006-01-02"), ch, dramaTheater[k], dramaIDs[k],
				float64(int(float64(plays)*(0.002+rnd.Float64()*0.006)*100))/100, plays, plays+rnd.Intn(3000))
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	fmt.Println("已写入示例数据")
	return nil
}

// 增量迁移：为已有数据库补充新列
func migrate(db *sql.DB) error {
	var has int
	db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('dramas') WHERE name='cover'`).Scan(&has)
	if has == 0 {
		_, err := db.Exec(`ALTER TABLE dramas ADD COLUMN cover TEXT NOT NULL DEFAULT ''`) // 封面图地址
		return err
	}
	return nil
}
