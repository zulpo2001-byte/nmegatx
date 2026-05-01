import { createRouter } from './router.js'
import { AppRoot } from './root.js'

const app = Vue.createApp(AppRoot)
app.use(ElementPlus)
app.use(createRouter())
app.mount('#app')
