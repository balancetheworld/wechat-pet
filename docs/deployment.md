# 部署与混元接入指南

本文档覆盖两件事：

1. 后端（Go + PostgreSQL + Redis）与小程序前端的部署方案选择及落地步骤；
2. 接入微信「AI 小程序成长计划」的免费混元额度。

成本数字为 2026-10 的活动价参考，实际以腾讯云官网为准。

---

## 一、部署前置条件

| 项目 | 状态 | 说明 |
| --- | --- | --- |
| 小程序 AppID / AppSecret | 待确认 | 生产环境 `WECHAT_APP_ID`、`WECHAT_APP_SECRET` 必填，否则启动即校验失败 |
| 已备案域名 | 已具备 | 小程序强制 HTTPS，且域名必须加入后台「服务器域名」白名单 |
| SSL 证书 | 已具备 | Nginx 反代层终止 TLS |
| 服务器 | 待选购 | 见第二节方案对比 |
| PostgreSQL | 待部署 | 54 个 migration 必须按序执行 |
| Redis | **实际未使用** | 全仓库只有 `REDIS_ADDR` 配置项与占位符，**没有任何 Redis 客户端代码**，依赖里也没有 redis 库。可以不装 |

### 小程序后台需要配置的域名

在 mp.weixin.qq.com → 开发管理 → 开发设置 → 服务器域名，加入：

- request 合法域名：`https://api.你的域名.com`
- uploadFile 合法域名：`https://api.你的域名.com`
- downloadFile 合法域名：`https://api.你的域名.com`

域名必须备案，且不支持 IP 和端口号。

---

## 二、部署方案对比

| 方案 | 参考月成本 | 适配工作量 | 优点 | 缺点 | 适用 |
| --- | --- | --- | --- | --- | --- |
| **A. 腾讯云轻量 + Docker Compose**（推荐起步） | 约 16 元/月（2核4G5M，188 元/年活动价） | 中：需新建 Dockerfile、compose、nginx 配置（这几个目录目前都是空的） | 最省钱、完全可控、与现有 `migrations/` 结构天然契合 | **不支持弹性扩容**，数据库自运维，单点 | 家庭级用量、快速上线 |
| **B. 轻量服务器 + 腾讯云托管 PG/Redis** | 约 16 + 60~100 元/月 | 中 | 数据库有自动备份与高可用，服务器仍可控 | 成本上升，需配安全组放行 | 数据重要后升级的目标 |
| **C. CloudBase 云托管** | 按量计费，几十元/月起 | 大：Go 服务需容器化改造，数据库要另配或换云开发数据库 | 免运维，与混元免费额度同一生态 | 改造量大、调试链路长、冷启动 | 不想管服务器时 |
| **D. 腾讯云 CVM 标准型** | 约 40~60 元/月起 | 中 | 可升配、稳定性好 | 新手配置繁琐，性价比低于轻量 | 长期正式运营 |

**推荐路径：先 A 上线，数据变重要后平滑升级到 B**（把 PG/Redis 换成托管实例，应用容器不改）。

### 实测数据：这台服务到底吃多少资源

用临时测量程序复刻 `cmd/api` 的初始化路径，逐阶段采样（SQLite 因无 CGO 环境跳过，该阶段开销极小）：

| 阶段 | 进程 Sys | HeapAlloc |
| --- | --- | --- |
| 空进程基线 | 7.0 MB | 1.4 MB |
| 配置加载后 | 7.3 MB | 1.5 MB |
| **分词器 gse 初始化后** | **282.6 MB** | 192.1 MB |
| **知识库索引构建后** | **574.4 MB** | 383.1 MB |

结论与预估差别很大，原因是中文分词词典极占内存：

- `gse` 词典加载一次约 **275 MB**，而进程里加载了**两次**——`internal/app/ask/bm25f.go:24` 与 `internal/platform/knowledge/index.go:26` 各有一份**实现完全相同**的 `DefaultTokenizer`，各自持有独立的 `sync.Once`，互不相干；
- 因此线上单进程常驻内存约 **550~600 MB**，不是常见的几十 MB 量级；
- 知识库语料本身只有 0.1 MB、20 个 chunk，内存开销可以忽略，大头全在分词词典。

