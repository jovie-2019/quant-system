// ObjectiveDistribution renders a scatter of every trial's objective
// against its index, with the best trial highlighted. This is a
// low-cost proxy for the classic "optimisation convergence" plot and
// surfaces outliers / ties at a glance.

import { useMemo } from 'react'
import ReactECharts from 'echarts-for-react'
import type { EChartsOption } from 'echarts'

import type { Trial } from '../../api/optimizations'

export interface ObjectiveDistributionProps {
  trials: Trial[]
  bestId: number
  height?: number | string
}

export default function ObjectiveDistribution({
  trials,
  bestId,
  height = 240,
}: ObjectiveDistributionProps) {
  const option = useMemo<EChartsOption>(() => {
    const valid = trials.filter((t) => Number.isFinite(t.objective) && t.objective > -1e8)
    const data = valid.map((t) => ({
      value: [t.id, Number(t.objective.toFixed(4))],
      itemStyle: { color: t.id === bestId ? '#f5222d' : '#1677ff' },
      symbolSize: t.id === bestId ? 14 : 6,
    }))
    return {
      grid: { left: 56, right: 24, top: 16, bottom: 40 },
      tooltip: {
        formatter: (p: any) => `trial #${p.value[0]} — objective ${p.value[1]}`,
      },
      xAxis: {
        type: 'value',
        name: 'trial',
        axisLabel: { fontSize: 11 },
      },
      yAxis: {
        type: 'value',
        scale: true,
        name: 'objective',
        axisLabel: { fontSize: 11 },
        splitLine: { lineStyle: { type: 'dashed', opacity: 0.4 } },
      },
      series: [
        {
          type: 'scatter',
          data: data as any,
          emphasis: { itemStyle: { shadowBlur: 6, shadowColor: 'rgba(0,0,0,0.3)' } },
        },
      ],
    }
  }, [trials, bestId])

  return (
    <ReactECharts
      option={option}
      notMerge
      lazyUpdate
      style={{ height, width: '100%' }}
    />
  )
}
