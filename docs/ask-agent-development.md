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

## 第十七步：TanStack Query、Zustand 和事件轮询基础层

### 实现内容

- 前端新增 `@tanstack/react-query`，并在应用根节点接入全局 `QueryClientProvider`。
- QueryClient 统一配置查询缓存、垃圾回收时间、失败重试和窗口聚焦策略。
- 新增完整问问 TypeScript 契约，覆盖 Session、Run、事件、宠物绑定、事实结果、追问、风险升级和结构化分析。
- 新增问问 API Service，封装自然语言创建会话、处理 Run、提交追问、读取会话和增量读取事件五个接口。
- 新增 Ask Zustand Store，只保存输入草稿、当前 Session/Run 标识和事件游标。
- 新增 `useAskSession`，使用 TanStack Mutation 编排 `create → process` 与 `reply → process`，使用 Query 管理会话和事件服务端状态。
- 事件查询使用 `after` 游标，只读取已消费序号之后的事件；事件写入 Query Cache 时按 `sequence` 去重并排序。
- Run 为 `queued` 或 `running` 时每 1.5 秒轮询，收到完成、追问、风险升级或失败事件后停止定时请求。
- 如果 process 响应因网络中断丢失，但事件轮询获取到服务端终态事件，页面状态以持久化事件为准。
- 登录清理或用户/家庭身份切换时，同时清空 Query Cache 和 Ask UI Store，避免显示上一身份的数据。

### 状态职责

TanStack Query 保存：

- `AskSession` 服务端快照。
- 当前 `AskExecution` 响应。
- 当前 Run 的已消费事件列表。
- 带 `after` 游标的增量事件响应。
- Mutation 的请求中、成功和错误状态。

Zustand 保存：

- 未提交的输入草稿。
- 当前 Session ID 和 Run ID。
- 最后消费的事件序号。

Zustand 不保存宠物列表、会话详情、Run DTO 或完整事件副本，避免两套状态源产生不一致。

### 当前实时策略

本步没有引入 SSE。当前后端已经提供持久化事件和 `after` 游标，前端先使用短轮询实现可恢复的增量消费。SSE 或微信小程序 chunked request 后续只能作为实时传输加速层，不能替代数据库事件、游标去重和断线补偿。

### 修改文件

- `web/package.json`、`web/pnpm-lock.yaml`：新增 TanStack Query 依赖。
- `web/src/app.ts`：接入 `QueryClientProvider`。
- `web/src/services/query-client.ts`：统一 QueryClient 配置。
- `web/src/types/ask.ts`：问问接口和事件类型契约。
- `web/src/services/ask.ts`：问问 HTTP API 封装。
- `web/src/stores/ask-store.ts`：问问客户端 UI 状态。
- `web/src/hooks/use-ask-session.ts`：请求编排、Query Cache 同步、事件轮询和工作流状态推导。
- `web/src/stores/auth-store.ts`：认证身份变化时清理问问缓存和 UI 状态。

### 验证结果

已通过：

```bash
cd web
pnpm typecheck
pnpm lint
```

Lint 没有错误，现有 `web/src/pages/profile/index.tsx` 保留两条与本次无关的 `react-hooks/exhaustive-deps` 警告。

微信构建首次在 macOS 环境触发 `system-configuration` 的 `Attempted to create a NULL object` panic；随后已在 `/tmp` 临时副本中重新执行并构建成功，全程未修改 `web/dist`。TypeScript 编译和 ESLint 也已确认新增代码可通过。

### 下一步计划

第十八步将 `useAskSession` 接入问问页面：实现文本输入提交、处理中状态、追问、风险升级、失败提示和多宠物事实结果展示；继续保持图片按钮不可操作，直到后端图片协议完成。

## 第十八步：问问页面接入服务端状态

### 实现内容

- 问问页面通过 `useAskSession` 发起自然语言咨询，不再由页面直接调用多个后端接口。
- 用户点击发送后，reducer 立即写入本地乐观 Turn 并进入处理中状态，不等待创建会话接口返回。
- 创建接口成功后，使用服务端返回的 Session、Run 和 Events 替换本地临时 Run，服务端快照成为后续状态基准。
- 页面按 Session 中的 `pets` 展示服务端实际绑定的宠物，支持一句话同时绑定多只宠物，不提供手动宠物选择覆盖自然语言结果。
- 页面按事件类型展示追问、多宠物事实结果、结构化分析、风险升级和失败状态。
- `fact.completed` 按服务端 `items` 顺序逐只展示；`found=false` 明确显示“暂无记录”。
- 输入为空或超过 4000 字符时在前端拦截；400、409、网络失败和 Run 失败分别映射到独立页面状态。
- 图片入口继续保持禁用，避免在后端尚无图片协议时形成不可用入口。

### 状态边界

- TanStack Query 保存 Session、Execution 和原始事件日志等服务端状态。
- reducer 保存当前页面需要渲染的会话投影和阶段。
- Zustand 只保存草稿、当前 Session/Run ID 和事件游标，不复制完整服务端快照。
- 页面只消费 Hook 返回的状态和动作，不负责判断事件顺序、去重或接口调用时序。

### 修改文件

- `web/src/pages/ask/index.tsx`：接入提交、追问、重试、重置和各运行阶段。
- `web/src/components/ask/ask-event.tsx`：按事件类型渲染事实、追问、分析、风险和失败结果。
- `web/src/components/ask/ask-event.scss`：补充事件结果视图样式。
- `web/src/hooks/use-ask-session.ts`：统一编排创建、处理、追问和状态同步。

### 当前边界

当前的 `snapshot.restored` 使用创建、处理和追问接口返回的 `AskExecution` 快照。页面重新进入后完整恢复历史仍未完成，因为后端目前只有 Session 详情接口，尚缺少同时返回 Turns、Runs 和 Events 的完整 Snapshot 接口。

### 下一步计划

第十九步实现真正的增量流：后端输出 NDJSON，H5 使用递归 `ReadableStream.read()`，微信小程序使用 chunked request；前端通过共享 decoder 处理半包和粘包，再统一交给 reducer。

## 第十九步：NDJSON 流式传输、递归读取和事件 reducer

### 后端流协议

- 新增 `GET /api/v1/ask/sessions/{session_id}/runs/{run_id}/events/stream?after={sequence}`。
- 响应类型为 `application/x-ndjson`，每一行只包含一个完整事件 JSON，不使用公共响应 wrapper。
- 事件增加 `run_id`，前端使用 `run_id + sequence` 作为幂等键。
- 服务端先读取 `after` 之后的持久化事件，再每 500 毫秒查询增量并立即 flush。
- 收到终态事件后关闭本次流；如果 25 秒内没有终态，也正常关闭，由客户端携带最新 `after` 重连。
- 流只是实时传输层，数据库事件和游标仍是断线恢复依据。

### 前端读取链路

```text
用户发送
→ local.submitted 立即创建乐观 Turn
→ REST 响应恢复 AskExecution 快照
→ 建立 events/stream?after=N
→ read() 递归读取下一个 Uint8Array
→ NDJSONDecoder 将字节追加到 buffer
→ 只按换行提取完整 JSON
→ 校验 AskEvent 协议
→ events.received 交给 reducer
→ 更新 Query Cache、游标和页面投影
```

- H5 使用 `fetch`、`response.body.getReader()` 和递归 `read()`。
- 微信小程序使用 `Taro.request` 的 `enableChunked` 与 `onChunkReceived`。
- 两端共用 `NDJSONDecoder`，同一个 chunk 可包含多行，一行也可以跨多个 chunk。
- 半包只留在 decoder buffer 中，不触发 reducer，因此不会渲染不完整 JSON。
- decoder 使用流式 UTF-8 解码；小程序运行时没有 `TextDecoder` 时使用内置后备实现，中文字符跨 chunk 不会乱码。
- 流关闭时调用 `finish()`；若最后仍有完整但没有换行的 JSON，则解析后再交付。
- JSON 语法错误或事件字段不符合协议时停止重连并进入失败状态，避免错误数据污染 reducer。

