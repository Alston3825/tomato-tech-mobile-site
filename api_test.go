package main

import (
	"database/sql"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func testServer(t *testing.T) *server {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	s := &server{db: db}
	for _, q := range []string{
		`INSERT INTO users(id,account,nickname,password_hash,created_at) VALUES(1,'test','测试','unused','2026-01-01')`,
		`INSERT INTO video_accounts(id,user_id,nickname,created_at) VALUES(1,1,'测试号','2026-01-01')`,
		`INSERT INTO theaters(id,name) VALUES(1,'测试剧场')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

func addTestApp(t *testing.T, s *server, id int, date, status string) {
	t.Helper()
	if _, err := s.db.Exec(`INSERT INTO video_accounts(id,user_id,nickname,created_at) VALUES(?,1,'测试',?)`, id+1, date); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO applications(id,video_account_id,theater_id,status,created_at) VALUES(?,?,1,?,?)`, id, id+1, status, date); err != nil {
		t.Fatal(err)
	}
}

func reviewRequest(s *server, id, status string) int {
	r := httptest.NewRequest("PATCH", "/api/admin/applications/"+id, strings.NewReader(fmt.Sprintf(`{"status":%q}`, status)))
	r.SetPathValue("id", id)
	w := httptest.NewRecorder()
	s.review(w, r, &user{ID: 1, Role: "admin"})
	return w.Code
}

func TestReviewAtomicity(t *testing.T) {
	for _, status := range []string{"已通过", "已驳回"} {
		t.Run(status, func(t *testing.T) {
			s := testServer(t)
			addTestApp(t, s, 1, now(), "待审核")
			// 注入通知写入失败，状态和审核时间必须一起回滚。
			if _, err := s.db.Exec(`CREATE TRIGGER fail_note BEFORE INSERT ON notifications BEGIN SELECT RAISE(ABORT,'test failure'); END`); err != nil {
				t.Fatal(err)
			}
			if code := reviewRequest(s, "1", status); code != 500 {
				t.Fatalf("code=%d", code)
			}
			var got string
			var reviewed sql.NullString
			if err := s.db.QueryRow(`SELECT status,reviewed_at FROM applications WHERE id=1`).Scan(&got, &reviewed); err != nil {
				t.Fatal(err)
			}
			if got != "待审核" || reviewed.Valid {
				t.Fatalf("partial update: %s %v", got, reviewed)
			}
			if _, err := s.db.Exec(`DROP TRIGGER fail_note`); err != nil {
				t.Fatal(err)
			}
			if code := reviewRequest(s, "1", status); code != 200 {
				t.Fatalf("code=%d", code)
			}
			if code := reviewRequest(s, "1", status); code != 400 {
				t.Fatalf("repeat code=%d", code)
			}
			if err := s.db.QueryRow(`SELECT status,reviewed_at FROM applications WHERE id=1`).Scan(&got, &reviewed); err != nil {
				t.Fatal(err)
			}
			if got != status || !reviewed.Valid {
				t.Fatalf("missing update: %s %v", got, reviewed)
			}
			var notes int
			if err := s.db.QueryRow(`SELECT COUNT(*) FROM notifications WHERE title=?`, "报白申请"+status).Scan(&notes); err != nil {
				t.Fatal(err)
			}
			if notes != 1 {
				t.Fatalf("notes=%d", notes)
			}
		})
	}
}

func TestConcurrentReview(t *testing.T) {
	s := testServer(t)
	addTestApp(t, s, 1, now(), "待审核")
	var wg sync.WaitGroup
	codes := make(chan int, 2)
	for _, status := range []string{"已通过", "已驳回"} {
		wg.Add(1)
		go func(status string) { defer wg.Done(); codes <- reviewRequest(s, "1", status) }(status)
	}
	wg.Wait()
	close(codes)
	counts := map[int]int{}
	for code := range codes {
		counts[code]++
	}
	if counts[200] != 1 || counts[400] != 1 {
		t.Fatalf("codes=%v", counts)
	}
	var notes int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM notifications`).Scan(&notes); err != nil {
		t.Fatal(err)
	}
	if notes != 1 {
		t.Fatalf("notes=%d", notes)
	}
}

func TestQuotaLimitsAndRejection(t *testing.T) {
	n := time.Now()
	cases := []struct {
		name                                           string
		current, earlier, rejected, previousYear, want int
	}{
		{"月末最后一个名额", 99, 0, 0, 0, 1},
		{"月上限", 100, 0, 0, 0, 0},
		{"年上限", 0, 200, 0, 0, 0},
		{"驳回与去年不占用", 0, 0, 100, 200, 100},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := testServer(t)
			id := 0
			add := func(count int, date, status string) {
				for i := 0; i < count; i++ {
					id++
					addTestApp(t, s, id, date, status)
				}
			}
			add(tc.current, n.Format("2006-01-02 15:04:05"), "待审核")
			// 一月没有同年更早月份，此时以本月记录验证年度上限。
			earlier := time.Date(n.Year(), 1, 1, 0, 0, 0, 0, n.Location())
			add(tc.earlier, earlier.Format("2006-01-02 15:04:05"), "已通过")
			add(tc.rejected, now(), "已驳回")
			add(tc.previousYear, fmt.Sprintf("%d-12-01 00:00:00", n.Year()-1), "已通过")
			var month, year, left int
			if err := s.db.QueryRow(`SELECT `+quotaCols()+` FROM theaters t WHERE t.id=1`).Scan(&month, &year, &left); err != nil {
				t.Fatal(err)
			}
			if left != tc.want {
				t.Fatalf("month=%d year=%d left=%d want=%d", month, year, left, tc.want)
			}
			if tc.current == 100 {
				if code := reviewRequest(s, "1", "已驳回"); code != 200 {
					t.Fatalf("review=%d", code)
				}
				if err := s.db.QueryRow(`SELECT `+quotaCols()+` FROM theaters t WHERE t.id=1`).Scan(&month, &year, &left); err != nil {
					t.Fatal(err)
				}
				if left != 1 {
					t.Fatalf("驳回未释放名额: %d", left)
				}
			}
		})
	}
}

func TestReviewValidation(t *testing.T) {
	s := testServer(t)
	if code := reviewRequest(s, "1", "无效"); code != 400 {
		t.Fatalf("invalid=%d", code)
	}
	if code := reviewRequest(s, "999", "已通过"); code != 404 {
		t.Fatalf("missing=%d", code)
	}
	if err := s.db.Close(); err != nil {
		t.Fatal(err)
	}
	if code := reviewRequest(s, "1", "已通过"); code != 500 {
		t.Fatalf("database error=%d", code)
	}
}
