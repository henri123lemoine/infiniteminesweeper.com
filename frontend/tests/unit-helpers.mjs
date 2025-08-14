export { }
import fs from 'node:fs'
import path from 'node:path'

const root = path.resolve(path.dirname(new URL(import.meta.url).pathname), '..', '..')
const file = path.join(root, 'frontend', 'src', 'useGameState.js')
const src = fs.readFileSync(file, 'utf8')

function extractConst(name) {
  const start = src.indexOf(`const ${name} =`)
  if (start === -1) return null
  const braceStart = src.indexOf('{', start)
  if (braceStart === -1) return null
  let i = braceStart, depth = 0
  while (i < src.length) {
    const ch = src[i++]
    if (ch === '{') depth++
    else if (ch === '}') {
      depth--
      if (depth === 0) break
    }
  }
  return src.slice(start, i)
}

const names = ['splitmix64', 'isMineWith', 'worldToChunk']
const pieces = names.map(extractConst)
for (let i = 0; i < pieces.length; i++) {
  if (!pieces[i]) { console.error('FAIL: missing', names[i]); process.exit(1) }
}

let bundle = ''
for (let i = 0; i < names.length; i++) bundle += pieces[i] + `;\n` + `globalThis.${names[i]} = ${names[i]};\n`
eval(bundle)

function assert(cond, msg) { if (!cond) { console.error('FAIL:', msg); process.exit(1) } }
function ok(msg) { console.log('OK  :', msg) }

for (const s of [0n, 1n, 2n, 123456789n, 0xffffffffffffffffn]) {
  const a = splitmix64(s)
  const b = splitmix64(s)
  assert(a === b, 'splitmix64 deterministic')
}
ok('splitmix64 determinism')

assert(isMineWith(0n, 0, 0) === false, 'density 0 yields false')
assert(isMineWith(0n, 1, 0) === true, 'density 1 yields true')
assert(isMineWith(0n, -5, 0) === false, 'negative density clamped to 0')
assert(isMineWith(0n, 2.5, 0) === true, 'density >1 clamped to 1')
assert(isMineWith('bad', 0.5, 0) === null, 'invalid seed rejected')
assert(isMineWith(0n, 0.5, 999999) === null, 'invalid cell rejected')
assert(isMineWith(0n, NaN, 0) === null, 'invalid density rejected')
ok('isMineWith validation & clamping')

const CHUNK = 64
function check(x, y) {
  const { chunkX, chunkY, cell } = worldToChunk(x, y)
  assert(Number.isInteger(chunkX) && Number.isInteger(chunkY), 'chunk ints')
  assert(cell >= 0 && cell < CHUNK*CHUNK, 'cell in range')
  const lx = cell % CHUNK, ly = Math.floor(cell / CHUNK)
  const wx = chunkX * CHUNK + lx, wy = chunkY * CHUNK + ly
  assert(wx === x && wy === y, `invert matches (${x},${y})`)
}
for (const [x,y] of [[0,0],[63,63],[64,64],[-1,-1],[-64,-64],[-65,66],[1024,-2048]]) check(x,y)
ok('worldToChunk mapping & invertibility')

console.log('\nAll helper unit checks passed.')

