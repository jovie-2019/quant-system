// RegimeHeatmap renders a Symbol × Timeframe grid of regime labels.
//
// A discrete colour scale maps each regime label to one of six colours so
// operators can scan the matrix at a glance. Cells with no data are drawn
// as an "unknown" grey so missing seeds are obvious.
//
// Clicking a cell surfaces the (symbol, interval) to the parent via
// onCellClick, which the Dashboard uses to drive the drill-down panel.

import { useMemo } from 'react'
import ReactECharts from 'echarts-for-react'
import type { EChartsOption } from 'echarts'

import type { RegimeLabel, RegimeMatrixRow } from '../../api/regime'

export interface RegimeHeatmapProps {
  symbols: string[]
  intervals: string[]
  rows: RegimeMatrixRow[]
  height?: number | string
  onCellClick?: (symbol: string, interval: string, row?: RegimeMatrixRow) => void
}

// Discrete label → ordinal id for the ECharts visualMap.
const LABEL_ORDER: RegimeLabel[] = [
  'trend_up',
  'trend_down',
  'range',
  'high_vol',
  'low_liq',
  'unknown',
]

const LABEL_COLORS: Record<RegimeLabel, string> = {
  trend_up: '#52c41a',
  trend_down: '#f5222d',
  range: '#faad14',
  high_vol: '#722ed1',
  low_liq: '#8c8c8c',
  unknown: '#d9d9d9',
}

const LABEL_TEXT: Record<RegimeLabel, string> = {
  trend_up: '↑ 趋势',
  trend_down: '↓ 趋势',
  range: '震荡',
  high_vol: '高波动',
  low_liq: '低流动',
  unknown: '未知',
}

export default function RegimeHeatmap({
  symbols,
  intervals,
  rows,
  height = 360,
  onCellClick,
}: RegimeHeatmapProps) {
  const option = useMemo<EChartsOption>(() => {
    // Row[x=intervalIdx, y=symbolIdx, value=ordinal, rec=row for tooltip].
    const byKey = new Map<string, RegimeMatrixRow>()
    for (const r of rows) {
      byKey.set(`${r.symbol}|${r.interval}`, r)
    }

    type Cell = [number, number, number, RegimeMatrixRow | null]
    const data: Cell[] = []
    symbols.forEach((sym, yi) => {
      intervals.forEach((iv, xi) => {
        const rec = byKey.get(`${sym}|${iv}`) ?? null
        const label = (rec?.regime ?? 'unknown') as RegimeLabel
        const ord = LABEL_ORDER.indexOf(label)
        data.push([xi, yi, ord >= 0 ? ord : LABEL_ORDER.length - 1, rec])
      })
    })

    return {
      grid: { left: 100, right: 24, top: 24, bottom: 64 },
      tooltip: {
        position: 'top',
        formatter: (p: any) => {
          const [xi, yi, , rec] = p.value as Cell
          const sym = symbols[yi]
          const iv = intervals[xi]
          if (!rec) return `${sym} · ${iv}<br/>no data`
          const bar = new Date(rec.bar_time).toISOString().slice(0, 16).replace('T', ' ')
          return [
            `<strong>${sym} · ${iv}</strong>`,
            `${LABEL_TEXT[rec.regime]}  (conf ${(rec.confidence * 100).toFixed(0)}%)`,
            `bar ${bar}`,
            `ADX ${rec.adx.toFixed(1)}  Hurst ${rec.hurst.toFixed(2)}`,
            `BBW ${rec.bbw.toFixed(4)}  ATR ${rec.atr.toFixed(4)}`,
          ].join('<br/>')
        },
      },
      xAxis: {
        type: 'category',
        data: intervals,
        splitArea: { show: true },
        axisLabel: { fontSize: 12 },
      },
      yAxis: {
        type: 'category',
        data: symbols,
        splitArea: { show: true },
        axisLabel: { fontSize: 12 },
      },
      visualMap: {
        show: false,
        type: 'piecewise',
        pieces: LABEL_ORDER.map((label, i) => ({
          value: i,
          label: LABEL_TEXT[label],
          color: LABEL_COLORS[label],
        })),
      },
      series: [
        {
          type: 'heatmap',
          // ECharts' strict typings want heatmap data as [number, number, number];
          // our Cell tuple carries the original row in the 4th slot for tooltip
          // lookup. The runtime accepts extra fields, so cast away.
          data: data as unknown as number[][],
          label: {
            show: true,
            formatter: (p: any) => {
              const [, , , rec] = p.value as Cell
              return rec ? LABEL_TEXT[rec.regime as RegimeLabel] : '—'
            },
            fontSize: 11,
            color: '#fff',
            fontWeight: 500,
          },
          itemStyle: {
            borderColor: '#fff',
            borderWidth: 1,
          },
          emphasis: {
            itemStyle: { shadowBlur: 6, shadowColor: 'rgba(0,0,0,0.25)' },
          },
        },
      ],
    }
  }, [symbols, intervals, rows])

  const onEvents = useMemo(
    () => ({
      click: (p: any) => {
        if (!onCellClick) return
        const [xi, yi, , rec] = p.value as [number, number, number, RegimeMatrixRow | null]
        onCellClick(symbols[yi], intervals[xi], rec ?? undefined)
      },
    }),
    [onCellClick, symbols, intervals],
  )

  return (
    <ReactECharts
      option={option}
      notMerge
      lazyUpdate
      style={{ height, width: '100%' }}
      onEvents={onEvents}
    />
  )
}

export { LABEL_COLORS, LABEL_TEXT, LABEL_ORDER }
