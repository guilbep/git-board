import type { RouteRecordRaw } from 'vue-router';

const routes: RouteRecordRaw[] = [
  // The active saved view is the ?view= query, so a view is bookmarkable.
  { path: '/', component: () => import('@/pages/IndexPage.vue') },
  { path: '/issues/:id', component: () => import('@/pages/IssuePage.vue') },
  // the Sync page's old address, and its parts': one Sync, in Settings
  { path: '/sync', redirect: { path: '/settings/sync', hash: '' } },
  // v1's GitHub Project conflicts page: Settings › Project now
  { path: '/conflicts', redirect: { path: '/settings/project', hash: '' } },
  { path: '/inbox', component: () => import('@/pages/InboxPage.vue') },
  // a new board's first run (docs/design/first-run/proposal.md)
  { path: '/start', component: () => import('@/pages/StartPage.vue') },
  // the cycle's (experimental): off, the page says how to turn it on
  { path: '/work', component: () => import('@/pages/WorkPage.vue') },
  { path: '/insights', component: () => import('@/pages/InsightsPage.vue') },
  { path: '/releases', component: () => import('@/pages/ReleasesPage.vue') },
  { path: '/releases/:tag', component: () => import('@/pages/ReleasePage.vue') },
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
