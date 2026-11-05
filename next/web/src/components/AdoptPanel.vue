<!-- First time (internal/release Proposals, Adopt): each overlay that
     doesn't say which commit it runs, with git-board's guess, from its
     release tag or its image's revision label; one it couldn't guess is
     typed in. Adopting writes each commit next to the digest. The States
     board's "First time: adopt what runs today". -->
<template>
  <section v-if="error || !rows || rows.length" class="gb-statecard gb-adopt" aria-labelledby="adopt-h">
    <h2 id="adopt-h">First time: adopt what runs today</h2>
    <p class="gb-adopt-lead">The overlays don't say which commit they run yet. Check each guess, then git-board writes it next to the digest.</p>
    <p v-if="!rows && !error" class="gb-hint" role="status">Reading what each overlay runs…</p>
    <div v-if="rows?.length" class="gb-table-wrap">
      <table class="gb-table">
        <thead><tr><th scope="col">Environment</th><th scope="col">Config repo</th><th scope="col">Runs</th><th scope="col">Commit</th><th scope="col">From</th></tr></thead>
        <tbody>
          <tr v-for="r in rows" :key="r.p.repo + r.p.path">
            <td><span :class="envClass(r.p.env)">{{ r.p.env }}</span></td>
            <td>{{ r.target }}</td>
            <td class="gb-mono">{{ r.p.runs }}</td>
            <td v-if="r.known" class="gb-mono">{{ sha7(r.p.commit) }}</td>
            <td v-else><input v-model="r.p.commit" class="epfl-input gb-adopt-input" :aria-label="`Commit ${r.p.env} runs on ${r.target}`" placeholder="No label: type the commit" autocomplete="off" data-1p-ignore /></td>
            <td>{{ from[r.p.from] }}</td>
          </tr>
        </tbody>
      </table>
    </div>
    <p v-if="error" class="epfl-alert epfl-alert-danger" role="alert">{{ error }}</p>
    <div v-if="rows?.length" class="gb-actions">
      <button type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="busy || !rows.every((r) => r.p.commit.trim())" @click="adopt">Adopt these</button>
      <span class="gb-note">Until then, git-board won't deploy there: it couldn't tell newer from older.</span>
    </div>
  </section>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { getProposals, type EnvState, type Proposal } from '@/api/board';
import { message } from '@/issue';
import { envMark, markClass, repoName, sha7 } from '@/releases';

// adopt: the page's write, busy while it runs; busy: a job runs, or the
// page waits for an answer
const props = defineProps<{ envs: EnvState[]; busy: boolean; adopt: (ps: Proposal[]) => Promise<void> }>();

const rows = ref<{ p: Proposal; known: boolean; target: string }[] | null>(null); // null: being read
const error = ref('');
const from = { tag: 'its tag', label: "the image's revision label", '': 'not found' };

const envClass = (env: string) => markClass(envMark(props.envs.find((e) => e.name === env)?.role ?? '', true));
// the deploy target's name, as the environment strip says it (enack8s)
const targetName = (p: Proposal) => props.envs.find((e) => e.name === p.env)?.targets.find((t) => t.repo === p.repo && t.path === p.path)?.name ?? repoName(p.repo);

// the guesses: it fetches, and asks the registries for revision labels
async function load() {
  try {
    rows.value = (await getProposals()).map((p) => ({ p: { ...p }, known: !!p.commit, target: targetName(p) }));
    error.value = '';
  } catch (e) {
    error.value = message(e);
  }
}

async function adopt() {
  if (!rows.value || props.busy) return;
  error.value = '';
  try {
    await props.adopt(rows.value.map((r) => ({ ...r.p, commit: r.p.commit.trim() })));
  } catch (e) {
    error.value = `Not adopted: ${message(e)}`;
  }
}

onMounted(load);
</script>

<style scoped>
.gb-adopt { margin-bottom: 1.5rem; }
.gb-adopt-lead { font-size: .9375rem; }
.gb-adopt-input { height: 2.25rem; font-size: .875rem; }
</style>
