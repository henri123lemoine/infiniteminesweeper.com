const esbuild = require("esbuild");
require('dotenv').config();

const dev = process.env.DEV === "true" || process.env.DEV === "1";

esbuild.build({
  entryPoints: ["frontend/app.jsx"],
  bundle: true,
  minify: true,
  outfile: "bundle.js",
  format: "iife",
  target: ["es2020"],
  loader: { ".js": "jsx", ".jsx": "jsx" },
  external: ["protobufjs", "pako"],
  define: {
    "DEV_MODE": JSON.stringify(dev),
  },
}).catch(() => process.exit(1));
