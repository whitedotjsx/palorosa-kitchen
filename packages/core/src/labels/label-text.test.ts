import { describe, expect, it } from 'vitest'
import { labelsFromLines, labelsFromPages } from './label-text'

describe('labelsFromLines', () => {
  it('joins wrapped field lines', () => {
    const labels = labelsFromLines([
      'Pedido: 73159',
      'Barrio: Chicó norte - Localidad: Chapinero, Bogotá',
      'calle 96 #21-70 edificio low apto 501',
      'Quien recibe: Arturo Trujillo',
      'Desayuno: Box Hombre X 1: Sándwich Jamón y',
      'queso, Yogurt con',
      'granola, Cumpleaños',
      'Adicionales: Globo con helio X 2',
      'Observaciones:',
    ])
    expect(labels).toHaveLength(1)
    expect(labels[0]).toMatchObject({
      orderNumber: '73159',
      breakfastText: 'Box Hombre X 1: Sándwich Jamón y queso, Yogurt con granola, Cumpleaños',
      additionalsText: 'Globo con helio X 2',
    })
  })

  it('starts a new label at every Pedido line', () => {
    const labels = labelsFromLines([
      'Pedido: 1',
      'Desayuno: Morning X 1',
      'Pedido: 2',
      'Desayuno: Libro X 1',
    ])
    expect(labels.map(label => label.orderNumber)).toEqual(['1', '2'])
    expect(labels[1]?.breakfastText).toBe('Libro X 1')
  })

  it('ignores page headers without a label body', () => {
    const labels = labelsFromLines(['RUTA 3: FELIPE', '72887 72894', 'Pedido: 5', 'Desayuno: Libro X 1'])
    expect(labels).toHaveLength(1)
    expect(labels[0]?.orderNumber).toBe('5')
  })
})

describe('labelsFromPages', () => {
  it('flattens pages', () => {
    const labels = labelsFromPages([
      ['Pedido: 1', 'Desayuno: Morning X 1'],
      ['Pedido: 2', 'Desayuno: Libro X 1'],
    ])
    expect(labels.map(label => label.orderNumber)).toEqual(['1', '2'])
  })
})
