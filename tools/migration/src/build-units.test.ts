import { describe, expect, it } from 'vitest'
import { buildUnits } from './build-units'
import { ReviewQueue, type ReviewDecisions } from './review'

function run (
  kitchenFoods: Record<string, string>,
  decorations: Record<string, string>,
  decisions: ReviewDecisions = {}
) {
  const queue = new ReviewQueue()
  const result = buildUnits({ kitchenFoods, decorations, decisions, queue })
  return { result, queue }
}

describe('buildUnits', () => {
  it('merges duplicates across kitchenFoods and decorations', () => {
    const { result } = run(
      { JUGO: 'Jugo de naranja semi natural pequeño' },
      { JUGO_DEC: 'Jugo de naranja semi natural pequeño', GLOBO: 'Globo con helio' }
    )
    expect(result.units).toHaveLength(1)
    expect(result.duplicateMerges).toHaveLength(1)
    expect(result.presentation).toHaveLength(1)
    expect(result.resolve('Jugo de naranja semi natural pequeño')?.unit.id).toBe(
      'jugo-de-naranja-semi-natural-pequeno'
    )
  })

  it('applies the quantity proposal by default', () => {
    const { result, queue } = run({ ENSALADAS: 'Dos mini ensaladas de frutas' }, {})
    expect(result.units[0]?.name).toBe('mini ensalada de fruta')
    expect(result.resolve('Dos mini ensaladas de frutas')?.quantity).toBe(2)
    const item = queue.items.find((entry) => entry.type === 'quantity_in_name')
    expect(item?.status).toBe('auto-applied')
  })

  it('keeps names with quantity one unchanged', () => {
    const { result, queue } = run({ DURAZNO: 'Un durazno' }, {})
    expect(result.units[0]?.name).toBe('Un durazno')
    expect(queue.countByType().quantity_in_name).toBeUndefined()
  })

  it('keeps the original name when a decision rejects the proposal', () => {
    const { result } = run({ ENSALADAS: 'Dos mini ensaladas de frutas' }, {}, {
      'quantity-in-name-dos-mini-ensaladas-de-frutas': { action: 'reject' },
    })
    expect(result.units[0]?.name).toBe('Dos mini ensaladas de frutas')
    expect(result.resolve('Dos mini ensaladas de frutas')?.quantity).toBe(1)
  })

  it('queues ambiguous and unknown names for review', () => {
    const { queue } = run({}, { QUESOS: 'Quesos casita (?)', MISTERIO: 'Motor de arranque' })
    expect(queue.countByType().ambiguous_unit).toBe(1)
    expect(queue.countByType().unknown_classification).toBe(1)
  })
})
