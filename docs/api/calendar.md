# 日历与提醒接口

以下接口均需要 JWT，并要求当前用户属于一个 active 家庭。`family_id` 取自登录态，不接受客户端传入；路径与请求体中的 `pet_id`、`record_id`、`reminder_id` 都会在服务端按当前家庭重新校验。

所有响应使用统一信封：

```json
{
  "code": 0,
  "msg": "ok",
  "data": {},
  "request_id": "request-id"
}
```

失败时 `code` 为非 0 业务码，`msg` 为对应提示，`data` 为 `null`。

## 查询月度标记

`GET /api/v1/calendar/months/:month?pet_id=:pet_id`

`month` 必须为 `YYYY-MM`；`pet_id` 可选，省略时返回当前家庭全部宠物的标记。

```json
{
  "month": "2026-10",
  "pet_id": "pet-id",
  "days": [
    {
      "date": "2026-10-01",
      "has_medical_record": true,
      "has_daily_record": false,
      "has_pending_reminder": true
    }
  ]
}
```

## 查询某日详情

`GET /api/v1/calendar/days/:date`

`date` 必须为 `YYYY-MM-DD`。响应包含当日汇总、待办提醒和记录；`records[].media[].url` 是服务端生成的受保护访问地址。

```json
{
  "date": "2026-10-01",
  "summary": {
    "record_count": 1,
    "pending_reminder_count": 1
  },
  "reminders": [],
  "records": [
    {
      "id": "record-id",
      "category": "medical",
      "medical_type": "vaccine",
      "custom_medical_type": "",
      "content": "接种狂犬疫苗",
      "occurred_at": "2026-10-01T10:00:00+08:00",
      "pet": {
        "id": "pet-id",
        "name": "团子",
        "avatar_asset_id": "asset-id"
      },
      "media": [],
      "created_by": {
        "user_id": "user-id",
        "nickname": "成员",
        "avatar_asset_id": ""
      },
      "reminder": null,
      "reminders": []
    }
  ]
}
```

## 创建日历记录

`POST /api/v1/calendar/records`

请求体：

```json
{
  "category": "medical",
  "medical_type": "vaccine",
  "custom_medical_type": "",
  "pet_id": "pet-id",
  "content": "接种狂犬疫苗",
  "media_asset_ids": [],
  "occurred_at": "2026-10-01T10:00:00+08:00",
  "reminders": [
    {
      "reminder_date": "2026-10-22",
      "repeat_type": "once",
      "repeat_interval_days": null,
      "advance_days": 3,
      "notification_channels": ["in_app", "push"]
    }
  ]
}
```

字段约束：

- `category` 仅支持 `medical`（医疗）和 `daily`（日常）。
- `pet_id` 不能为空，且必须属于当前家庭。
- 日常记录必须填写 `content` 或 `media_asset_ids` 之一，且不能携带医疗类型或提醒。
- `content` 最长 2000 个字符；`media_asset_ids` 最多 9 张，服务端会校验图片属于当前家庭。
- `occurred_at` 为 RFC3339；省略时取当前时间。
- 医疗记录的 `medical_type` 支持 `vaccine`、`deworming`、`checkup`、`visit`、`medication`、`other`；`other` 必须填写 `custom_medical_type`（最长 50 个字符），其他类型不能填写该字段。
- 提醒可用 `reminder`（单条）或 `reminders`（多条）表达，两者不能同时出现。
- `reminder_date` 为 `YYYY-MM-DD`；`repeat_type` 支持 `once`、`monthly`、`yearly`、`custom_days`；`custom_days` 必须给出大于 0 的 `repeat_interval_days`，其他周期不能给出该字段。
- `advance_days` 省略时默认 3，允许 0 到 30。
- `notification_channels` 省略时默认 `["in_app"]`，支持 `in_app` 与 `push`。

响应为创建后的记录，结构与「查询某日详情」中的 `records` 条目一致。

## 更新日历记录

`PATCH /api/v1/calendar/records/:record_id`

部分更新，至少给出一个字段，未出现的字段保持原值。

```json
{
  "content": "已改为接种第二针",
  "occurred_at": "2026-10-02T10:00:00+08:00",
  "media_asset_ids": ["asset-id"]
}
```

- `content` 去掉首尾空白后最长 2000 个字符。
- `media_asset_ids` 省略或为 `null` 表示图片不变，空数组表示清空图片，非空数组表示整体替换。
- 请求体可以为空时按「没有可更新字段」返回参数错误。
- 记录不存在或不属于当前家庭时返回 404。

响应为更新后的记录，结构与创建接口一致。

## 删除日历记录

`DELETE /api/v1/calendar/records/:record_id`

软删除，响应 `data` 为空对象 `{}`。记录不存在或不属于当前家庭时返回 404。

## 记录提醒订阅结果

`POST /api/v1/calendar/subscriptions`

小程序调用 `wx.requestSubscribeMessage` 后回调该接口，把用户授权结果回传服务端。请求体可以为空，视为 `accepted=false`。

```json
{
  "accepted": true
}
```

- `accepted=true` 时，服务端给当前用户与提醒模板累加一次推送配额，并返回当前剩余次数。
- `accepted=false` 时不修改配额，只返回当前剩余次数，供前端展示。
- 服务端未配置 `WECHAT_REMINDER_TEMPLATE_ID` 时返回 400「微信提醒未配置」。

```json
{
  "remaining": 1
}
```

## 完成待办提醒

`POST /api/v1/calendar/reminders/:reminder_id/complete`

请求体可以为空；`completed_at` 省略时取当前时间。

```json
{
  "completed_at": "2026-10-22T09:30:00+08:00",
  "content": "已完成接种",
  "media_asset_ids": []
}
```

`content` 最长 2000 个字符，`media_asset_ids` 规则与创建记录一致。响应中的 `completed_record` 与 `completed_reminder` 结构分别与记录、提醒一致；按周期产生下一条提醒时 `next_reminder` 不为空，否则为 `null`。

```json
{
  "completed_record": {},
  "completed_reminder": {},
  "next_reminder": null
}
```

## 推送派发与配额

提醒推送由服务端定时任务派发，不提供手动触发接口：

- 只派发 `status=pending`、包含 `push` 渠道且已到提醒日期的提醒。
- 当前用户或模板没有剩余配额、或用户没有 `openid` 时跳过该条提醒，不发送消息。
- 单条提醒最多尝试 3 次，超过后标记为 `failed`。
- 状态为 `sending` 且超过 5 分钟未落定的提醒可被重新领取，避免进程中断后永久卡在该状态。
