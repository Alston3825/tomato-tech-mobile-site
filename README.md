# 番茄科技 · 达人工作台

Go + SQLite 单二进制，前端静态文件通过 `go:embed` 打包。

```bash
go build -o tomato-site .
ADMIN_PASSWORD=你的管理员密码 ./tomato-site -addr :8080 -db tomato.db
```

- 首次启动自动建表并写入示例剧场、剧目、报表；管理员账号 `admin`（未设置 `ADMIN_PASSWORD` 时随机密码打印在日志中）。
- 示例视频号 ID：`demo001`、`demo002`，添加后可看到收益。
- 管理员审核报白：`PATCH /api/admin/applications/{id}`，body `{"status":"已通过"}`（通知达人）。报白名额自动计算：每剧场每年 200、每月 100，待审核+已通过占用，已驳回释放。
- 表结构见 `db.go`：users / sessions / video_accounts / theaters / dramas / applications / income_reports / notifications。
