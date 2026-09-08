# 宠物问问 Agent 开发记录

本文记录“宠物问问”后端 Agent 的设计、实现步骤和验证结果。每完成一个开发步骤，都在本文追加对应的总结，作为当前实现状态和后续工作的依据。

## 一、总体定位

宠物问问不是宠物疾病诊断工具，而是基于宠物长期档案的健康观察与风险提示助手。

核心闭环：

```text
宠物档案 → 宠物问问 → 健康记录/日历 → 下一次宠物问问
```

Agent 的职责是：

- 读取当前宠物的可信档案和近期健康记录。
- 结合用户文字和图片进行信息整理。
- 先通过确定性规则识别高风险情况。
- 在信息不足时进行有限追问。
- 输出结构化的观察结论、行动建议和就医升级条件。
- 将问问过程沉淀为可追踪的健康记录。

Agent 不负责：

- 疾病确诊。
- 开具处方或指导用药剂量。
- 替代兽医进行专业影像诊断。
- 未经用户确认修改宠物正式健康档案。

## 二、架构原则

后端 Agent 采用受规则约束的工作流编排器，而不是“用户输入直接转发给大模型”。

```text
请求进入
→ 权限校验
→ 宠物上下文组装
→ 确定性风险规则
→ AI 信息提取和分析
→ 结构化输出校验
→ 保存问问记录和事件
→ 可选创建观察提醒
```

主要分层：

```text
internal/httpapi/ask       HTTP 接口和参数处理
internal/app/ask           Agent 业务、状态机和工作流
internal/platform/ai       AI Provider 适配
internal/platform/knowledge 审核后的宠物知识库
```

重要不变量：

- 数据库是会话、执行状态和事件的唯一事实源。
- 模型不能降低确定性风险规则已经确定的风险等级。
- 模型输出必须经过结构化 Schema 和安全表达校验。
- 所有执行都要记录规则版本、Prompt 版本和模型版本。
- Run 终态不能被迟到结果覆盖。
- 所有家庭数据必须通过 `family_id` 隔离。

## 三、第一步：建立 Agent 核心领域模型

### 实现内容

新增目录：

```text
internal/app/ask
```

新增文件：

- `model.go`
- `state.go`
- `state_test.go`

建立三层运行对象：

```text
Session
  └── Turn
        └── Run
```

- `Session`：一次完整的宠物问问会话。
- `Turn`：用户的一轮输入或回答追问。
- `Run`：这一轮实际执行规则和 AI 的过程。

### 状态定义

会话状态：

```text
active
completed
escalated
canceled
```

Run 状态：

```text
queued
running
waiting_input
completed
escalated
failed
canceled
interrupted
```

风险等级：

```text
unknown
green
yellow
red
```

### 状态转移规则

```text
queued → running
queued → canceled

running → waiting_input
running → completed
running → escalated
running → failed
running → canceled
running → interrupted
```

已完成状态不能重新进入运行状态。`waiting_input` 也不会直接变成 `completed`；用户回答追问时，后续会创建新的 Turn 和 Run。

### 验证结果

测试覆盖：

- 合法和非法 Run 状态转移。
- Session 状态转移。
- `nil` 对象保护。
- 状态变更结果。

验证命令：

```bash
go test ./internal/app/ask
```

结果：通过。

## 四、第二步：建立 Agent 持久化基础

### 实现内容

新增迁移：

- `migrations/000010_init_ask.up.sql`
- `migrations/000010_init_ask.down.sql`

新增仓储实现：

- `internal/app/ask/repository.go`
- `internal/app/ask/repository_test.go`

扩展领域模型：

- 在 `internal/app/ask/model.go` 中增加 `Event`。

### 数据表

```text
ask_sessions
ask_turns
ask_runs
ask_events
ask_messages
```

职责：

- `ask_sessions`：保存一次完整问问会话。
- `ask_turns`：保存用户每一轮输入。
- `ask_runs`：保存每一轮 Agent 执行。
- `ask_events`：保存状态、风险判断、模型结果等审计事件。
- `ask_messages`：保存用户、助手和追问文本。

### 仓储能力

当前 Repository 已支持：

- 原子创建首轮 `Session + Turn + Run`。
- 原子创建后续 `Turn + Run`。
- 按家庭读取问问会话。
- 使用旧状态条件更新 Run。
- 追加带序号的事件。
- 按序读取 Run 事件。

### 可靠性设计

首轮创建使用事务：

```text
Session + Turn + Run
```

三者必须全部成功，否则整体回滚。

Run 状态更新使用旧状态条件：

```sql
UPDATE ask_runs
SET status = ?
WHERE id = ? AND status = ?
```

并发或迟到更新会返回 `ErrRunStateConflict`，不能覆盖已经推进的状态。

事件序列通过以下唯一约束防止重复：

```text
(run_id, sequence)
```

重复事件会返回 `ErrEventConflict`。

### 验证结果

验证过：

- SQLite 迁移从 `000001` 到 `000010` 全部执行成功。
- 会话、轮次、Run 和事件的写入与读取。
- Run 并发状态冲突。
- 事件序列冲突。
- 事务失败后的完整回滚。

验证命令：

```bash
GOCACHE=/tmp/pet-go-build go test ./...
```

结果：全部通过。

## 五、当前状态

已经完成：

- Agent 领域状态模型。
- Session、Turn、Run 关系。
- Run 和 Session 状态机。
- Agent 数据库迁移。
- SQL Repository 基础能力。
- 状态冲突和事件幂等保护。

尚未完成：

- `AskService` 业务服务。
- HTTP API。
- 家庭和宠物权限校验接入。
- 确定性风险规则。
- AI Provider。
- 宠物上下文编译。
- 结构化输出校验。
- 健康记录和日历提醒闭环。

## 六、第三步：实现 AskService 和确定性执行器

