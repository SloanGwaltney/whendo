import { vitePreprocess } from '@sveltejs/vite-plugin-svelte'

export default {
  // Consult https://svelte.dev/docs#compile-time-svelte-preprocess
  // for more information about preprocessors
  preprocess: vitePreprocess(),
  // Enforce Svelte 5 runes everywhere so legacy Svelte 4 syntax is a compile error.
  compilerOptions: {
    runes: true,
  },
}
