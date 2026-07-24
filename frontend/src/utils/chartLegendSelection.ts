import {
  Chart,
  Legend,
  type ChartEvent,
  type ChartType,
  type LegendElement,
  type LegendItem
} from 'chart.js'

function itemVisible(legend: LegendElement<ChartType>, item: LegendItem): boolean {
  if (typeof item.datasetIndex === 'number') {
    return legend.chart.isDatasetVisible(item.datasetIndex)
  }
  if (typeof item.index === 'number') {
    return legend.chart.getDataVisibility(item.index)
  }
  return false
}

function setItemVisible(
  legend: LegendElement<ChartType>,
  item: LegendItem,
  visible: boolean
): void {
  if (typeof item.datasetIndex === 'number') {
    legend.chart.setDatasetVisibility(item.datasetIndex, visible)
    return
  }
  if (typeof item.index === 'number' && legend.chart.getDataVisibility(item.index) !== visible) {
    legend.chart.toggleDataVisibility(item.index)
  }
}

/**
 * First click focuses one series. Further clicks accumulate or remove series;
 * selecting every legend item naturally returns the chart to its all-visible state.
 */
export function focusOrAccumulateLegendClick(
  _event: ChartEvent,
  clickedItem: LegendItem,
  legend: LegendElement<ChartType>
): void {
  const items = legend.legendItems || []
  if (!items.length) return

  const visibleItems = items.filter((item) => itemVisible(legend, item))
  if (visibleItems.length === items.length) {
    for (const item of items) setItemVisible(legend, item, item === clickedItem)
  } else if (!itemVisible(legend, clickedItem)) {
    setItemVisible(legend, clickedItem, true)
  } else if (visibleItems.length > 1) {
    setItemVisible(legend, clickedItem, false)
  }

  legend.chart.update()
}

// Install before any chart component registers the Legend plugin.
if (Legend.defaults) {
  Legend.defaults.onClick = focusOrAccumulateLegendClick
}

// Also cover charts registered before this module in tests or alternate entry points.
if (Chart.defaults.plugins.legend) {
  Chart.defaults.plugins.legend.onClick = focusOrAccumulateLegendClick
}
