import {createApp} from 'vue'
import App from './App.vue'
import './style.css';

const mount = () => createApp(App).mount('#app')

// Wait for the Wails bridge to be injected before mounting
if ((window as any).go) {
  mount()
} else {
  window.addEventListener('wails:loaded', mount)
}
