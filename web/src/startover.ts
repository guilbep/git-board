// Settings › Sync, Start over (internal/server/startover.go): the cards'
// words, in the order a board starts over, each with its command and what
// it loses; the repository's name typed to confirm; a result's line.
import type { StartOverResult, StartOverStatus } from './api/board';

export type CardId = 'reset' | 'push-prune' | 'take' | 'prune';

export interface Card {
  id: CardId;
  title: string;
  does: string;
  loses: string;
  when: string;
  command: string;
  button: string; // '': no button, as for git-bug's data before the switch
}

const reset: Card = {
  id: 'reset',
  title: 'Empty this clone’s board',
  does: 'Deletes every issue, person, sync link and release record here, with the copies fetched from origin, and unsets who you are. Writes a mark: until origin has it too, a pull or a push refuses.',
  loses: 'What only this clone holds. The switch’s mark and the numbers’ counter stay, so no number is handed out twice.',
  when: 'First, to start the board over. Nothing reaches origin until the next card.',
  command: 'git board reset --hard',
  button: 'Empty this clone’s board',
};

const pushPrune: Card = {
  id: 'push-prune',
  title: 'Make origin’s board this clone’s',
  does: 'Pushes this clone’s board to origin, and deletes there what this clone doesn’t have. A ref origin moved meanwhile is overwritten.',
  loses: 'Origin’s board as it was, kept first in a bundle here. Every other clone’s pulls and pushes then refuse, until it takes origin’s board.',
  when: 'Second, after emptying this clone’s board. Stop the automated syncs of the other clones first.',
  command: 'git board push --prune',
  button: 'Make origin’s board this clone’s',
};

const take: Card = {
  id: 'take',
  title: 'Take origin’s board',
  does: 'Makes this clone’s board origin’s, its marks included, and drops what only this clone had.',
  loses: 'The issues, comments and changes this clone never pushed.',
  when: 'On every other clone, once one clone started over: their pulls and pushes refuse until then.',
  command: 'git board reset --hard origin',
  button: 'Take origin’s board',
};

function prune(switched: boolean): Card {
  return {
    id: 'prune',
    title: 'Remove git-bug’s data',
    does: switched
      ? 'Deletes git-bug’s refs (refs/bugs, refs/identities and its old release records), its cache, .git/git-bug, and its git-bug.* config keys. git-board’s board stays.'
      : 'git board switch first: git-bug’s issues are this repository’s only copy until then.',
    loses: 'git-bug’s copy of the issues, which the switch carried into git-board. Here it goes to a bundle first. On origin, nothing backs them up.',
    when: 'Once no clone and no automation uses git-bug any more.',
    command: 'git board prune',
    button: switched ? 'Remove git-bug’s data' : '',
  };
}

// cardsFor is Start over's cards: the resets, origin's with an origin,
// then git-bug's data while there is some.
export function cardsFor(st: StartOverStatus): Card[] {
  const cards = st.remote ? [reset, pushPrune, take] : [reset];
  if (st.git_bug.refs + st.git_bug.keys > 0 || st.git_bug.cache) cards.push(prune(st.switched));
  return cards;
}

// the way to start over everywhere, as the guide says (docs/guide/forges.md)
export const steps: string[] = [
  'Empty this clone’s board.',
  'Make origin’s board this clone’s.',
  'On every other clone, stop its automated syncs, then take origin’s board.',
  'Pull from the forges, on their rows: it refills the board.',
];

export const notes: string[] = [
  'A forge pull brings back every issue and comment that came from a forge, not yet what git-board itself posted to one. A repository where git-board never posted misses nothing.',
  'Every clone needs this version of git-board or a later one first: an earlier one doesn’t know the mark that keeps a clone left behind from doubling the board.',
  'Each action first writes what it deletes to a bundle in the git directory, and names it; git-bug’s refs on origin excepted.',
];

// canConfirm: the repository's name, typed exactly, spaces at the ends aside
export function canConfirm(typed: string, repo: string): boolean {
  return repo !== '' && typed.trim() === repo;
}

export function resultLine(r: StartOverResult): string {
  const kept = r.backup ? `, backed up in ${r.backup}` : ''; // on origin alone: nothing kept here
  const line = r.deleted ? `${r.deleted} refs deleted${kept}` : 'Nothing to delete';
  return r.me ? `${line}; board.me is unset: its person is gone` : line;
}
