// BacktestWorkbench — the first page in the observation panel overhaul that
// gives the user direct, hands-on access to the backtest engine:
//
//   • Left pane: a form that maps 1:1 to BacktestRequest, with the
//     momentum strategy params as JSON so any registered strategy type
//     can be exercised without code changes.
//   • Right pane: live metric cards + the equity curve of the most
//     recent run, updated the moment the POST returns.
//   • Bottom strip: the last N runs pulled from GET /api/v1/backtests,
//     clicking a row rehydrates the right pane with that run.
//
// The page is intentionally dense — users coming here want to iterate
// quickly on parameter tweaks, not hunt for buttons.

import { useEffect, useMemo, useState } from 'react'
import {
  Alert,
  Button,
  Card,
  Col,
  Divider,
  Form,
  Input,
  InputNumber,
  Row,
  Select,
  Space,
  Table,
  Tag,
  Typography,
  message,
} from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { PlayCircleOutlined, ReloadOutlined } from '@ant-design/icons'

import EquityCurveChart from '../components/charts/EquityCurveChart'
import MetricsCards from '../components/charts/MetricsCards'
import {
  useBacktests,
  useCreateBacktest,
  type BacktestRecord,
  type BacktestRequest,
} from '../api/backtests'

const { Title, Text, Paragraph } = Typography
const { TextArea } = Input

// Default momentum params; the form exposes the JSON so users can tune
// any registered strategy type simply by editing this field.
const DEFAULT_MOMENTUM_PARAMS = `{
  "symbol": "BTCUSDT",
  "window_size": 20,
  "breakout_threshold": 0.0005,
  "order_qty": 0.1,
  "time_in_force": "IOC",
  "cooldown_ms": 0
}`

interface FormValues {
  strategy_type: string
  strategy_params_json: string
  dataset_source: 'synthetic' | 'clickhouse'
  symbol: string
  num_events: number
  seed: number
  start_price: number
  volatility_bps: number
  trend_bps_per_step: number
  start_equity: number
  slippage_bps: number
  fee_bps: number
  max_order_qty: number
  max_order_amount: number
}