### 实现内容

新增文件：

- `internal/app/ask/service.go`
- `internal/app/ask/executor.go`
- `internal/app/ask/service_test.go`

扩展文件：

- `internal/app/ask/repository.go`
- `internal/app/ask/repository_test.go`

### 关键设计

`AskService` 当前负责：

- 校验家庭、用户、宠物 ID 和问题内容。
- 通过宠物 Repository 确认宠物属于当前家庭。
- 生成 Session、Turn 和 Run 标识。
- 使用事务创建首轮 `Session + Turn + Run + run.queued` 事件。
- 处理排队中的 Run。
- 写入 `run.started` 事件。
- 调用 Executor 获取确定性决策。
- 将 Run 推进到 `waiting_input`、`completed`、`escalated` 或 `failed`。
- 将状态、风险等级和事件原子写入数据库。
- 已经处理过的 Run 再次处理时直接返回持久化结果，不重复调用 Executor。

当前提供 `DeterministicExecutor`，它不调用外部模型，只返回一个待补充信息的追问：

```text
请补充宠物目前最明显的一个症状，以及症状从什么时候开始。
```

这个实现用于先验证 Agent 生命周期，不代表最终的宠物健康分析逻辑。

### 仓储可靠性增强

`TransitionRun` 现在在同一个事务中完成：

```text
Run 状态更新
→ Turn 状态同步
→ Session 状态和风险等级同步
→ 终态事件写入
```

这样不会出现 Run 已完成但事件没有落库的半成功状态。

Repository 还新增了按 Session 隔离读取 Run 和 Turn 的能力，避免只凭 Run ID 跨会话读取数据。

### 测试覆盖

- 创建首轮会话和 Run。
- 宠物不存在和输入为空的参数校验。
- `queued → running → waiting_input` 生命周期。
- 红色风险 Run 推进到 `escalated` 和 Session `escalated`。
- Executor 失败时进入 `failed`。
- 重复处理已完成或等待输入的 Run 不重复执行。
- Session、Turn、Run、Event 的事务状态同步。

### 验证结果

```bash
GOCACHE=/tmp/pet-go-build go test ./internal/app/ask
GOCACHE=/tmp/pet-go-build go test ./...
```

结果：问问包测试和全量 Go 测试全部通过。

### 当前限制

- 还没有 HTTP API，外部请求暂时无法调用 `AskService`。
- `DeterministicExecutor` 只用于生命周期验证，不提供真实分析。
- 当前还未实现追问 Turn 的创建接口。
- 还未接入确定性风险规则和图片资源。
- 当前使用随机 ID，尚未加入幂等键。

## 七、第四步：接入问问 HTTP API

### 实现内容

新增文件：

- `internal/app/ask/dto.go`
- `internal/httpapi/ask/handler.go`
- `internal/httpapi/ask/routes.go`
- `internal/httpapi/ask_routes_test.go`
- `docs/api/ask.md`

扩展文件：

- `internal/app/ask/service.go`
- `internal/app/ask/repository.go`
- `internal/httpapi/v1.go`
- `internal/httpapi/router.go`
- `cmd/api/main.go`

### 接口

```text
POST /api/v1/pets/:pet_id/ask/sessions
POST /api/v1/ask/sessions/:session_id/runs/:run_id/process
GET  /api/v1/ask/sessions/:session_id
GET  /api/v1/ask/sessions/:session_id/runs/:run_id/events
```

### 关键设计

- 所有接口复用 JWT 和 active 家庭成员中间件。
- 创建接口再次通过宠物 Repository 校验宠物归属当前家庭。
- 事件查询同时校验 Session 和 Run，避免只凭 ID 跨家庭读取。
- 领域模型通过 DTO 输出，JSON 字段使用项目约定的 snake_case。
- `after` 参数支持事件游标，便于后续轮询或实时事件补发。
- API 层不直接操作数据库，只调用 `AskService`。
- `cmd/api/main.go` 统一组装问问 Repository、Service 和确定性 Executor。

### 当前执行模式

当前 API 是同步触发模式：

```text
创建会话 → 返回 queued
→ 调用 process
→ 返回 waiting_input / completed / escalated / failed
```

后续替换为异步 Worker 时，API 路径可以保持不变，前端只需轮询会话或事件接口。

### 测试覆盖

- 未携带 JWT 时返回未认证。
- 当前家庭成员可以为本家庭宠物创建问问会话。
- 创建结果包含 Session、Run 和 `run.queued` 事件。
- Process 接口返回确定性追问和 `waiting_input` 状态。
- `after=1` 只返回后续事件。
- 不存在的 Session 返回 404。

### 验证结果

```bash
GOCACHE=/tmp/pet-go-build go test ./internal/httpapi
GOCACHE=/tmp/pet-go-build go test ./...
```

结果：问问服务、HTTP 路由和全量 Go 测试全部通过。

### 当前限制

- 当前 Executor 仍是确定性本地实现。
- 暂不支持创建追问后的新 Turn。
- 暂不支持图片上传、图片质量检测和多模态输入。
- 暂不包含确定性红黄绿风险规则。
- 暂未加入 Idempotency-Key。

## 八、第五步：实现确定性风险规则层

### 实现内容

新增文件：

- `internal/app/ask/rules.go`
- `internal/app/ask/rules_test.go`

扩展文件：

- `internal/app/ask/service.go`
- `internal/app/ask/service_test.go`

### 关键设计

当前规则版本为：

```text
ask-rules-v1
```

首版只处理需要立即就医的红色风险，覆盖：

```text
呼吸困难、呼吸急促、张口呼吸
持续或反复抽搐
大量出血、止不住血
疑似中毒、误食药物或清洁剂
意识不清、失去意识、叫不醒
无法站立、突然瘫倒、四肢无力
```

规则输出包含：

