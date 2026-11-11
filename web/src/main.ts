// The web app: Vue, its router, the app's CSS (the EPFL design system's
// tokens and the app's own: src/css/app.css).
import { createApp } from 'vue';
import '@/css/app.css';
import App from './App.vue';
import router from './router';

createApp(App).use(router).mount('#app');