这对选型的直接影响：

- **2核2G：有风险。** 进程 600 MB + PostgreSQL 约 300~400 MB + Nginx 与系统约 400 MB ≈ 1.4 GB，仅剩约 600 MB 余量，叠加图片处理与 AI 流式响应的峰值容易触发 OOM。
- **2核4G：稳妥，是正确的选择。** 余量充足，符合家庭级用量。
- **4核8G：用不上**，CPU 不是瓶颈，多花的钱买不到收益。

> 可选优化：合并两处 `DefaultTokenizer` 为单一实例，进程内存可从约 574 MB 降到约 300 MB，省下约 275 MB。届时 2核2G 也可考虑。此项为锦上添花，不影响当前上线。

> 另一个隐藏成本：**SQLite 依赖 CGO**（`mattn/go-sqlite3`）。若容器要用 SQLite，基础镜像必须带 gcc/musl-dev（选 `debian-slim` 而非 `alpine`），或确保 `CGO_ENABLED=1`。用 PostgreSQL 则无此问题。

### 方案 A 实施步骤

1. 购买腾讯云轻量应用服务器（2核4G5M 起，选 Docker 镜像或纯净 Linux），开放 80/443 端口。
2. 域名解析 `api.你的域名.com` 到服务器公网 IP。
3. 在服务器上准备好生产配置（**不要提交到 Git**）：

```bash
# /opt/pet/.env  —— APP_ENV=production 时程序不再读取 .env，必须走真实环境变量
APP_ENV=production
HTTP_ADDR=:8080
DATABASE_DRIVER=postgres
DATABASE_DSN=postgres://pet:强密码@postgres:5432/pet?sslmode=disable
JWT_SECRET=随机32位以上字符串
JWT_EXPIRE_MINUTES=120
WECHAT_APP_ID=wx真实AppID
WECHAT_APP_SECRET=真实AppSecret
WECHAT_REMINDER_TEMPLATE_ID=
REMINDER_SEND_HOUR=9
STORAGE_DRIVER=local
LOCAL_UPLOAD_DIR=/app/data/uploads
PUBLIC_BASE_URL=https://api.你的域名.com
REDIS_ADDR=redis:6379
AI_ENABLED=true
AI_PROVIDER=openai_chat
AI_API_KEY=云开发APIKey
AI_BASE_URL=https://环境ID.api.tcloudbasegateway.com/v1/ai/cloudbase
AI_MODEL=hy3-preview
AI_TIMEOUT_SECONDS=60
```

> `JWT_SECRET` 用 `openssl rand -hex 32` 生成。

4. 构建后端镜像并启动：

```bash
GOOS=linux GOARCH=amd64 go build -o bin/api ./cmd/api
# 或直接在服务器 make build
```

5. 执行数据库迁移（**必须按序**）：

```bash
export DATABASE_DSN='postgres://...'
./scripts/migrate-up.sh
# 依赖 golang-migrate：go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
```

6. 前端构建并上传：

```bash
cd web
TARO_APP_API_BASE_URL=https://api.你的域名.com pnpm run build:weapp
```

然后用微信开发者工具打开 `web/dist`，填入真实 AppID，点「上传」→ 在 mp 后台提交审核。

### 方案 A 的两个坑

- **本地存储会丢**：`STORAGE_DRIVER=local` 时上传的图片在容器里，重建即丢失。必须把 `LOCAL_UPLOAD_DIR` 挂到宿主机 volume；量起来后建议改 `STORAGE_DRIVER=cos` 并配 COS 三件套。
- **`PUBLIC_BASE_URL` 必填**：生产环境 + local 存储时缺它会直接启动失败，它用于生成图片的受保护访问地址。

### 关于弹性扩容：方案 A 没有，这是它的取舍

需要明确预期：

