<!-- The inbox bell, in the bar (docs/design/issue-head/README.md, 16g): a
     bell in the text color when the issue reaches your inbox, a grey
     bell-off when it doesn't, a grey bell when a project role brings its
     creation and closing only. Its popover offers git-board's three
     choices (internal/server/inbox.go), applied on click. Without an
     identity, no bell: the inbox page says why. -->
<template>
  <GbPopover v-if="follow" :label="bell.label" title="Your inbox" :button-class="`gb-icon-btn gb-bell is-${bell.tone}`" align="right">
    <template #button><GbIcon :name="bell.shape" /></template>
    <fieldset class="gb-pop-radios" :disabled="busy">
      <legend class="gb-sr">Your inbox and this issue</legend>
      <label>
        <input type="radio" :name="`follow-${id}`" :checked="follow.set === null" @change="set(null)" />
        <span><b>Automatic</b> <span class="gb-pop-muted">{{ automatic }}</span></span>
      </label>
      <label>
        <input type="radio" :name="`follow-${id}`" :checked="follow.set === true" @change="set(true)" />
        <span><b>Following</b> <span class="gb-pop-muted">Every change reaches your inbox.</span></span>
      </label>
      <label>
        <input type="radio" :name="`follow-${id}`" :checked="follow.set === false" @change="set(false)" />
        <span><b>Not following</b> <span class="gb-pop-muted">Only a mention of you, or being added or removed.</span></span>
      </label>
    </fieldset>
    <p v-if="error" class="gb-pop-note" role="alert">Not saved. {{ error }}</p>
  </GbPopover>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { ApiError, getFollow, setFollow, type FollowState, type IssueEvent } from '@/api/board';
import { bellState } from '@/inboxcheck';
import { message } from '@/issue';
import GbIcon from '@/components/GbIcon.vue';
import GbPopover from '@/components/GbPopover.vue';

const props = defineProps<{ id: string; labels: string[]; events: IssueEvent[] }>();

const follow = ref<FollowState | null>(null);
const error = ref('');
const busy = ref(false);
async function load() {
  try {
    follow.value = await getFollow(props.id);
    error.value = '';
  } catch (e) {
    follow.value = null;
    // no identity, no login: no bell, the inbox page says why
    if (!(e instanceof ApiError && (e.code === 'no-identity' || e.code === 'no-login'))) error.value = message(e);
  }
}
watch(() => [props.id, props.labels.join(','), props.events.length], load, { immediate: true });

async function set(v: boolean | null) {
  busy.value = true;
  try {
    follow.value = await setFollow(props.id, v);
    error.value = '';
  } catch (e) {
    error.value = message(e);
  } finally {
    busy.value = false;
  }
}

const bell = computed(() => bellState(follow.value ?? { following: false, reasons: [], set: null }));
// what Automatic gives, by your part in the issue, while it is the choice
const automatic = computed(() => {
  const f = follow.value;
  if (!f || f.set !== null) return 'By your part in it: author, assignee, a mention, a role.';
  return f.following ? `Now following: ${f.reasons.join(' · ')}.` : `Now not following: ${f.reasons.length ? f.reasons.join(' · ') : 'you’re not in it'}.`;
});
</script>
