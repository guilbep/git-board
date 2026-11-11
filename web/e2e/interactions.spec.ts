// Each act a person does on a page (interactions.ts), on every pull
// request, counted and never timed (a shared runner's clock is noise; the
// nightly times them): an optimistic act shows while every answer is held,
// and no act asks more requests than its share.
import { expect, test } from '@playwright/test';
import { errors, identity, issue } from './board';
import { interactions } from './interactions';

for (const it of interactions) {
  test(`${it.name}: ${it.optimistic ? 'shows before the answer, ' : ''}at most ${it.maxRequests} requests`, async ({ page, request }) => {
    const errs = errors(page);
    await identity(request);
    const title = `Interaction ${it.name} ${Date.now()}`;
    const made = await issue(request, title);
    const given = { issue: { id: made.id, title } };
    await it.arrange(page, given);
    await page.waitForLoadState('networkidle');

    const asked: string[] = [];
    page.on('request', (r) => {
      const u = new URL(r.url());
      if (u.pathname.startsWith('/api/') && u.pathname !== '/api/slow' && u.pathname !== '/api/events') asked.push(`${r.method()} ${u.pathname}`);
    });
    const held: (() => Promise<void>)[] = [];
    if (it.optimistic) {
      await page.route('**/api/**', (route) => void held.push(() => route.continue()));
    }
    await it.act(page, given);
    await it.done(page, given); // with every answer held, for an optimistic act
    if (it.optimistic) {
      for (const go of held) await go().catch(() => {}); // the answers, let go
      await page.unroute('**/api/**');
    }
    await page.waitForLoadState('networkidle');
    expect(asked.length, `${it.name}: requests\n  ${asked.join('\n  ')}`).toBeLessThanOrEqual(it.maxRequests);
    expect(errs).toEqual([]);
  });
}
