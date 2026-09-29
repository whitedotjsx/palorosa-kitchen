import { fileURLToPath } from 'node:url'

export const rootDir = fileURLToPath(new URL('../../../', import.meta.url))
export const dataDir = fileURLToPath(new URL('../../../data/', import.meta.url))
