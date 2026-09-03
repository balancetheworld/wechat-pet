# 上游同步：文件一本地修复 → 文件三（GitHub balancetheworld/wechat-pet）

补丁基线：文件三 HEAD = `6cb5b16`。
补丁文件：`wechat-pet-upstream-4bugfixes.patch`（3 个后端文件，均已在文件一 Postgres 环境回归验证 200）。

## 修复内容

### ① internal/httpapi/pet/routes.go —— 静态路由 resource 参数恒空导致 500
`GET/PATCH /:pet_id/profile`、`GET /:pet_id/dates` 是静态路由，handler 里 `c.Param("resource")` 恒为空串，被 resourceSpec 当作未知资源返回 500。
修复：删除两条带病静态路由，请求落入已有的泛化 `/:resource` 路由（能正确传入资源名）；同时补上缺失的两段式 `PATCH /:pet_id/:resource`。
验证：`GET /dates`、`GET/PATCH /profile`、`GET/POST growth-events`、`PUT health` 全部 200。

### ② internal/app/pet/profile.go —— DATE 列 RFC3339 污染 + 派生字段全废
pgx 驱动把 Postgres DATE 列返回为 `time.Time`（JSON 序列化成 `2024-03-15T00:00:00Z`），导致：
- `GetProfile` 用 `time.Parse("2006-01-02")` 解析失败 → age / companion_days / next_birthday_days 全为 0/null；
- 前端拿到带 `T00:00:00Z` 后缀的日期串，时间线/体重排序显示异常。

修复：新增 `dateOnlyString / dateOnly / dateOnlyFields`（vaccinated_at、measured_at、occurred_at）归一化为 `YYYY-MM-DD`，应用于 `GetProfile` 扫描后与泛化 Resource GET 输出。
验证：`birthday:"2024-03-15"`、`age:2`、`companion_days:824`、`next_birthday_days:192`。
（SQLite 无此问题——DATE 列存 TEXT；仅 Postgres 部署受影响。）

### ③ internal/app/calendar/repository.go —— CompleteReminder 事务内外键顺序错误
`CompleteReminder` 在事务内先 `UPDATE calendar_reminders` 写 `completed_record_id`（外键 → calendar_records），后 INSERT 完成记录。Postgres 事务内外键立即检查，UPDATE 时目标记录尚不存在 → SQLSTATE 23503 → 接口恒 500。
SQLite 默认不启用外键（go-sqlite3 需显式 PRAGMA），所以单测/CI 未暴露。
修复：事务内顺序调整为 insertRecord/insertMedia → UPDATE 提醒状态；`affected == 0` 仍返回 ErrReminderCompleted，由 defer rollback 回滚已插记录，语义不变。
验证：仓库层与 HTTP 层均通过，月度重复提醒正确生成下一期（next_reminder=2026-10-03）。

### ④（前端侧建议，不在本补丁内）体重 float32 精度
pets weight 列为 float32，4.2 存取后变 `4.199999809265137`。文件一在前端展示层四舍五入 2 位小数解决（web/src 前端若展示体重建议同样处理，或后端列改 NUMERIC）。

## 在文件三应用的方法

```powershell
cd "D:\CQUPT\前端\小程序比赛\项目\wechat-pet 周-二版\wechat-pet"

# 先确认工作区干净
git status

# 应用补丁（补丁路径按实际位置调整）
git apply "D:\CQUPT\前端\小程序比赛\项目\wechat-pet 周\wechat-pet\docs\upstream-sync\wechat-pet-upstream-4bugfixes.patch"

# 本地验证
go build ./... && go vet ./...

# 提交并推送
git add internal/
git commit -m "fix(pet,calendar): 修复 Postgres 下的三个真实 bug

- pet/routes: 静态路由 /profile /dates 的 resource 参数恒空导致 500，改走泛化 :resource 路由并补 PATCH 两段式
- pet/profile: pgx DATE 列返回 RFC3339 致派生字段(age/companion_days)全废与日期串污染，输出统一归一化 YYYY-MM-DD
- calendar/repository: CompleteReminder 事务内先 UPDATE 外键后插记录，Postgres 立即外键检查违反 23503，调整为先插后更"
git push
```

## 注意

- 补丁基于 6cb5b16 生成；若同事又有新提交，`git apply` 失败时改用 `git apply --3way`。
- 三处修复互不依赖，也可以拆成三个提交分别 cherry-pick。
- 文件一 server/ 与文件三当前仅在上述 3 个文件上存在差异（另有 config.yaml、data/ 上传目录为文件一本地产物，无需同步）。
