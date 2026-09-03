export default defineAppConfig({
  pages: [
    'pages/index/index',
    'pages/calendar/index',
    'pages/ask/index',
    'pages/family/create/index',
    'pages/family/join/index',
    'pages/family/pending/index',
    'pages/family/members/index',
    'pages/pets/detail/index',
    'pages/pets/edit/index',
    'pages/profile/index',
    'pages/profile/edit/index',
    'pages/onboarding/profile',
    'pages/share/index',
    // 注：宠物小册原型单页（pet-manual/pages/index/index）已退役——
    // tabBar 四页已接管其视觉（首页=档案册 / 日历 / 问问），
    // 入口文件保留未删，必要时改回本行即可重新挂载对照。
  ],
  window: {
    backgroundTextStyle: 'light',
    // 全局沉浸式导航（同步文件三 6cb5b16）。三个 tab 页与档案页本就是
    // 页面级 custom（pet-manual 全屏设计）；此开关让 family/pets/share/
    // onboarding 等二级页与文件三行为对齐（内容顶到状态栏下）。
    navigationStyle: 'custom',
    navigationBarBackgroundColor: '#fff',
    navigationBarTitleText: 'WeChat',
    navigationBarTextStyle: 'black',
  },
  tabBar: {
    // 自定义 tabBar：视觉复刻文件二（Pet-Manual）底部导航
    // （玻璃胶囊 app-tabs + 左下角家庭按钮），实现见 src/custom-tab-bar/。
    // 与原型一致仅 3 个 tab（档案/日历/AI）；「我的」页代码保留但
    // 不挂 tabBar 入口（职能由 onboarding 资料页 + ⌂ 家庭中心承载），
    // 后续需要时在 custom-tab-bar TABS 中加回即可。
    // list 仅作路由声明（微信要求 ≥2 项），实际渲染由自定义组件接管。
    custom: true,
    color: '#4a6489',
    selectedColor: '#fffefb',
    backgroundColor: '#ffffff',
    borderStyle: 'white',
    list: [
      {
        pagePath: 'pages/index/index',
        text: '宠物档案',
      },
      {
        pagePath: 'pages/calendar/index',
        text: '宠物日历',
      },
      {
        pagePath: 'pages/ask/index',
        text: 'AI 助手',
      },
    ],
  },
})