- **腾讯云轻量应用服务器不支持弹性伸缩。** 它是固定套餐的包年包月 VPS，CPU/内存/带宽绑定在一台宿主机上，无法按负载自动增减实例。这正是它能打到 1.7 折、年付不到 200 元的原因。
- **Docker Compose 也不是编排系统。** `docker compose up --scale api=3` 确实能起多个副本，但要自己配 Nginx upstream 做负载均衡；而且 SQLite 多副本会写冲突，单机扩副本既没有容灾意义，也可能把内存从 600 MB 乘到 1.8 GB。
- **扩容的正确姿势是换套餐**：控制台里升级到更高规格，停机几分钟，数据盘保留。从 2核4G 升到 4核8G 是一次平滑操作，不需要重新部署。

如果业务真的会出现突发流量（比如活动推广），那说明方案 A 已经不合适了，应迁移到：

| 目标 | 特点 |
| --- | --- |
| CloudBase 云托管 | 按请求量自动扩缩容，可设最小实例数避免冷启动 |
| 腾讯云 TKE / EKS | Kubernetes HPA，真正的弹性，但运维复杂度高 |

对本项目的判断：家庭为单位的宠物小程序，用户量级是几十到几百人，负载平稳，**弹性能力的价值远低于它带来的成本**，先用固定容量是对的。

### 真正的瓶颈是带宽，不是 CPU 和内存

5M 带宽理论上限约 **640 KB/s**。宠物照片普遍 1~3 MB 一张，意味着：

- 单张原图加载约 2~5 秒，体验明显卡顿；
- 图片流量会快速吃掉月流量包（2核4G5M 套餐通常几百 GB/月）。

因此上线前应做两件事：

1. **上传前压缩**：小程序端选图后压到长边 1080px 以内再上传，体积通常能降到 200 KB 以下；
2. **列表用缩略图**：列表页加载缩略图，点击才加载原图。

这两项能直接决定用户体验，优先级高于任何服务器升配。

---

## 三、紧急上线：先跑 K3s，后迁腾讯云

适用于「域名已备案但腾讯云备案来不及」的场景。**关键前提：域名的 ICP 备案是跟着域名走的，不跟着服务器走。域名已备案 = 现在就能用。**

### 3.1 关于"腾讯云还要重新备案"的澄清

这是一笔差很大的误算：

| 常见误解 | 实际情况 |
| --- | --- |
| 买腾讯云要重新备案，又要等 20 天 | 已备案域名只需办**接入备案**，不是重新备案 |
| 必须管局审核完才能用 | 腾讯云官方文档明确：**接入备案腾讯云初审通过后域名即可解析访问、解除拦截**，不必等管局那 20 个工作日 |
| 接入备案要很久 | 腾讯云初审通常 **1~2 个工作日**，管局审核 1~20 个工作日（可并行，不阻塞访问） |

所以后续迁腾讯云的真实阻塞是 **1~2 天初审**，而不是一个月的备案流程。

> 注意：接入备案期间**不能修改原备案主体/网站信息**，主体必须与原备案一致。多个接入商可以共存，不必注销原来的备案。

### 3.2 迁移路径：域名不变，零改动的迁移

这是整个方案最重要的设计原则——**迁移时不要换域名**：

```text
现在：  域名 DNS → K3s 集群公网 IP
之后：  域名 DNS → 腾讯云轻量服务器 IP   ← 只改这一条 A 记录
```

因为小程序后台的 request 合法域名存的是**域名**而不是 IP，所以：

- 小程序后台**不需要改配置**（别忘了每月只有 5 次修改额度）；
- 前端**不需要重新构建发布**（`TARO_APP_API_BASE_URL` 没变）；
- 需要搬的只有两样东西：**数据库**和**上传的图片**。

迁移窗口只需 DNS 的 TTL 生效时间。

### 3.3 K3s 部署产物（双环境）

`dev` 与 `prod` 两套环境部署在同一个集群上，用 namespace 隔离，共用 `base/` 一份模板，
只有环境变量与入口域名不同。详细步骤见 **`deployments/k3s/README.md`**。

```text
deployments/
├── docker/
│   ├── Dockerfile          后端多阶段构建（CGO_ENABLED=0，配 PostgreSQL）
│   └── Dockerfile.migrate  打包 migrations 的迁移工具镜像
└── k3s/
    ├── base/               两环境共用模板：configmap / postgres / PVC / Job / api
    └── overlays/
        ├── dev/            → namespace pet-dev
        └── prod/           → namespace pet-prod
```

