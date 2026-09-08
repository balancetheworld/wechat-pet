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
