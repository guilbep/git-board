// What a person does on a page, each held (ADR 0023; CONTRIBUTING.md): a
// new act on a page adds its entry here. Every pull request checks each
// one (interactions.spec.ts): an optimistic one shows before the server
// answers, and none asks more requests than its share. The nightly times
// each one on the 5,000-issue board (web/perf/interactions.mjs), a
// `5k/web/Interaction<Name>` line of testdata/perf/budgets.txt its budget.
import { expect, type Page } from '@playwright/test';

// the board an act runs on: an issue the runner made or found
export interface Given {
  issue: { id: string; title: string };
}

export interface Interaction {
  name: string; // CamelCase: the benchmark's name
  optimistic?: boolean; // shows before the server answers
  maxRequests: number; // API requests from the act to its end
  arrange: (page: Page, g: Given) => Promise<void>; // the page before the act
  act: (page: Page, g: Given) => Promise<void>;
  done: (page: Page, g: Given) => Promise<void>; // waits for the act's end, as a person sees it
}

const panelTitle = (page: Page) => page.locator('.gb-panel .gb-head-title');

// a comment's text, its own each run: a board the nightly reuses keeps
// the ones before
let comment = '';

export const interactions: Interaction[] = [
  {
    name: 'OpenPalette',
    maxRequests: 0,
    arrange: async (page) => {
      await page.goto('/#/');
      await expect(page.getByRole('button', { name: 'Search: filter or jump to' })).toBeVisible();
    },
    act: (page) => page.getByRole('button', { name: 'Search: filter or jump to' }).click(),
    done: (page) => expect(page.locator('.cp-input')).toBeFocused(),
  },
  {
    name: 'OpenShortcuts',
    maxRequests: 0,
    arrange: async (page) => {
      await page.goto('/#/');
      await expect(page.getByRole('button', { name: 'Keyboard shortcuts' })).toBeVisible();
    },
    act: (page) => page.getByRole('button', { name: 'Keyboard shortcuts' }).click(),
    done: (page) => expect(page.getByRole('heading', { name: 'Keyboard shortcuts' })).toBeVisible(),
  },
  {
    name: 'OpenIssue',
    maxRequests: 6, // its thread, its cycle, follow, the uploads' settings, the inbox's line
    arrange: async (page, g) => {
      await page.goto(`/#/?filter=${g.issue.id.slice(0, 12)}`); // the issue itself, on any board
      const card = page.getByRole('button', { name: g.issue.title }).first();
      await expect(card).toBeVisible();
      await card.hover(); // as a person's pointer comes to it: its thread read ahead
      await page.waitForTimeout(100);
    },
    // a card on the board, a title in the table: the issue's own button
    act: (page, g) => page.getByRole('button', { name: g.issue.title }).first().click(),
    done: (page, g) => expect(panelTitle(page)).toHaveText(g.issue.title),
  },
  {
    name: 'RenameIssue',
    optimistic: true,
    maxRequests: 1, // the PATCH
    arrange: async (page, g) => {
      await page.goto(`/#/?filter=&issue=${g.issue.id}`);
      await expect(panelTitle(page)).toHaveText(g.issue.title);
      await page.locator('.gb-panel').getByRole('button', { name: 'More' }).click();
      await page.getByRole('menuitem', { name: 'Edit the title' }).click();
      await page.locator('.gb-rename input').fill(`${g.issue.title} renamed`);
    },
    act: (page) => page.locator('.gb-panel').getByRole('button', { name: 'Save', exact: true }).click(),
    done: (page, g) => expect(panelTitle(page)).toHaveText(`${g.issue.title} renamed`),
  },
  {
    name: 'PostComment',
    optimistic: true,
    maxRequests: 2, // the POST; the bell's follow, which a comment turns on
    arrange: async (page, g) => {
      await page.goto(`/#/?filter=&issue=${g.issue.id}`);
      await expect(panelTitle(page)).toHaveText(g.issue.title);
      comment = `A comment, ${Date.now()}`;
      await page.locator('.gb-panel').getByPlaceholder('Leave a comment').fill(comment);
    },
    act: (page) => page.locator('.gb-panel').getByRole('button', { name: 'Comment', exact: true }).click(),
    done: (page) => expect(page.locator('.gb-panel').getByText(comment)).toBeVisible(),
  },
  {
    name: 'ApplyFilter',
    maxRequests: 4, // the lists' ids, the filters' labels and counts
    arrange: async (page) => {
      await page.goto('/#/');
      await expect(page.locator('#filter')).toBeVisible();
    },
    act: async (page) => {
      await page.locator('#filter').fill('is:closed');
      await page.locator('#filter').press('Enter');
    },
    done: (page) => expect(page).toHaveURL(/filter=is%3Aclosed|filter=is:closed/),
  },
];