### reducer 事件规则

```text
run.queued / run.started  → thinking
assistant.question       → waiting_input
fact.completed           → completed
run.completed            → completed
risk.escalated           → escalated
run.failed               → failed
未知事件                  → 保留事件，不改变页面阶段
```

- `local.submitted` 先生成临时 Run，使用户发送后立刻看到处理中状态。
- `snapshot.restored` 用服务端 Run ID 替换临时 Run，并合并快照中的事件。
- `events.received` 先按 `sequence` 排序，再逐条归约。
- 已见过的 `run_id + sequence` 直接忽略，保证快照和流重复到达时不会重复渲染。
- 迟到的旧事件仍补入事件日志，但不能把较新的完成态回退为思考态。
- 只有当前 Run 的事件可以改变当前页面阶段，历史 Run 的迟到事件只更新对应历史 Turn。

### 重连规则

- 服务端 25 秒正常关闭或网络中断：1 秒后使用最新游标重连。
- 同一时刻只保留一个重连定时器，页面卸载或 Run 切换时同时关闭请求和定时器。
- 收到追问、事实完成、分析完成、风险升级或失败事件后关闭流，不再重连。
- HTTP 4xx/5xx、JSON 解析错误和事件协议错误不无限重连，直接进入失败状态。
- Human in the loop 当前只保留 `assistant.question → reply` 基础路径，审批、工具确认和复杂消歧延后实现。

### 测试覆盖

- NDJSON 半包不会提前输出。
- 一个 chunk 中多个 JSON 可以一次输出。
- 递归 `read()` 只交付完整 JSON。
- 没有原生 `TextDecoder` 时中文跨字节 chunk 仍能正确恢复。
- 本地提交立即创建乐观状态。
- 快照替换临时 Run。
- 重复事件幂等去重。
- 所有终态事件映射到正确阶段。
- 未知事件被保留但不改变阶段。
- 终态先到、旧快照后到时不会发生状态回退。

### 修改文件

- `internal/app/ask/dto.go`：事件 DTO 增加 `run_id`。
- `internal/httpapi/ask/handler.go`：实现 NDJSON 增量流和终态关闭。
- `internal/httpapi/ask/routes.go`：注册事件流路由。
- `internal/httpapi/ask_routes_test.go`：增加流接口契约测试。
- `web/src/services/ndjson.ts`：实现 buffer、UTF-8 解码和递归 `read()`。
- `web/src/services/ask-stream.ts`：实现 H5/微信双 Transport、事件校验和错误分类。
- `web/src/hooks/ask-reducer.ts`：实现统一 reducer、幂等和时序保护。
- `web/src/hooks/use-ask-session.ts`：接入快照、增量、游标和重连。
- `web/src/services/ndjson.test.ts`、`web/src/hooks/ask-reducer.test.ts`：补充核心单元测试。
- `web/package.json`、`web/pnpm-lock.yaml`：增加兼容当前工程的 Vitest 0.34.6 和测试命令。

### 验证结果

已通过：

```bash
cd web
pnpm test
pnpm typecheck
pnpm lint

cd ..
GOCACHE=/tmp/pet-go-build go test ./...
git diff --check
```

前端共 2 个测试文件、8 个用例通过。Lint 没有错误，仍有 `web/src/pages/profile/index.tsx` 中两条与本步无关的既有 Hook 依赖警告。

### 当前未完成项

- 尚未提供完整会话 Snapshot 接口，页面重进后不能恢复 Turns、Runs 和 Events 全量视图。
- 尚未实现图片上传、图片质量检测和多模态分析事件。
- 尚未实现复杂 Human in the loop。
- 尚未做本轮页面视觉检查，按当前约定由后续实际使用反馈驱动。

## 第二十步：完整 Snapshot 与 Run 版本化状态

### 本步目标

本步解决两个会直接影响 Agent 可恢复性和并发正确性的问题：

- 页面只有单次接口返回的局部执行结果，重新进入页面后无法恢复完整会话。
- Run 状态更新只校验旧状态，同一状态下的迟到任务仍可能覆盖更新后的执行结果。

Snapshot 只表示前端恢复所需的权威 UI 状态，不保存完整 Prompt，也不把每次组装的模型上下文重复写入数据库。

### 数据库变更

新增迁移 `000012_ask_run_version`，为 `ask_runs` 增加：

```text
row_version INTEGER NOT NULL DEFAULT 1
clarification_count INTEGER NOT NULL DEFAULT 0
```

- `row_version` 是 Run 的乐观锁版本。新 Run 从 1 开始，每次状态切换成功后加 1。
- `clarification_count` 为下一步同 Run 追问恢复预留，当前创建值为 0，尚不改变 Reply 行为。
- 迁移会清理已有 `waiting_input` Run 上错误写入的 `completed_at`。
- 迁移会为历史单宠物会话补齐 `ask_session_pets`，新建会话无论绑定一只还是多只宠物都会持久化绑定关系。

### 版本化状态更新

Run 状态切换现在同时校验状态和版本：

```sql
WHERE id = ? AND status = ? AND row_version = ?
```

更新成功时在同一个事务内完成：

```text
Run 状态更新并递增 row_version
→ Turn 状态同步
→ Session 状态和风险等级同步
→ ask_events 追加持久化事件
→ 提交事务
```

任一校验失败都返回状态冲突，事件不会单独写入。这样即使两个 Worker 同时读取到 `running`，也只有持有当前版本的 Worker 能提交最终结果。

`waiting_input` 现在明确是暂停态，不再写入 `completed_at`。只有 `completed`、`escalated`、`failed`、`canceled` 和 `interrupted` 才会记录 Run 完成时间。

当前一次普通执行的版本变化为：

```text
queued(row_version=1)
→ running(row_version=2)
→ waiting_input 或终态(row_version=3)
```

### 完整 Snapshot 接口

新增：

```http
GET /api/v1/ask/sessions/{session_id}/snapshot
```

响应数据结构：

```text
session
pets[]
turns[]
  turn
  run
  messages[]
  events[]
event_cursors[]
  run_id
  sequence
```

- `turns` 按 `turn_index` 升序返回。
- 每个 Turn 返回当前 `selected_run_id` 指向的 Run。
- 每个 Run 的事件按 `sequence` 升序返回。
- `event_cursors` 返回每个 Run 已持久化的最新事件序号，前端可以从该游标继续接增量流。
- Run DTO 新增 `run_index`、`row_version` 和 `clarification_count`。
- Snapshot 查询前先使用当前家庭 ID 读取 Session，不属于当前家庭的会话不会被聚合查询返回。

### 后端实现思路

Service 先按 `family_id + session_id` 验证会话归属并加载绑定宠物，再由 Repository 读取所有 Turn 及其选中 Run。Repository 随后一次读取该 Session 的持久化事件，在内存中按 `run_id` 归组，避免对每个 Run 分别查询事件。

数据库仍保存 Session、Turn、Run 和结构化事件这些可恢复事实。模型执行时按需组装的 `ContextSnapshot` 继续只受上下文预算控制，不因为新增 UI Snapshot 而扩大模型输入。

### 修改文件

- `migrations/000012_ask_run_version.up.sql`、`migrations/000012_ask_run_version.down.sql`：新增 Run 版本和追问计数字段，修复历史等待态时间并补齐单宠物绑定。
- `internal/app/ask/model.go`：增加 Run 版本字段和 Snapshot 领域模型。
- `internal/app/ask/repository.go`：实现版本化 CAS、Snapshot 聚合查询和正确的等待态时间语义。
- `internal/app/ask/service.go`：新增 Snapshot 用例，传递当前 Run 版本并同步内存结果。
- `internal/app/ask/dto.go`：增加 Turn、Snapshot、事件游标 DTO 和 Run 版本字段。
- `internal/httpapi/ask/handler.go`、`internal/httpapi/ask/routes.go`：注册并实现 Snapshot 接口。
- `internal/app/ask/repository_test.go`、`internal/app/ask/service_test.go`、`internal/httpapi/ask_routes_test.go`：覆盖版本冲突、等待态、Snapshot 顺序和 HTTP 契约。

