export const routes = {
  // tabs：存放底部tabBar对应的页面
  tabs: {
    home: '/pages/index/index',
    calendar: '/pages/calendar/index',
    ask: '/pages/ask/index',
    profile: '/pages/profile/index',
  },
  // pages：普通非tab页面，需要navigateTo跳转打开的子页面
  pages: {
    createFamily: '/pages/family/create/index',
    joinFamily: '/pages/family/join/index',
    petDetail: '/pages/pets/detail/index',
    petEdit: '/pages/pets/edit/index',
    profileEdit: '/pages/profile/edit/index',
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
