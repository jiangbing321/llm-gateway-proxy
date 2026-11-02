// 模型列表
export default {
  title: '网关模型列表',
  desc: '来自 GET /v1/models（聚合全部启用的厂商）。客户端请求的 model 命中即路由到对应厂商。',
  loading: '加载中…',
  refresh: '刷新模型列表',
  count: '共 {n} 个',
  th: {
    action: '操作',
    name: '名称',
    provider: '所属厂商',
    type: '类型',
  },
  empty: '尚未加载。请先登录，再点击「刷新模型列表」。',
  toast: {
    loaded: '已加载 {n} 个模型',
    copied: '已复制：{id}',
  },
}
