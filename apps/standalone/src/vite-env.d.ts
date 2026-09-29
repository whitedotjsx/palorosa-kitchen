/// <reference types="vite/client" />

declare module 'virtual:catalog-snapshot' {
  import type { Catalog } from '@palorosa-kitchen/core'
  export const catalog: Catalog
  export const catalogMeta: { mtime: string }
}