### 验证结果

已通过：

```bash
GOCACHE=/tmp/pet-go-build go test ./...
git diff --check
```

后端所有包测试通过，`gofmt -d` 对本步涉及的 Go 文件无输出。

### 当前边界

- Reply 仍沿用“创建新 Turn 和新 Run”的旧协议，本步没有提前改变追问请求语义。
- 前端尚未调用完整 Snapshot 接口，也尚未在事件序号缺口时触发 Snapshot 恢复。
- `clarification_count` 当前只完成存储与输出，下一步才在同 Run 恢复事务中递增。
- 请求幂等键尚未实现。

### 下一步计划

第二十一步实现同一个 Run 的追问暂停与恢复，并增加创建会话、回答追问的请求幂等：Reply 携带 `expected_version`，在一个事务内写入回答、递增 `clarification_count`、将 Run 从 `waiting_input` 恢复为 `queued`，保持 `run_id` 和 `turn_id` 不变。随后再调整前端 Snapshot 恢复和事件缺口处理。

## 第二十一步：同 Run 追问恢复与请求幂等

### 本步目标

第二十步完成了 Run 版本和完整 Snapshot，本步将追问真正改造成一个可暂停、可恢复的执行过程，并解决移动网络环境下重复提交的问题。

原实现存在三个问题：

- 用户每回答一次追问就创建新的 Turn 和 Run，一个问题被拆成多个不连续的执行单元。
- 用户回答没有进入 `ask_messages`，改成同 Run 后如果不补消息上下文，Executor 无法看到补充内容。
- 创建和回答接口没有幂等键，请求在服务端成功但客户端超时后，重试可能重复创建资源或重复写入回答。

### 数据库变更

新增迁移 `000013_ask_idempotency`，创建 `ask_idempotency_keys`：

```text
id
family_id
user_id
operation
idempotency_key
request_hash
response_data
session_id
turn_id
run_id
created_at
```

唯一约束：

```text
user_id + operation + idempotency_key
```

`request_hash` 使用家庭、资源 ID、请求内容和期望版本等实际输入计算，避免用户切换家庭后错误重放旧资源。`response_data` 保存首次事务提交时的执行结果，使重试返回首次响应，而不是读取一个可能已经继续变化的 Run。

### 幂等协议

以下接口要求 `Idempotency-Key`：

```http
POST /api/v1/pets/{pet_id}/ask/sessions
POST /api/v1/ask/sessions
POST /api/v1/ask/sessions/{session_id}/runs/{run_id}/reply
```

规则：

```text
新 key
→ 在业务事务内写资源和幂等结果

相同 key + 相同请求哈希
→ 返回首次持久化的 ExecutionResult

相同 key + 不同请求哈希
→ 409 Conflict
```

幂等键不能为空，最大 128 个字符。创建会话时，Session、宠物绑定、Turn、Run、初始用户消息、`run.queued` 事件和幂等记录处于同一个事务；任一写入失败都会全部回滚。

### 同 Run 追问恢复

Reply 请求体改为：

```json
{
  "input": "从今天早上开始，已经吐了两次",
  "expected_version": 3
}
```

Service 先验证：

- 当前用户和家庭身份。
- Session 属于当前家庭且仍为 `active`。
- Run 属于 Session 且状态为 `waiting_input`。
- `expected_version` 等于 Run 当前 `row_version`。
- Run 是当前 Turn 的 `selected_run_id`。
- `clarification_count` 尚未达到上限。

随后 Repository 在一个事务中执行：

```text
写入 ask_messages 用户回答
→ Run waiting_input → queued
→ row_version + 1
→ clarification_count + 1
→ 清空 completed_at 和 error_code
→ Turn 恢复 queued
→ Session 保持 active
→ 追加同 Run 的 run.queued 事件
→ 写入幂等结果
→ 提交事务
```

整个过程保持以下身份不变：

```text
session_id 不变
turn_id 不变
run_id 不变
```

例如一次追问链的状态与事件序号为：

```text
run.queued sequence=1, row_version=1
run.started sequence=2, row_version=2
assistant.question sequence=3, row_version=3
用户回答
run.queued sequence=4, row_version=4, clarification_count=1
run.started sequence=5, row_version=5
下一结果 sequence=6, row_version=6
```

### 消息上下文与安全规则

现在会写入三类消息：

```text
user      原始问题和用户补充回答
question  Agent 主动追问
assistant 后续正式回答预留
```

每次 Run 再执行前，Repository 按当前 `session_id + run_id` 读取消息，Service 将其放入 `ContextSnapshot.Messages`。上下文版本升级为 `ask-context-v5`，消息文本继续经过单项裁剪和总字符预算，不会无上限增长。

确定性风险规则会同时检查原始 Turn 输入和该 Run 下的所有用户回答。因此原问题是“最近没精神”，用户补充“现在呼吸困难”时，恢复执行会优先触发红色风险升级，不会进入普通模型追问。

Agent 最多允许三次补充。第三次补充后如果 Executor 仍返回 `waiting_input`，Service 将结果收敛为：

```text
status=failed
error_code=clarification_limit_reached
```

避免 Run 永久停留在不可继续回答的等待状态。

### 前端请求编排

- `web/src/services/ask.ts` 统一为创建和回答请求写入 `Idempotency-Key`，页面仍不直接调用 `Taro.request`。
- Reply DTO 增加 `expected_version`，值来自当前服务端 Run 的 `row_version`。
- 一次逻辑提交的幂等键保存在 `useAskSession` 的 `ref` 中，不进入 TanStack Query 或 Zustand。
- 认证刷新导致的底层重试天然复用相同请求参数和幂等键。
- 网络错误后用户再次提交相同输入时复用待处理 key，不重复创建乐观消息。
- 一旦业务响应成功，或收到不可重试的业务错误，待处理 key 会被清理。

同一个 Run 现在可能对应多条用户消息。Reducer 为每条页面消息保留独立的客户端 ID，事件仍使用 `run_id + sequence` 去重，并将恢复后的新事件归到同 Run 的最新用户消息之后，避免新追问显示在用户回答之前。

Zustand 新增 `activeRunVersion`。同 Run 从 `waiting_input` 恢复为 `queued` 时 ID 不变，但版本变化会重新建立事件流；游标继续从当前 Run 已知的最大 `sequence` 开始，不会重复消费旧事件。

页面发送逻辑改为依据 `run.status` 判断新问题或追问回答。这样回答请求发生网络错误后，即使页面阶段变成 `network_error`，再次点击发送仍会重试原 Reply，而不会错误创建新 Session。

### 修改文件

- `migrations/000013_ask_idempotency.up.sql`、`migrations/000013_ask_idempotency.down.sql`：新增请求幂等表。
- `internal/app/ask/model.go`：新增消息上下文、Message 和 IdempotencyRecord。
- `internal/app/ask/state.go`：允许 `waiting_input → queued`。
- `internal/app/ask/output.go`：校验追问必须包含非空问题。
- `internal/app/ask/context.go`：将消息纳入上下文裁剪和字符预算。
- `internal/app/ask/repository.go`：实现幂等创建、同 Run 恢复、消息持久化和幂等重放查询。
- `internal/app/ask/service.go`：实现幂等入口、同 Run Reply、消息上下文、补充回答风险复检和追问上限。
- `internal/httpapi/ask/handler.go`：读取幂等头、用户身份和 `expected_version`。
- `web/src/types/ask.ts`、`web/src/services/ask.ts`：同步 Run 与 Reply 请求契约。
- `web/src/hooks/use-ask-session.ts`：管理逻辑请求幂等键、版本化流重连和请求编排。
- `web/src/hooks/ask-reducer.ts`、`web/src/pages/ask/index.tsx`、`web/src/stores/ask-store.ts`：支持同 Run 多条用户消息和版本变化。
- `docs/api/ask.md`：同步接口协议。

