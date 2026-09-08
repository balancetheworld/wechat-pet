# 宠物问问接口

以下接口均需要 JWT，并要求当前用户属于一个 active 家庭。所有 `pet_id`、`session_id` 和 `run_id` 都会在服务端按当前家庭重新校验。

## 创建问问会话

`POST /api/v1/pets/:pet_id/ask/sessions`

请求体：

```json
{
  "input": "最近两天食欲不太好"
}
```

当前版本只创建排队中的 Run，不调用真实模型。

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
      "status": "queued",
      "risk_level": "unknown"
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

当前使用确定性 Executor，成功后会进入 `waiting_input`，并返回一个追问事件。后续接入真实 Agent 后，接口路径保持不变。

可能的 Run 状态：

```text
waiting_input
completed
escalated
failed
```

## 回复问问追问

`POST /api/v1/ask/sessions/:session_id/runs/:run_id/reply`

只有状态为 `waiting_input` 的 Run 可以接收回答。接口会在当前会话中创建新的 Turn 和 `queued` Run，并返回 `run.queued` 事件；随后调用执行接口处理新 Run。

请求体：

```json
{
  "input": "从今天早上开始，已经吐了两次"
}
```

每个会话最多三轮 Turn。回答内容不能为空，长度不能超过 4000 个字符。会话结束、Run 不在等待输入状态或超过轮次上限时返回冲突错误。

## 查询问问会话

`GET /api/v1/ask/sessions/:session_id`

只返回当前家庭中的会话。不存在或不属于当前家庭时返回对应错误。

## 查询问问事件

`GET /api/v1/ask/sessions/:session_id/runs/:run_id/events?after=1`

`after` 为可选事件序号游标，默认从 0 开始。服务端会同时验证 Session 和 Run 的归属关系。

响应事件字段：

```json
{
  "sequence": 2,
  "type": "run.started",
  "data": {},
  "created_at": "2026-09-08T00:00:00Z"
}
```
