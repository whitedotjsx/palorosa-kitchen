import { slugify } from '@palorosa-kitchen/core'

export type ReviewType =
  | 'duplicate_unit'
  | 'quantity_in_name'
  | 'ambiguous_unit'
  | 'unknown_classification'
  | 'duplicate_choice'
  | 'product_without_recipe'
  | 'unmapped_decoration'
  | 'variation_unmapped'
  | 'variation_suggests_choice'
  | 'evaluation_product_missing_in_wp'
  | 'excluded_wp_product'

export type ReviewSeverity = 'info' | 'decision'
export type ReviewStatus = 'info' | 'pending' | 'auto-applied' | 'resolved'

export interface ReviewItem {
  id: string
  type: ReviewType
  severity: ReviewSeverity
  title: string
  detail: string
  proposal?: string
  refs: string[]
  status: ReviewStatus
  resolution?: string
}

export interface ReviewInput {
  type: ReviewType
  severity: ReviewSeverity
  title: string
  detail: string
  proposal?: string
  refs: string[]
  id?: string
  status?: ReviewStatus
  resolution?: string
}

export interface ReviewDecisions {
  [id: string]: { action: 'accept' | 'reject' }
}

export function makeReviewId (type: ReviewType, subject: string): string {
  return `${type.replace(/_/g, '-')}-${slugify(subject)}`
}

export class ReviewQueue {
  readonly items: ReviewItem[] = []

  add (input: ReviewInput): ReviewItem {
    const id = input.id ?? makeReviewId(input.type, input.title)
    const status = input.status ?? (input.severity === 'info' ? 'info' : 'pending')
    const item: ReviewItem = {
      id,
      type: input.type,
      severity: input.severity,
      title: input.title,
      detail: input.detail,
      refs: input.refs,
      status,
      ...(input.proposal ? { proposal: input.proposal } : {}),
      ...(input.resolution ? { resolution: input.resolution } : {}),
    }
    this.items.push(item)
    return item
  }

  countByType (): Record<string, number> {
    const counts: Record<string, number> = {}
    for (const item of this.items) counts[item.type] = (counts[item.type] ?? 0) + 1
    return counts
  }

  sorted (): ReviewItem[] {
    return [...this.items].sort((a, b) => a.type.localeCompare(b.type) || a.id.localeCompare(b.id))
  }
}
