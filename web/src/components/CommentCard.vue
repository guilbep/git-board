<!-- A comment, or an issue's description: who and when, its earlier versions,
     editing in place (refused if someone changed it meanwhile), and per
     forge whether it's there yet. -->
<template>
  <article class="gb-comment" :class="{ 'gb-comment-mine': mine }" :aria-labelledby="`c-${uid}`">
    <header :id="`c-${uid}`" class="gb-comment-head">
      <img v-if="event.author.avatar_url" class="gb-avatar gb-avatar-sm" :src="event.author.avatar_url" alt="" />
      <span v-else class="gb-avatar gb-avatar-sm" :class="{ 'gb-avatar-dark': mine }" aria-hidden="true">{{ initials(event.author.name) }}</span>
      <b>{{ event.author.name }}</b>
      <span class="gb-time">{{ verb }} {{ on(event.at, true) }}</span>
      <span v-if="review?.state" class="gb-mono gb-time">at {{ review.commit?.slice(0, 7) }}</span>
      <span v-if="event.kind === 'created'" class="gb-tag">Description</span>
      <details v-if="event.edited && event.history" class="gb-hist">
        <summary>edited<GbIcon name="down" small /></summary>
        <div class="gb-hist-pop">
          <ol>
            <li v-for="(v, i) in versions" :key="i">
              <a href="#" :aria-current="i === shown" @click.prevent="shown = i">
                {{ when(v.at, true) }}{{ i === 0 ? ' · latest' : i === versions.length - 1 ? ' · original' : '' }}
              </a>
            </li>
          </ol>
          <p v-if="shown > 0" class="gb-hist-old"><b>{{ versions[shown]!.author.name }}, {{ when(versions[shown]!.at, true) }}</b><br />{{ versions[shown]!.body }}</p>
        </div>
      </details>
      <span class="gb-grow" />
      <button v-if="!editing && editable(event)" type="button" class="gb-icon-btn" :aria-label="`Edit ${mine ? 'your' : event.author.name + '’s'} ${event.kind === 'created' ? 'description' : 'comment'}`" @click="startEdit">
        <GbIcon name="edit" />
      </button>
    </header>

    <div v-if="editing">
      <MarkdownEditor v-model="draft" :forges="forges" label="Edit the text" :disabled="busy" @submit="saveEdit" />
      <div v-if="changed" class="epfl-alert epfl-alert-warning gb-alert-row" role="alert">
        <GbIcon name="warn" small />
        <span><b>Someone changed this text meanwhile.</b> Saving yours replaces it; theirs stays in the edit history.</span>
      </div>
      <p v-else-if="error" class="gb-save-error" role="alert"><GbIcon name="error" small /> <b>Not saved.</b> {{ error }}</p>
      <div class="gb-composer-foot">
        <span class="gb-hint">{{ busy ? 'Saving…' : 'Saved in git-bug, on the forges at the next sync.' }}</span>
        <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="busy" @click="editing = false">Cancel</button>
        <button v-if="changed" type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="busy" @click="saveEdit(true)">Save mine anyway</button>
        <button v-else type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="busy || draft === event.body" @click="saveEdit()">Save</button>
      </div>
    </div>
    <template v-else>
      <div v-if="review?.path" class="gb-comment-anchor">
        <button type="button" class="gb-comment-line gb-mono" @click="emit('line', review.path)">{{ review.path }}:{{ review.line }}</button>
        <span v-if="review.outdated" class="gb-tag">outdated</span>
        <span v-if="review.resolved" class="gb-tag">resolved</span>
        <pre v-if="review.text" class="gb-mono gb-comment-quote">{{ review.text }}</pre>
      </div>
      <MarkdownBody v-if="event.html" class="gb-comment-body" :html="event.html" :forges="forges" @ref="(n, e) => emit('ref', n, e)" />
      <p v-else-if="!review" class="gb-comment-body gb-hint">No description.</p>
    </template>

    <footer v-if="forges?.length && !review" class="gb-comment-foot">
      <span v-for="f in forges" :key="f.url" class="gb-fstate" :class="{ 'gb-fstate-local': event.pending?.includes(f.name) || event.edit_pending?.includes(f.name) }">
        <template v-if="event.pending?.includes(f.name)"><GbIcon name="clock" small /> Not on {{ f.name }} yet, next sync</template>
        <template v-else-if="event.edit_pending?.includes(f.name)"><GbIcon name="clock" small /> Latest edit not on {{ f.name }} yet, next sync</template>
        <template v-else><GbIcon name="check" small /><a :href="f.url" target="_blank" rel="noopener noreferrer">On {{ f.name }}</a></template>
      </span>
    </footer>
  </article>
</template>

<script setup lang="ts">
import { computed, ref, useId } from 'vue';
import { ApiError, type Forge, type IssueEvent } from '@/api/board';
import { initials, on, when } from '@/me';
import GbIcon from '@/components/GbIcon.vue';
import MarkdownBody from '@/components/MarkdownBody.vue';
import MarkdownEditor from '@/components/MarkdownEditor.vue';
import { editable, verbOf } from '@/review';

const props = defineProps<{
  event: IssueEvent;
  forges?: Forge[] | null | undefined;
  mine: boolean;
  // saves an edit; base is the text it started from (409 "changed" if not current)
  save: (id: string, body: string, base: string) => Promise<unknown>;
}>();
const emit = defineEmits<{ ref: [n: string, event: MouseEvent]; line: [path: string] }>();

const uid = useId();
const editing = ref(false);
const draft = ref('');
const base = ref('');
const busy = ref(false);
const error = ref('');
const changed = ref(false);
const shown = ref(0);

// a line comment, a reply to one, or a review: on the pull request, not
// the issue, so no per-forge footer
const review = computed(() => props.event.review);
const verb = computed(() => verbOf(props.event));

// newest first, as the menu lists them
const versions = computed(() => [...(props.event.history ?? [])].reverse());

function startEdit() {
  draft.value = props.event.body ?? '';
  base.value = props.event.body ?? '';
  error.value = '';
  changed.value = false;
  editing.value = true;
}

async function saveEdit(anyway = false) {
  busy.value = true;
  error.value = '';
  try {
    // "anyway": against what the comment holds now
    await props.save(props.event.id, draft.value, anyway ? '' : base.value);
    editing.value = false;
  } catch (e) {
    if (e instanceof ApiError && e.code === 'changed') changed.value = true;
    else error.value = e instanceof Error ? e.message : String(e);
  } finally {
    busy.value = false;
  }
}
</script>

<style scoped>
.gb-save-error { display: flex; gap: .375rem; align-items: center; margin: 0; padding: .5rem 1rem; color: var(--danger-fg); background: var(--danger-bg); font-size: .875rem; }
.gb-alert-row { margin: 0; border-radius: 0; }
.gb-comment-anchor { display: flex; flex-wrap: wrap; align-items: center; gap: .375rem; padding: .5rem 1rem 0; }
.gb-comment-line { padding: 0; border: 0; background: none; color: var(--primary); font-weight: 700; text-decoration: underline; cursor: pointer; }
.gb-comment-quote { flex-basis: 100%; margin: 0; padding: .25rem .5rem; background: var(--bg-subtle); white-space: pre-wrap; overflow-wrap: anywhere; }
</style>
