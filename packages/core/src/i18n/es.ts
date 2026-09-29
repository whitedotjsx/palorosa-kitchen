import type {
  IgnoredReason,
  LineWarning,
  MeasureUnit,
  ProductCategory,
  UnitCategory,
  UnresolvedReason,
} from '../schema'

export const unitCategoryLabels: Record<UnitCategory, string> = {
  drink: 'Bebidas',
  main: 'Comidas principales',
  side: 'Acompañamientos',
  dessert: 'Postres y dulces',
  fruit: 'Frutas',
  condiment: 'Condimentos',
  other: 'Otros',
}

export const productCategoryLabels: Record<ProductCategory, string> = {
  breakfast: 'Desayunos',
  add_on: 'Adicionales',
  flower: 'Flores',
  stuffed_animal: 'Peluches',
  accessory: 'Accesorios',
  gift_box: 'Detalles y cajas',
  bag: 'Bolsos',
}

export const measureLabels: Record<MeasureUnit, string> = {
  unit: 'Unidad',
  portion: 'Porción',
  glass: 'Vaso',
  bottle: 'Botella',
  slice: 'Tajada',
  spoon: 'Cucharada',
  gram: 'Gramo',
}

export const unresolvedReasonLabels: Record<UnresolvedReason, string> = {
  unknown_product: 'Producto no reconocido',
  missing_recipe: 'Producto sin receta',
  missing_choice: 'Opción no reconocida',
  ambiguous_match: 'Coincidencia ambigua',
  missing_unit: 'Unidad no encontrada',
}

export const ignoredReasonLabels: Record<IgnoredReason, string> = {
  not_kitchen: 'No va a cocina',
  inactive_unit: 'Unidad inactiva',
}

export const warningLabels: Record<LineWarning, string> = {
  missing_quantity: 'Cantidad no indicada en el rótulo; se asumió 1',
}

export const listLabels = {
  title: 'Lista de cocina',
  deliveryDate: 'Fecha de entrega',
  generatedAt: 'Generado',
  quantity: 'Cantidad',
  item: 'Ítem',
  category: 'Categoría',
  measure: 'Medida',
  note: 'Nota',
  source: 'Pedidos',
  unresolved: 'Sin resolver',
  ignored: 'No va a cocina',
  warnings: 'Avisos',
  empty: 'No hay unidades para esta fecha',
}

export const diffLabels = {
  title: 'Cambios en la lista',
  empty: 'Sin cambios en la lista',
}

export const templateLabels = {
  title: 'Cocina en Palorosa',
  others: 'OTROS (SIN MAPEAR)',
  unresolved: 'SIN RESOLVER',
}

export const botLabels = {
  commands: 'Comandos: "lista", "lista mañana", "estado".',
  noList: 'No hay lista publicada para {date}.',
  statusReady: 'Bot activo. Hoy es {date}. Listas publicadas: {dates}.',
  statusNoLists: 'Bot activo. Hoy es {date}. Aún no hay listas publicadas.',
  newOrder: 'Pedido nuevo',
  updatedOrder: 'Pedido actualizado',
}

export const serverLabels = {
  title: 'Cocina Palorosa',
  listNotBuilt: 'La lista no está construida todavía. Ejecuta pnpm build:list en la máquina del servidor.',
  editorUnavailable: 'El editor de catálogo no está corriendo.',
  editorLocalOnly: 'El editor de catálogo solo se puede abrir desde la máquina del servidor.',
  pdfUnavailable: 'No se pudo generar el PDF de la plantilla.',
  notFound: 'No encontrado',
  invalidRequest: 'Solicitud inválida',
}

export const appLabels = {
  title: 'Lista de cocina',
  importTitle: 'Importar pedidos',
  dropHint: 'Arrastra aquí el PDF de rótulos, el export de órdenes (XLSX o CSV) o un JSON de pedidos',
  chooseFile: 'Elegir archivo',
  clear: 'Limpiar',
  catalog: 'Catálogo',
  snapshotDate: 'Snapshot del catálogo',
  deliveryDate: 'Fecha de entrega',
  ordersCount: 'Pedidos',
  linesCount: 'Líneas',
  print: 'Imprimir',
  copy: 'Copiar texto',
  copied: 'Texto copiado',
  downloadCsv: 'Descargar CSV',
  unresolvedTitle: 'Sin resolver',
  ignoredProductsTitle: 'No van a cocina',
  ignoredUnitsTitle: 'No van a cocina',
  warningsTitle: 'Avisos',
  skippedTitle: 'Entradas ignoradas',
  mappingsTitle: 'Asignaciones guardadas',
  mapToProduct: 'Asignar producto',
  selectProduct: 'Selecciona un producto',
  mappingClear: 'Quitar',
  mappingHint: 'Asigna el texto a un producto del catálogo para incluirlo en la lista.',
  noOrders: 'Importa un archivo para generar la lista.',
  reading: 'Leyendo archivo…',
  pdfFallback: 'Se usó el lector PDF avanzado (pdf.js).',
  pdfFailed: 'No se pudo leer el PDF.',
  pdfEmpty: 'No se encontraron rótulos en el PDF.',
  jsonFailed: 'No se pudo leer el JSON.',
  jsonEmpty: 'No se encontraron pedidos en el JSON.',
  xlsxFailed: 'No se pudo leer el XLSX.',
  unsupportedFile: 'Formato no soportado. Usa un PDF de rótulos, un export XLSX o CSV, o un JSON de pedidos.',
  unsupportedTable: 'No se reconoció el formato del archivo. Se espera el export de órdenes con columnas "ID orden" y "Productos".',
  unreadableStreams: 'Rótulos sin texto reconocible',
  references: 'Pedidos',
  published: 'Publicada en el servidor',
  publishFailed: 'No se pudo publicar la lista en el servidor',
}
