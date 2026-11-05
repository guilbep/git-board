import type { RouteRecordRaw } from 'vue-router';

const routes: RouteRecordRaw[] = [
  // The active saved view is the ?view= query, so a view is bookmarkable.
  { path: '/', component: () => import('@/pages/IndexPage.vue') },
  { path: '/issues/:id', component: () => import('@/pages/IssuePage.vue') },
  // the Sync page's old address, and its parts': one Sync, in Settings
  { path: '/sync', redirect: { path: '/settings/sync', hash: '' } },
  { path: '/inbox', component: () => import('@/pages/InboxPage.vue') },
  { path: '/insights', component: () => import('@/pages/InsightsPage.vue') },
  // a tab per address, #/settings/sync…; none: the Overview (settingstabs.ts);
  // a field of Fields has one too, #/settings/fields/est
  { path: '/settings/:tab?/:name?', component: () => import('@/pages/SettingsPage.vue') },

  // Always leave this as the last one.
  {
    path: '/:catchAll(.*)*',
    component: () => import('@/pages/ErrorNotFound.vue'),
  },
];

export default routes;
