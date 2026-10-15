import type { RouteRecordRaw } from 'vue-router';

const routes: RouteRecordRaw[] = [
  // The active saved view is the ?view= query, so a view is bookmarkable.
  { path: '/', component: () => import('@/pages/IndexPage.vue') },
  { path: '/conflicts', component: () => import('@/pages/ConflictsPage.vue') },

  // Always leave this as the last one.
  {
    path: '/:catchAll(.*)*',
    component: () => import('@/pages/ErrorNotFound.vue'),
  },
];

export default routes;