| | dev | prod |
| --- | --- | --- |
| namespace | `pet-dev` | `pet-prod` |
| `APP_ENV` | `development` | `production` |
| 数据库 | 独立 PostgreSQL 实例 | 独立 PostgreSQL 实例 |

两套环境的 Kustomize 构建均已实测通过（`kubectl kustomize overlays/dev|prod`）。

> ⚠️ **资源门槛**：每套 `request` 约 768Mi（api 512Mi + postgres 256Mi），两套合计约 **1.5Gi request / 3Gi limit**。集群可用内存建议 ≥ 4Gi，部署前先确认节点资源。

### 3.4 部署步骤

```bash
# ① 构建镜像
cd /Users/zyj/Developer/miniprogram/pet
docker build -f deployments/docker/Dockerfile -t pet-api:latest .
docker build -f deployments/docker/Dockerfile.migrate -t pet-migrate:latest .

# K3s 用 containerd，无私有仓库时本地导入
docker save pet-api:latest -o pet-api.tar
sudo k3s ctr images import pet-api.tar
# pet-migrate 同理

# ② 改域名：把 overlays/*/20-patch-config.yaml 与 70-ingress.yaml 里的
#    example.com 换成真实域名（两个域名都提前加进小程序白名单，别浪费当月 5 次额度）

# ③ 创建密钥：两个 namespace 各一份，勿提交 Git
#    完整命令见 deployments/k3s/README.md 第三节

# ④ 应用两套环境
cd deployments/k3s
kubectl apply -k overlays/dev
kubectl apply -k overlays/prod

# ⑤ 若 Job 因数据库未就绪失败，删掉重跑
kubectl -n pet-dev delete job pet-migrate && kubectl apply -k overlays/dev
kubectl -n pet-prod delete job pet-migrate && kubectl apply -k overlays/prod

# ⑥ 确认 54 个 migration 全部成功
kubectl -n pet-dev logs job/pet-migrate
kubectl -n pet-prod logs job/pet-migrate

# ⑦ 验证
kubectl -n pet-dev get pods,svc,ingress
kubectl -n pet-dev port-forward svc/pet-api 8081:8080
curl -i http://127.0.0.1:8081/healthz
```

### 3.5 三个容易踩的坑

- **健康检查端点是 `/healthz`**，不带 `/api/v1` 前缀（`internal/httpapi/router.go:45`）。写错会导致探针失败、Pod 永远起不来。
- **容器内存不能低于 512Mi**。实测单进程常驻 550~600 MB（两份 gse 分词词典），设 256Mi 会直接 OOM 重启循环。
- **必须设 `TZ=Asia/Shanghai`**。日历提醒按本地整点触发，容器默认 UTC 会让提醒错 8 小时。

另外，`local-path` 是 K3s 默认的 StorageClass，一般开箱可用；若你的集群用了别的存储方案，改 `storageClassName` 即可。

### 3.6 双环境：开发 + 生产怎么配

**先澄清概念：体验版不是一个"环境"，它是一个"发布渠道"。**

微信小程序的三个阶段：

| 版本 | 谁能用 | 怎么产生 | 是否校验域名白名单 |
| --- | --- | --- | --- |
| 开发版 | 开发者 + 体验成员，扫码可见 | 开发者工具点「预览」 | **校验** |
| 体验版 | 后台配置的体验成员 | 开发者工具点「上传」→ 后台设为体验版 | **校验** |
| 正式版 | 所有微信用户 | 提交审核通过后发布 | 校验 |

⚠️ **最大的坑**：开发者工具里那个「不校验合法域名」开关**只在电脑上的模拟器生效**。真机预览、体验版、正式版一律强制校验。所以**没有任何办法绕过域名要求**——这也是为什么备案躲不掉。

域名白名单的硬约束：只认域名不认 IP、**每类最多 20 个**、**每月只能修改 5 次**。因此两个环境的域名要**一次性配齐**。

推荐做法（环境隔离靠后端，不靠小程序版本）：