- `risk_level`：当前只会输出 `red` 或 `unknown`。
- `trigger_code`：稳定的规则命中码。
- `message`：只描述观察到的风险，不输出疾病诊断。
- `action`：固定的低风险行动建议和立即就医建议。

规则处理了常见的否定和假设表达：

```text
没有呼吸困难       → 不命中
如果呼吸困难怎么办 → 不命中
如何判断呼吸困难   → 不命中
之前没有，但现在呼吸困难 → 命中
```

### Agent 流程变化

Run 进入 `running` 后，先执行规则层：

```text
原始 Turn 文本
→ DeterministicRuleEngine
→ 命中 red：直接 escalated，跳过 Executor
→ 未命中：继续执行 Executor
```

因此真实模型未来即使返回较低风险，也不能覆盖规则层已经确认的红色风险。

红色结果会写入：

```text
risk.escalated
```

事件数据包含 `trigger_code`、`message` 和 `action`，方便审计和后续生成就医摘要。

### 测试覆盖

- 六类红色风险的命中。
- 否定表达不误触发。
- 假设提问不误触发。
- 否定后再次出现实际症状可以命中。
- 空输入返回 `unknown`。
- 规则命中时 Executor 不执行。
- 命中后 Run 进入 `escalated`，Session 进入 `escalated`。

### 验证结果

```bash
GOCACHE=/tmp/pet-go-build go test ./internal/app/ask
GOCACHE=/tmp/pet-go-build go test ./...
```

结果：问问包和全量 Go 测试全部通过。

### 当前限制

- 当前只有红色规则，尚未实现黄色和绿色规则。
- 规则基于中文文本关键词，不处理图片内容。
- 否定识别是有限窗口规则，不能替代语言模型语义理解。
- 规则配置暂时写在 Go 代码中，后续需要审核和版本化配置管理。
- 结构化分析结果和模型输出安全校验尚未实现。

## 九、下一步计划

第六步实现问问追问 Turn：用户回答 `waiting_input` 后创建新的 Turn 和 Run，继承原 Session，最多允许三轮追问，并保证原有红色风险规则每轮都重新执行。

## 十、后续步骤记录模板

后续每次开发按以下结构追加：

```text
## 第 N 步：步骤名称

### 实现内容

### 关键设计

### 修改文件

### 验证结果

### 当前限制

### 下一步计划
```

## 第六步：实现追问 Turn

### 实现内容

完成用户回答追问后的新 Turn/Run 创建流程：

- 新增 `POST /api/v1/ask/sessions/:session_id/runs/:run_id/reply` 接口。
- 仅允许 `waiting_input` 状态的原 Run 接收回答。
- 新 Turn 使用当前 `Session.TurnCount` 作为 `turn_index`，新 Run 从 `run_index = 0` 开始。
- 会话最多允许三轮 Turn，达到上限后拒绝继续追问。
- 新回答创建后仍保持 `queued`，由现有 `/process` 接口触发执行，因此每轮都会重新经过红色风险规则。
- 回复时必须命中当前会话最新 Turn 的选中 Run，旧的 `waiting_input` Run 不能再次分叉创建轮次。

### 关键设计

回复流程保持与首轮一致的事件驱动生命周期：

```text
waiting_input Run
→ 校验 Session、Run 和轮次上限
→ 原子创建 Turn、Run、run.queued 事件
→ 更新 Session.turn_count
→ 调用 process
→ 规则层优先判断，再进入 Executor
```

数据库事务会同时写入新 Turn、新 Run、会话计数和 `run.queued` 事件；任一步失败都会回滚，避免出现孤立轮次或计数不一致。更新会话计数时同时校验原 `turn_count`，降低并发回复造成重复轮次的风险。

### 修改文件

- `internal/app/ask/repository.go`：新增 `CreateFollowUpTurnRun`，在事务内递增会话轮次。
- `internal/app/ask/service.go`：新增 `Reply`、`MaxTurns` 及状态校验。
- `internal/httpapi/ask/routes.go`：注册回复路由。
- `internal/httpapi/ask/handler.go`：解析回答并调用 Service。
- `internal/app/ask/service_test.go`：覆盖追问创建、状态校验和三轮上限。
- `internal/app/ask/repository_test.go`：覆盖事务回滚。
- `internal/httpapi/ask_routes_test.go`：覆盖回复接口成功响应。
- `docs/api/ask.md`：补充回复接口契约。

### 验证结果

已通过：

```bash
GOCACHE=/tmp/pet-go-build go test ./internal/app/ask ./internal/httpapi
GOCACHE=/tmp/pet-go-build go test ./...
gofmt -w internal/app/ask/repository.go internal/app/ask/service.go internal/httpapi/ask/routes.go internal/httpapi/ask/handler.go internal/app/ask/service_test.go internal/app/ask/repository_test.go internal/httpapi/ask_routes_test.go
git diff --check
```

测试覆盖：

- `waiting_input` Run 可以创建下一轮。
- 新轮次索引递增，`Session.TurnCount` 同步递增。
- 非等待输入 Run、已结束会话和超过三轮不能回复。
- Turn、Run、事件或会话计数写入失败时事务回滚。
- HTTP 回复接口返回新的 `queued` Run。
- 旧的等待输入 Run 不能重复回答，最新追问中的红色风险仍会升级。

### 当前限制

- 回复接口只创建新 Run，不自动执行；客户端需要继续调用 `/process`。
- 当前 Executor 仍是确定性追问实现，尚未接入真实多模态模型。
- 轮次上限固定为代码常量，尚未按场景配置。

### 下一步计划

第七步实现结构化分析结果契约和安全校验：定义统一的观察结论、可能原因、家庭建议和就医升级条件，并在 Executor 输出后进行字段校验，防止模型返回诊断、处方或不完整结果。

## 第七步：结构化分析结果与安全校验

