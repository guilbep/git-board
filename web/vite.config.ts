// The web app's build: Vue on Vite, no framework around it (ADR 0023).
// It builds into dist/spa, which the binary embeds (web/embed.go); each
// compressible file gets a .br beside it, which the server sends to a
// browser that takes brotli (surface/http/static.go).
import { fileURLToPath, URL } from 'node:url';
import { readdirSync, readFileSync, statSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { brotliCompressSync, constants } from 'node:zlib';
import { defineConfig, type Plugin } from 'vite';
import vue from '@vitejs/plugin-vue';

const out = fileURLToPath(new URL('./dist/spa', import.meta.url));

// brotli: each .js .css .html .svg .json of 1 KB or more, at its best
function brotli(): Plugin {
  const walk = (dir: string): string[] => readdirSync(dir).flatMap((f) => (statSync(join(dir, f)).isDirectory() ? walk(join(dir, f)) : [join(dir, f)]));
  return {
    name: 'git-board-brotli',
    apply: 'build',
    closeBundle() {
      for (const f of walk(out)) {
        if (!/\.(js|css|html|svg|json|map|txt)$/.test(f)) continue;
        const data = readFileSync(f);
        if (data.length < 1024) continue;
        writeFileSync(`${f}.br`, brotliCompressSync(data, { params: { [constants.BROTLI_PARAM_QUALITY]: 11, [constants.BROTLI_PARAM_SIZE_HINT]: data.length } }));
      }
    },
  };
}

// the dev server's page talks to `git board serve` on :8080 and to Vite's
// own socket: its CSP says so; the built page's allows 'self' alone
function devCSP(): Plugin {
  return {
    name: 'git-board-dev-csp',
    apply: 'serve',
    transformIndexHtml: (html) => html.replace("img-src 'self' data: https:;", "img-src 'self' data: https:; connect-src 'self' ws://localhost:* http://localhost:8080; worker-src 'self' blob:;"),
  };
}

export default defineConfig({
  plugins: [vue(), brotli(), devCSP()],
  resolve: { alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) } },
  build: { outDir: out, emptyOutDir: true, target: 'baseline-widely-available', reportCompressedSize: false },
  // the guides the Settings page shows (src/api/texts.ts) are the repo's
  server: { port: 9000, fs: { allow: ['..'] } },
});
