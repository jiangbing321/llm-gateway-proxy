import type { MessageSchema } from '../zh'

const playground: MessageSchema['playground'] = {
  proxyStopped: 'Proxy not running',
  pickModel: 'Select a model…',
  stream: 'Stream',
  clear: 'Clear chat',
  systemPh: 'System prompt (optional)',
  you: 'You',
  assistant: 'Assistant',
  emptyL1: 'Pick a model and send a message to test.',
  emptyL2: 'Requests go through the full proxy chain (routing → protocol conversion → recording/stats), matching real client behavior.',
  inputPh: 'Type a message, Enter to send (Shift+Enter for newline)',
  sending: 'Generating…',
  send: 'Send',
  streamTag: ' · stream',
  callFailed: 'Request failed: {e}',
}

export default playground