export default function BacktestWorkbench() {
  const [form] = Form.useForm<FormValues>()
  const [active, setActive] = useState<BacktestRecord | null>(null)
  const [paramsError, setParamsError] = useState<string | null>(null)

  const { data: list, refetch, isFetching } = useBacktests(20)
  const mutation = useCreateBacktest()

  // Hydrate the "active" panel with the newest completed run on first load
  // so the page is never empty.
  useEffect(() => {
    if (!active && list?.items?.length) {
      const first = list.items.find((r) => r.status === 'done') ?? list.items[0]
      setActive(first ?? null)
    }
  }, [list, active])

  // If the Parameter Optimization page handed us a prefill payload in
  // sessionStorage (via its "Promote to Backtest" CTA), merge it into the
  // form and clear the stash so subsequent visits start fresh.
  useEffect(() => {
    const raw = sessionStorage.getItem('quant_backtest_prefill')
    if (!raw) return
    try {
      const payload = JSON.parse(raw) as {
        strategy_type?: string
        merged_params?: Record<string, unknown>
      }
      if (payload.strategy_type) {
        form.setFieldValue('strategy_type', payload.strategy_type)
      }
      if (payload.merged_params) {
        form.setFieldValue(
          'strategy_params_json',
          JSON.stringify(payload.merged_params, null, 2),
        )
      }
      message.info('已从参数优化页载入候选参数')
    } catch {
      // Ignore — bad prefill should not break the page.
    } finally {
      sessionStorage.removeItem('quant_backtest_prefill')
    }
  }, [form])

  async function onSubmit() {
    let values: FormValues
    try {
      values = await form.validateFields()
    } catch {
      return
    }
    let params: Record<string, unknown>
    try {
      params = JSON.parse(values.strategy_params_json)
      setParamsError(null)
    } catch (err) {
      setParamsError(err instanceof Error ? err.message : String(err))
      return
    }

    const req: BacktestRequest = {
      strategy_type: values.strategy_type,
      strategy_params: params,
      dataset: {
        source: values.dataset_source,
        symbol: values.symbol,
        num_events: values.num_events,
        seed: values.seed,
        start_price: values.start_price,
        volatility_bps: values.volatility_bps,
        trend_bps_per_step: values.trend_bps_per_step,
      },
      start_equity: values.start_equity,
      slippage_bps: values.slippage_bps,
      fee_bps: values.fee_bps,
      risk: {
        max_order_qty: values.max_order_qty || undefined,
        max_order_amount: values.max_order_amount || undefined,
      },
    }

    try {
      const rec = await mutation.mutateAsync(req)
      setActive(rec)
      if (rec.status === 'failed') {
        message.error(`回测失败: ${rec.error ?? '未知错误'}`)
      } else {
        message.success('回测完成')
      }
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err)
      message.error(`提交失败: ${msg}`)
    }
  }

  const columns: ColumnsType<BacktestRecord> = useMemo(
    () => [
      {
        title: 'ID',
        dataIndex: 'id',
        width: 220,
        ellipsis: true,
        render: (id: string) => <Text code>{id}</Text>,
      },
      {
        title: '状态',
        dataIndex: 'status',
        width: 90,
        render: (s: BacktestRecord['status']) => {
          const color =
            s === 'done'
              ? 'success'
              : s === 'failed'
              ? 'error'
              : s === 'running'
              ? 'processing'
              : 'default'
          return <Tag color={color}>{s}</Tag>
        },
      },
      {
        title: '策略',
        dataIndex: ['request', 'strategy_type'],
        width: 120,
      },
      {
        title: '标的',
        dataIndex: ['request', 'dataset', 'symbol'],
        width: 120,
      },
      {
        title: 'Events',
        dataIndex: ['result', 'Events'],
        width: 90,
        render: (v?: number) => v ?? '—',
      },
      {
        title: 'Fills',
        dataIndex: ['result', 'Fills'],
        width: 70,
        render: (v?: number) => v ?? '—',
      },
      {
        title: 'Sharpe',
        dataIndex: ['result', 'Metrics', 'Sharpe'],
        width: 90,
        render: (v?: number) =>
          v === undefined ? '—' : Number.isFinite(v) ? v.toFixed(2) : '∞',
      },
      {
        title: 'Return',
        dataIndex: ['result', 'Metrics', 'TotalReturn'],
        width: 90,
        render: (v?: number) => {
          if (v === undefined) return '—'
          const pct = (v * 100).toFixed(2)
          return (
            <span style={{ color: v >= 0 ? '#3f8600' : '#cf1322' }}>
              {v >= 0 ? '+' : ''}
              {pct}%
            </span>
          )
        },
      },
      {
        title: 'MDD',
        dataIndex: ['result', 'Metrics', 'MaxDrawdown'],
        width: 80,
        render: (v?: number) => (v === undefined ? '—' : `${(v * 100).toFixed(2)}%`),
      },
      {
        title: '创建时间',
        dataIndex: 'created_at',
        width: 170,
        render: (s: string) => new Date(s).toLocaleString(),
      },
    ],
    [],
  )

  return (
    <Space direction="vertical" size="large" style={{ width: '100%' }}>
      <Title level={4} style={{ marginBottom: 0 }}>
        回测工作台
      </Title>
      <Paragraph type="secondary" style={{ marginTop: -12 }}>
        选择策略、配置参数与行情数据，即可发起一次回测。结果会立即落到右侧的指标卡与资金曲线。
      </Paragraph>

      <Row gutter={16} wrap={false} style={{ minHeight: 520 }}>
        {/* Left: form --------------------------------------------------- */}
        <Col flex="420px">
          <Card size="small" title="配置" style={{ height: '100%' }}>
            <Form<FormValues>
              form={form}
              layout="vertical"
              initialValues={{
                strategy_type: 'momentum',
                strategy_params_json: DEFAULT_MOMENTUM_PARAMS,
                dataset_source: 'synthetic',
                symbol: 'BTCUSDT',
                num_events: 500,
                seed: 42,
                start_price: 50000,
                volatility_bps: 20,
                trend_bps_per_step: 5,
                start_equity: 10000,
                slippage_bps: 2,
                fee_bps: 10,
                max_order_qty: 10,
                max_order_amount: 1_000_000,
              }}
              size="small"
            >
              <Form.Item name="strategy_type" label="策略类型" rules={[{ required: true }]}>
                <Select
                  options={[
                    { value: 'momentum', label: 'momentum' },
                    { value: 'template', label: 'template' },
                  ]}
                />
              </Form.Item>

              <Form.Item
                name="strategy_params_json"
                label="策略参数 (JSON)"
                validateStatus={paramsError ? 'error' : undefined}
                help={paramsError ?? undefined}
                rules={[{ required: true }]}
              >
                <TextArea rows={7} style={{ fontFamily: 'Menlo, Consolas, monospace', fontSize: 12 }} />
              </Form.Item>

              <Divider style={{ margin: '8px 0' }}>数据集</Divider>

              <Row gutter={8}>
                <Col span={12}>
                  <Form.Item name="dataset_source" label="来源">
                    <Select
                      options={[
                        { value: 'synthetic', label: 'synthetic (合成)' },
                        { value: 'clickhouse', label: 'clickhouse (历史)' },
                      ]}
                    />
                  </Form.Item>
                </Col>
                <Col span={12}>
                  <Form.Item name="symbol" label="标的" rules={[{ required: true }]}>
                    <Input />
                  </Form.Item>
                </Col>
              </Row>

              <Row gutter={8}>
                <Col span={12}>
                  <Form.Item name="num_events" label="事件数" rules={[{ required: true }]}>
                    <InputNumber min={1} max={100_000} style={{ width: '100%' }} />
                  </Form.Item>
                </Col>
                <Col span={12}>
                  <Form.Item name="seed" label="随机种子">
                    <InputNumber style={{ width: '100%' }} />
                  </Form.Item>
                </Col>
              </Row>

              <Row gutter={8}>
                <Col span={12}>
                  <Form.Item name="start_price" label="起始价格">
                    <InputNumber min={0} style={{ width: '100%' }} />
                  </Form.Item>
                </Col>
                <Col span={12}>
                  <Form.Item name="volatility_bps" label="波动率 (bps)">
                    <InputNumber min={0} max={10_000} style={{ width: '100%' }} />
                  </Form.Item>
                </Col>
              </Row>

              <Form.Item name="trend_bps_per_step" label="趋势漂移 (bps/step)">
                <InputNumber style={{ width: '100%' }} />
              </Form.Item>

              <Divider style={{ margin: '8px 0' }}>账户 & 撮合</Divider>

              <Row gutter={8}>
                <Col span={12}>
                  <Form.Item name="start_equity" label="初始资金">
                    <InputNumber min={0} style={{ width: '100%' }} />
                  </Form.Item>
                </Col>
                <Col span={12}>
                  <Form.Item name="slippage_bps" label="滑点 (bps)">
                    <InputNumber min={0} max={1_000} style={{ width: '100%' }} />
                  </Form.Item>
                </Col>
              </Row>

              <Row gutter={8}>
                <Col span={12}>
                  <Form.Item name="fee_bps" label="手续费 (bps)">
                    <InputNumber min={0} max={1_000} style={{ width: '100%' }} />
                  </Form.Item>
                </Col>
                <Col span={12}>
                  <Form.Item name="max_order_qty" label="单笔限量">
                    <InputNumber min={0} style={{ width: '100%' }} />
                  </Form.Item>
                </Col>
              </Row>

              <Form.Item name="max_order_amount" label="单笔限额">
                <InputNumber min={0} style={{ width: '100%' }} />
              </Form.Item>

              <Button
                type="primary"
                icon={<PlayCircleOutlined />}
                block
                loading={mutation.isPending}
                onClick={onSubmit}
              >
                运行回测
              </Button>
            </Form>
          </Card>
        </Col>

        {/* Right: metrics + equity curve ------------------------------- */}
        <Col flex="auto">
          <Space direction="vertical" size="middle" style={{ width: '100%' }}>
            {active?.status === 'failed' && (
              <Alert
                showIcon
                type="error"
                message="回测失败"
                description={active.error ?? '未知错误'}
              />
            )}

            {active?.result && (
              <MetricsCards
                metrics={active.result.Metrics}
                startEquity={active.request.start_equity}
              />
            )}

            <Card
              size="small"
              title={
                active ? (
                  <Space>
                    <span>资金曲线</span>
                    <Text code style={{ fontSize: 12 }}>
                      {active.id}
                    </Text>
                  </Space>
                ) : (
                  '资金曲线'
                )
              }
              extra={
                active?.result ? (
                  <Text type="secondary" style={{ fontSize: 12 }}>
                    {active.result.Events} events · {active.result.Fills} fills
                  </Text>
                ) : null
              }
            >
              {active?.result?.Equity?.length ? (
                <EquityCurveChart equity={active.result.Equity} height={360} />
              ) : (
                <Alert
                  type="info"
                  message="尚无回测结果"
                  description="左侧填写配置后点击「运行回测」，或从下方历史列表选择一条。"
                />
              )}
            </Card>
          </Space>
        </Col>
      </Row>

      {/* Bottom: recent runs --------------------------------------------- */}
      <Card
        size="small"
        title="最近回测"
        extra={
          <Button
            size="small"
            icon={<ReloadOutlined />}
            loading={isFetching}
            onClick={() => refetch()}
          >
            刷新
          </Button>
        }
      >
        <Table<BacktestRecord>
          columns={columns}
          dataSource={list?.items ?? []}
          rowKey="id"
          size="small"
          pagination={{ pageSize: 10 }}
          onRow={(rec) => ({
            onClick: () => setActive(rec),
            style: {
              cursor: 'pointer',
              background: active?.id === rec.id ? '#e6f4ff' : undefined,
            },
          })}
        />
      </Card>
    </Space>
  )
}
