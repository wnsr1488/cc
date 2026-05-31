import { createRouter, createWebHistory } from 'vue-router'

import { isAuthenticated } from '@/stores/auth'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/login',
      name: 'login',
      component: () => import('@/views/LoginView.vue'),
      meta: { public: true },
    },
    {
      path: '/',
      component: () => import('@/layouts/AppLayout.vue'),
      redirect: '/dashboard',
      children: [
        {
          path: 'dashboard',
          name: 'dashboard',
          component: () => import('@/views/DashboardView.vue'),
        },
        {
          path: 'servers',
          name: 'servers',
          component: () => import('@/views/ServersView.vue'),
        },
        {
          path: 'servers/:id',
          name: 'server-detail',
          component: () => import('@/views/ServerDetailView.vue'),
        },
        {
          path: 'firewall/blacklist',
          name: 'blacklist',
          component: () => import('@/views/FirewallListView.vue'),
          props: { mode: 'blacklist' },
        },
        {
          path: 'firewall/whitelist',
          name: 'whitelist',
          component: () => import('@/views/FirewallListView.vue'),
          props: { mode: 'whitelist' },
        },
        {
          path: 'geo',
          name: 'geo',
          component: () => import('@/views/GeoView.vue'),
        },
        {
          path: 'policies',
          name: 'policies',
          component: () => import('@/views/PoliciesView.vue'),
        },
        {
          path: 'monitor',
          name: 'monitor',
          component: () => import('@/views/MonitorView.vue'),
        },
        {
          path: 'audit',
          name: 'audit',
          component: () => import('@/views/AuditView.vue'),
        },
        {
          path: 'settings',
          name: 'settings',
          component: () => import('@/views/SettingsView.vue'),
        },
      ],
    },
  ],
})

router.beforeEach((to) => {
  if (!to.meta.public && !isAuthenticated()) {
    return { name: 'login', query: { redirect: to.fullPath } }
  }
  if (to.name === 'login' && isAuthenticated()) {
    return { name: 'dashboard' }
  }
  return true
})

export default router
