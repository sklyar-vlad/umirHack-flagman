import logo from './logo.svg'
import grid from './grid.svg'
import box from './box.svg'
import truck from './truck.svg'
import receipt from './receipt.svg'
import stop from './stop.svg'
import spark from './spark.svg'
import user from './user.svg'
import menu from './menu.svg'
import check from './check.svg'
import alert from './alert.svg'
import chevron from './chevron.svg'
import plus from './plus.svg'
import close from './close.svg'
import search from './search.svg'
import adminBook from './admin-book.svg'
import adminUsers from './admin-users.svg'
import adminSettings from './admin-settings.svg'
import adminHistory from './admin-history.svg'

export const icons = {
  logo,
  grid,
  box,
  truck,
  receipt,
  stop,
  spark,
  user,
  menu,
  check,
  alert,
  chevron,
  plus,
  close,
  search,
  adminBook,
  adminUsers,
  adminSettings,
  adminHistory,
} as const

export type IconName = keyof typeof icons
