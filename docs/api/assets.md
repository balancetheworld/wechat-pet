# 资产接口

## 上传资产

`POST /api/v1/assets/upload`

请求需携带 JWT。multipart 字段为 `type` 和 `file`。头像类文件上限为 5 MiB，其他媒体上限为 50 MiB。

## 读取本地资产

`GET /api/v1/uploads/:asset_id`

请求需携带 JWT。小程序图片组件无法设置请求头时，可通过 `access_token` 查询参数传递当前登录令牌。服务端只允许资产所属用户或其当前家庭成员读取资源。
