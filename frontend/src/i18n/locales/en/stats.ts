import type { MessageSchema } from '../zh'

const stats: MessageSchema['stats'] = {
  title: 'Usage stats',
  range1: '1 day',
  range7: '7 days',
  range30: '30 days',
  stat: {
    requests: 'Requests',
    tokens: 'Total tokens',
    inOut: 'Input / Output',
    cost: 'Est. cost',
    errRate: 'Error rate',
  },
  todayTag: ' (Today)',
  chartTip: '{date}{today}\n{req} requests · ↑{in} ↓{out} · ¥{cost}',
  daily: {
    title: 'Daily usage',
    viewChart: 'Chart',
    viewTable: 'Table',
  },
  th: {
    date: 'Date',
    provider: 'Provider',
    model: 'Model',
    requests: 'Requests',
    errors: 'Errors',
    input: 'Input tokens',
    output: 'Output tokens',
    cost: 'Cost',
  },
  legendIn: 'Input tokens',
  legendOut: 'Output tokens',
  emptyDays: 'No data yet. Stats are collected automatically as requests go through the proxy.',
  emptyRows: 'No data.',
  detailTitle: 'Breakdown (provider × model)',
  pricing: {
    title: 'Pricing rules',
    add: 'Add',
    desc: 'Matched in order (wildcard *), first hit wins; unmatched models count as free. Unit: CNY / 1M tokens.',
    thProvider: 'Provider pattern',
    thModel: 'Model pattern',
    thInput: 'Input ¥/1M',
    thOutput: 'Output ¥/1M',
    thNote: 'Note',
    empty: 'No rules (everything counts as free).',
    saved: 'Pricing rules saved',
  },
}

export default stats