### 实现内容

新增完成态分析结果校验。`RunCompleted` 必须提供以下字段：

- `current_assessment`：当前行动判断。
- `observations`：确定观察到的情况。
- `possible_causes`：不超过三个可能方向。
- `home_actions`：低风险家庭建议。
- `escalation_conditions`：明确的升级就医条件。

缺少字段、字段类型错误、空数组、可能方向超过三个，或包含“确诊”“诊断为”“处方”“用药剂量”等高风险表达时，Run 会被转换为 `failed`，不会向用户展示未经校验的完成结果。

### 关键设计

安全校验只约束完成态输出：追问仍使用 `assistant.question`，红色规则仍使用 `risk.escalated`，执行异常仍使用 `run.failed`。完成态风险等级必须是 `green` 或 `yellow`，不允许用 `unknown` 伪装分析结果。

校验发生在 Executor 返回之后、事件落库之前；失败时只保存固定错误信息 `当前暂时无法完成分析`，不保存模型的原始危险文本。

### 修改文件

- `internal/app/ask/output.go`：新增结构化分析结果字段和安全校验。
- `internal/app/ask/output_test.go`：覆盖合法输出、缺失字段、危险表达和非完成态。
- `internal/app/ask/service.go`：在 Executor 输出后接入校验，非法完成结果转为失败。
- `docs/ask-agent-development.md`：记录结构化输出契约和限制。

### 验证结果

已通过：

```bash
GOCACHE=/tmp/pet-go-build go test ./internal/app/ask ./internal/httpapi
GOCACHE=/tmp/pet-go-build go test ./...
gofmt -w internal/app/ask/output.go internal/app/ask/service.go
git diff --check
```

### 当前限制

- 当前没有真实 AI Executor 生成完成态结果，校验暂由测试和后续 Provider 接入使用。
- 关键词拦截不能替代专业安全审核，后续仍需知识库和二次模型校验。
- 图片质量检测、宠物档案上下文组装和健康记录沉淀尚未接入。

### 下一步计划

第八步接入宠物上下文快照：在每次 Run 执行前读取宠物基础档案和近期问问记录，形成版本化上下文，并限制模型只能使用当前家庭和当前宠物的数据。

## 第八步：接入宠物上下文快照

### 实现内容

在 Run 进入 Executor 前组装 `ContextSnapshot`，当前包含：

- 当前家庭下的宠物 ID 和名称。
- 当前问问会话最近最多 5 个 Turn 的输入、轮次索引和状态。
- 可用时补充宠物 Profile 中的品种、性别、绝育状态和生日。
- 快照版本字段和采集时间。

快照通过 `RunInput.Context` 传递给 Executor，规则层和事件状态仍由 Service 控制。宠物基础信息通过已有 `pet.Repository.Get(family_id, pet_id)` 查询，历史 Turn 通过问问 Repository 按 `session_id` 查询，避免跨家庭读取。

### 关键设计

上下文在 Run 实际从 `queued` 进入 `running` 前读取，确保 Executor 使用的是当前会话状态。历史输入按轮次升序恢复，最多保留 5 轮，避免无限增长 Prompt。

Profile 通过可选接口读取。未启用 Profile 资源或扩展表不可用时保留基础宠物信息，避免旧部署因扩展表缺失导致问问 Run 失败；未成功读取的字段不会被伪造填充。

### 修改文件

- `internal/app/ask/model.go`：新增 `PetContext`、`ContextTurn` 和 `ContextSnapshot`，增加上下文版本和采集时间。
- `internal/app/ask/repository.go`：新增 SQL Repository 的历史 Turn 查询。
- `internal/app/ask/service.go`：在执行前组装宠物和历史 Turn 快照并注入 `RunInput`。
- `internal/app/ask/service_test.go`：验证 Executor 可以收到当前宠物和历史输入。

### 验证结果

已通过：

```bash
GOCACHE=/tmp/pet-go-build go test ./internal/app/ask ./internal/httpapi
GOCACHE=/tmp/pet-go-build go test ./...
gofmt -w internal/app/ask/model.go internal/app/ask/repository.go internal/app/ask/service.go internal/app/ask/service_test.go
git diff --check
```

### 当前限制

- 过敏、长期用药等健康资源字段采用可选降级策略。
- 快照暂未包含日历事件、健康记录和历史相似症状聚合。
- 当前只限制快照查询范围，真实 AI Provider 的 Prompt 编排和敏感字段脱敏尚未实现。

### 下一步计划

第九步实现健康档案适配器和上下文版本快照：读取经过家庭隔离的宠物 Profile、健康信息和近期日历事件，生成可审计的上下文版本，并为后续“再次发生”识别提供数据基础。

## 第九步：健康档案适配器和上下文版本

### 实现内容

- 在宠物 Profile Repository 增加 `GetHealth`，读取当前家庭下宠物的健康状态、过敏和长期用药。
- `ContextSnapshot` 在可用时加入健康档案字段。
- 上下文版本升级为 `ask-context-v2`，并保留 `CapturedAt` 采集时间。
- 健康档案读取采用可选能力接口，不要求所有测试替身或旧数据库都具备扩展表。

### 关键设计

健康档案属于用户确认过的正式资料，可以作为模型上下文使用；AI 输出中的推测不会反向写入这些字段。Profile 或健康表不可用时只保留基础宠物信息，避免把空值误认为“没有过敏”或“没有长期用药”。

上下文版本用于绑定字段契约：当快照结构新增健康字段后使用 `ask-context-v2`，后续 Prompt、模型调用日志和审计记录可以据此还原当时使用的上下文格式。

### 修改文件

- `internal/app/pet/profile.go`：新增 `PetHealth` 和 `GetHealth`。
- `internal/app/ask/service.go`：读取可用的 Profile 与健康档案并写入快照，升级上下文版本。
- `internal/app/ask/service_test.go`：验证品种、绝育、过敏和长期用药进入 Executor 上下文。
- `docs/ask-agent-development.md`：记录健康档案适配和版本策略。

