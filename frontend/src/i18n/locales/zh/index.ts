import common from './common'
import nav from './nav'
import shell from './shell'
import dashboard from './dashboard'
import providers from './providers'
import models from './models'
import stats from './stats'
import playground from './playground'
import recordings from './recordings'
import logs from './logs'
import settings from './settings'

// zh 为基准语言；导出 MessageSchema 供 en 各模块做同构类型校验（编辑器内即报漏译/多译）
export const zh = {
  common,
  nav,
  shell,
  dashboard,
  providers,
  models,
  stats,
  playground,
  recordings,
  logs,
  settings,
}

export type MessageSchema = typeof zh
