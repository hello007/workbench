import { createApp } from 'vue'
import ElementPlus from 'element-plus'
import zhCn from 'element-plus/es/locale/lang/zh-cn'
import 'element-plus/dist/index.css'
// Element Plus 暗色变量集：html.dark 时覆盖 EP 组件 token（背景/文字/边框等）
import 'element-plus/theme-chalk/dark/css-vars.css'
import './style.css'
import router from './router'
import pinia from './store'
import App from './App.vue'

const app = createApp(App)

app.use(ElementPlus, { locale: zhCn })
app.use(pinia)
app.use(router)
app.mount('#app')
