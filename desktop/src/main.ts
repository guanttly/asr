import { createPinia } from 'pinia'
import { createApp } from 'vue'
import App from './App.vue'
import './styles/main.css'
import 'uno.css'
import { appendRuntimeLog } from './utils/debug'

window.addEventListener('error', (event) => {
	const detail = {
		message: event.message,
		filename: event.filename,
		lineno: event.lineno,
		colno: event.colno,
		stack: event.error instanceof Error ? event.error.stack : undefined,
	}
	void appendRuntimeLog('frontend.error', JSON.stringify(detail))
})

window.addEventListener('unhandledrejection', (event) => {
	const reason = event.reason instanceof Error
		? { message: event.reason.message, stack: event.reason.stack }
		: { reason: String(event.reason) }
	void appendRuntimeLog('frontend.rejection', JSON.stringify(reason))
})

void appendRuntimeLog('frontend.bootstrap', JSON.stringify({ href: window.location.href, search: window.location.search }))

const app = createApp(App)
app.use(createPinia())
app.mount('#app')

void appendRuntimeLog('frontend.bootstrap', 'vue app mounted')
