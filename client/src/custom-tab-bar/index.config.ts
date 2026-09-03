/* custom-tab-bar 组件配置（Taro 类型中仅声明了 definePageConfig 全局，
   组件配置用普通对象导出即可，效果等价） */
export default {
  component: true,
  // 样式隔离：阻止 pages 引入的 pet-manual/proto.scss 全局样式
  // （.app-tabs grid 布局等）覆盖组件内部样式。
  styleIsolation: 'isolated',
}
