import type { MessageSchema } from '../zh'

const shell: MessageSchema['shell'] = {
  proxyRunning: 'Proxy running',
  proxyStopped: 'Proxy stopped',
  minimize: 'Minimize',
  maximize: 'Maximize/Restore',
  toTray: 'Minimize to tray',
  closeToTrayHint: 'Closing the window keeps it in the tray',
  dragResize: 'Drag to resize',
  expandNav: 'Expand sidebar',
  collapseNav: 'Collapse sidebar',
  proxyStatusChanged: 'Proxy status changed: ',
  updateAvailable: 'Version {version} is available',
  downloadUpdate: 'Download update',
  releaseNotes: 'Release notes',
  dismissUpdate: 'Dismiss update notice',
}

export default shell
