import neostandard from 'neostandard'

export default [
  ...neostandard({ ts: true }),
  {
    ignores: ['**/dist/**', 'data/**', 'apps/wp-plugin/plugin/**', 'tools/migration/source/**'],
  },
]
