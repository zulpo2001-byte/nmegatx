import { request, tokenKey } from '../api.js'

export const DashboardPage = {
  template: `<el-container style="height:100vh">
  <el-aside width="220px" style="background:#1f2d3d"><div style="color:#fff;padding:16px;font-weight:700">NME</div>
    <el-menu :default-active="active" @select="onMenu" background-color="#1f2d3d" text-color="#fff" active-text-color="#ffd04b">
      <el-menu-item index="dashboard">概览</el-menu-item><el-menu-item index="orders">订单</el-menu-item><el-menu-item index="paypal">PayPal</el-menu-item><el-menu-item index="stripe">Stripe</el-menu-item><el-menu-item index="webhooks">A/B端点</el-menu-item>
    </el-menu></el-aside>
  <el-container><el-header><el-space><span>{{me.email}}</span><el-button @click="logout">退出</el-button></el-space></el-header>
  <el-main>
    <el-card v-if="active==='dashboard'"><el-row :gutter="12"><el-col :span="6"><el-statistic title="订单总数" :value="dashboard.orders_total||0"/></el-col><el-col :span="6"><el-statistic title="成功" :value="dashboard.completed||0"/></el-col><el-col :span="6"><el-statistic title="待处理" :value="dashboard.pending||0"/></el-col><el-col :span="6"><el-statistic title="成功率" :value="dashboard.success_rate||0" suffix="%"/></el-col></el-row></el-card>
    <el-card v-if="active==='orders'"><el-table :data="orders"><el-table-column prop="a_order_id" label="A单号"/><el-table-column prop="amount" label="金额"/><el-table-column prop="payment_method" label="通道"/><el-table-column prop="status" label="状态"/></el-table></el-card>
    <el-card v-if="active==='paypal'"><el-button type="primary" @click="openPP">新增</el-button><el-table :data="paypal"><el-table-column prop="label" label="标签"/><el-table-column prop="mode" label="模式"/><el-table-column prop="email" label="邮箱"/></el-table></el-card>
    <el-card v-if="active==='stripe'"><el-button type="primary" @click="openST">新增</el-button><el-table :data="stripe"><el-table-column prop="label" label="标签"/><el-table-column prop="enabled" label="启用"/></el-table></el-card>
    <el-card v-if="active==='webhooks'"><el-button @click="openWH('a')">新增A</el-button><el-button @click="openWH('b')">新增B</el-button><el-table :data="webhooks"><el-table-column prop="type" label="类型"/><el-table-column prop="label" label="标签"/><el-table-column prop="url" label="URL"/></el-table></el-card>
  </el-main></el-container></el-container>

  <el-dialog v-model="pp.open" title="PayPal"><el-form :model="pp.form"><el-form-item label="标签"><el-input v-model="pp.form.label"/></el-form-item><el-form-item label="模式"><el-select v-model="pp.form.mode"><el-option value="email"/><el-option value="rest"/></el-select></el-form-item><el-form-item label="邮箱"><el-input v-model="pp.form.email"/></el-form-item></el-form><template #footer><el-button @click="pp.open=false">取消</el-button><el-button type="primary" @click="savePP">保存</el-button></template></el-dialog>
  <el-dialog v-model="st.open" title="Stripe"><el-form :model="st.form"><el-form-item label="标签"><el-input v-model="st.form.label"/></el-form-item><el-form-item label="Secret"><el-input v-model="st.form.secret_key"/></el-form-item></el-form><template #footer><el-button @click="st.open=false">取消</el-button><el-button type="primary" @click="saveST">保存</el-button></template></el-dialog>
  <el-dialog v-model="wh.open" :title="'新增'+wh.type.toUpperCase()+'端点'"><el-form :model="wh.form"><el-form-item label="标签"><el-input v-model="wh.form.label"/></el-form-item><el-form-item label="URL"><el-input v-model="wh.form.url"/></el-form-item><el-form-item label="支付方式"><el-select v-model="wh.form.payment_method"><el-option value="all"/><el-option value="paypal"/><el-option value="stripe"/></el-select></el-form-item></el-form><template #footer><el-button @click="wh.open=false">取消</el-button><el-button type="primary" @click="saveWH">创建</el-button></template></el-dialog>` ,
  data: () => ({ active: 'dashboard', me: {}, dashboard: {}, orders: [], paypal: [], stripe: [], webhooks: [], pp: { open: false, form: { label: '', mode: 'email', email: '' } }, st: { open: false, form: { label: '', secret_key: '' } }, wh: { open: false, type: 'a', form: { label: '', url: '', payment_method: 'all' } } }),
  async mounted() {
    if (!localStorage.getItem(tokenKey)) return this.$router.push('/login')
    try { this.me = await request('/api/auth/me'); await this.load('dashboard') } catch { this.logout() }
  },
  methods: {
    logout() { localStorage.removeItem(tokenKey); localStorage.removeItem('nme_refresh_token'); this.$router.push('/login') },
    async onMenu(k) { this.active = k; await this.load(k) },
    async load(k) {
      try {
        if (k === 'dashboard') this.dashboard = await request('/api/user/dashboard')
        if (k === 'orders') this.orders = (await request('/api/user/orders')).items || []
        if (k === 'paypal') this.paypal = (await request('/api/user/paypal')).items || []
        if (k === 'stripe') this.stripe = (await request('/api/user/stripe')).items || []
        if (k === 'webhooks') this.webhooks = (await request('/api/user/webhooks')).items || []
      } catch (e) { ElementPlus.ElMessage.error(e.message) }
    },
    openPP() { this.pp = { open: true, form: { label: '', mode: 'email', email: '' } } },
    async savePP() { await request('/api/user/paypal', 'POST', this.pp.form); this.pp.open = false; this.load('paypal') },
    openST() { this.st = { open: true, form: { label: '', secret_key: '' } } },
    async saveST() { await request('/api/user/stripe', 'POST', this.st.form); this.st.open = false; this.load('stripe') },
    openWH(t) { this.wh = { open: true, type: t, form: { label: '', url: '', payment_method: 'all' } } },
    async saveWH() { await request('/api/user/webhooks/' + this.wh.type, 'POST', this.wh.form); this.wh.open = false; this.load('webhooks') },
  },
}
