import { createRouter, createWebHashHistory } from 'vue-router';
import routes from './routes';
import { begin, routeName } from '@/interaction';

// The pages, by the address's hash (#/issues/12): the binary serves one
// index.html, and a deep link needs no server route.
const router = createRouter({
  // a part of a page (#/page#part), else the top
  scrollBehavior: (to) => (to.hash ? { el: to.hash } : { left: 0, top: 0 }),
  routes,
  history: createWebHashHistory(),
});

// a page opened is an interaction: timed to the paint after its requests
router.beforeEach((to, from) => {
  if (to.path !== from.path) begin(`open ${routeName(to.path)}`);
});

export default router;
