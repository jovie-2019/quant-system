// MetricsCards renders the six headline metrics from a backtest result.
// The layout intentionally caps at a single row on wide screens so the
// information density matches what a trader glances at before drilling
// into the equity curve.
//
// Non-finite values (e.g. infinite profit factor when no losses) arrive
// from the server sanitized to 1e18; we render those as "∞" to preserve
// the semantic.

import { Card, Col, Row, Statistic, Tooltip } from 'antd'
import {
  FallOutlined,
  LineChartOutlined,
  PercentageOutlined,
  RiseOutlined,
  SwapOutlined,
  TrophyOutlined,
} from '@ant-design/icons'

import type { Metrics } from '../../api/backtests'

const INF_THRESHOLD = 1e17 // server-side sanitise sentinel is 1e18

function formatNumber(x: number, digits = 2): string {
  if (!Number.isFinite(x)) return '—'
  if (Math.abs(x) >= INF_THRESHOLD) return '∞'
  return x.toFixed(digits)
}

function formatPct(x: number, digits = 2): string {
  if (!Number.isFinite(x)) return '—'
  if (Math.abs(x) >= INF_THRESHOLD) return '∞'
  return `${(x * 100).toFixed(digits)}%`
}

export interface MetricsCardsProps {
  metrics: Metrics
  startEquity?: number
}

export default function MetricsCards({ metrics, startEquity }: MetricsCardsProps) {
  const pnl =
    startEquity !== undefined ? metrics.FinalEquity - startEquity : undefined

  return (
    <Row gutter={[12, 12]}>
      <Col xs={12} sm={8} md={6} lg={4}>
        <Card size="small" styles={{ body: { padding: 12 } }}>
          <Statistic
            title={
              <Tooltip title="最终资产，含未平仓的盯市价值">
                <span>
                  <LineChartOutlined /> 终值
                </span>
              </Tooltip>
            }
            value={metrics.FinalEquity}
            precision={2}
            suffix={pnl !== undefined ? (
              <span style={{ fontSize: 12, color: pnl >= 0 ? '#3f8600' : '#cf1322' }}>
                {' '}
                ({pnl >= 0 ? '+' : ''}{pnl.toFixed(2)})
              </span>
            ) : null}
          />
        </Card>
      </Col>

      <Col xs={12} sm={8} md={6} lg={4}>
        <Card size="small" styles={{ body: { padding: 12 } }}>
          <Statistic
            title={<><RiseOutlined /> 总收益率</>}
            value={formatPct(metrics.TotalReturn)}
            valueStyle={{ color: metrics.TotalReturn >= 0 ? '#3f8600' : '#cf1322' }}
          />
        </Card>
      </Col>

      <Col xs={12} sm={8} md={6} lg={4}>
        <Card size="small" styles={{ body: { padding: 12 } }}>
          <Statistic
            title={<><FallOutlined /> 最大回撤</>}
            value={formatPct(metrics.MaxDrawdown)}
            valueStyle={{ color: '#cf1322' }}
          />
        </Card>
      </Col>

      <Col xs={12} sm={8} md={6} lg={4}>
        <Card size="small" styles={{ body: { padding: 12 } }}>
          <Statistic
            title={
              <Tooltip title="年化夏普，年化因子从事件间隔推断">
                <span>Sharpe</span>
              </Tooltip>
            }
            value={formatNumber(metrics.Sharpe)}
          />
        </Card>
      </Col>

      <Col xs={12} sm={8} md={6} lg={4}>
        <Card size="small" styles={{ body: { padding: 12 } }}>
          <Statistic
            title={<><TrophyOutlined /> 胜率</>}
            value={formatPct(metrics.WinRate)}
          />
        </Card>
      </Col>

      <Col xs={12} sm={8} md={6} lg={4}>
        <Card size="small" styles={{ body: { padding: 12 } }}>
          <Statistic
            title={
              <Tooltip title="盈亏比 = 盈利总和 / |亏损总和|">
                <span><PercentageOutlined /> PF</span>
              </Tooltip>
            }
            value={formatNumber(metrics.ProfitFactor)}
          />
        </Card>
      </Col>

      <Col xs={12} sm={8} md={6} lg={4}>
        <Card size="small" styles={{ body: { padding: 12 } }}>
          <Statistic
            title={<><SwapOutlined /> 成交笔数</>}
            value={metrics.NumTrades}
          />
        </Card>
      </Col>

      <Col xs={12} sm={8} md={6} lg={4}>
        <Card size="small" styles={{ body: { padding: 12 } }}>
          <Statistic
            title={
              <Tooltip title="换手率 = 累计名义成交额 / 平均资产">
                <span>Turnover</span>
              </Tooltip>
            }
            value={formatNumber(metrics.Turnover)}
            suffix="x"
          />
        </Card>
      </Col>
    </Row>
  )
}
