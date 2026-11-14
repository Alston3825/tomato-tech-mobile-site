package main

import (
	"crypto/rand"
	"database/sql"
	"embed"
	"encoding/hex"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"os"

	"golang.org/x/crypto/bcrypt"
)

//go:embed web
var webFS embed.FS

func main() {
	addr := flag.String("addr", ":8080", "监听地址")
	dbPath := flag.String("db", "tomato.db", "SQLite 文件路径")
	uploads := flag.String("uploads", "uploads", "封面图上传目录")
	flag.Parse()

	db, err := openDB(*dbPath)
	if err != nil {
		log.Fatal(err)
	}
	s := &server{db: db, uploadDir: *uploads}
	ensureAdmin(db)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/auth/register", s.register)
	mux.HandleFunc("POST /api/auth/login", s.login)
	mux.HandleFunc("POST /api/auth/logout", s.auth(false, s.logout))
	mux.HandleFunc("GET /api/me", s.auth(false, s.me))
	mux.HandleFunc("PUT /api/me/theme", s.auth(false, s.setTheme))

	mux.HandleFunc("GET /api/accounts", s.auth(false, s.listAccounts))
	mux.HandleFunc("POST /api/accounts", s.auth(false, s.addAccount))
	mux.HandleFunc("DELETE /api/accounts/{id}", s.auth(false, s.delAccount))

	mux.HandleFunc("GET /api/theaters", s.auth(false, s.listTheaters))
	mux.HandleFunc("GET /api/dramas", s.auth(false, s.listDramas))
	mux.HandleFunc("GET /api/dramas/{id}", s.auth(false, s.getDrama))
	mux.HandleFunc("GET /api/hot", s.auth(false, s.hot))

	mux.HandleFunc("GET /api/applications", s.auth(false, s.listApps))
	mux.HandleFunc("POST /api/applications", s.auth(false, s.createApp))
	mux.HandleFunc("GET /api/admin/applications", s.auth(true, s.adminApps))
	mux.HandleFunc("PATCH /api/admin/applications/{id}", s.auth(true, s.review))

	mux.HandleFunc("POST /api/admin/dramas", s.auth(true, s.saveDrama))
	mux.HandleFunc("PUT /api/admin/dramas/{id}", s.auth(true, s.saveDrama))
	mux.HandleFunc("DELETE /api/admin/dramas/{id}", s.auth(true, s.delDrama))
	mux.HandleFunc("POST /api/admin/theaters", s.auth(true, s.saveTheater))
	mux.HandleFunc("PUT /api/admin/theaters/{id}", s.auth(true, s.saveTheater))
	mux.HandleFunc("DELETE /api/admin/theaters/{id}", s.auth(true, s.delTheater))
	mux.HandleFunc("POST /api/admin/upload", s.auth(true, s.upload))
	mux.HandleFunc("POST /api/admin/import-dramas", s.auth(true, s.importDramas))
	mux.HandleFunc("POST /api/admin/import-income", s.auth(true, s.importIncome))
	mux.HandleFunc("GET /api/admin/users", s.auth(true, s.adminUsers))
	mux.HandleFunc("POST /api/admin/users/{id}/reset-password", s.auth(true, s.resetPassword))
	mux.Handle("GET /uploads/", http.StripPrefix("/uploads/", http.FileServer(http.Dir(*uploads))))

	mux.HandleFunc("GET /api/income/overview", s.auth(false, s.overview))
	mux.HandleFunc("GET /api/income/by-video", s.auth(false, s.incomeBy("video")))
	mux.HandleFunc("GET /api/income/by-drama", s.auth(false, s.incomeBy("drama")))
	mux.HandleFunc("GET /api/income/details", s.auth(false, s.incomeBy("detail")))
	mux.HandleFunc("GET /api/rank", s.auth(false, s.rank))

	mux.HandleFunc("GET /api/notifications", s.auth(false, s.notes))
	mux.HandleFunc("POST /api/notifications/read-all", s.auth(false, s.readAll))

	sub, _ := fs.Sub(webFS, "web")
	static := http.FileServer(http.FS(sub))
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache") // 每次校验，避免浏览器沿用旧版前端
		static.ServeHTTP(w, r)
	}))

	log.Println("番茄科技达人工作台已启动", *addr)
	log.Fatal(http.ListenAndServe(*addr, mux))
}

// 首次启动创建管理员账号 admin，密码取环境变量 ADMIN_PASSWORD，未设置则随机生成并打印一次
func ensureAdmin(db *sql.DB) {
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM users WHERE account='admin'`).Scan(&n)
	if n > 0 {
		return
	}
	pwd := os.Getenv("ADMIN_PASSWORD")
	if pwd == "" {
		b := make([]byte, 6)
		rand.Read(b)
		pwd = hex.EncodeToString(b)
		log.Println("已创建管理员 admin，初始密码：", pwd)
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte(pwd), bcrypt.DefaultCost)
	db.Exec(`INSERT INTO users(account,nickname,password_hash,role,created_at) VALUES('admin','管理员',?,'admin',?)`, hash, now())
}
