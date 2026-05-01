import { request, tokenKey } from '../api.js'

export const LoginPage = {
  template: `
  <div style="height:100vh;display:flex;align-items:center;justify-content:center;">
    <el-card style="width:420px">
      <template #header><b>登录 NME</b></template>
      <el-form :model="form" label-width="80px">
        <el-form-item label="邮箱"><el-input v-model="form.email" /></el-form-item>
        <el-form-item label="密码"><el-input v-model="form.password" show-password /></el-form-item>
        <el-button type="primary" :loading="loading" @click="submit" style="width:100%">登录</el-button>
      </el-form>
    </el-card>
  </div>`,
  data: () => ({ form: { email: 'admin@zulpo.com', password: 'AA123456' }, loading: false }),
  methods: {
    async submit() {
      this.loading = true
      try {
        const data = await request('/api/auth/login', 'POST', this.form)
        localStorage.setItem(tokenKey, data.access_token || '')
        localStorage.setItem('nme_refresh_token', data.refresh_token || '')
        this.$router.push('/dashboard')
      } catch (e) {
        ElementPlus.ElMessage.error(e.message)
      } finally {
        this.loading = false
      }
    },
  },
}