### 测试覆盖

- 创建会话同 key 同请求只生成一个 Session 和一条原始用户消息。
- 创建会话同 key 不同请求返回冲突。
- Reply 保持 Session、Turn 和 Run ID 不变。
- Reply 正确递增 `row_version`、`clarification_count` 和事件序号。
- Reply 同 key 同请求只写入一次用户回答。
- Reply 同 key 不同请求返回冲突。
- 旧 `expected_version` 无法恢复 Run。
- 原始问题、追问和用户回答按顺序进入消息历史。
- 用户补充中的红旗症状触发确定性红色升级。
- 达到三次补充后不再生成第四次追问。
- 前端同 Run 回答保留独立用户消息，新事件追加到回答之后。

### 验证结果

已通过：

```bash
GOCACHE=/tmp/pet-go-build go test ./...

cd web
pnpm test
pnpm typecheck
pnpm lint

cd ..
git diff --check
```

前端共 2 个测试文件、9 个用例通过。ESLint 没有错误，仍只有 `web/src/pages/profile/index.tsx` 两条与本步无关的既有 Hook 依赖警告。

数据库迁移已在独立临时 SQLite 数据库中验证：从 `000001` 顺序升级到 `000013` 成功，`000013` 回滚后再次升级成功。未操作项目实际数据库。

### 当前边界

- 完整 Snapshot 后端接口已经存在，但前端页面重新进入时尚未主动读取并恢复完整历史。
- 前端尚未检测事件 `sequence` 缺口并自动回源 Snapshot。
- `response_data` 当前保存内部 ExecutionResult，未来如调整领域模型需要考虑幂等记录的兼容读取期限。
- 仍使用同步 Process 接口驱动 Executor，尚未拆出后台 `AskRunWorker`。
- Human in the loop 当前只覆盖文本追问暂停与恢复，工具审批和复杂消歧仍未实现。

### 下一步计划

第二十二步接通前端完整 Snapshot 恢复和事件缺口补偿：页面有活动 Session 时先加载 Snapshot，按 Turn、Run、事件游标恢复 reducer；流收到非连续 `sequence` 时暂停增量归约，重新获取 Snapshot，再从新游标建立事件流。

## 第二十二步：前端 Snapshot 恢复与事件缺口补偿

### 本步目标

第二十一步已经提供了完整 Snapshot 接口和按 Run 递增的事件序号，但前端仍然只依赖当前页面内存中的单次 `ExecutionResult`。页面重新进入、网络断线或事件流丢包时，内存状态可能缺少历史 Turn，或者无法判断当前游标与服务端是否连续。

本步把 Snapshot 设为前端恢复的权威来源，并在增量流消费前增加序号连续性检查。

### Snapshot 前端契约

新增 `AskSnapshot` 类型，对应服务端：

```text
session
pets
turns[]
  turn
  run
  messages[]
  events[]
event_cursors[]
```

`getAskSnapshot(sessionID)` 统一通过 `web/src/services/ask.ts` 请求 `/api/v1/ask/sessions/:session_id/snapshot`，并写入 TanStack Query：

```text
['ask', 'snapshot', sessionID]
['ask', 'session', sessionID]
['ask', 'execution', sessionID, runID]
['ask', 'events', sessionID, runID]
```

这样 Snapshot、单 Run 执行结果和事件日志共享同一缓存边界，页面组件不直接发起底层请求。

### Reducer 恢复规则

新增 `snapshot.loaded` 动作。Reducer 收到完整 Snapshot 后：

1. 按服务端 Turn 顺序重建全部会话消息。
2. 使用每个 Turn 的 `messages` 恢复原始问题和同 Run 的用户补充回答，无消息时回退到 `turn.input`。
3. 将 Run 事件按 `sequence` 排序并重建 `cursors`、`seenEvents`。
4. 以最新 Turn 的 Run 作为当前活动 Run，依据 Run 状态和最后事件恢复页面阶段。
5. Snapshot 恢复会清除旧错误状态，避免网络错误覆盖已经恢复的权威状态。

恢复期间仍可能存在尚未落库的乐观消息。原始提交通过输入内容匹配服务端 Turn，避免重复显示；同 Run 的补充回答输入不同于原问题时继续保留在最新 Turn 后面，待服务端事件到达后再追加事件。

### 页面进入时的恢复顺序

当 Zustand 中存在 `activeSessionId` 和 `activeRunId` 时，`useAskSession` 的事件流 Effect 先执行：

```text
读取完整 Snapshot
→ snapshot.loaded 重建 reducer
→ 更新当前 Run 和最新 sequence
→ 从该 sequence 建立 NDJSON 事件流
```

Snapshot 请求失败时保留原有内存状态并尝试建立事件流；后续事件缺口恢复失败则进入网络错误阶段并按既有重连策略继续尝试。

### 事件序号缺口检测

事件流每次收到一批事件时，先按 `sequence` 排序，并与当前 Run 的 `lastEventSequence` 比较：

```text
sequence <= cursor       → 已消费事件，交给去重逻辑
sequence == cursor + 1   → 可以增量归约
sequence > cursor + 1    → 检测到缺口，暂停本批归约
```

缺口不会把不连续事件写入 Reducer。客户端会关闭当前流并执行：

```text
重新 GET Snapshot
→ 用 Snapshot 覆盖会话状态和事件游标
→ 从新的最大 sequence 重建事件流
```

恢复流程使用 `recovering` 标志防止同一时间重复请求 Snapshot。重连和组件卸载都会清理定时器与流连接，避免旧连接在新 Run 上继续派发事件。

### 测试覆盖

- 完整 Snapshot 可以从多个 Turn 重建会话状态。
- Snapshot 会把服务端原始问题与本地乐观提交合并为一条消息。
- 同 Run 的未落库补充回答在 Snapshot 恢复后仍然保留。
- Snapshot 恢复会重建最新事件游标并依据终态事件恢复阶段。
- 已消费事件不会被判定为缺口。
- 单批事件跳过序号或批内不连续时会判定为缺口。

### 修改文件

- `web/src/types/ask.ts`：新增 Snapshot、Snapshot Turn 和事件游标类型。
- `web/src/services/ask.ts`：新增 Snapshot 查询 API。
- `web/src/hooks/ask-reducer.ts`：新增完整 Snapshot 恢复和序号缺口检测。
- `web/src/hooks/use-ask-session.ts`：接通页面进入恢复、缺口回源和从新游标重建事件流。
- `web/src/hooks/ask-reducer.test.ts`：补充 Snapshot 恢复与事件缺口测试。

### 验证结果

```text
Go 全部测试通过
前端 2 个测试文件、12 个用例通过
TypeScript 类型检查通过
ESLint 0 个错误，仍只有 profile 页面既有的 2 条 Hook 警告
```

### 当前边界

- 当前活动 Session ID 仍由 Zustand 生命周期维护，尚未增加跨进程持久化；同一小程序运行期间重新进入页面可以恢复。
- 事件流仍按当前活动 Run 建立；历史 Turn 的事件由 Snapshot 一次性恢复，不为每个历史 Run 长连接。
- Snapshot 恢复失败时会保留内存状态并尝试普通重连，尚未增加独立的恢复失败退避上限和人工刷新入口。

## 第二十三步：多 Run 快照与 Worker 执行边界

### 本步目标

第 22 步解决了当前活动 Run 的 Snapshot 恢复和事件缺口。本步把一个 Turn 下的多个 Run 暴露为可审计数据，并抽出后台执行 Worker 边界，为后续异步任务队列接入做准备。

Snapshot 的多 Run 结构解决了一个 Turn 中多次执行、重试和追问恢复时的审计问题。当前选中的 Run 仍由 `turn.selected_run_id` 确定，历史 Run 不会覆盖页面当前状态。

