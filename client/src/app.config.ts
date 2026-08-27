export default defineAppConfig({
  pages: [
    'pages/index/index',
    'pages/calendar/index',
    'pages/ask/index',
    'pages/profile/index',
    'pages/family/create/index',
    'pages/family/join/index',
    'pages/family/pending/index',
    'pages/family/members/index',
    'pages/pets/detail/index',
    'pages/pets/edit/index',
    'pages/profile/edit/index',
    'pages/onboarding/profile',
    'pages/share/index',
  ],
  window: {
    backgroundTextStyle: 'light',
    navigationBarBackgroundColor: '#fff',
    navigationBarTitleText: 'WeChat',
    navigationBarTextStyle: 'black',
  },
  tabBar: {
    color: '#666',
    selectedColor: '#222',
    backgroundColor: '#fff',
    borderStyle: 'black',
    list: [
      {
        pagePath: 'pages/index/index',
        text: '首页',
      },
      {
        pagePath: 'pages/calendar/index',
        text: '日历',
      },
      {
        pagePath: 'pages/ask/index',
        text: '问问',
      },
      {
        pagePath: 'pages/profile/index',
        text: '我的',
      },
    ],
  },
})
