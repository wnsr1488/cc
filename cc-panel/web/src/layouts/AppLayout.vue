<script setup lang="ts">
import {
  DataLine,
  Document,
  Location,
  House,
  Lock,
  Monitor,
  Operation,
  Setting,
  SwitchButton,
  Unlock,
} from '@element-plus/icons-vue'
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import { clearSession, getCurrentUser } from '@/stores/auth'

const route = useRoute()
const router = useRouter()
const user = computed(() => getCurrentUser())

const menuItems = [
  { index: '/dashboard', title: '总览', icon: House },
  { index: '/servers', title: '服务器', icon: Monitor },
  { index: '/firewall/blacklist', title: '黑名单', icon: Lock },
  { index: '/firewall/whitelist', title: '白名单', icon: Unlock },
  { index: '/geo', title: '地区封禁', icon: Location },
  { index: '/policies', title: '自动策略', icon: Operation },
  { index: '/monitor', title: '系统监控', icon: DataLine },
  { index: '/audit', title: '审计日志', icon: Document },
  { index: '/settings', title: '系统设置', icon: Setting },
]

function logout() {
  clearSession()
  router.push('/login')
}
</script>

<template>
  <el-container class="app-shell">
    <el-aside width="240px" class="sidebar">
      <div class="sidebar-brand">
        <div class="brand-mark">CC</div>
        <div>
          <strong>CC Panel</strong>
          <span>防护管理</span>
        </div>
      </div>
      <el-menu :default-active="route.path" router background-color="#0f172a" text-color="#cbd5e1">
        <el-menu-item v-for="item in menuItems" :key="item.index" :index="item.index">
          <el-icon><component :is="item.icon" /></el-icon>
          <span>{{ item.title }}</span>
        </el-menu-item>
      </el-menu>
    </el-aside>

    <el-container>
      <el-header class="topbar">
        <div class="topbar-title">
          <el-icon><DataLine /></el-icon>
          <span>服务器防 CC 管理面板</span>
        </div>
        <div class="user-box">
          <span>{{ user?.username }}</span>
          <el-tag type="success" effect="plain">{{ user?.role }}</el-tag>
          <el-button :icon="SwitchButton" @click="logout">退出</el-button>
        </div>
      </el-header>

      <el-main class="main-content">
        <RouterView :key="$route.fullPath" />
      </el-main>
    </el-container>
  </el-container>
</template>

<style scoped>
.app-shell {
  min-height: 100vh;
}

.sidebar {
  color: #cbd5e1;
  background: #0f172a;
}

.sidebar-brand {
  display: flex;
  gap: 12px;
  align-items: center;
  height: 72px;
  padding: 0 20px;
  color: #fff;
}

.brand-mark {
  display: grid;
  width: 42px;
  height: 42px;
  place-items: center;
  border-radius: 12px;
  background: #2563eb;
  font-weight: 800;
}

.sidebar-brand strong,
.sidebar-brand span {
  display: block;
}

.sidebar-brand span {
  margin-top: 2px;
  color: #94a3b8;
  font-size: 12px;
}

.topbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 72px;
  background: #fff;
  box-shadow: 0 1px 0 rgb(15 23 42 / 8%);
}

.topbar-title,
.user-box {
  display: flex;
  gap: 12px;
  align-items: center;
}

.main-content {
  padding: 24px;
}
</style>
