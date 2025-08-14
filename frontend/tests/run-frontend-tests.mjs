export { }
import fs from 'node:fs'
import path from 'node:path'

const repoRoot = path.resolve(path.dirname(new URL(import.meta.url).pathname), '..', '..')
const useGameStatePath = path.join(repoRoot, 'frontend', 'src', 'useGameState.js')

function assert(cond, msg) { if (!cond) { console.error('FAIL:', msg); process.exitCode = 1 } }
function ok(msg) { console.log('OK  :', msg) }

const src = fs.readFileSync(useGameStatePath, 'utf8')
const chunkMatch = src.match(/export\s+const\s+CHUNK\s*=\s*(\d+)/)
assert(!!chunkMatch, 'CHUNK constant is exported')
if (chunkMatch) {
  const val = Number(chunkMatch[1])
  assert(val === 64, `CHUNK expected 64, got ${val}`)
  if (val === 64) ok('CHUNK === 64')
}

assert(src.includes('const worldToChunk ='), 'worldToChunk helper present')
if (src.includes('const worldToChunk =')) ok('worldToChunk present')

assert(src.includes('const isMineWith ='), 'isMineWith helper present')
if (src.includes('const isMineWith =')) ok('isMineWith present')

const paletteInit = src.includes('const minimapPalette = new Uint32Array(256)')
assert(paletteInit, 'minimap palette has 256 entries')
if (paletteInit) ok('minimap palette sized 256')

assert(src.includes('function encodeMsg('), 'encodeMsg present')
assert(src.includes('function decodeMsg('), 'decodeMsg present')
ok('encodeMsg/decodeMsg present')

if (process.exitCode) {
  console.error('\nSome frontend checks failed.')
  process.exit(process.exitCode)
} else {
  console.log('\nAll frontend checks passed.')
}

