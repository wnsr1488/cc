<script setup lang="ts">
import { Lock, User } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import { login } from '@/api'
import { setSession } from '@/stores/auth'

const route = useRoute()
const router = useRouter()
const loading = ref(false)
const form = reactive({
  username: 'admin',
  password: 'change-me',
})

async function submit() {
  loading.value = true
  try {
    const result = await login(form.username, form.password)
    setSession(result.token, result.user)
    ElMessage.success('登录成功')
    await router.push((route.query.redirect as string) || '/dashboard')
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <main class="login-page">
    <section class="login-panel">
      <div class="brand">
        <div class="logo">CC</div>
        <div>
          <h1>CC Panel</h1>
          <p>服务器防 CC 管理面板</p>
        </div>
      </div>

      <el-card class="login-card">
        <h2>管理员登录</h2>
        <el-form label-position="top" @submit.prevent="submit">
          <el-form-item label="用户名">
            <el-input v-model="form.username" :prefix-icon="User" size="large" />
          </el-form-item>
          <el-form-item label="密码">
            <el-input
              v-model="form.password"
              :prefix-icon="Lock"
              size="large"
              type="password"
              show-password
              @keyup.enter="submit"
            />
          </el-form-item>
          <el-button type="primary" size="large" :loading="loading" class="login-button" @click="submit">
            登录
          </el-button>
        </el-form>
      </el-card>
    </section>
  </main>
</template>

<style scoped>
.login-page {
  display: grid;
  min-height: 100vh;
  place-items: center;
  background:
    radial-gradient(circle at top left, rgb(59 130 246 / 22%), transparent 34%),
    linear-gradient(135deg, #0f172a, #172554 58%, #0f172a);
}

.login-panel {
  width: 420px;
}

.brand {
  display: flex;
  gap: 16px;
  align-items: center;
  margin-bottom: 24px;
  color: white;
}

.logo {
  display: grid;
  width: 58px;
  height: 58px;
  place-items: center;
  border-radius: 16px;
  font-weight: 800;
  background: #2563eb;
}

.brand h1 {
  margin: 0;
  font-size: 30px;
}

.brand p {
  margin: 4px 0 0;
  color: #bfdbfe;
}

.login-card {
  border: none;
  border-radius: 18px;
}

.login-card h2 {
  margin: 0 0 22px;
}

.login-button {
  width: 100%;
}
</style>
