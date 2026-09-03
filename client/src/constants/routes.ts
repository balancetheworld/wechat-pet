export const routes = {
  // tabs：底部 tabBar 实际渲染的 3 个页面（与文件二原型一致）
  tabs: {
    home: '/pages/index/index',
    calendar: '/pages/calendar/index',
    ask: '/pages/ask/index',
  },
  // pages：普通非tab页面，需要navigateTo跳转打开的子页面
  pages: {
    createFamily: '/pages/family/create/index',
    joinFamily: '/pages/family/join/index',
    pendingFamily: '/pages/family/pending/index',
    familyMembers: '/pages/family/members/index',
    petDetail: '/pages/pets/detail/index',
    petEdit: '/pages/pets/edit/index',
    // 「我的」页代码保留、暂不使用（无 tabBar 入口），
    // 从普通页面路由恢复，需要时用 navigateTo 打开。
    profile: '/pages/profile/index',
    profileEdit: '/pages/profile/edit/index',
    profileOnboarding: '/pages/onboarding/profile',
    share: '/pages/share/index',
  },
} as const

export function withQuery(path: string, params: Record<string, string | number | undefined>) {
  const query = Object.entries(params)
    .filter(([, value]) => value !== undefined)
    .map(([key, value]) => `${encodeURIComponent(key)}=${encodeURIComponent(String(value))}`)
    .join('&')

  return query ? `${path}?${query}` : path
}
