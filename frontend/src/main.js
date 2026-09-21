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
import { installBrowserTransport } from './transport'

// 浏览器访问模式 polyfill：必须在 createApp/mount 前安装。wailsjs 生成 wrapper
// （App.js/runtime.js）在方法调用时才解引用 window.go/window.runtime，组件亦在
// setup/onMounted 阶段才调用 EventsOn，挂载前注入即可全链路生效；被静态 import
// 的模块均不在模块求值期触碰 window.go/window.runtime（已逐一核对），顺序安全。
// 桌面模式（存在 window.go.main.App）下本调用为 no-op，行为零变化。
installBrowserTransport()

const app = createApp(App)

app.use(ElementPlus, { locale: zhCn })
app.use(pinia)
app.use(router)
app.mount('#app')
