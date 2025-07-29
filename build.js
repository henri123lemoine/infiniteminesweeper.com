const esbuild = require('esbuild');

esbuild.build({
  entryPoints: ['frontend/app.jsx'],
  bundle: true,
  minify: true,
  outfile: 'bundle.js',
  format: 'iife',
  target: ["es2020"],
  loader: {'.js': 'jsx', '.jsx': 'jsx'},
  external: ['protobufjs', 'pako']
}).catch(() => process.exit(1));