新增 `RunWorker` 作为后台执行边界：

```text
Enqueue(RunJob)
→ Worker 调用 Service.ProcessRun
→ Service 统一负责 CAS、规则、Executor 和事件持久化
```

Worker 支持显式 `Start`、`Enqueue` 和 `Close`，使用有界队列，队列满时调用方可以等待或响应取消。当前主程序仍使用同步 `ProcessRun` 接口，Worker 只作为可测试的后台执行基础，不改变现有 HTTP 行为；后续接入任务队列时可以把创建或 Reply 成功后的 Run 投递到该边界。

### 修改文件

- `internal/app/ask/model.go`、`internal/app/ask/repository.go`、`internal/app/ask/dto.go`：将一个 Turn 下全部 Run、事件和消息聚合到 Snapshot。
- `internal/app/ask/worker.go`：新增可独立启动、投递和停止的 Run Worker 边界。
- `internal/app/ask/worker_test.go`、`internal/app/ask/repository_test.go`、`internal/httpapi/ask_routes_test.go`：覆盖 Worker 配置、Snapshot 多 Run 和消息聚合。
- `web/src/types/ask.ts`：增加 Snapshot 的 `runs[]` 类型，兼容当前选中 Run 展示。

### 验证结果

```text
Go 全部测试通过
前端 2 个测试文件、12 个用例通过
TypeScript 类型检查通过
ESLint 0 个错误，仍只有 profile 页面既有的 2 条 Hook 警告
```

### 下一步计划

第二十四步接入 Worker 的真实投递入口：创建会话和 Reply 成功后将 Run 放入后台队列，并增加任务去重、失败重试和优雅停机策略。

## 第二十四步：异步 Run 投递与 Worker 生命周期

### 本步目标

第 23 步只建立了 Worker 抽象，本步将创建会话和 Reply 成功后的 `queued` Run 真正投递到后台执行，并移除前端正常流程对同步 `/process` 请求的依赖。

### 异步执行流程

创建和 Reply 继续先完成数据库事务，再执行投递：

```text
持久化 Session / Run / Message / run.queued / 幂等结果
→ Enqueue(RunJob)
→ HTTP 返回 queued 快照
→ Worker 调用 Service.ProcessRunVersion
→ CAS 推进 running 和最终状态
→ 前端通过 Snapshot 和事件流恢复结果
```

`POST /ask/sessions/:session_id/runs/:run_id/process` 继续保留为兼容和人工重试入口，但前端正常提交和回答不再自动调用该接口。

### 去重与版本保护

任务身份由以下字段共同确定：

```text
family_id + session_id + run_id + row_version
```

Worker 的 `pending` 集合覆盖排队中和执行中的任务，相同版本重复投递时直接返回成功。追问恢复会复用 `run_id`，但 `row_version` 会递增，因此新版本仍可正常入队。

Worker 调用 `ProcessRunVersion` 时再次核对持久化 Run 的当前版本。旧版本任务即使晚到，也只返回当前持久化结果，不会误执行追问后的新一代 `queued` Run。Run 状态 CAS 和事件唯一约束继续作为数据库层的最终重复执行保护。

### 失败重试与停机

Worker 使用有界队列，当前主程序配置为：

```text
queue_size=100
max_attempts=3
retry_delay=500ms
```

`ProcessRunVersion` 返回错误时在 Worker 内有限重试；耗尽后通过结构化日志记录任务标识和错误。Executor 返回的受控失败仍由 Service 写为 `run.failed`，不作为 Worker 基础设施错误重复执行。

进程收到 `SIGINT` 或 `SIGTERM` 后先停止 HTTP Server 接收新请求，再关闭 Worker。`Close` 会停止新投递、等待并发投递退出、关闭队列并处理完已经接收的任务，最后才允许数据库连接关闭。

### 修改文件

- `internal/app/ask/worker.go`：增加版本化任务身份、进程内去重、有限重试和队列排空。
- `internal/app/ask/worker_test.go`：覆盖配置校验、重复任务、新版本任务、失败重试、取消和优雅停止。
- `internal/app/ask/service.go`、`internal/app/ask/service_test.go`：增加 `ProcessRunVersion` 和旧任务版本保护。
- `internal/httpapi/ask/handler.go`、`internal/httpapi/ask/routes.go`、`internal/httpapi/router.go`、`internal/httpapi/v1.go`：在创建和 Reply 事务成功后投递 Run。
- `internal/httpapi/ask_routes_test.go`：验证创建和 Reply 的投递任务及版本。
- `cmd/api/main.go`：组装并启动 Worker，接入 HTTP Server 与 Worker 的停机顺序。
- `web/src/hooks/use-ask-session.ts`：正常创建和 Reply 后改为依赖 Snapshot 与事件流，不再同步调用 Process。
- `docs/api/ask.md`：同步异步执行契约。

### 验证结果

```text
Go Ask 和 HTTP 测试通过
Go race 检测通过
前端 2 个测试文件、12 个用例通过
TypeScript 类型检查通过
gofmt 通过
git diff --check 通过
```

### 当前边界

- 队列和去重集合仍在单进程内存中，进程崩溃会丢失尚未执行的任务。
- 数据库中的 `queued` Run 尚未在进程启动时扫描恢复。
- `running` Run 尚未引入执行租约和超时回收；进程在 CAS 到 `running` 后退出时可能留下悬挂状态。
- Worker 当前为单消费者，尚未增加并发度配置和跨实例协调。

### 下一步计划

第二十五步实现可恢复执行：启动时扫描并恢复 `queued` Run，为 `running` Run 增加执行租约、超时回收和跨实例领取语义，消除数据库提交与内存投递之间的崩溃窗口。

## 第二十五步：持久化租约与崩溃恢复

### 本步目标

第 24 步接通了进程内队列，但数据库事务提交和内存投递之间仍存在崩溃窗口；Worker 在 Run 进入 `running` 后退出也会留下无法自动恢复的状态。本步将数据库提升为任务调度的事实源，内存队列只保留低延迟通知职责。

### 持久化字段

迁移 `000014_ask_run_leases` 为 `ask_runs` 增加：

```text
lease_owner
lease_expires_at
attempt_count
next_attempt_at
```

- `lease_owner` 标识当前执行实例。
- `lease_expires_at` 是执行所有权的有效期。
- `attempt_count` 记录当前执行代次已经领取的次数。
- `next_attempt_at` 持久化失败后的下一次可领取时间。

创建的新 Run 从零次尝试开始。用户回答追问并恢复同一个 Run 时，`attempt_count` 会归零，使每次补充后的新执行代次拥有独立的重试预算。

### 领取与恢复

Worker 启动时立即扫描数据库，之后每秒扫描一次可运行任务：

```text
到期 queued Run
或租约过期的 running Run
→ 条件更新领取租约
→ attempt_count + 1
→ ProcessRunVersion
```

PostgreSQL 领取事务使用行锁；最终更新仍同时校验状态、`row_version` 和租约条件。多个实例即使扫描到同一候选项，也只有一个能成功领取。每次成功领取都会递增 `row_version`，使租约过期前的旧 Worker 无法提交迟到结果。

过期 `running` Run 被领取时会：

```text
running → queued
row_version + 1
写入 run.recovered
重新绑定新租约
```

版本递增会使旧 Worker 持有的最终状态 CAS 失败，避免租约回收后迟到结果覆盖新执行。

### 续租、重试与失败

执行期间按租约时长的三分之一续租。当前配置为 30 秒租约和 2 分钟单次执行超时。续租失败会取消执行上下文，等待租约过期后的数据库恢复。

基础设施错误不会只保存在内存：

```text
清理当前租约
→ queued + row_version 递增
→ 写入 run.retry_scheduled
→ 保存 next_attempt_at
→ 后续轮询重新领取
```

