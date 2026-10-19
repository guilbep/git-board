import type { RouteRecordRaw } from 'vue-router';

const routes: RouteRecordRaw[] = [
  // The active saved view is the ?view= query, so a view is bookmarkable.
  { path: '/', component: () => import('@/pages/IndexPage.vue') },
  { path: '/conflicts', component: () => import('@/pages/ConflictsPage.vue') },
  { path: '/issues/:id', component: () => import('@/pages/IssuePage.vue') },
  { path: '/sync', component: () => import('@/pages/ForgeSyncPage.vue') },
  { path: '/insights', component: () => import('@/pages/InsightsPage.vue') },
  { path: '/work', component: () => import('@/pages/WorkPage.vue') },
  { path: '/releases', component: () => import('@/pages/ReleasesPage.vue') },
  { path: '/inbox', component: () => import('@/pages/InboxPage.vue') },
  // a tab per address, #/settings/forges…; none: General (settingstabs.ts)
  { path: '/settings/:tab?', component: () => import('@/pages/SettingsPage.vue') },

  // Always leave this as the last one.
  {
    path: '/:catchAll(.*)*',
    component: () => import('@/pages/ErrorNotFound.vue'),
  },
];

export default routes;
