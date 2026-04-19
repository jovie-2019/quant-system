// RegimeDashboard — the second page of the observation panel overhaul.
//
// Layout:
//   Top bar: compact form (venue / symbols / intervals / method / refresh)
//   Main:    Symbol × Timeframe heatmap driven by GET /api/v1/regime/matrix
//   Drawer:  when a cell is clicked, open a Drawer showing the time series
//            (ADX + Hurst + label strip) for that (symbol, interval).
//
// A "重新计算" button on the top bar invokes POST /api/v1/regime/compute
// over the last 24h for the focused symbol; once ClickHouse gains real
// data via kline-backfill, this is the operator's one-click refresh.

import { useMemo, useState } from 'react'
import {
  Alert,
  Button,
  Card,
  Drawer,
  Form,
  Select,
  Space,
  Tag,
  Typography,
  message,
} from 'antd'
import { ReloadOutlined, ThunderboltOutlined } from '@ant-design/icons'

import RegimeHeatmap, {
  LABEL_COLORS,
  LABEL_TEXT,
} from '../components/charts/RegimeHeatmap'
import RegimeTimeline from '../components/charts/RegimeTimeline'
import {
  useComputeRegime,
  useRegimeHistory,
  useRegimeMatrix,
  type RegimeLabel,
  type RegimeMatrixRow,
} from '../api/regime'

const { Title, Text, Paragraph } = Typography

interface FormValues {
  venue: string
  symbols: string[]
  intervals: string[]
}

const DEFAULT_SYMBOLS = ['BTCUSDT', 'ETHUSDT', 'SOLUSDT']
const DEFAULT_INTERVALS = ['1m', '5m', '15m', '1h', '4h', '1d']

export default function RegimeDashboard() {
  const [form] = Form.useForm<FormValues>()
  const [drill, setDrill] = useState<{ symbol: string; interval: string } | null>(null)

  // Pull current form values so the live query key tracks edits.
  const venue = Form.useWatch('venue', form) ?? 'binance'
  const symbols: string[] = Form.useWatch('symbols', form) ?? DEFAULT_SYMBOLS
  const intervals: string[] = Form.useWatch('intervals', form) ?? DEFAULT_INTERVALS

  const matrixQuery = useRegimeMatrix({
    venue,
    symbols,
    intervals,
    method: 'threshold',
    refetchMs: 30_000,
  })

  const historyQuery = useRegimeHistory({
    venue,
    symbol: drill?.symbol ?? '',
    interval: drill?.interval ?? '',
    method: 'threshold',
    limit: 500,
    enabled: !!drill,
  })

  const compute = useComputeRegime()

  const onCellClick = (symbol: string, interval: string, row?: RegimeMatrixRow) => {
    setDrill({ symbol, interval })
    if (row && row.regime === 'unknown') {
      // Non-blocking hint: the cell has a stored record with "unknown"
      // regime. Nothing else to do — show the drill-down as usual.
    }
  }

  const onRefreshCompute = async () => {
    if (!drill) {
      message.info('请先在矩阵里选一个单元格，再点击重新计算')
      return
    }
    const end = Date.now()
    const start = end - 24 * 3600 * 1000
    try {
      await compute.mutateAsync({
        venue,
        symbol: drill.symbol,
        interval: drill.interval,
        start_ms: start,
        end_ms: end,
      })
      message.success(`${drill.symbol} ${drill.interval} 已重新分类最近 24h`)
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err)
      message.error(`计算失败: ${msg}`)
    }
  }

  const legend = useMemo(() => {
    return (['trend_up', 'trend_down', 'range', 'high_vol', 'low_liq', 'unknown'] as RegimeLabel[]).map(
      (k) => (
        <Tag color={LABEL_COLORS[k]} key={k} style={{ color: '#fff', border: 'none' }}>
          {LABEL_TEXT[k]}
        </Tag>
      ),
    )
  }, [])

  const matrixError = matrixQuery.error as Error | null

  return (
    <Space direction="vertical" size="large" style={{ width: '100%' }}>
      <div>
        <Title level={4} style={{ marginBottom: 0 }}>
          市场状态矩阵 (Regime Dashboard)
        </Title>
        <Paragraph type="secondary" style={{ marginTop: 4 }}>
          ADX / Hurst / BBW / ATR 组合判断的阈值分类结果。点击单元格查看时间序列。
        </Paragraph>
      </div>

      <Card size="small">
        <Form<FormValues>
          form={form}
          layout="inline"
          initialValues={{
            venue: 'binance',
            symbols: DEFAULT_SYMBOLS,
            intervals: DEFAULT_INTERVALS,
          }}
        >
          <Form.Item name="venue" label="交易所">
            <Select
              style={{ width: 140 }}
              options={[
                { value: 'binance', label: 'binance' },
                { value: 'okx', label: 'okx' },
              ]}
            />
          </Form.Item>
          <Form.Item name="symbols" label="币对">
            <Select
              mode="tags"
              style={{ minWidth: 320 }}
              placeholder="输入 BTCUSDT 后回车"
              tokenSeparators={[',', ' ']}
            />
          </Form.Item>
          <Form.Item name="intervals" label="周期">
            <Select
              mode="multiple"
              style={{ minWidth: 320 }}
              options={['1m', '5m', '15m', '30m', '1h', '4h', '1d'].map((v) => ({
                value: v,
                label: v,
              }))}
            />
          </Form.Item>
          <Form.Item>
            <Button
              icon={<ReloadOutlined />}
              loading={matrixQuery.isFetching}
              onClick={() => matrixQuery.refetch()}
            >
              刷新
            </Button>
          </Form.Item>
        </Form>
        <div style={{ marginTop: 12 }}>
          <Text type="secondary" style={{ marginRight: 8 }}>
            图例：
          </Text>
          {legend}
        </div>
      </Card>

      {matrixError && (
        <Alert
          type="error"
          showIcon
          message="矩阵数据获取失败"
          description={matrixError.message}
        />
      )}

      <Card size="small" title="Symbol × Timeframe">
        <RegimeHeatmap
          symbols={symbols}
          intervals={intervals}
          rows={matrixQuery.data?.rows ?? []}
          onCellClick={onCellClick}
        />
      </Card>

      <Drawer
        open={!!drill}
        onClose={() => setDrill(null)}
        title={
          drill ? (
            <Space>
              <span>
                {drill.symbol} · {drill.interval}
              </span>
              <Tag color="blue">{(historyQuery.data?.count ?? 0)} bars</Tag>
            </Space>
          ) : (
            '时间序列'
          )
        }
        width={720}
        extra={
          <Button
            icon={<ThunderboltOutlined />}
            loading={compute.isPending}
            onClick={onRefreshCompute}
            type="primary"
          >
            重算最近 24h
          </Button>
        }
      >
        {historyQuery.isLoading ? (
          <Alert type="info" message="加载中..." />
        ) : historyQuery.error ? (
          <Alert
            type="error"
            showIcon
            message="历史数据获取失败"
            description={(historyQuery.error as Error).message}
          />
        ) : (historyQuery.data?.items?.length ?? 0) === 0 ? (
          <Alert
            type="warning"
            showIcon
            message="暂无分类数据"
            description="点击「重算最近 24h」触发分类，前提是 ClickHouse 已有历史 K 线。"
          />
        ) : (
          <RegimeTimeline records={historyQuery.data!.items} height={440} />
        )}
      </Drawer>
    </Space>
  )
}