当前最多领取三次。达到上限后 Run、Turn 会进入 `failed`，写入 `run.failed`，错误码为 `worker_attempts_exhausted`。受控 Executor 失败仍由 Service 按原协议直接写入 `run.failed`。

正常进入 `waiting_input` 或终态时，状态事务会清理租约和下次重试时间。`attempt_count` 与 `next_attempt_at` 会通过 Run DTO 暴露用于审计，租约所有者和过期时间不对客户端公开。

### HTTP 与停机边界

兼容 `/process` 接口不再直接调用 Executor。它读取当前 Run，仅在状态仍为 `queued` 时重新通知 Worker，因此不会绕过租约与跨实例领取规则。

优雅停机仍按以下顺序执行：

```text
停止 HTTP 接收新请求
→ 停止 Worker 接收新内存任务
→ 排空已接收任务
→ 关闭数据库
```

进程非正常退出时无需依赖内存清理；新实例会从数据库恢复 `queued` 或租约过期的 `running` Run。

### 修改文件

- `migrations/000014_ask_run_leases.up.sql`、`migrations/000014_ask_run_leases.down.sql`：增加租约、尝试次数、重试时间和可运行索引。
- `internal/app/ask/model.go`、`internal/app/ask/dto.go`：增加内部租约字段和客户端审计字段。
- `internal/app/ask/repository.go`：实现候选扫描、原子领取、续租、持久化重试、过期执行回收和次数耗尽失败。
- `internal/app/ask/service.go`：正常完成和追问恢复时同步清理或重置租约状态。
- `internal/app/ask/worker.go`：接入数据库轮询、领取、心跳续租和执行超时。
- `internal/httpapi/ask/handler.go`：将兼容 Process 接口改为重新投递。
- `internal/app/ask/repository_test.go`、`internal/app/ask/worker_test.go`、`internal/app/ask/state_test.go`、`internal/httpapi/ask_routes_test.go`：覆盖租约竞争、崩溃恢复、迟到提交、重试上限、启动恢复、续租和超时。
- `web/src/types/ask.ts`、`web/src/hooks/ask-reducer.test.ts`：同步 Run 审计字段。

### 验证结果

```text
Go 全部测试通过
go vet 全部通过
Go Ask 和 HTTP race 检测通过
前端 2 个测试文件、12 个用例通过
TypeScript 类型检查通过
本步前端文件 ESLint 0 个错误
迁移 000001 → 000014、000014 回滚及再次升级通过
gofmt 通过
git diff --check 通过
```

完整 ESLint 仍被 `web/src/pages/onboarding/profile.tsx` 的 1 条既有 import 顺序错误阻断；另有 onboarding/profile 和 profile 页面的 3 条既有 Hook 警告，均与本步无关。

### 当前边界

- 当前轮询按单批 20 个候选顺序执行，尚未引入并行消费者。
- 重试间隔为固定 500ms，尚未增加指数退避和随机抖动。
- 租约续期依赖 Executor 响应 context 取消；不响应取消的第三方调用仍需由 Provider 自身设置超时。
- 当前通过数据库行锁和条件更新协调实例，没有引入独立消息中间件。

### 下一步计划

第二十六步接入真实 AI Provider 边界：配置请求超时、结构化输出、错误分类和可重试策略，使 Provider 限流、超时和不可重试响应能够正确映射到现有 Worker 与 Run 状态机。

## 第二十六步：真实 AI Provider 与可重试错误边界

### 本步目标

第 25 步已经建立数据库租约和可恢复 Worker，但 Executor 仍是本地确定性实现，也没有区分 Provider 的暂时故障和永久错误。本步接入 OpenAI Responses API，并让超时、限流和服务不可用进入 Worker 的持久化重试流程。

### Provider 与结构化输出

新增 `internal/platform/ai/openai.go`，使用官方 `openai-go` SDK 调用 Responses API。当前项目使用 Go 1.24，因此固定使用兼容该版本的 `github.com/openai/openai-go v1.12.0`，不升级要求 Go 1.25 的 v3。

Provider 请求使用严格 JSON Schema，输出只允许两种状态：

```text
waiting_input
completed
```

`waiting_input` 必须包含一个非空追问；`completed` 必须包含风险等级、当前判断、观察项、可能原因、家庭行动和升级就医条件。SDK 返回后仍调用应用层 `ValidateAnalysisOutput`，结构或安全表达不符合契约时按 `provider_output_invalid` 失败，不持久化原始模型文本。

Prompt 只包含问题、宠物资料、近期 Turn、当前 Run 消息、近期记录和归一化事件，不发送家庭、用户、Session、Turn、Run、宠物等内部 ID。Prompt 版本升级为 `ask-prompt-v2`，便于后续审计区分占位执行器和真实 Provider。

### 超时与错误分类

Provider 使用独立请求超时，并关闭 SDK 内部重试，避免 SDK 重试与数据库 Worker 重试叠加。应用层新增 `ExecutorError`，统一携带稳定错误码、是否可重试和可选 `Retry-After`。

当前分类规则：

```text
超时、请求取消                 provider_timeout / provider_canceled，可重试
429                            provider_rate_limited，可重试
配额耗尽（EXCEED_TOKEN_QUOTA_LIMIT、QUOTA_EXCEEDED）
                               provider_quota_exhausted，不可重试
409、5xx、网络错误             provider_unavailable，可重试
401、403                       provider_auth_failed，不可重试
400、404、422                  provider_request_invalid，不可重试
非法结构化输出                 provider_output_invalid，不可重试
其他明确的 Provider HTTP 错误  provider_failed，不可重试
```

可重试错误返回 Worker 时，Run 暂时保持 `running`。Worker 随后清理租约、将 Run 恢复为 `queued`、写入 `run.retry_scheduled`，并保存 `next_attempt_at`。429 返回有效 `Retry-After` 且大于默认间隔时优先使用 Provider 等待时间；达到最大尝试次数后仍按现有协议进入 `worker_attempts_exhausted`。

不可重试错误由 Service 直接持久化为 `run.failed`。未使用 `ExecutorError` 的旧 Executor 错误继续映射为 `executor_failed`，避免改变已有应用层契约。

### 配置与装配

新增配置：

```text
AI_PROVIDER
AI_API_KEY
AI_BASE_URL
AI_MODEL
AI_TIMEOUT_SECONDS
```

`AI_ENABLED=false` 时继续装配 `DeterministicExecutor`，保持本地开发和未配置环境的原有行为。开启后必须提供 API Key、模型名和正数超时时间；API Key 会在配置日志中脱敏。默认 Provider 为 `openai`，默认 Base URL 为 `https://api.openai.com/v1`，默认超时为 30 秒。设置 `AI_PROVIDER=hunyuan` 后使用腾讯混元 OpenAI 兼容接口；若未显式设置 Base URL，则使用 `https://api.hunyuan.cloud.tencent.com/v1`，模型可配置为 `hunyuan-turbos-latest` 等混元模型名。

### 修改文件

- `internal/platform/ai/openai.go`、`internal/platform/ai/openai_test.go`：实现 Responses Provider、严格结构化输出、超时和错误分类。
- `internal/app/ask/executor.go`：新增应用层 Executor 错误契约。
- `internal/app/ask/service.go`、`internal/app/ask/service_test.go`：区分可重试与不可重试错误，并升级 Prompt 版本。
- `internal/app/ask/worker.go`、`internal/app/ask/worker_test.go`：支持 Provider `Retry-After`。
- `internal/pkg/config/config.go`、`internal/pkg/config/config_test.go`：增加 AI 配置、校验和密钥脱敏。
- `cmd/api/main.go`：按 `AI_ENABLED` 装配真实 Provider 或确定性 Executor。
- `.env.example`、`config.example.yaml`：补充 AI 配置示例。
- `go.mod`、`go.sum`：增加兼容 Go 1.24 的 OpenAI 官方 SDK。
- `docs/api/ask.md`：补充 Provider 配置和错误重试协议。

