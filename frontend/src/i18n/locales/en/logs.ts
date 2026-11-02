import type { MessageSchema } from '../zh'

const logs: MessageSchema['logs'] = {
  title: 'Request logs (last 200)',
  loading: 'Loading…',
  loadHistory: 'Load history',
  clear: 'Clear',
  count: '{n} entries',
  th: {
    time: 'Time',
    provider: 'Provider',
    pathModel: 'Path / Model',
    status: 'Status',
    duration: 'Duration',
    stream: 'Stream',
    error: 'Error',
  },
  transformTip: 'Protocol conversion: {tag}',
  transformBadge: 'Protocol converted',
  empty: 'No logs yet. They appear live as requests go through the proxy (or click "Load history").',
  confirmTitle: 'Clear request logs',
  confirmBody: 'This clears the request logs of the current run; on-disk history is kept.',
}

export default logs
