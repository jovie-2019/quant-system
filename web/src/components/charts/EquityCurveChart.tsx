// EquityCurveChart renders a backtest's mark-to-market curve as a smooth
// area chart. Cash is overlaid as a dashed line so the viewer can see how
// much of the equity is committed to open positions at any given bar.
//
// The component accepts raw EquityPoint[] from the API and is fully
// controlled — re-rendering on prop change is free because ECharts handles
// the diffing internally.

import { useMemo } from 'react'
import ReactECharts from 'echarts-for-react'
import type { EChartsOption } from 'echarts'

import type { EquityPoint } from '../../api/backtests'

export interface EquityCurveChartProps {
  equity: EquityPoint[]
  height?: number | string
  title?: string
}

export default function EquityCurveChart({
  equity,
  height = 360,
  title,
}: EquityCurveChartProps) {
  const option = useMemo<EChartsOption>(() => {
    const timeAxis = equity.map((p) => new Date(p.TSMS).toISOString())
    const mtm = equity.map((p) => Number(p.MarkToMarket.toFixed(4)))
    const cash = equity.map((p) => Number(p.Cash.toFixed(4)))

    return {
      title: title
        ? { text: title, left: 'left', textStyle: { fontSize: 14, fontWeight: 500 } }
        : undefined,
      grid: { left: 56, right: 24, top: title ? 40 : 16, bottom: 48 },
      tooltip: {
        trigger: 'axis',
        axisPointer: { type: 'line' },
        valueFormatter: (v) => (typeof v === 'number' ? v.toFixed(2) : String(v)),
      },
      legend: {
        data: ['Mark-to-Market', 'Cash'],
        top: title ? 40 : 8,
        right: 0,
        textStyle: { fontSize: 12 },
      },
      xAxis: {
        type: 'category',
        data: timeAxis,
        boundaryGap: false,
        axisLabel: {
          formatter: (value: string) => value.slice(5, 16).replace('T', ' '),
          fontSize: 11,
        },
      },
      yAxis: {
        type: 'value',
        scale: true,
        axisLabel: { fontSize: 11 },
        splitLine: { lineStyle: { type: 'dashed', opacity: 0.4 } },
      },
      dataZoom: [
        { type: 'inside', throttle: 50 },
        { type: 'slider', height: 18, bottom: 4 },
      ],
      series: [
        {
          name: 'Mark-to-Market',
          type: 'line',
          smooth: true,
          showSymbol: false,
          data: mtm,
          lineStyle: { color: '#1677ff', width: 2 },
          areaStyle: {
            color: {
              type: 'linear',
              x: 0,
              y: 0,
              x2: 0,
              y2: 1,
              colorStops: [
                { offset: 0, color: 'rgba(22,119,255,0.35)' },
                { offset: 1, color: 'rgba(22,119,255,0.02)' },
              ],
            },
          },
        },
        {
          name: 'Cash',
          type: 'line',
          smooth: true,
          showSymbol: false,
          data: cash,
          lineStyle: { color: '#8c8c8c', width: 1.2, type: 'dashed' },
        },
      ],
    }
  }, [equity, title])

  // lazyUpdate + silent minimises re-render cost on streaming updates; for
  // now backtest equity is static but enabling these costs nothing.
  return (
    <ReactECharts
      option={option}
      notMerge={false}
      lazyUpdate
      style={{ height, width: '100%' }}
    />
  )
}