| | 开发环境 | 生产环境 |
| --- | --- | --- |
| 域名 | `https://dev-api.你的域名.com` | `https://api.你的域名.com` |
| 后端 | K3s 集群 | 腾讯云轻量 |
| 数据库 | K3s 内 PostgreSQL | 腾讯云服务器上的 PostgreSQL |
| 后端 `APP_ENV` | `development` | `production` |
| 面向版本 | 体验版 / 真机调试 | 正式版 |

前端靠 `TARO_APP_API_BASE_URL` 区分（`web/config/index.ts:10`，环境变量优先级高于 `NODE_ENV` 的默认值）：

```bash
# 开发/体验包（指向 K3s）
TARO_APP_API_BASE_URL=https://dev-api.你的域名.com pnpm run build:weapp

# 生产包（指向腾讯云）
NODE_ENV=production TARO_APP_API_BASE_URL=https://api.你的域名.com pnpm run build:weapp
```

两条不能省的注意点：

- **两个后端必须用不同的数据库**。同一个 AppID 下，用户体验版和正式版拿到的是**同一个 openid**；若两个环境共用一个库，测试数据会污染生产数据。
- 构建产物都输出到 `web/dist`，**打完包立刻上传**，别把开发包误传成正式版。稳妥做法是每次构建前先备份 `dist` 目录。

---

## 四、接入免费混元额度

### 3.1 额度政策要点

微信「AI 小程序成长计划」（2026-01-01 至 2026-12-31）：

- 2026-07 加码后：**10 亿 Token 文生文 + 10 万张文生图**，模型为混元 Hy3 系列；
- 资源包自申请成功起 **6 个月有效**；
- 报名入口：微信公众平台 → 行业能力 → AI 小程序成长计划（本轮已全行业开放）；
- 没有云开发环境的可免费获得 6 个月个人版环境，已有环境可领 120 元代金券。

### 3.2 开通四步

1. 访问 tcb.cloud.tencent.com，注册并创建**个人版**环境（免费 6 个月），记下**环境 ID**；
2. 同步报名「AI 小程序成长计划」，Token 额度自动到账；
3. CloudBase 控制台 → **AI → 生文模型**，勾选 Hy3 preview（或其他所需混元模型）；
4. CloudBase 控制台 → **环境设置 → API Key**，创建并复制 API Key。

得到两个关键值：

```text
BASE_URL = https://<环境ID>.api.tcloudbasegateway.com/v1/ai/cloudbase
API_KEY  = 控制台复制的 API Key
```

### 3.3 ⚠️ 必须先解决：协议不匹配

这是本项目接入混元最大的障碍，务必先看懂：

| | 本项目生产代码 | CloudBase 混元网关 |
| --- | --- | --- |
| 接口 | **Responses API**（`/responses`） | **Chat Completions**（`/chat/completions`）、Anthropic Messages |
| 事件流 | `response.output_text.delta` 等 | `choices[0].delta.content` |
| 结构化输出 | `text.format.json_schema` + `strict` | `response_format.json_schema`（strict 支持情况待实测） |

项目代码位置：

- `internal/platform/ai/openai_provider.go` —— 适配器，内部调用 `client.Responses.New(...)`；
- `cmd/api/main.go:153~211` —— Provider 构造与 Profile 注入，`AdapterVersion` 硬编码为 `openai-responses-v1`；
- `internal/pkg/config/config.go:187` —— `AI_PROVIDER` 目前只接受 `openai`。

**结论：直接把 CloudBase 的 Base URL 填进 `AI_BASE_URL` 是跑不通的**，请求会打到 `.../v1/ai/cloudbase/responses` 而网关没有这个路由。

### 3.4 先实测，再改代码

动手改适配器之前，先跑这三条命令确认混元的能力边界（把 `<ENV_ID>`、`<API_KEY>` 换成真实值）：

