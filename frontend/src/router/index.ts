import { createRouter, createWebHistory } from 'vue-router'
import AppLayout from '@/layouts/AppLayout.vue'

declare module 'vue-router' {
  interface RouteMeta {
    title?: string
  }
}

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/',
      component: AppLayout,
      children: [
        { path: '', redirect: '/dashboard' },
        {
          path: '/dashboard',
          name: 'dashboard',
          component: () => import('@/pages/dashboard/Dashboard.vue'),
          meta: { title: '监控中心' },
        },
        {
          path: '/accounts',
          name: 'accounts',
          component: () => import('@/pages/accounts/Accounts.vue'),
          meta: { title: '账号管理' },
        },
        {
          path: '/goods',
          name: 'goods',
          component: () => import('@/pages/goods/Goods.vue'),
          meta: { title: '商品列表' },
        },
        {
          path: '/tasks',
          name: 'tasks',
          component: () => import('@/pages/tasks/Tasks.vue'),
          meta: { title: '抢购工作台' },
        },
        {
          path: '/logs',
          name: 'logs',
          component: () => import('@/pages/logs/Logs.vue'),
          meta: { title: '运行日志' },
        },
        {
          path: '/settings',
          name: 'settings',
          component: () => import('@/pages/settings/Settings.vue'),
          meta: { title: '通知设置' },
        },
      ],
    },
    { path: '/:pathMatch(.*)*', redirect: '/dashboard' },
  ],
})

export default router