### 验证结果

已通过：

```bash
GOCACHE=/tmp/pet-go-build go test ./internal/app/ask ./internal/app/pet ./internal/httpapi
GOCACHE=/tmp/pet-go-build go test ./...
gofmt -w internal/app/pet/profile.go internal/app/ask/service.go internal/app/ask/service_test.go
git diff --check
```

### 当前限制

- 日历事件和健康记录尚未接入快照，当前只读取宠物 Profile、健康资源和问问历史 Turn。
- 读取扩展档案失败时会降级且不记录具体缺失原因，后续需要增加上下文来源清单和审计事件。
- 健康字段当前仍以原始文本传递，尚未做字段级脱敏和 Prompt 编排。

### 下一步计划

第十步为 Calendar 增加按家庭、宠物和时间范围读取近期记录的接口，并将经过筛选的记录加入上下文，支持“近期就医”“重复呕吐”等纵向观察；同时记录每个上下文来源的版本和截断原因。

## 第十步：接入近期日历记录

### 实现内容

- Calendar Repository 新增按家庭、宠物、起始时间和数量读取近期记录的接口。
- 查询只返回未删除记录，并按发生时间倒序排列。
- Ask Service 增加可选 Calendar 上下文源，生产启动时注入 Calendar Repository。
- 快照新增 `RecentRecords`，当前默认读取最近 90 天、最多 5 条记录。
- 快照新增 `Sources` 和 `Events`，上下文版本升级为 `ask-context-v4`。

### 关键设计

日历查询同时约束 `family_id` 和 `pet_id`，避免把同一家庭其他宠物的记录带入当前 Agent。记录只保留问问需要的摘要字段，不加载媒体、提醒和创建人等无关信息，降低 Prompt 体积和隐私暴露面。

每个来源记录来源名、版本、条数和是否达到截断上限；问问文本按有限关键词归一化为稳定标签，日历记录归一化为 `medical_record` 或 `daily_record`，原始摘要最多保留 200 个字符。

Calendar 是可选上下文源：未注入或查询失败时，仍使用宠物档案、健康信息和问问历史完成执行，不阻断核心问问流程。

### 修改文件

- `internal/app/calendar/dto.go`：新增 `ContextRecord`。
- `internal/app/calendar/repository.go`：新增近期记录查询。
- `internal/app/calendar/repository_test.go`：验证家庭、宠物、时间范围、删除状态和数量限制。
- `internal/app/ask/model.go`：为快照增加近期日历记录。
- `internal/app/ask/service.go`：增加可选 Calendar Repository 和 90 天记录读取。
- `internal/app/ask/service_test.go`：验证日历记录进入 Executor 上下文。
- `internal/app/ask/context.go`：新增来源事件归一化和摘要截断。
- `internal/app/ask/context_test.go`：验证症状标签和摘要长度。
- `cmd/api/main.go`：启动时注入 Calendar Repository。
- `docs/ask-agent-development.md`：记录日历上下文边界。

### 验证结果

已通过：

```bash
GOCACHE=/tmp/pet-go-build go test ./internal/app/ask ./internal/app/calendar ./internal/httpapi ./cmd/api
GOCACHE=/tmp/pet-go-build go test ./...
gofmt -w internal/app/calendar/dto.go internal/app/calendar/repository.go internal/app/ask/model.go internal/app/ask/service.go internal/app/ask/service_test.go cmd/api/main.go
git diff --check
```

### 当前限制

- 当前只传递日历记录摘要，尚未做相似症状分类和重复事件聚合。
- 上下文源失败时只做静默降级，尚未写入来源缺失事件。
- 日历记录内容仍是原始文本，尚未做脱敏、长度截断和模型 Prompt 模板化。

### 下一步计划

第十一步实现上下文来源审计和摘要归一化：对问问历史、健康档案、日历记录分别记录来源、版本、数量和截断原因，并为相似症状识别提供稳定的事件标签。

## 第十一步：上下文来源审计和事件归一化

### 实现内容

- `ContextSnapshot` 新增 `Sources`，记录来源名、来源版本、条数和是否达到截断上限。
- `ContextSnapshot` 新增 `Events`，统一承载问问历史和日历事件标签。
- 问问文本支持有限症状标签：`symptom_vomiting`、`symptom_diarrhea`、`symptom_cough`、`symptom_sneezing`、`symptom_low_energy`、`symptom_appetite`、`symptom_skin`。
- 日历记录归一化为 `medical_record` 或 `daily_record`。
- 事件摘要统一截断为最多 200 个字符，避免原始长文本无限进入 Prompt。
- 上下文版本升级为 `ask-context-v4`。

### 关键设计

来源审计只描述上下文实际提供给 Executor 的数据，不把缺失的可选来源标记为“无记录”。达到当前数量上限时标记 `truncated`，让后续 Prompt 或审计层知道快照可能不是完整历史。

事件标签只是检索和关联线索，不是疾病判断。否定语义、严重程度和风险等级仍由前面的确定性规则及后续分析流程处理，标签归一化不会改变红色风险决策。

### 修改文件

- `internal/app/ask/model.go`：新增 `ContextSource`、`ContextEvent` 和 Turn 创建时间。
- `internal/app/ask/repository.go`：查询历史 Turn 的创建时间。
- `internal/app/ask/context.go`：实现症状标签、日历标签和摘要截断。
- `internal/app/ask/service.go`：生成来源审计和归一化事件，升级上下文版本。
- `internal/app/ask/context_test.go`：验证标签和摘要长度。
- `internal/app/ask/service_test.go`：验证来源清单和事件进入 Executor 上下文。

### 验证结果

已通过：