```bash
# ① Chat Completions 是否通、模型名是否正确
curl -s "https://<ENV_ID>.api.tcloudbasegateway.com/v1/ai/cloudbase/chat/completions" \
  -H "Authorization: Bearer <API_KEY>" \
  -H "Content-Type: application/json" \
  -d '{"model":"hy3-preview","messages":[{"role":"user","content":"用一句话介绍你自己"}]}'

# ② 确认网关确实没有 Responses 路由（预期返回 404）
curl -s -o /dev/null -w 'responses_status=%{http_code}\n' \
  "https://<ENV_ID>.api.tcloudbasegateway.com/v1/ai/cloudbase/responses" \
  -H "Authorization: Bearer <API_KEY>" \
  -H "Content-Type: application/json" \
  -d '{"model":"hy3-preview","input":"你好"}'

# ③ 关键：strict JSON Schema 是否支持（决定 record_array_v1 契约能否保住）
curl -s "https://<ENV_ID>.api.tcloudbasegateway.com/v1/ai/cloudbase/chat/completions" \
  -H "Authorization: Bearer <API_KEY>" \
  -H "Content-Type: application/json" \
  -d '{"model":"hy3-preview","messages":[{"role":"user","content":"输出一只猫的健康记录"}],"response_format":{"type":"json_schema","json_schema":{"name":"pet_ask","strict":true,"schema":{"type":"object","properties":{"records":{"type":"array","items":{"type":"object","properties":{"type":{"type":"string"}},"required":["type"],"additionalProperties":false}}},"required":["records"],"additionalProperties":false}}}}'
```

判读标准：

- ① 返回正常 JSON → 网关可用，模型名确认；
- ② 返回 404 → 确认必须走适配器改造路线；
- ③ 若返回 400 提示不支持 `strict` 或 `response_format` → **混元撑不住你的强结构化契约**，需要退到「非 strict + 业务侧容错解析」，或改用其他支持 Responses API 的网关。

### 3.5 适配器改造方案（实测 ③ 通过后再动手）

| 步骤 | 文件 | 动作 |
| --- | --- | --- |
| 1 | `internal/platform/ai/chat_completions_provider.go` | 新增，实现 `Provider` 接口的 `Complete` / `Stream`，内部用 `client.Chat.Completions.New` 与 `NewStreaming`；把 `ResponseSchema` 映射到 `response_format.json_schema`；SSE 事件从 `response.output_text.delta` 改为 `choices[].delta.content` |
| 2 | `internal/platform/ai/provider_config.go` | 保持不变，`OpenAIConfig` 可直接复用 |
| 3 | `internal/pkg/config/config.go` | `Validate` 中放开 `AI_PROVIDER`，允许 `openai`（Responses）与 `openai_chat`；补充对应校验分支 |
| 4 | `cmd/api/main.go` | 按 `cfg.AIProvider` 分支构造 Provider；`Profile.AdapterVersion` 相应改为 `openai-chat-v1`；`Capabilities` 按实测结果设置（尤其中 `StructuredOutput`、`ImageInput`） |
| 5 | 验证 | 跑 `go run ./cmd/t3provider` 与 `node scripts/strict_provider_verify.mjs` 两套探针 |

改造原则：保留现有 Responses 实现不动，新增一个可切换的适配器，`AI_PROVIDER` 决定走哪条路，随时可回滚。

### 3.6 一个必须确认的风险

早期活动条款写明资源包「仅限微信小程序和云开发控制台中调用」。CloudBase 文档未限制调用来源，通过 API Key 走网关理论上任何服务端都能调并扣额度，但**这点没有官方明确保证**。

上线前请验证：从你的服务器调用一次后，去 CloudBase 控制台 → AI 用量看是否扣减。若发现自建服务器调用不消耗资源包而是产生账单，应立即停止并改用云函数中转。

---

## 五、上线验收清单

- [ ] `APP_ENV=production` 下服务能启动（配置校验通过，无占位符）
- [ ] 54 个 migration 全部执行成功
- [ ] HTTPS 域名可访问，证书有效
- [ ] 小程序后台服务器域名已配置并生效
- [ ] 微信登录（`code2session`）链路打通
- [ ] 上传的图片能持久化保存并可访问
- [ ] AI 问答跑通：非流式与流式两条通道都验证过
- [ ] CloudBase 控制台能看到 Token 用量，且用量随你的调用增长（说明免费额度确实在抵扣）
- [ ] `.env` 与密钥未进入 Git
