// The project file as the Advanced tab's editor holds it, unsaved edits
// and all. The form tabs don't come through here: each saves its own
// section through the API (configmodel.ts). Labels' Adopt used to stage
// its change in this draft; it saves through the API directly now.
import { ref } from 'vue';

export const configText = ref('');
