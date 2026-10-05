// WCAG contrast audit for the panel design tokens.
// Run: node apps/panel/design/check-contrast.mjs
// Every text pair must reach AA (4.5:1) or better.

const hex = (h) => {
  h = h.replace('#', '')
  if (h.length === 3) h = h.split('').map((c) => c + c).join('')
  return [0, 2, 4].map((i) => parseInt(h.slice(i, i + 2), 16))
}

const lum = (h) => {
  const [r, g, b] = hex(h).map((v) => {
    const s = v / 255
    return s <= 0.03928 ? s / 12.92 : Math.pow((s + 0.055) / 1.055, 2.4)
  })
  return 0.2126 * r + 0.7152 * g + 0.0722 * b
}

const ratio = (a, b) => {
  const [l1, l2] = [lum(a), lum(b)].sort((x, y) => y - x)
  return (l1 + 0.05) / (l2 + 0.05)
}

const T = {
  bg: '#f9eadc',
  surface: '#fffaf5',
  alt: '#f9e9e5',
  text: '#6a4b20',
  muted: '#7d5c45',
  accent: '#a55d26',
  accentHover: '#8f4d1e',
  success: '#646d1c',
  successSoft: '#f4f3e4',
  warning: '#9c5722',
  warningSoft: '#fdeee2',
  danger: '#9c3a1c',
  onDarkOk: '#c6caa0',
  onDarkWarn: '#e6c86a',
}

const pairs = [
  ['text on bg', T.text, T.bg],
  ['text on surface', T.text, T.surface],
  ['text on sidebar (alt)', T.text, T.alt],
  ['muted on bg', T.muted, T.bg],
  ['muted on surface', T.muted, T.surface],
  ['muted on sidebar (alt)', T.muted, T.alt],
  ['accent link on surface', T.accent, T.surface],
  ['accent link hover on surface', T.accentHover, T.surface],
  ['cream text on accent', T.surface, T.accent],
  ['cream text on accent hover', T.surface, T.accentHover],
  ['log text on brown pane', T.bg, T.text],
  ['log ok on brown', T.onDarkOk, T.text],
  ['log warn on brown', T.onDarkWarn, T.text],
  ['log button text on muted', T.bg, T.muted],
  ['log button text on accent hover', T.bg, T.accentHover],
  ['success stamp on soft', T.success, T.successSoft],
  ['warning stamp on soft', T.warning, T.warningSoft],
  ['danger on surface', T.danger, T.surface],
]

let fails = 0
for (const [label, fg, bg] of pairs) {
  const r = ratio(fg, bg)
  const verdict = r >= 4.5 ? 'AA' : r >= 3 ? 'AA-large' : 'FAIL'
  if (verdict === 'FAIL') fails++
  console.log(`${r.toFixed(2).padStart(6)}  ${verdict.padEnd(9)} ${label}`)
}
console.log(fails === 0 ? '\nAll text pairs meet AA.' : `\n${fails} pair(s) below AA.`)
process.exit(fails === 0 ? 0 : 1)
