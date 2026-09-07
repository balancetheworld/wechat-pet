# 宠物家庭小程序

一个用于记录宠物档案、健康信息、成长足迹、生日纪念和家庭日历的微信小程序。

项目包含可运行的微信小程序前端和 Go API 后端。

## 项目结构

```text
pet-family-miniapp
├── web     Taro + React + TypeScript 微信小程序
├── cmd     Go + Gin API 入口
├── internal Go 后端业务实现
├── migrations 数据库迁移
├── docs    项目设计、数据库、接口和流程文档
└── deployments
    ├── compose
    └── nginx


开发规范
页面不能直接调用 Taro.request。
所有请求必须经过 web/src/services。
handler 不能直接操作数据库。
service 负责业务规则和权限判断。
repository 只负责数据库访问。
所有家庭数据必须通过 family_id 隔离。
所有需要登录的接口必须验证 JWT。
所有需要家庭权限的接口必须验证 active 成员关系。
数据库结构必须通过 migration 管理。
新增接口时必须同步更新接口文档和 OpenAPI 文件（当前仓库尚未维护 OpenAPI 文件）。
新增数据库字段时必须同步更新数据库文档和 migration。
AI 将来只能通过 service 层修改业务数据。
AppSecret、JWT secret 和对象存储密钥不能提交到 Git。

生产环境需配置 `TARO_APP_API_BASE_URL` 和 `PUBLIC_BASE_URL`；后者用于生成本地媒体的受保护访问地址。
