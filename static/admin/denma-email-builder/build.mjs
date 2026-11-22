// Builds the visual e-mail builder into <dist>/email-builder/email-builder.umd.js (called by ../build.mjs).
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const self = fileURLToPath(import.meta.url);
const root = path.dirname(self);

export async function build(dist) {
  const install = Bun.spawnSync([process.execPath, 'install', '--frozen-lockfile'], { cwd: root, stdout: 'inherit', stderr: 'inherit' });
  if (install.exitCode !== 0) throw new Error('email-builder: bun install failed');

  // Bundled in a new process, which sees the packages just installed.
  const bundle = Bun.spawnSync([process.execPath, self, dist], { stdout: 'inherit', stderr: 'inherit' });
  if (bundle.exitCode !== 0) throw new Error('email-builder: build failed');
}

async function bundle(dist) {
  const result = await Bun.build({
    entrypoints: [path.join(root, 'src/denma.tsx')],
    outdir: path.join(dist, 'email-builder'),
    naming: 'email-builder.umd.js',
    format: 'iife',
    minify: true,
    target: 'browser',
    define: { 'process.env.NODE_ENV': '"production"', 'import.meta.env': '{"MODE":"production"}' },
  });
  if (!result.success) throw new AggregateError(result.logs, 'email-builder: build failed');
  console.log('built the e-mail builder -> dist/email-builder');
}

if (import.meta.main) await bundle(path.resolve(process.argv[2]));