```bash
GOCACHE=/tmp/pet-go-build go test ./internal/app/ask ./internal/app/calendar ./internal/httpapi
GOCACHE=/tmp/pet-go-build go test ./...
gofmt -w internal/app/ask/model.go internal/app/ask/repository.go internal/app/ask/service.go internal/app/ask/context.go internal/app/ask/context_test.go internal/app/ask/service_test.go
git diff --check
```

### 当前限制

- 标签仍是有限关键词规则，不能识别复杂同义表达或上下文否定。
- `truncated` 由达到查询上限推断，尚未通过额外计数查询确认是否真的还有更多记录。
- 来源审计目前只随内存快照传给 Executor，尚未单独落库为审计事件。

### 下一步计划

第十二步实现上下文来源审计落库和相似事件查询：保存每次 Run 使用的上下文摘要元数据，并按宠物、标签和时间范围查找历史相似事件，为“近两个月第 3 次呕吐”等提示提供可解释依据。

## 第十三步：支持自然语言定位多只宠物

### 实现内容

- 新增 `ResolvePets`，按家庭宠物列表解析问题中出现的一个或多个宠物名称。
- 新增 `POST /api/v1/ask/sessions`，请求体使用 `{ "input": "旺仔和球球上次洗澡分别是什么时候" }`，不再要求前端先选择宠物。
- 新增 `ask_session_pets` 关联表，保存会话涉及的宠物、原始提及名称和输入顺序。
- `Session` 保留原有 `pet_id` 作为兼容字段，同时增加 `pets` 列表返回完整绑定结果。
- 会话处理和详情读取时重新加载多宠物关联，确保创建、执行、查询返回一致。

### 解析和安全边界

解析只在当前家庭的宠物列表内进行，不会把其他家庭或未提及的宠物带入会话。解析状态分为：

```text
none       未识别到宠物名称，要求补充名称
resolved   一个或多个名称唯一匹配，创建会话
ambiguous  名称对应多个同名宠物，要求用户消歧
```

名称包含关系按较长名称优先，例如“球球”不会误匹配名为“球”的宠物；当用户明确同时提及“球”和“球球”时，两者都会保留。

### 修改文件

- `internal/app/ask/pet_resolver.go`：多宠物名称解析和包含关系处理。
- `internal/app/ask/pet_resolver_test.go`：覆盖多名称、重名、缺失名称和名称包含边界。
- `internal/app/ask/model.go`：新增 `SessionPet` 和会话宠物集合。
- `internal/app/ask/repository.go`：新增多宠物会话原子写入及关联读取。
- `internal/app/ask/service.go`：新增自然语言创建会话入口，并在处理链路加载会话宠物。
- `internal/app/ask/dto.go`：会话响应新增 `pets`。
- `internal/httpapi/ask/handler.go`、`internal/httpapi/ask/routes.go`：新增自然语言创建接口。
- `migrations/000011_ask_session_pets.up.sql`、`migrations/000011_ask_session_pets.down.sql`：新增关联表迁移。

### 当前限制

- 当前只完成“识别并绑定多只宠物”，尚未实现按每只宠物分别查询洗澡、疫苗等事实并生成逐项结果。
- 同名宠物会返回 409，当前错误响应还未携带候选宠物数组，前端可先提示用户补充更具体名称。
- 旧的单宠物接口继续可用；生产环境需要执行 `000011` 迁移后才能持久化多宠物关联。

### 验证结果

已通过：

```bash
GOCACHE=/tmp/pet-go-build go test ./...
git diff --check
```

### 下一步计划

第十四步实现多宠物上下文和确定性事实查询：按会话宠物集合逐个加载档案、日历记录，并为“分别是什么时候”返回结构化 `items` 数组，避免让模型自由拼接宠物与答案的对应关系。

## 第十四步：多宠物确定性事实查询

### 实现内容

- 新增事实意图识别，当前支持洗澡、疫苗、驱虫、体检、就医和用药六类历史事实。
- 在红色风险规则之后增加事实查询分支；命中事实意图且 Calendar Repository 可用时，不调用 Agent Executor。
- 根据会话中的 `Session.Pets` 顺序逐只查询最近一条记录，保证“旺仔和球球分别是什么时候”中的宠物与结果一一对应。
- 查询结果统一生成 `fact.completed` 事件，数据结构包含 `fact_type` 和 `items`。
- `items` 使用稳定的 snake_case 字段：`pet_id`、`pet_name`、`found`、`occurred_at`、`content`。
- 未找到某只宠物的记录时保留该宠物位置并返回 `found: false`，不会改写为推测日期。
- Calendar 查询同时限制 `family_id`、`pet_id`，忽略软删除记录，并按发生时间倒序、记录 ID 倒序取最近一条。
- 洗澡事实匹配日历文本中的“洗澡”或“洗浴”；疫苗、驱虫、体检、就医和用药匹配标准 `medical_type`。

### 处理优先级

```text
红色风险规则
→ 确定性事实意图识别
→ 按会话宠物逐只读取 Calendar
→ fact.completed
→ 非事实问题或事实数据源不可用时回退 Agent Executor
```

事实分支只负责可审计的历史记录读取，不负责疾病判断，也不展示疾病概率。红色风险始终优先，包含呼吸困难、持续抽搐等紧急表达的问题不会被“洗澡/疫苗”等关键词改写为普通事实查询。

### 输出示例

```json
{
  "fact_type": "bath",
  "items": [
    {
      "pet_id": "pet-1",
      "pet_name": "旺仔",
      "found": true,
      "occurred_at": "2026-08-20T10:00:00Z",
      "content": "旺仔洗澡"
    },
    {
      "pet_id": "pet-2",
      "pet_name": "球球",
      "found": false,
      "occurred_at": "",
      "content": ""
    }
  ]
}
```

### 修改文件

