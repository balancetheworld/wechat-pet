# 宠物问问接口

以下接口均需要 JWT，并要求当前用户属于一个 active 家庭。所有 `pet_id`、`session_id` 和 `run_id` 都会在服务端按当前家庭重新校验。

## 创建问问会话

`POST /api/v1/pets/:pet_id/ask/sessions`

或由问题中的宠物名称自动定位单只或多只宠物：

`POST /api/v1/ask/sessions`

请求必须携带：

```http
Idempotency-Key: ask-client-generated-key
```

幂等键不能为空且不能超过 128 个字符。同一用户、同一操作使用相同幂等键和相同请求时返回首次结果；相同幂等键用于不同请求时返回 409。

请求体：

```json
{
  "input": "最近两天食欲不太好"
}
```

当前版本先在事务中创建排队中的 Run，再投递到进程内 Worker 异步执行。接口响应仍返回事务提交时的 `queued` 快照，后续状态通过 Snapshot 或事件流获取。

自动定位接口通常要求问题中包含宠物名称。对于“你好”等本地规则可确定的简单闲聊，以及“我家有哪些宠物”等家庭宠物列表查询，服务端会使用家庭中的一只宠物完成现有会话数据约束；该绑定不表示回答只与该宠物有关。家庭宠物列表查询会按当前登录用户的 `family_id` 读取完整列表，不读取其他家庭数据。

响应中的关键字段：

```json
{
  "code": 0,
  "data": {
    "session": {
      "id": "session-id",
      "pet_id": "pet-id",
      "status": "active",
      "risk_level": "unknown",
      "turn_count": 1
    },
    "run": {
      "id": "run-id",
      "turn_id": "turn-id",
      "run_index": 0,
      "row_version": 1,
      "clarification_count": 0,
      "attempt_count": 0,
      "status": "queued",
      "risk_level": "unknown",
      "next_attempt_at": null
    },
    "events": [
      {
        "sequence": 1,
        "type": "run.queued",
        "data": {}
      }
    ]
  }
}
```

## 执行问问 Run

`POST /api/v1/ask/sessions/:session_id/runs/:run_id/process`

该接口保留为兼容和人工重试入口。正常创建和 Reply 流程不需要调用；接口读取当前 Run，若仍为 `queued` 则重新投递 Worker，不会在 HTTP 请求内直接执行。Run 先经过高危规则和意图路由，再按 `casual_chat`、`pet_health`、`pet_fact`、`family_query`、`ambiguous` 或 `unsupported` 分支执行。

高置信短问候和家庭宠物列表查询由本地规则直接识别。其他输入在启用 AI 时由 Provider 分类；只有 `pet_health` 分支读取宠物档案、健康信息和近期记录。`pet_fact` 只查询对应记录，`family_query` 只调用家庭宠物列表读取能力，闲聊和能力外问题不会加载宠物健康上下文。当前是受控工作流，不是允许模型自主选择任意工具的 ReAct Agent。

可能的 Run 状态：

```text
waiting_input
completed
escalated
failed
```

Worker 每次执行前会领取数据库租约。基础设施错误会写入 `run.retry_scheduled` 并在 `next_attempt_at` 后重试；进程中断留下的过期 `running` Run 会写入 `run.recovered` 后重新执行。超过最大尝试次数会写入 `run.failed`，`error_code` 为 `worker_attempts_exhausted`。

`AI_ENABLED=false` 时使用本地确定性 Executor；`AI_ENABLED=true` 时根据 `AI_PROVIDER` 选择 OpenAI Responses API 或腾讯混元 OpenAI 兼容的 Chat Completions API，并要求配置 `AI_API_KEY`、`AI_MODEL` 和正数 `AI_TIMEOUT_SECONDS`。`AI_BASE_URL` 可选；`AI_PROVIDER=hunyuan` 时留空会使用 `https://api.hunyuan.cloud.tencent.com/v1`，OpenAI 留空会使用 `https://api.openai.com/v1`。Provider SDK 内部重试关闭，由 Worker 统一控制持久化重试。

Provider 错误按以下规则处理：

- 超时、请求取消、409、429、5xx 和网络错误可重试，Run 保持 `running`，随后由 Worker 持久化为待重试状态；Provider 返回 `EXCEED_TOKEN_QUOTA_LIMIT` 或 `QUOTA_EXCEEDED` 时映射为 `provider_quota_exhausted`，不可重试。
- 429 响应存在有效 `Retry-After` 时，Worker 使用该等待时间；否则使用默认重试间隔。
- 401、403、400、404、422 和非法结构化输出不可重试，Run 直接进入 `failed`。
- 客户端只收到稳定的 `error_code` 和通用失败提示，不返回 Provider 原始错误内容。

Provider 返回配额耗尽错误时，Run 的 `error_code` 为 `provider_quota_exhausted`，客户端显示“AI 服务额度暂时不可用，请检查额度后重试”。

