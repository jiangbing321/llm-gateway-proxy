import type { MessageSchema } from '../zh'

const nav: MessageSchema['nav'] = {
  group: {
    gateway: 'Gateway',
    tools: 'Tools',
  },
  dashboard: 'Dashboard',
  providers: 'Providers',
  models: 'Models',
  stats: 'Usage',
  playground: 'Playground',
  recordings: 'Recordings',
  logs: 'Request Logs',
  settings: 'Proxy Settings',
}

export default nav
