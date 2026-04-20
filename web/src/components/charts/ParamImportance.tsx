// ParamImportance renders a bar chart of per-parameter importance
// scores (0..1, summing to ≤1 when variation exists). The chart doubles
// as a legend for the optimiser's sensitivity analysis.

import { useMemo } from 'react'
import ReactECharts from 'echarts-for-react'
import type { EChartsOption } from 'echarts'

export interface ParamImportanceProps {
  importance: Record<string, number>
  height?: number | string
}

export default function ParamImportance({ importance, height = 220 }: ParamImportanceProps) {
  const option = useMemo<EChartsOption>(() => {
    const entries = Object.entries(importance)
      .filter(([, v]) => Number.isFinite(v))
      .sort((a, b) => b[1] - a[1])
    const names = entries.map(([k]) => k)
    const vals = entries.map(([, v]) => Number((v * 100).toFixed(2)))
    return {
      grid: { left: 100, right: 24, top: 16, bottom: 24 },
      tooltip: { valueFormatter: (v) => `${v}%` },
      xAxis: {
        type: 'value',
        max: Math.max(100, Math.ceil(Math.max(...vals, 0))),
        axisLabel: { formatter: '{value}%' },
      },
      yAxis: {
        type: 'category',
        data: names,
        inverse: true,
      },
      series: [
        {
          type: 'bar',
          data: vals,
          barWidth: 18,
          itemStyle: { color: '#1677ff' },
          label: {
            show: true,
            position: 'right',
            formatter: (p: any) => `${p.value}%`,
            fontSize: 11,
          },
        },
      ],
    }
  }, [importance])

  return (
    <ReactECharts
      option={option}
      notMerge
      lazyUpdate
      style={{ height, width: '100%' }}
    />
  )
}
