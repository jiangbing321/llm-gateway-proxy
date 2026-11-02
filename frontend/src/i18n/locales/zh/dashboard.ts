// 仪表盘
export default {
  health: {
    title: '厂商健康',
    checking: '检测中…',
    checkNow: '立即检测',
    down: '不可用',
  },
  proxy: {
    title: '本地代理（三协议）',
    desc: '同时暴露 {bOpenAI}（{cCC}）、{bAnthropic}（{cMsg}）与 {bResp}（{cRes}）端点；按 model 路由到对应厂商（见「厂商管理」），跨协议自动转换（含工具调用与流式）。Claude Code 可设 {cEnv} 指向本代理。',
    bOpenAI: 'OpenAI 格式',
    bAnthropic: 'Anthropic 格式',
    bResp: 'Responses 格式',
    running: '运行中',
    stopped: '已停止',
    stop: '停止代理',
    start: '启动代理',
    startedToast: '代理已启动',
    stoppedToast: '代理已停止',
  },
  wsl: {
    title: 'WSL 访问地址（远程 WSL 项目用）',
    desc: 'VSCode Remote-WSL 里的扩展访问宿主机代理时，Base URL 填：',
    tip: 'WSL 内也可用 {code} 查看网关 IP（即宿主机地址）。',
  },
  curl: {
    title: 'curl 示例',
    desc: '模型 ID 见「模型列表」页；示例中的 MODEL-ID 请替换为实际配置的模型名。',
    python: 'Python OpenAI SDK：',
    sampleMsg: '你好',
    noKeyNote: '本地代理未设 key 时任意值即可',
  },
  info: {
    title: '说明',
    l1: '· 监听地址在「代理设置」中配置（默认 127.0.0.1 仅本机，0.0.0.0 供 WSL/局域网）。',
    l2: '· 厂商与模型在「厂商管理」中配置，按模型名路由，跨协议自动转换。',
    l3: '· 关闭窗口将驻留系统托盘，代理继续运行；托盘菜单可退出。',
  },
}