### 验证结果

```text
OpenAI Provider 定向测试通过
```

### 当前边界

- 当前只接入文本 Responses API，尚未处理问问图片输入和多模态内容。
- 当前 Prompt 为代码内版本化常量，尚未接入外部模板管理或离线评测数据集。
- Worker 仍使用固定默认退避；只有 Provider 明确返回更长的 `Retry-After` 时覆盖该间隔。
- 本步测试使用本地模拟 HTTP Server，没有向真实 OpenAI 服务发送请求。

### 下一步计划

第二十七步补充 Provider 可观测性和质量评测：记录不含敏感正文的请求耗时、模型名、Token 用量和错误分类，并建立覆盖追问、普通建议、红色规则优先及危险表达拒绝的离线评测集。

## 第二十七步：Provider 可观测性与离线质量评测

### 本步目标

第 26 步已经完成真实 Provider 的调用和错误分类。本步增加可运营的调用指标，并把关键安全边界固定为不访问外部模型即可执行的离线评测。

### Provider 观测数据

`OpenAIExecutor` 增加可注入的观测回调。每次调用结束后只报告结构化元数据：

```text
model
duration
input_tokens
output_tokens
total_tokens
status
error_code
retryable
```

观测回调不接收 Prompt、模型输出、宠物名称、用户输入、Session ID 或 Run ID。主程序将这些字段写入现有 JSON `slog`，用于定位延迟、Token 消耗、Provider 限流和不可重试错误；请求失败也会执行回调，错误码使用应用层稳定分类。

### 离线质量评测

新增 Go 离线评测用例，不调用真实 AI 服务，覆盖：

- 红色规则命中时必须优先升级。
- 否定表达不能误触发红色风险。
- 合法追问结构可以通过输出校验。
- 合法普通分析结构可以通过输出校验。
- 包含“确诊”等危险诊断表达的结果必须被拒绝。

该评测不是模型能力评分，而是上线前的安全和协议回归门槛。后续接入真实模型评测时，应在同一组案例上增加模型响应适配层，并继续复用应用层输出校验。

### 修改文件

- `internal/platform/ai/openai.go`：增加 Provider 观测结构和回调。
- `internal/platform/ai/openai_test.go`：覆盖成功调用 Token 观测和错误分类观测。
- `internal/app/ask/eval_test.go`：增加红色优先、追问、普通分析和危险表达离线评测。
- `cmd/api/main.go`：将观测元数据接入 JSON 日志。
- `docs/ask-agent-development.md`：记录观测字段和评测边界。

### 验证结果

```text
Provider、Ask 和 API 定向测试通过
Go 全量测试、go vet 和相关 race 测试通过
前端测试和 TypeScript 检查未受影响
```

### 当前边界

- 当前观测数据写入结构化日志，尚未接入 Prometheus、OpenTelemetry 或集中式日志查询。
- 当前 Token 成本没有按模型价格换算，也没有用户或家庭维度聚合。
- 离线评测覆盖协议和安全规则，尚未覆盖真实模型的事实准确率、建议有用性和中文表达质量。

### 下一步计划

第 28 步开始前端收尾，优先完成真实设备验收和体验补齐：验证微信开发者工具/真机上的事件流、断线恢复、后台切换和错误重试，再处理图片问问入口。

## 第二十八步：前端事件流生命周期恢复

### 本步目标

异步 Worker 已经成为正常执行路径，前端不能再把网络断开后的“重试”当作重新调用兼容 `/process` 接口。页面进入后台后也不应继续持有流连接；返回前台时需要先用 Snapshot 重新取得权威状态，再继续消费事件。

### 实现内容

`useAskSession` 增加页面展示状态和连接版本。页面隐藏时清理当前流连接；重新展示时通过既有 Snapshot 初始化流程恢复会话、游标和当前 Run，再建立新的事件流。

网络错误提示中的操作改为“重新连接”。它只触发连接重建，不调用 `/process`，因此不会把用户网络问题误当成一次新的 Worker 投递。Reducer 允许 `network_error` 回到 `reconnecting`，页面会显示恢复中的状态。

### 修改文件

- `web/src/hooks/use-ask-session.ts`：接入页面显示/隐藏生命周期和显式连接重建。
- `web/src/hooks/ask-reducer.ts`：允许网络错误进入重连态。
- `web/src/hooks/ask-reducer.test.ts`：覆盖网络错误重连状态转换。
- `web/src/pages/ask/index.tsx`：将网络错误操作改为重新连接。

### 验证结果

```text
前端 2 个测试文件、13 个用例通过
TypeScript 类型检查通过
ESLint 0 个错误
```

### 当前边界

- 微信开发者工具和真机上的真实事件流、断网、后台切换仍需在已登录的设备环境中人工验收。
- 当前图片入口继续禁用；后端尚无问问媒体上传和多模态分析协议，不能提前开放。

### 下一步计划

在微信开发者工具和真机环境按创建、追问、Provider 暂时失败、断网重连、后台切换和重新进入页面的顺序验收文本闭环；通过后再单独设计图片问问接口和前端入口。

## 第二十九步：可验证的分析过程展示

### 本步目标

前端展示分析阶段时，只呈现用户可以核验的处理进度，不展示原始思维链（Chain of Thought）或模型内部推理文本。

### 实现内容

后端在 Run 执行期间持久化 `run.progress` 事件，前端按事件流和 Snapshot 恢复并展示以下阶段：

```text
正在整理宠物的症状描述
已关联宠物资料和近期记录
正在进行风险初筛
正在生成答复
```

每个事件只包含固定的 `stage` 和 `message` 字段，不包含用户输入、模型输出、宠物隐私上下文或内部标识。命中红色风险时，在风险初筛后直接展示升级提示，不产生生成答复阶段。

### 修改文件

- `internal/app/ask/repository.go`、`internal/app/ask/service.go`：持久化执行阶段事件。
- `internal/app/ask/service_test.go`、`internal/httpapi/ask_routes_test.go`：覆盖阶段顺序和接口返回。
- `web/src/types/ask.ts`、`web/src/components/ask/ask-event.tsx`、`web/src/components/ask/ask-event.scss`：增加阶段事件类型和展示样式。
- `web/src/pages/ask/index.tsx`、`web/src/hooks/ask-reducer.test.ts`：接入事件列表并覆盖恢复场景。
- `docs/api/ask.md`：补充 `run.progress` 事件协议。

### 验证结果

已通过：

```text
Ask 与 HTTP 路由 Go 测试
前端测试、TypeScript 类型检查和 ESLint
```

### 当前限制

- 当前仍使用确定性 Executor，阶段事件表示服务端工作流边界，不代表模型逐 token 输出。
- 当前 HTTP 事件流仍是 NDJSON；Provider 流式输出接入后，需要在同一协议中增加安全过滤后的增量事件。
- 阶段文案为固定版本，后续可根据真实 Provider 的执行状态补充更细粒度但仍可验证的进度。

### 下一步计划

第三十步接入 Provider 流式增量，在安全过滤后增加 `assistant.delta` 事件，并让前端按增量事件实现真实打字机效果。

## 第三十步：Provider 流式增量与打字机预览

### 本步目标

在不暴露原始思维链和未校验模型文本的前提下，接入 Provider Responses 流式接口，并让前端在终态结果到达前显示增量预览。

### 实现内容

OpenAI Executor 使用 `Responses.NewStreaming` 接收流。流结束后先解析并通过结构化输出安全校验，再将追问文本或当前判断切分为 `assistant.delta` 事件持久化。前端收到增量时逐段渲染；收到 `assistant.question` 或 `run.completed` 等终态事件后隐藏增量，展示权威结构化结果。

### 安全边界

- 增量来源仅限已通过 `ValidateAnalysisOutput` 的 `question` 或 `current_assessment`。
- 不转发 Provider 原始事件、JSON 片段、推理摘要或内部上下文。
- 增量事件写入和终态事件使用同一 Run 事件序列，Snapshot 和断线恢复无需额外协议。