- `internal/app/ask/fact.go`：事实类型识别和逐宠物结果组装。
- `internal/app/ask/fact_test.go`：覆盖事实关键词、未知问题、输入顺序、无记录和查询错误。
- `internal/app/ask/model.go`：新增 `FactType` 枚举。
- `internal/app/ask/output.go`：允许并校验事实完成态输出。
- `internal/app/ask/service.go`：接入事实分支和 Executor 回退逻辑。
- `internal/app/ask/service_test.go`：验证多宠物事实查询完成态、风险等级和 Executor 未执行。
- `internal/app/calendar/dto.go`：新增 `FactRecord`。
- `internal/app/calendar/repository.go`：新增按家庭、宠物和事实类型读取最近记录的接口。
- `internal/app/calendar/repository_test.go`：验证家庭/宠物隔离、软删除、最近记录排序和事实类型匹配。

### 当前限制

- 事实类型仍使用有限关键词规则，暂不处理复杂同义句、否定句和跨轮次省略主语。
- 洗澡记录依赖日历 `content` 文本中的“洗澡”或“洗浴”，历史数据若未按统一措辞记录可能无法命中。
- 医疗事实依赖标准 `medical_type`，自定义医疗类型暂不参与事实分类。
- 同名宠物仍需先由上一步的消歧流程解决；事实查询不会自行猜测宠物身份。
- “体检”识别已收窄为体检、体查、健康检查、做检查等表达，普通症状中的“检查”不会直接进入事实分支。

### 验证结果

已通过：

```bash
gofmt -w internal/app/ask/fact.go internal/app/ask/fact_test.go internal/app/ask/service_test.go internal/app/calendar/repository_test.go
GOCACHE=/tmp/pet-go-build go test ./internal/app/ask ./internal/app/calendar
GOCACHE=/tmp/pet-go-build go test ./...
git diff --check
```

### 下一步计划

第十五步建议实现事实查询结果的 HTTP 契约测试和前端消费协议：验证创建会话、执行 Run、事件轮询三个接口返回的 `pets`、`fact.completed` 和 snake_case 字段，并定义无记录、数据源降级和同名消歧的前端展示状态。

## 第十五步：事实查询 HTTP 契约和前端消费协议

### 实现内容

- 新增问问 HTTP 生命周期契约测试，覆盖自然语言创建会话、处理 Run 和事件轮询三个接口。
- 创建接口固定使用：`POST /api/v1/ask/sessions`，请求体为 `{ "input": "..." }`。
- 创建成功后从响应的 `data.session.id` 和 `data.run.id` 读取会话与执行标识，不依赖 ID 格式。
- 处理接口固定使用：`POST /api/v1/ask/sessions/{session_id}/runs/{run_id}/process`。
- 事件接口固定使用：`GET /api/v1/ask/sessions/{session_id}/runs/{run_id}/events?after={sequence}`，`after` 为已消费的最大序号。
- `fact.completed` 事件的 `data` 直接是结构化 JSON，前端可读取 `fact_type` 和 `items`，无需解析模型自然语言。
- `items` 中每个宠物都保留一项；`found=false` 表示当前没有匹配记录，前端应展示“暂无记录”，不能展示为空日期或自行推测。
- 会话响应中的 `data.session.pets` 是宠物绑定和展示顺序的来源，`items` 顺序与其一致。
- 同名宠物消歧仍由创建接口返回 409；无宠物名称时返回 400；前端应保留原输入并提示补充名称。

### 前端消费状态

```text
创建中
→ queued
→ 调用 process
→ completed / waiting_input / escalated / failed
→ 通过 events?after=... 增量消费事件
```

事实查询建议按事件类型渲染：

- `fact.completed`：按 `items` 顺序展示宠物名、是否找到记录、发生时间和记录摘要。
- `risk.escalated`：展示立即就医行动，不继续展示普通分析内容。
- `assistant.question`：展示追问并提交到 `reply` 接口。
- `run.failed`：展示通用失败提示，不暴露内部错误。

事件轮询应保存最后一个 `sequence`，下一次请求传入该值；重复收到相同事件时按序号去重。事件的 `data` 是 JSON 对象，字段名统一为 snake_case。

### 修改文件

- `internal/httpapi/ask_routes_test.go`：新增多宠物事实查询 HTTP 契约测试，并补充多宠物关联表测试夹具。
- `docs/ask-agent-development.md`：记录接口调用顺序、状态机和前端事实结果消费协议。

### 验证结果

已通过：

```bash
gofmt -w internal/httpapi/ask_routes_test.go
GOCACHE=/tmp/pet-go-build go test ./internal/httpapi
GOCACHE=/tmp/pet-go-build go test ./...
git diff --check
```

### 下一步计划

第十六步开始前端问问页面：先封装问问 API 和事件轮询服务，再实现输入框、创建中/追问/风险升级/事实结果/失败等状态视图；页面不再要求用户手动选择宠物，宠物绑定以服务端 `session.pets` 为准。

## 第十六步：前端问问模块整体规划

### 当前前端现状

- `web/src/pages/ask/index.tsx` 当前是静态视觉页面，已有背景图、AI 角色、预设问题和输入栏，但尚未连接问问后端。
- `web/src/services/request.ts` 已提供统一鉴权、登录刷新、业务错误和网络错误处理，应作为问问 API 的唯一请求入口。
- 项目使用 Taro + React + TypeScript，跨页面状态使用 Zustand；问问页面已经注册为底部 Tab，不新增独立入口。
- 宠物列表由现有 Pet Store 管理，但问问不以当前选中宠物作为查询条件，服务端 `session.pets` 才是本次问题的真实绑定结果。

### 前端分层

