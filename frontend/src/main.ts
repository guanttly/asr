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

  const userStore = useUserStore(pinia)
  const appStore = useAppStore(pinia)
  await userStore.bootstrap()
  await appStore.bootstrapProductFeatures()
  await appStore.bootstrapWorkflowBindings()

  app.use(router)
  app.mount('#app')
}

bootstrap()
