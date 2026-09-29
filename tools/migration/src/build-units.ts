import {
  normalizeName,
  slugify,
  type KitchenUnit,
} from '@palorosa-kitchen/core'
import { classifyName, inferMeasure, inferUnitCategory } from './classify'
import { detectLeadingQuantity } from './quantity'
import { makeReviewId, type ReviewDecisions, type ReviewQueue } from './review'

export interface DuplicateMerge {
  keptId: string
  keptName: string
  mergedName: string
  refs: string[]
}

export interface PresentationEntry {
  name: string
  refs: string[]
}

export interface QuantityApplied {
  reviewId: string
  rawName: string
  baseName: string
  quantity: number
  refs: string[]
}

export interface ResolvedUnit {
  unit: KitchenUnit
  quantity: number
}

export interface BuildUnitsInput {
  kitchenFoods: Record<string, string>
  decorations: Record<string, string>
  decisions: ReviewDecisions
  queue: ReviewQueue
}

export interface BuildUnitsResult {
  units: KitchenUnit[]
  duplicateMerges: DuplicateMerge[]
  presentation: PresentationEntry[]
  quantityApplied: QuantityApplied[]
  resolve: (rawName: string) => ResolvedUnit | null
}

interface UnitDraft {
  unit: KitchenUnit
  aliases: Set<string>
}

const AMBIGUOUS_NAMES = new Set([
  'bowl grande',
  'bowl en copa de cristal',
  'cristal zone dulce',
  'cristal zone salado',
  'quesos casita',
  'tablas de quesos barquito',
  'frasco largo de quesos',
  'pinchos de quesos',
])

export function buildUnits (input: BuildUnitsInput): BuildUnitsResult {
  const drafts = new Map<string, UnitDraft>()
  const rawIndex = new Map<string, { unitKey: string; quantity: number }>()
  const rawSources = new Map<string, string[]>()
  const usedSlugs = new Set<string>()
  const duplicateMerges: DuplicateMerge[] = []
  const presentation: PresentationEntry[] = []
  const quantityApplied: QuantityApplied[] = []

  const uniqueId = (base: string): string => {
    const seed = base || 'unit'
    let id = seed
    let counter = 2
    while (usedSlugs.has(id)) {
      id = `${seed}-${counter}`
      counter += 1
    }
    usedSlugs.add(id)
    return id
  }

  const sourceOf = (ref: string): string => ref.slice(0, ref.indexOf('.'))

  const register = (rawName: string, ref: string): void => {
    const normalizedRaw = normalizeName(rawName)
    const existingRaw = rawIndex.get(normalizedRaw)
    if (existingRaw) {
      const existingDraft = drafts.get(existingRaw.unitKey)
      existingDraft?.aliases.add(rawName)
      const refs = rawSources.get(normalizedRaw) ?? []
      const source = sourceOf(ref)
      if (refs.length > 0 && !refs.some((existingRef) => sourceOf(existingRef) === source)) {
        const keptName = existingDraft?.unit.name ?? rawName
        duplicateMerges.push({
          keptId: existingDraft?.unit.id ?? '',
          keptName,
          mergedName: rawName,
          refs: [ref],
        })
        input.queue.add({
          type: 'duplicate_unit',
          severity: 'info',
          title: rawName,
          detail: `Merged with "${keptName}".`,
          refs: [ref],
        })
      }
      refs.push(ref)
      rawSources.set(normalizedRaw, refs)
      return
    }
    rawSources.set(normalizedRaw, [ref])

    const quantityMatch = detectLeadingQuantity(rawName)
    const reviewId = quantityMatch ? makeReviewId('quantity_in_name', rawName) : null
    const rejected = reviewId ? input.decisions[reviewId]?.action === 'reject' : false
    const accepted = quantityMatch !== null && quantityMatch.quantity > 1 && !rejected
    const unitName = accepted && quantityMatch ? quantityMatch.baseName : rawName
    const multiplier = accepted && quantityMatch ? quantityMatch.quantity : 1
    const normalizedUnit = normalizeName(unitName)

    const existingDraft = drafts.get(normalizedUnit)
    if (existingDraft) {
      existingDraft.aliases.add(rawName)
      duplicateMerges.push({
        keptId: existingDraft.unit.id,
        keptName: existingDraft.unit.name,
        mergedName: rawName,
        refs: [ref],
      })
      input.queue.add({
        type: 'duplicate_unit',
        severity: 'info',
        title: rawName,
        detail: `Merged with "${existingDraft.unit.name}".`,
        refs: [ref],
      })
    } else {
      const unit: KitchenUnit = {
        id: uniqueId(slugify(unitName)),
        name: unitName,
        category: inferUnitCategory(unitName),
        measure: inferMeasure(unitName),
        aliases: [rawName],
        isKitchen: true,
        active: true,
      }
      drafts.set(normalizedUnit, { unit, aliases: new Set([rawName]) })
    }
    rawIndex.set(normalizedRaw, { unitKey: normalizedUnit, quantity: multiplier })

    if (accepted && quantityMatch && reviewId) {
      quantityApplied.push({
        reviewId,
        rawName,
        baseName: quantityMatch.baseName,
        quantity: multiplier,
        refs: [ref],
      })
      input.queue.add({
        id: reviewId,
        type: 'quantity_in_name',
        severity: 'decision',
        status: rejected ? 'info' : 'auto-applied',
        title: rawName,
        detail: `Name embeds a quantity. Proposal: unit "${quantityMatch.baseName}" with quantity ${quantityMatch.quantity}. Set this id to reject in review-decisions.json to keep the original name.`,
        proposal: `${quantityMatch.baseName} x${quantityMatch.quantity}`,
        refs: [ref],
      })
    }

    if (rawName.includes('?') || AMBIGUOUS_NAMES.has(normalizedUnit) || normalizedUnit.startsWith('bowl ')) {
      input.queue.add({
        id: makeReviewId('ambiguous_unit', rawName),
        type: 'ambiguous_unit',
        severity: 'decision',
        title: rawName,
        detail: 'Generic unit name. Define its components or split it into concrete units.',
        refs: [ref],
      })
    }
  }

  for (const [key, name] of Object.entries(input.kitchenFoods)) {
    register(name, `kitchenFoods.${key}`)
  }

  for (const [key, name] of Object.entries(input.decorations)) {
    const ref = `decorations.${key}`
    const classification = classifyName(name)
    if (classification === 'presentation') {
      const entry = presentation.find((item) => item.name === name)
      if (entry) entry.refs.push(ref)
      else presentation.push({ name, refs: [ref] })
      continue
    }
    register(name, ref)
    if (classification === 'unknown') {
      input.queue.add({
        id: makeReviewId('unknown_classification', name),
        type: 'unknown_classification',
        severity: 'decision',
        title: name,
        detail: 'Not recognized as food or presentation. Currently treated as a kitchen unit.',
        proposal: 'food',
        refs: [ref],
      })
    }
  }

  const units = [...drafts.values()]
    .map((draft) => {
      draft.unit.aliases = [...draft.aliases].sort()
      return draft.unit
    })
    .sort((a, b) => a.id.localeCompare(b.id))

  const resolve = (rawName: string): ResolvedUnit | null => {
    const entry = rawIndex.get(normalizeName(rawName))
    if (!entry) return null
    const draft = drafts.get(entry.unitKey)
    if (!draft) return null
    return { unit: draft.unit, quantity: entry.quantity }
  }

  return { units, duplicateMerges, presentation, quantityApplied, resolve }
}
