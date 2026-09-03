/**
 * API 全局配置
 * 接口根路径：/api/v1（见接口文档第 0 节）
 */

// 后端 API 基地址
// 本地联调：双端直连 wechat-pet 后端（Gin，监听 127.0.0.1:8080）
// 正式上线：小程序环境需改为已备案的 https 域名
export const API_BASE_URL = 'http://127.0.0.1:8080/api/v1'

// 后端是否已就绪
// false：请求层直接走本地 mock 回退，不发任何网络请求
//       （避免小程序端未配置合法域名时报错、H5 端连接失败）
// true：正式对接后端，同时确保 API_BASE_URL 已改为真实地址、
//       小程序后台已配置 request 合法域名
export const API_ENABLED = true

// Token 在 storage 中的 key
// 与主项目 stores/auth-store.ts 的 storageKey 保持一致，
// 使 pet-manual 请求自动带上主项目登录后的 token
export const TOKEN_KEY = 'auth_token'

// 请求超时（毫秒）
export const REQUEST_TIMEOUT = 15000

// 分页默认值
export const DEFAULT_PAGE_SIZE = 20
