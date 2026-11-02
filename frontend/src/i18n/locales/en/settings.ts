import type { MessageSchema } from '../zh'

const settings: MessageSchema['settings'] = {
  title: 'Proxy Settings',
  autoStart: 'Launch at Windows sign-in (starts in the background tray; the proxy turns on with the app)',
  autoStartOn: 'Auto-start enabled',
  autoStartOff: 'Auto-start disabled',
  autoProxy: 'Start the proxy on launch',
  bind: 'Listen address',
  bindLoopback: '127.0.0.1 — localhost only (default, safest)',
  bindAll: '0.0.0.0 — all interfaces (reachable from WSL / LAN)',
  bindWarn: 'All interfaces are open without a local API key — any device on the LAN can use your account to call models. Set an API key below.',
  port: 'Listen port',
  apiKey: 'Local API key (empty = no auth; when set, requests must send x-api-key or Bearer)',
  apiKeyPh: 'e.g. sk-local-xxxx (optional)',
  genKey: 'Generate',
  saving: 'Saving…',
  saveApply: 'Save and apply',
  saved: 'Config saved and applied',
  language: {
    label: 'Language',
    auto: 'Follow system',
    zh: '中文',
    en: 'English',
    hint: 'UI and notifications apply immediately; the tray menu updates after restart',
  },
}

export default settings