## 回复问问追问

`POST /api/v1/ask/sessions/:session_id/runs/:run_id/reply`

只有状态为 `waiting_input` 的 Run 可以接收回答。请求必须携带新的 `Idempotency-Key`，回答成功后原 Run 恢复为 `queued`，不会创建新的 Turn 或 Run。

请求体：

```json
{
  "input": "从今天早上开始，已经吐了两次",
  "expected_version": 3
}
```

`expected_version` 必须等于当前 Run 的 `row_version`。事务成功后：

```text
waiting_input(row_version=N)
→ queued(row_version=N+1, clarification_count+1)
```

同一事务还会写入用户回答、追加新的 `run.queued` 事件和幂等结果，提交后将新版本 Run 投递到 Worker。回答内容不能为空，长度不能超过 4000 个字符。最多允许三次追问补充；达到上限后若 Agent 仍要求追问，Run 会收敛为 `failed`。

## 查询问问会话

`GET /api/v1/ask/sessions/:session_id`

只返回当前家庭中的会话。不存在或不属于当前家庭时返回对应错误。

## 查询完整会话 Snapshot

`GET /api/v1/ask/sessions/:session_id/snapshot`

返回 Session、绑定宠物、全部 Turn、每个 Turn 的全部 Run、选中 Run 快捷字段、Run 消息、Run 事件和 `event_cursors`。`runs` 中每项包含 `run`、`messages` 和 `events`；快捷字段对应 `turn.selected_run_id`。`messages` 包含 `role`、`content` 和 `created_at`，用于恢复同一 Run 内的原始问题、Agent 追问和用户补充回答。前端页面恢复时以该接口为权威状态，再从当前 Run 的最新事件游标继续消费增量。

## 查询问问事件

`GET /api/v1/ask/sessions/:session_id/runs/:run_id/events?after=1`

`after` 为可选事件序号游标，默认从 0 开始。服务端会同时验证 Session 和 Run 的归属关系。

响应事件字段：

```json
{
  "run_id": "run-id",
  "sequence": 2,
  "type": "run.started",
  "data": {},
  "created_at": "2026-09-08T00:00:00Z"
}
```

执行过程中会追加 `run.progress` 事件，用于展示可验证的分析阶段，不暴露模型原始思维链、用户原文或隐私上下文：

```json
{
  "run_id": "run-id",
  "sequence": 3,
  "type": "run.progress",
  "data": {
    "stage": "risk_checking",
    "message": "正在进行风险初筛"
  },
  "created_at": "2026-09-08T00:00:00Z"
}
```

当前阶段按执行顺序为：

```text
intent_routing      正在理解你的问题
input_reviewing     正在整理宠物的症状描述
context_ready       已关联宠物资料和近期记录
risk_checking       正在进行风险初筛
response_generating 正在生成答复
```

所有输入都会产生 `intent_routing`。健康分析才会继续产生 `input_reviewing`、`context_ready`、`risk_checking` 和 `response_generating`；命中立即就医风险时会直接产生 `risk.escalated`，不会读取健康上下文或发送 `response_generating`。

闲聊和能力外问题使用 `assistant.completed` 返回普通文本答复：

```json
{
  "run_id": "run-id",
  "sequence": 4,
  "type": "assistant.completed",
  "data": {
    "answer": "你好，我可以陪你聊聊，也可以帮你查看宠物记录或整理健康问题。",
    "intent": "casual_chat"
  },
  "created_at": "2026-09-08T00:00:00Z"
}
```

家庭宠物列表查询使用 `family.pets.completed` 返回当前家庭中的宠物：

```json
{
  "run_id": "run-id",
  "sequence": 4,
  "type": "family.pets.completed",
  "data": {
    "pets": [
      {"pet_id": "pet-1", "pet_name": "旺仔"},
      {"pet_id": "pet-2", "pet_name": "球球"}
    ]
  },
  "created_at": "2026-09-08T00:00:00Z"
}
```

Provider 流式结果在完整结构化输出通过安全校验后，会按安全字段追加 `assistant.delta` 事件。增量只包含追问文本或当前判断，不包含原始 JSON、模型推理过程或未校验内容；Run 进入终态后，前端以 `assistant.question`、`assistant.completed` 或 `run.completed` 结果替换增量预览。

```json
{
  "run_id": "run-id",
  "sequence": 4,
  "type": "assistant.delta",
  "data": {"delta": "目前需要密切观察"},
  "created_at": "2026-09-08T00:00:00Z"
}
```

## 流式读取问问事件

`GET /api/v1/ask/sessions/:session_id/runs/:run_id/events/stream?after=1`

响应类型为 `application/x-ndjson`，每一行是一个完整事件对象，不使用公共 JSON 响应包装。连接结束后客户端使用最新 `sequence` 作为 `after` 重连。
