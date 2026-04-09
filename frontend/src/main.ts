import { createPinia } from 'pinia'
import { createApp } from 'vue'

import App from './App.vue'
import router from './router'
import { useAppStore } from './stores/app'
import { useUserStore } from './stores/user'
import './styles/main.css'
import 'uno.css'

async function bootstrap() {
  const app = createApp(App)
  const pinia = createPinia()

  app.use(pinia)
  app.use(router)

  const userStore = useUserStore(pinia)
  const appStore = useAppStore(pinia)
  await userStore.bootstrap()
  await appStore.bootstrapWorkflowBindings()

  app.mount('#app')
}

bootstrap()
