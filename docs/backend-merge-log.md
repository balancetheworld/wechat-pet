# 后端搬运决策日志：文件三 → 文件一（2026-09-03 00:35）

## 任务定义
- **文件一**：`D:\CQUPT\前端\小程序比赛\项目\wechat-pet 周\wechat-pet`（本地主线，client/ + server/）
- **文件三**：`D:\CQUPT\前端\小程序比赛\项目\wechat-pet 周-二版\wechat-pet`（GitHub 上游 balancetheworld/wechat-pet，HEAD=71c8a54）
- **指令**：把文件三的后端搬进文件一；一切冲突取文件三版；有选择处自主择优，但全部记录于本日志。

## 执行方式
整体替换 `server/` 下的后端文件（git 工作区此前 clean，可随时 `git checkout -- server/` 回滚）：
`cmd/`、`internal/`、`migrations/`、`scripts/`、`Makefile`、`config.example.yaml`、`go.mod`、`go.sum` 全部删除后从文件三复制。

## 决策记录（逐条可查）

| # | 决策点 | 选择 | 理由 |
|---|---|---|---|
| D1 | 后端摆放位置 | **保留 `server/` 子目录**，不采用文件三的「后端在仓库根」结构 | 文件一是 client/ + server/ 双目录 monorepo；若把后端搬到根目录会与 client/ 混居且改动面失控。仅后端搬运，不动仓库骨架 |
| D2 | Go module 名 | **取文件三的 `github.com/balancetheworld/wechat-pet`**（原文件一为 `.../wechat-pet/server`） | 文件三所有代码 import 路径基于自己的 module 名；保留它 = 零代码改动、与上游后续拉取无 diff。冲突取文件三版的原则直接适用 |
| D3 | 文件一独有的 `internal/service/auth` | **删除**（随整体替换自然消失） | 侦察确认：文件一中无任何代码引用它（死代码）；文件三已无此目录；wechat 客户端逻辑由 `internal/app/auth` 消费。符合「冲突取文件三」 |
| D4 | 本地 `server/config.yaml`（未跟踪） | **保留不动** | 含本地开发 DSN/微信密钥；两版 config 键集合完全一致（已 diff config.go 的 yaml 标签），旧配置直接兼容，无需改写 |
| D5 | 文件三根目录的 `.env.example` | **复制为 `server/.env.example`** | 内容全部是后端环境变量（STORAGE_DRIVER/COS_*/REDIS_ADDR 等），属后端文档；.gitignore 已有 `.env` 忽略 + `!.env.example` 保留规则 |
| D6 | 文件三根目录的 `.tool-versions` | **不搬运** | 内容混含 nodejs/pnpm（前端工具链版本），属仓库级文件而非后端文件；文件一根目录未要求对齐 |
| D7 | 文件三根目录的 `AGENTS.md` / `README.md` / `.github` / `.vscode` / `.editorconfig` | **不搬运** | 属仓库级协作/文档文件，不是后端代码；文件一已有自己的文档体系（docs/） |
| D8 | 数据库迁移 000006（pet_profiles） | **随包带入**，但**不执行 migrate-up** | 搬运任务只搬代码；用户本地 Postgres 是否升级 schema 由用户自行决定（运行 `make migrate-up` 或 `./scripts/migrate-up.sh`） |
| D9 | 验证标准 | go build + go vet 必须全过；go test 尽力 | 见下方验证记录 |

## 验证记录（2026-09-03 00:35）
- `go build ./...` ✅（自动拉取文件三新增依赖：tencentyun/cos-go-sdk-v5、joho/godotenv、mapstructure 等）
- `go vet ./...` ✅
- `go test ./...`：**15 包通过**（asset、middleware、config、jwt、response、storage、wechat、file 等），3 处失败全部同一根因——**测试用 SQLite 需 CGO，本机无 gcc**：
  - `internal/pkg/database` TestOpenSQLite：`CGO_ENABLED=0 stub`
  - `internal/httpapi` TestFamilyRoutesEnforceRoleAndFamilyScope / TestPetRoutesCompleteCRUDFlow：同上
  - **已交叉验证：在文件三原始仓库跑同样命令，失败完全相同** → 环境限制，非搬运引入。要跑全量测试需安装 mingw-w64 gcc 后 `CGO_ENABLED=1 go test ./...`

## 搬运带来的后端新能力（相对文件一原状）
1. **宠物富档案**：migration 000006——pets 表扩列（breed/gender/sterilized/birthday/home_date/avatar_asset_id/cover_asset_id）+ 12 张新表（证件/个性/问答/健康/疾病/疫苗/生日纪念×3/体重/成长足迹×2）
2. **档案接口族**：`GET/PATCH /api/v1/pets/:id/profile`、`GET /pets/:id/dates`、泛化 `/pets/:id/:resource(+/:resource_id)`；PetProfile 派生 age/companion_days/next_birthday_days
3. **文件上传**：`POST /api/v1/assets/upload` + `internal/platform/storage`（COS 与本地双实现）+ `internal/service/file`
4. **新增依赖**：腾讯 COS SDK 等（见 go.mod diff）

## 对前端（client/）的连带影响——本次未改，仅提示
- 前端 `types/pet.ts`、services 仍未接新接口（文件三前端同样没接），后续联调时需前端补 asset 上传与 profile 字段映射
- 本地 client 的 pet-manual services `getPets` 取 `data.items` 的解析 bug 依旧存在（后端 List 返回数组）

## 回滚方式
`git checkout -- server/ && git clean -fd server/`（注意 clean 会删掉未跟踪的 config.yaml 和 .env.example，先备份）
