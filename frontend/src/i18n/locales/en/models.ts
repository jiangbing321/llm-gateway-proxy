import type { MessageSchema } from '../zh'

const models: MessageSchema['models'] = {
  title: 'Model list',
  desc: 'From GET /v1/models (aggregated across all enabled providers). A request whose model matches is routed to that provider.',
  loading: 'Loading…',
  refresh: 'Refresh models',
  count: '{n} total',
  th: {
    action: 'Actions',
    name: 'Name',
    provider: 'Provider',
    type: 'Type',
  },
  empty: 'Not loaded yet. Sign in first, then click "Refresh models".',
  toast: {
    loaded: 'Loaded {n} models',
    copied: 'Copied: {id}',
  },
}

export default models
