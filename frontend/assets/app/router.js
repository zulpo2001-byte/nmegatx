import { LoginPage } from './views/login.js'
import { DashboardPage } from './views/dashboard.js'

export function createRouter() {
  return VueRouter.createRouter({
    history: VueRouter.createWebHashHistory(),
    routes: [
      { path: '/', redirect: '/login' },
      { path: '/login', component: LoginPage },
      { path: '/dashboard', component: DashboardPage },
    ],
  })
}
