import type { MessageSchema } from '../zh'

const dashboard: MessageSchema['dashboard'] = {
  health: {
    title: 'Provider health',
    checking: 'Checking…',
    checkNow: 'Check now',
    down: 'Down',
  },
  proxy: {
    title: 'Local proxy (3 protocols)',
    desc: 'Exposes {bOpenAI} ({cCC}), {bAnthropic} ({cMsg}) and {bResp} ({cRes}) endpoints; routes by model to the matching provider (see Providers) with automatic cross-protocol conversion (tool calls and streaming included). Point Claude Code at this proxy via {cEnv}.',
    bOpenAI: 'OpenAI-format',
    bAnthropic: 'Anthropic-format',
    bResp: 'Responses-format',
    running: 'Running',
    stopped: 'Stopped',
    stop: 'Stop proxy',
    start: 'Start proxy',
    startedToast: 'Proxy started',
    stoppedToast: 'Proxy stopped',
  },
  wsl: {
    title: 'WSL access URL (for remote WSL projects)',
    desc: 'For extensions in VSCode Remote-WSL, set Base URL to the host proxy address:',
    tip: 'Inside WSL, {code} also shows the gateway IP (i.e. the host address).',
  },
  curl: {
    title: 'curl example',
    desc: 'Model IDs are listed on the Models page; replace MODEL-ID in the examples with a real model name.',
    python: 'Python OpenAI SDK:',
    sampleMsg: 'hello',
    noKeyNote: 'any value works when no key is set',
  },
  info: {
    title: 'Notes',
    l1: '· Listen address is configured in Proxy Settings (127.0.0.1 localhost-only by default; 0.0.0.0 for WSL/LAN).',
    l2: '· Providers and models are configured in Providers; routing is by model name with automatic cross-protocol conversion.',
    l3: '· Closing the window keeps the app in the system tray with the proxy running; quit from the tray menu.',
  },
}

export default dashboard
