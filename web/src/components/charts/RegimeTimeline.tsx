// RegimeTimeline is the drill-down chart shown when the operator clicks
// a cell in the matrix. It layers three pieces of information:
//
//   1. ADX + Hurst lines so the operator can see *why* the classifier
//      chose the label it chose.
//   2. A scatter/strip of regime labels drawn as coloured dots along
//      the x axis — dense enough to reveal regime-change clusters.
//   3. A tooltip showing the raw features on hover.

import { useMemo } from 'react'
import ReactECharts from 'echarts-for-react'
import type { EChartsOption } from 'echarts'

import type { RegimeLabel, RegimeRecord } from '../../api/regime'
import { LABEL_COLORS, LABEL_TEXT } from './RegimeHeatmap'

export interface RegimeTimelineProps {
  records: RegimeRecord[]
  height?: number | string
}

export default function RegimeTimeline({ records, height = 320 }: RegimeTimelineProps) {
  const option = useMemo<EChartsOption>(() => {
    const times = records.map((r) => new Date(r.BarTime).toISOString())
    const adx = records.map((r) => r.Features.ADX)
    const hurst = records.map((r) => r.Features.Hurst)

    // Scatter points for labels, one per record, y-coordinate fixed at
    // 0 but colour encodes the label so clusters are visible at a glance.
    const scatter = records.map((r) => ({
      value: [new Date(r.BarTime).toISOString(), 0],
      itemStyle: { color: LABEL_COLORS[r.Regime as RegimeLabel] ?? '#8c8c8c' },
      label: LABEL_TEXT[r.Regime as RegimeLabel],
    }))

    return {
      grid: [
        { left: 56, right: 24, top: 40, height: '60%' },
        { left: 56, right: 24, top: '75%', height: '10%' },
      ],
      legend: {
        data: ['ADX', 'Hurst', 'Regime'],
        top: 8,
        right: 0,
      },
      tooltip: {
        trigger: 'axis',
        axisPointer: { type: 'cross' },
      },
      xAxis: [
        {
          gridIndex: 0,
          type: 'category',
          data: times,
          axisLabel: { show: false },
        },
        {
          gridIndex: 1,
          type: 'category',
          data: times,
          axisLabel: {
            formatter: (v: string) => v.slice(5, 16).replace('T', ' '),
            fontSize: 10,
          },
        },
      ],
      yAxis: [
        {
          gridIndex: 0,
          type: 'value',
          name: 'ADX / Hurst',
          min: 0,
          max: (value: { max: number }) => Math.max(value.max, 1.0),
        },
        {
          gridIndex: 1,
          type: 'value',
          show: false,
          min: -1,
          max: 1,
        },
      ],
      dataZoom: [
        { type: 'inside', xAxisIndex: [0, 1], throttle: 50 },
        { type: 'slider', xAxisIndex: [0, 1], height: 16, bottom: 0 },
      ],
      series: [
        {
          name: 'ADX',
          type: 'line',
          xAxisIndex: 0,
          yAxisIndex: 0,
          showSymbol: false,
          smooth: true,
          data: adx,
          lineStyle: { color: '#1677ff' },
        },
        {
          name: 'Hurst',
          type: 'line',
          xAxisIndex: 0,
          yAxisIndex: 0,
          showSymbol: false,
          smooth: true,
          data: hurst,
          lineStyle: { color: '#faad14' },
        },
        {
          name: 'Regime',
          type: 'scatter',
          xAxisIndex: 1,
          yAxisIndex: 1,
          symbolSize: 8,
          data: scatter as any,
        },
      ],
    }
  }, [records])

  return (
    <ReactECharts
      option={option}
      notMerge
      lazyUpdate
      style={{ height, width: '100%' }}
    />
  )
}
