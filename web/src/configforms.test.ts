// The settings' forms bind native inputs with :value plus @input (or
// @change on a select) — the component v-model props (:model-value,
// @update:model-value) do nothing on a plain element: Vue renders them
// as a bogus attribute and the listener never fires. This guards the
// convention across the settings' components.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';

// ImageLightbox binds a Quasar dialog, a component: the props are right there
const allowed = new Set(['ImageLightbox.vue']);

const dir = join(import.meta.dirname, 'components');
const files = readdirSync(dir).filter((f) => f.endsWith('.vue'));

test('the settings components never bind model-value on native elements', () => {
  const bad: string[] = [];
  for (const f of files) {
    if (allowed.has(f)) continue;
    const text = readFileSync(join(dir, f), 'utf8');
    if (/:model-value|@update:model-value|@update:value/.test(text)) bad.push(f);
  }
  assert.deepEqual(bad, []);
});

test('the settings row inputs read the value out of the event', () => {
  // @input="set(x: $event)" stores the event, not the text: the value goes
  // through inputValue($event) or ($event.target as HTMLInputElement).value.
  // Scoped to the settings' forms — a view may pass the event on purpose
  // (IssueBoard's moveAll reads the target and resets the select itself).
  const bare: string[] = [];
  for (const f of files) {
    if (allowed.has(f)) continue;
    const lines = readFileSync(join(dir, f), 'utf8').split('\n');
    const text = lines.join('\n');
    if (!/configforms\/|@\/configmodel/.test(text)) continue;
    lines.forEach((line, i) => {
      if (/@(?:input|change)="[^"]*\$event/.test(line) && !/inputValue\(\$event\)|\$event\.target|\(\$event\)/.test(line)) {
        bare.push(`${f}:${i + 1}`);
      }
    });
  }
  assert.deepEqual(bare, []);
});