```text
pages/ask/index.tsx
页面布局、输入交互、页面生命周期
        ↓
hooks/use-ask-session.ts
创建会话、处理 Run、追问、事件轮询和状态转换
        ↓
stores/ask-store.ts
当前会话、运行状态、事件序号、消息和结果
        ↓
services/ask.ts
HTTP API 和响应类型约束
        ↓
services/request.ts
鉴权、重试、业务错误和网络错误
```

建议新增文件：

- `web/src/types/ask.ts`：会话、Run、宠物绑定、事件、事实结果和结构化分析结果类型。
- `web/src/services/ask.ts`：封装创建会话、处理 Run、追问、获取会话和增量事件接口。
- `web/src/stores/ask-store.ts`：保存当前会话和展示所需的最小状态，不保存完整宠物档案或全部历史上下文。
- `web/src/hooks/use-ask-session.ts`：封装问问工作流，页面不直接编排多个接口调用。
- 后续按需要拆分 `web/src/components/ask/` 下的输入栏、消息列表、事实结果、风险提示和追问组件。

### 核心状态机

```text
idle
→ creating
→ queued
→ processing
→ completed
→ waiting_input → replying → processing
                    ↓
                 completed

processing → escalated
processing → failed
creating   → input_error / ambiguous / network_error
```

状态含义：

- `idle`：没有当前会话，可输入问题或点击预设问题。
- `creating`：正在调用 `POST /api/v1/ask/sessions`，输入框和发送按钮防重复提交。
- `queued`：已拿到 `session.id`、`run.id`，准备处理本次 Run。
- `processing`：调用 process 接口或等待事件，展示处理中状态。
- `waiting_input`：收到 `assistant.question`，允许用户回答当前追问。
- `completed`：收到 `fact.completed` 或普通结构化分析完成事件，展示结果和后续操作。
- `escalated`：收到 `risk.escalated`，优先展示就医行动，不渲染普通分析结果。
- `failed`：收到 `run.failed` 或请求失败，展示通用失败提示并允许重新发起。
- `input_error`：400 参数问题，保留原输入并提示补充宠物名称或问题内容。
- `ambiguous`：409 同名宠物歧义，保留原输入，提示补充更具体的宠物名称。

### 接口调用时序

```text
用户提交输入
→ createAskSession(input)
→ 保存 session、run 和 session.pets
→ processAskRun(session.id, run.id)
→ 合并响应中的 events
→ 使用最后 sequence 调用 getAskEvents(after)
→ 按事件类型更新页面状态
```

当前后端的 process 接口是同步完成 Run 的主要入口，但前端仍按事件增量消费设计：响应事件先入状态，轮询只请求更大的 `after`，按 `sequence` 去重。这样可以兼容后续异步 Agent、网络重试和页面重新进入。

### 页面展示规划

- 空状态：保留现有 AI 角色和预设问题，预设问题只负责填入输入框或直接提交，不绑定某一只宠物。
- 会话头部：展示“问问”标题和当前会话状态；创建成功后展示服务端返回的宠物名称，不提供手动切换来覆盖绑定。
- 消息区：按用户输入、追问、结果和风险事件分组，结果内容不依赖字符串解析。
- 多宠物事实结果：按 `session.pets` / `items` 顺序逐行展示宠物名、记录状态、发生时间和摘要；`found=false` 展示“暂无记录”。
- 普通分析结果：固定展示当前判断、观察到的情况、可能原因、接下来怎么做、需要就医的情况五个字段。
- 追问：一次只显示一个问题，提交后回到 `processing`，最多遵守后端三轮限制。
- 红色风险：使用高优先级风险提示和立即就医行动，不和绿色普通分析混排。
- 失败状态：不暴露内部错误、SQL 或模型信息，提供重试当前问题和修改输入两个动作。

### 输入和图片边界

- 文本输入是第十六步的主流程，提交前校验非空和 4000 字符限制。
- 当前后端创建会话接口只接受 `input`，尚未提供问问图片字段和图片分析接口；现有“添加图片”视觉入口不能伪装成已支持能力。
- 图片能力单独作为后续步骤：先复用资产上传服务，再扩展问问请求的媒体字段、质量检测状态和事件协议，完成接口契约后再开放按钮。

### 状态持久化边界

- Zustand 只保存当前会话的最小 UI 状态、事件序号和结构化结果。
- 不把完整上下文、宠物档案或所有事件长期写入本地存储；重新进入页面时以服务端会话和事件接口恢复。
- MVP 暂不做问问历史列表，历史数据以后通过独立分页接口接入，避免把当前会话状态和历史查询耦合。

### 开发顺序

1. 定义 `types/ask.ts`，对齐后端 snake_case JSON 字段和事件数据联合类型。
2. 实现 `services/ask.ts`，封装创建、处理、追问、会话详情和事件查询。
3. 实现 `stores/ask-store.ts` 与 `use-ask-session.ts`，完成状态机、事件去重和错误归一化。
4. 将 `pages/ask/index.tsx` 改造成可提交的文本问问页，先支持普通分析、追问、失败和风险状态。
5. 增加多宠物 `fact.completed` 结果组件，验证“旺仔和球球分别是什么时候”的逐项展示。
6. 补充页面级交互测试和 TypeScript 类型检查，再进行微信开发者工具真机验证。
7. 图片问问、观察提醒、问问历史和就医摘要作为后续独立步骤，不和首个文本闭环同时上线。

### 前端验证标准

```bash
cd web
pnpm typecheck
pnpm lint
pnpm build:weapp
```

验收至少包含：空输入拦截、无宠物名称 400、同名宠物 409、单宠物普通分析、多宠物事实结果、追问往返、红色风险、事件重复去重、网络失败重试和登录失效跳转。

### 下一步计划

第十七步实现问问前端基础契约：先新增 `types/ask.ts` 和 `services/ask.ts`，让页面具备可调用后端的类型安全 API；暂不修改视觉布局和图片入口，完成接口层后再接入状态机。
