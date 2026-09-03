export default defineAppConfig({
  pages: [
    'pages/onboarding/profile',
    'pages/index/index',
    'pages/calendar/index',
    'pages/ask/index',
    'pages/profile/index',
    'pages/account/index',
    'pages/family/create/index',
    'pages/family/join/index',
    'pages/family/pending/index',
    'pages/family/members/index',
    'pages/pets/detail/index',
    'pages/pets/edit/index',
    'pages/profile/edit/index',
    'pages/share/index',
  ],
  window: {
    backgroundTextStyle: 'light',
    navigationStyle: 'custom',
    navigationBarBackgroundColor: '#fff',
    navigationBarTitleText: 'WeChat',
    navigationBarTextStyle: 'black',
  },
  tabBar: {
    custom: true,
    color: '#666',
    selectedColor: '#222',
    backgroundColor: '#fff',
    borderStyle: 'black',
    list: [
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
        text: '档案',
      },
    ],
  },
})
