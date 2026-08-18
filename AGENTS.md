# 项目开发协作规范

## 1. 项目概述

本项目是一个宠物家庭管理微信小程序。

项目采用单仓库双工程结构：

- `client`：Taro + React + TypeScript 微信小程序
- `server`：Go + Gin 后端 API
- `docs`：产品、架构、数据库、接口和流程文档

前端和后端放在同一个仓库中，但必须保持职责独立。

## 2. 语言要求

项目文档、提交说明和开发交流默认使用简体中文。

代码中的变量名、函数名、类型名和接口字段使用英文。

数据库字段使用 snake_case。

前端 TypeScript 字段使用 camelCase。

后端返回 JSON 字段使用 snake_case。

## 3. 修改原则

1. 修改前先阅读目标文件和相邻模块。
2. 只修改当前需求涉及的文件。
3. 不进行无关重构。
4. 不随意更改已有变量名、函数名和目录结构。
5. 不批量格式化无关文件。
6. 不修改构建产物。
7. 不修改 `client/dist`。
8. 不修改 `client/.taro`。
9. 不提交真实环境变量和密钥。
10. 不执行未经确认的删除操作。

## 4. 前端分层

前端目录职责如下：

```text
client/src/pages
页面组件、页面生命周期、页面级数据加载

client/src/components
可复用 UI 组件

client/src/services
HTTP 请求和业务 API 封装

client/src/stores
跨页面共享状态

client/src/hooks
可复用的 React Hook

client/src/types
接口类型和业务类型

client/src/constants
路由、错误码、枚举和存储 key

client/src/utils
纯工具函数

client/src/styles
全局样式、变量和混入