### 修改文件

- `internal/app/ask/executor.go`、`internal/app/ask/service.go`：增加流式 Executor 能力和 `assistant.delta` 事件持久化。
- `internal/platform/ai/openai.go`：接入 Responses Streaming 并输出安全增量。
- `web/src/types/ask.ts`、`web/src/components/ask/ask-event.tsx`、`web/src/components/ask/ask-event.scss`、`web/src/pages/ask/index.tsx`：增加增量事件渲染和终态替换。
- `docs/api/ask.md`：补充增量事件协议。

### 当前限制

- 为保证安全，增量在完整结果校验后才发出；Provider 网络传输虽为流式，但不会展示未校验的半截 JSON。
- 当前增量仅展示一个安全字段，结构化分析卡片仍以终态事件为准。

### 下一步计划

第三十一步完善前端真实设备验收和增量动画节奏，再评估是否需要将安全的字段级校验前移到流式解析阶段。

## 第三十一步：终态 Snapshot 同步与事件流收敛

### 本步目标

Worker 在后台完成 Run 后，前端事件流收到的终态事件不会携带完整 Run 版本字段。页面需要在终态后重新读取 Snapshot，才能使用最新的 `row_version` 回复追问，并避免已完成会话继续重连。

### 实现内容

`useAskSession` 在收到 `assistant.question`、`run.completed`、`risk.escalated` 或 `run.failed` 后关闭当前流并恢复一次 Snapshot。恢复结果为终态 Run 时不再建立新的事件流；恢复期间忽略旧连接的 `onClose` 和 `onError`，避免重复排队重连。

### 验证结果

```text
前端 2 个测试文件、15 个用例通过
TypeScript 类型检查通过
ESLint 0 个错误，保留 3 条既有警告
```

### 下一步计划

在微信开发者工具和真机环境验证真实 Provider 的创建、追问、额度不足、断网重连、后台切换和重新进入页面；验证通过后再设计图片问问的媒体上传和多模态协议。

## 第三十二步：意图路由与按需上下文

### 本步目标

健康分析流程之前缺少问题类型判断，导致“你好”等闲聊也会加载宠物上下文并调用健康分析 Prompt。失败时页面还会把普通闲聊展示为“分析未完成”。本步在高危规则和健康分析之间增加意图路由，让不同问题进入独立的受控分支。

### 执行流程

```text
用户输入
→ 高危规则
→ Intent Router
   ├─ casual_chat
   ├─ pet_health
   ├─ pet_fact
   ├─ ambiguous
   └─ unsupported
→ 按意图加载上下文
→ 分支终态事件
```

高置信短问候优先使用本地规则，Provider 不可用或额度不足时仍可回复。其他输入在启用 AI 时调用真实 Provider 分类。只有 `pet_health` 分支加载宠物档案、健康资料和近期记录；`pet_fact` 使用确定性记录查询；模糊问题进入追问；闲聊和能力外问题直接返回普通文本。

当前会话表要求 `pet_id` 非空。自动定位接口收到本地规则可确定的简单闲聊时，会使用家庭中的一只宠物完成技术绑定，但执行过程不会读取其档案或健康记录。其他未包含宠物名称的自动定位请求仍保持原有校验，避免健康问题错误关联到默认宠物。

### 事件协议

所有 Run 在路由前写入：

```text
run.progress
stage=intent_routing
message=正在理解你的问题
```

闲聊和能力外问题以 `assistant.completed` 结束，健康分析继续使用 `assistant.question` 或 `run.completed`，事实查询使用 `fact.completed`。前端将 `assistant.completed` 作为流终态，并使用现有打字机组件展示回答。`run.failed` 优先展示服务端返回的稳定错误文案。

### 架构边界

当前实现是“模型路由 + 确定性规则 + 受控工作流”，不是完整 ReAct。模型只能输出受约束的意图和结构化分析结果，不能自行循环选择工具。这样可以先固定医疗风险规则、数据读取边界和事件审计协议，再根据后续工具数量决定是否引入 Planner 或有限状态的工具循环。

### 修改文件

- `internal/app/ask/intent.go`：定义意图、路由输入和本地短问候规则。
- `internal/app/ask/service.go`：增加高危优先、意图分支和按需健康上下文。
- `internal/platform/ai/openai.go`、`internal/platform/ai/hunyuan.go`：接入真实 Provider 意图分类并区分观测操作。
- `internal/httpapi/ask/handler.go`：将 `assistant.completed` 识别为流终态。
- `web/src/types/ask.ts`、`web/src/components/ask/ask-event.tsx`、`web/src/hooks/ask-reducer.ts`、`web/src/hooks/use-ask-session.ts`、`web/src/pages/ask/index.tsx`：接入路由阶段、普通文本终态和稳定失败文案。
- `internal/app/ask/service_test.go`、`internal/platform/ai/hunyuan_test.go`、`internal/httpapi/ask_routes_test.go`、`web/src/hooks/ask-reducer.test.ts`：覆盖路由、上下文边界、终态和恢复行为。

### 当前限制

- 自动定位接口只对本地高置信闲聊放宽宠物名称要求；需要模型判断的无宠物输入仍需先选择宠物或包含宠物名称。
- 意图分类依赖 Provider 时会额外消耗一次模型请求和 Token。
- 当前未实现自主工具循环、长期记忆检索和多模态输入。

### 下一步计划

先在微信开发者工具中验证闲聊、健康问题、事实查询、模糊问题、高危问题和额度错误六条路径，再完善真实模型路由评测和 Provider 降级策略。

## 第三十三步：家庭级查询 Tool

### 本步目标

自动定位接口此前要求问题中出现宠物名称，因此“你知道我家有哪些宠物吗”会在意图路由前返回 400。与此同时，Agent 没有家庭级查询意图和对应的数据读取边界。

### 执行流程

```text
用户输入
→ 高危规则
→ Intent Router
→ family_query
→ list_family_pets
→ family.pets.completed
```

本地高置信规则识别家庭宠物列表问题，使自动定位接口可以先创建会话。由于当前 Session 要求 `pet_id` 非空，会话技术性绑定家庭中的第一只宠物；执行时重新按 `session.family_id` 调用宠物 Repository 的 `List`，返回当前家庭完整宠物列表。

### 架构边界

`list_family_pets` 是受控只读 Tool。模型只能将问题路由到 `family_query`，不能指定任意家庭 ID，也不能直接调用 Repository。该分支不加载健康上下文，不调用健康 Executor，不支持创建、修改或删除宠物。写操作仍需后续增加用户确认、权限、幂等和审计协议。

### 修改文件

- `internal/app/ask/intent.go`、`internal/platform/ai/intent.go`：增加 `family_query` 意图、本地识别和 Provider 结构化枚举。
- `internal/app/ask/family.go`、`internal/app/ask/service.go`：增加家庭宠物列表结果和受控执行分支。
- `web/src/types/ask.ts`、`web/src/components/ask/ask-event.tsx`、`web/src/hooks/ask-reducer.ts`、`web/src/hooks/use-ask-session.ts`、`web/src/pages/ask/index.tsx`：展示列表并将事件识别为终态。
- `internal/app/ask/service_test.go`、`internal/httpapi/ask_routes_test.go`、`internal/platform/ai/intent_test.go`、`web/src/hooks/ask-reducer.test.ts`：覆盖意图、服务、HTTP 契约和前端状态。

### 当前限制

- 当前只支持家庭宠物列表这一项家庭级只读 Tool。
- 尚未实现模型自主工具选择循环，因此仍不是 ReAct。
- 家庭中没有宠物时，自动创建会话会返回“当前家庭暂无宠物”。

### 下一步计划

继续补充按 `pet_id` 查询基础资料、健康资料和近期记录的显式只读 Tool 契约，再根据 Tool 数量决定引入固定 Planner 还是有限状态工具循环。
