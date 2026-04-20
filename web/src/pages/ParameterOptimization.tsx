// ParameterOptimization page — the third page in the observation panel
// overhaul. It lets the user:
//
//   • Specify a search space (one row per parameter) and a fixed base-
//     params JSON block that applies to every trial.
//   • Pick grid / random, max trials, seed, and an objective preset.
//   • Submit → the backend runs the optimisation (MVP: synchronous for
//     small jobs) and returns the full Result.
//   • Inspect: Top-N trials, objective distribution scatter, parameter
//     importance bars, best params JSON with a one-click "Promote to
//     Backtest" CTA that hands off to the BacktestWorkbench page.
//
// Keyboard: the trials table is sorted by objective desc by default;
// clicking a row marks it as the "active best preview" in the right pane.

import { useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
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
import {
  MinusCircleOutlined,
  PlayCircleOutlined,
  PlusOutlined,
  ReloadOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons'

import ParamImportance from '../components/charts/ParamImportance'
import ObjectiveDistribution from '../components/charts/ObjectiveDistribution'
import {
  useCreateOptimization,
  useOptimizations,
  type OptParamPayload,
  type OptimizationRecord,
  type Trial,
} from '../api/optimizations'

const { Title, Text, Paragraph } = Typography
const { TextArea } = Input

interface FormValues {
  strategy_type: string
  base_params_json: string
  params: OptParamPayload[]
  algorithm: 'grid' | 'random'
  max_trials: number
  seed: number
  objective: 'sharpe_penalty_dd' | 'total_return' | 'calmar' | 'profit_factor'
  symbol: string
  num_events: number
  dataset_seed: number
  start_price: number
  volatility_bps: number
  trend_bps_per_step: number
  start_equity: number
  slippage_bps: number
  fee_bps: number
}

const DEFAULT_BASE_PARAMS = `{
  "symbol": "BTCUSDT",
  "order_qty": 0.1,
  "time_in_force": "IOC",
  "cooldown_ms": 0
}`

const DEFAULT_PARAMS: OptParamPayload[] = [
  { name: 'window_size', type: 'int', min: 5, max: 40, step: 5 },
  { name: 'breakout_threshold', type: 'float', min: 0.0002, max: 0.003, step: 0.0004 },
]

export default function ParameterOptimization() {
  const navigate = useNavigate()
  const [form] = Form.useForm<FormValues>()
  const [active, setActive] = useState<OptimizationRecord | null>(null)
  const [selectedTrialId, setSelectedTrialId] = useState<number | null>(null)
  const [baseParamsError, setBaseParamsError] = useState<string | null>(null)

  const { data: list, refetch, isFetching } = useOptimizations(20)
  const mutation = useCreateOptimization()

  // Seed the right pane with the most recent done run on first load.
  useEffect(() => {
    if (!active && list?.items?.length) {
      const first = list.items.find((r) => r.status === 'done') ?? list.items[0]
      setActive(first ?? null)
      if (first?.result?.best) setSelectedTrialId(first.result.best.id)
    }
  }, [list, active])

  async function onSubmit() {
    let values: FormValues
    try {
      values = await form.validateFields()
    } catch {
      return
    }
    let baseParams: Record<string, unknown>
    try {
      baseParams = JSON.parse(values.base_params_json)
      setBaseParamsError(null)
    } catch (err) {
      setBaseParamsError(err instanceof Error ? err.message : String(err))
      return
    }

    try {
      const rec = await mutation.mutateAsync({
        strategy_type: values.strategy_type,
        base_params: baseParams,
        params: values.params,
        dataset: {
          source: 'synthetic',
          symbol: values.symbol,
          num_events: values.num_events,
          seed: values.dataset_seed,
          start_price: values.start_price,
          volatility_bps: values.volatility_bps,
          trend_bps_per_step: values.trend_bps_per_step,
        },
        start_equity: values.start_equity,
        slippage_bps: values.slippage_bps,
        fee_bps: values.fee_bps,
        algorithm: values.algorithm,
        max_trials: values.max_trials,
        seed: values.seed,
        objective: values.objective,
      })
      setActive(rec)
      if (rec.result?.best) setSelectedTrialId(rec.result.best.id)
      if (rec.status === 'failed') {
        message.error(`优化失败: ${rec.error ?? '未知错误'}`)
      } else {
        message.success(`完成 ${rec.result?.trials.length ?? 0} 个 trial`)
      }
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err)
      message.error(`提交失败: ${msg}`)
    }
  }

  const trials = active?.result?.trials ?? []
  const selectedTrial: Trial | undefined = useMemo(
    () =>
      trials.find((t) => t.id === selectedTrialId) ?? active?.result?.best,
    [trials, selectedTrialId, active],
  )

  const columns: ColumnsType<Trial> = useMemo(
    () => [
      { title: '#', dataIndex: 'id', width: 60, sorter: (a, b) => a.id - b.id },
      {
        title: 'Objective',
        dataIndex: 'objective',
        width: 110,
        defaultSortOrder: 'descend',
        sorter: (a, b) => a.objective - b.objective,
        render: (v: number, row) => {
          const finite = Number.isFinite(v) && v > -1e8
          return (
            <span style={{ color: !finite ? '#999' : row.id === active?.result?.best.id ? '#cf1322' : undefined }}>
              {finite ? v.toFixed(4) : '—'}
            </span>
          )
        },
      },
      {
        title: 'Sharpe',
        dataIndex: ['metrics', 'Sharpe'],
        width: 90,
        sorter: (a, b) => (a.metrics.Sharpe ?? 0) - (b.metrics.Sharpe ?? 0),
        render: (v?: number) => (Number.isFinite(v) ? (v as number).toFixed(2) : '—'),
      },
      {
        title: 'Return',
        dataIndex: ['metrics', 'TotalReturn'],
        width: 90,
        sorter: (a, b) => (a.metrics.TotalReturn ?? 0) - (b.metrics.TotalReturn ?? 0),
        render: (v?: number) => {
          if (!Number.isFinite(v)) return '—'
          const pct = ((v as number) * 100).toFixed(2)
          return (
            <span style={{ color: (v as number) >= 0 ? '#3f8600' : '#cf1322' }}>
              {(v as number) >= 0 ? '+' : ''}
              {pct}%
            </span>
          )
        },
      },
      {
        title: 'MDD',
        dataIndex: ['metrics', 'MaxDrawdown'],
        width: 90,
        render: (v?: number) => (Number.isFinite(v) ? `${((v as number) * 100).toFixed(2)}%` : '—'),
      },
      {
        title: 'Trades',
        dataIndex: ['metrics', 'NumTrades'],
        width: 80,
      },
      {
        title: 'Params',
        dataIndex: 'params',
        render: (params: Record<string, unknown>) => (
          <Text code style={{ fontSize: 11 }}>
            {Object.entries(params)
              .map(([k, v]) => `${k}=${typeof v === 'number' ? Number(v).toPrecision(4) : JSON.stringify(v)}`)
              .join(' · ')}
          </Text>
        ),
      },
    ],
    [active],
  )

  const onPromoteToBacktest = () => {
    if (!selectedTrial || !active) return
    // Stash the best params so the BacktestWorkbench page can rehydrate them.
    // A future revision can wire a proper context store; for the MVP we use
    // sessionStorage which survives a same-tab navigation but not a reload.
    const payload = {
      strategy_type: active.request.strategy_type,
      merged_params: {
        ...active.request.base_params,
        ...selectedTrial.params,
      },
      source_optimization: active.id,
      source_trial: selectedTrial.id,
    }
    sessionStorage.setItem('quant_backtest_prefill', JSON.stringify(payload))
    message.success('已将最优参数暂存，跳转到回测工作台')
    navigate('/backtests')
  }

  return (
    <Space direction="vertical" size="large" style={{ width: '100%' }}>
      <div>
        <Title level={4} style={{ marginBottom: 0 }}>
          参数优化 (Parameter Optimization)
        </Title>
        <Paragraph type="secondary" style={{ marginTop: 4 }}>
          对策略参数做网格/随机搜索；每个 trial 跑一次回测并用选定目标函数打分。右上角的稳定性分数越接近 1，表示最优参数周围 ±20% 也能保持 80% 以上表现。
        </Paragraph>
      </div>

      <Row gutter={16} wrap={false}>
        {/* ---- Left: form ---- */}
        <Col flex="440px">
          <Card size="small" title="优化配置" style={{ minHeight: 480 }}>
            <Form<FormValues>
              form={form}
              layout="vertical"
              size="small"
              initialValues={{
                strategy_type: 'momentum',
                base_params_json: DEFAULT_BASE_PARAMS,
                params: DEFAULT_PARAMS,
                algorithm: 'grid',
                max_trials: 100,
                seed: 42,
                objective: 'sharpe_penalty_dd',
                symbol: 'BTCUSDT',
                num_events: 500,
                dataset_seed: 42,
                start_price: 50_000,
                volatility_bps: 20,
                trend_bps_per_step: 5,
                start_equity: 10_000,
                slippage_bps: 2,
                fee_bps: 10,
              }}
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
                name="base_params_json"
                label="基础参数 (JSON，每个 trial 都会合并)"
                validateStatus={baseParamsError ? 'error' : undefined}
                help={baseParamsError ?? undefined}
                rules={[{ required: true }]}
              >
                <TextArea rows={5} style={{ fontFamily: 'Menlo, Consolas, monospace', fontSize: 12 }} />
              </Form.Item>

              <Divider style={{ margin: '8px 0' }}>搜索空间</Divider>

              <Form.List name="params">
                {(fields, { add, remove }) => (
                  <Space direction="vertical" size="small" style={{ width: '100%' }}>
                    {fields.map((field) => (
                      <Space key={field.key} align="start" wrap style={{ width: '100%' }}>
                        <Form.Item {...field} name={[field.name, 'name']} rules={[{ required: true }]} noStyle>
                          <Input placeholder="name" style={{ width: 140 }} />
                        </Form.Item>
                        <Form.Item {...field} name={[field.name, 'type']} noStyle>
                          <Select
                            style={{ width: 100 }}
                            options={[
                              { value: 'int', label: 'int' },
                              { value: 'float', label: 'float' },
                              { value: 'categorical', label: 'cat' },
                            ]}
                          />
                        </Form.Item>
                        <Form.Item {...field} name={[field.name, 'min']} noStyle>
                          <InputNumber placeholder="min" style={{ width: 90 }} />
                        </Form.Item>
                        <Form.Item {...field} name={[field.name, 'max']} noStyle>
                          <InputNumber placeholder="max" style={{ width: 90 }} />
                        </Form.Item>
                        <Form.Item {...field} name={[field.name, 'step']} noStyle>
                          <InputNumber placeholder="step" style={{ width: 90 }} />
                        </Form.Item>
                        <Button
                          type="text"
                          danger
                          icon={<MinusCircleOutlined />}
                          onClick={() => remove(field.name)}
                        />
                      </Space>
                    ))}
                    <Button
                      type="dashed"
                      icon={<PlusOutlined />}
                      onClick={() => add({ name: '', type: 'float', min: 0, max: 1, step: 0.1 } as OptParamPayload)}
                      block
                    >
                      新增参数
                    </Button>
                  </Space>
                )}
              </Form.List>

              <Divider style={{ margin: '12px 0' }}>搜索策略</Divider>

              <Row gutter={8}>
                <Col span={12}>
                  <Form.Item name="algorithm" label="算法">
                    <Select
                      options={[
                        { value: 'grid', label: 'grid (穷举)' },
                        { value: 'random', label: 'random (随机)' },
                      ]}
                    />
                  </Form.Item>
                </Col>
                <Col span={12}>
                  <Form.Item name="max_trials" label="Max trials">
                    <InputNumber min={1} max={5000} style={{ width: '100%' }} />
                  </Form.Item>
                </Col>
              </Row>

              <Row gutter={8}>
                <Col span={12}>
                  <Form.Item name="seed" label="种子">
                    <InputNumber style={{ width: '100%' }} />
                  </Form.Item>
                </Col>
                <Col span={12}>
                  <Form.Item name="objective" label="目标函数">
                    <Select
                      options={[
                        { value: 'sharpe_penalty_dd', label: 'Sharpe − DD 惩罚' },
                        { value: 'total_return', label: 'Total Return' },
                        { value: 'calmar', label: 'Calmar' },
                        { value: 'profit_factor', label: 'Profit Factor' },
                      ]}
                    />
                  </Form.Item>
                </Col>
              </Row>

              <Divider style={{ margin: '8px 0' }}>合成数据集</Divider>

              <Row gutter={8}>
                <Col span={12}>
                  <Form.Item name="symbol" label="标的" rules={[{ required: true }]}>
                    <Input />
                  </Form.Item>
                </Col>
                <Col span={12}>
                  <Form.Item name="num_events" label="事件数">
                    <InputNumber min={50} max={100_000} style={{ width: '100%' }} />
                  </Form.Item>
                </Col>
              </Row>
              <Row gutter={8}>
                <Col span={12}>
                  <Form.Item name="dataset_seed" label="数据种子">
                    <InputNumber style={{ width: '100%' }} />
                  </Form.Item>
                </Col>
                <Col span={12}>
                  <Form.Item name="start_price" label="起价">
                    <InputNumber min={0} style={{ width: '100%' }} />
                  </Form.Item>
                </Col>
              </Row>
              <Row gutter={8}>
                <Col span={12}>
                  <Form.Item name="volatility_bps" label="波动 bps">
                    <InputNumber min={0} max={1_000} style={{ width: '100%' }} />
                  </Form.Item>
                </Col>
                <Col span={12}>
                  <Form.Item name="trend_bps_per_step" label="趋势 bps/step">
                    <InputNumber style={{ width: '100%' }} />
                  </Form.Item>
                </Col>
              </Row>

              <Divider style={{ margin: '8px 0' }}>撮合</Divider>

              <Row gutter={8}>
                <Col span={8}>
                  <Form.Item name="start_equity" label="初始资金">
                    <InputNumber min={0} style={{ width: '100%' }} />
                  </Form.Item>
                </Col>
                <Col span={8}>
                  <Form.Item name="slippage_bps" label="滑点 bps">
                    <InputNumber min={0} max={1_000} style={{ width: '100%' }} />
                  </Form.Item>
                </Col>
                <Col span={8}>
                  <Form.Item name="fee_bps" label="费率 bps">
                    <InputNumber min={0} max={1_000} style={{ width: '100%' }} />
                  </Form.Item>
                </Col>
              </Row>

              <Button
                type="primary"
                icon={<PlayCircleOutlined />}
                loading={mutation.isPending}
                onClick={onSubmit}
                block
              >
                开始优化
              </Button>
            </Form>
          </Card>
        </Col>

        {/* ---- Right: results ---- */}
        <Col flex="auto">
          <Space direction="vertical" size="middle" style={{ width: '100%' }}>
            {active?.status === 'failed' && (
              <Alert
                showIcon
                type="error"
                message="优化失败"
                description={active.error ?? '未知错误'}
              />
            )}

            {active?.result && (
              <Card size="small">
                <Row gutter={12}>
                  <Col span={6}>
                    <Text type="secondary">算法</Text>
                    <div style={{ fontSize: 18 }}>{active.result.algorithm}</div>
                  </Col>
                  <Col span={6}>
                    <Text type="secondary">Trials</Text>
                    <div style={{ fontSize: 18 }}>{active.result.trials.length}</div>
                  </Col>
                  <Col span={6}>
                    <Text type="secondary">最优目标</Text>
                    <div style={{ fontSize: 18, color: '#cf1322' }}>
                      {Number.isFinite(active.result.best.objective) && active.result.best.objective > -1e8
                        ? active.result.best.objective.toFixed(4)
                        : '—'}
                    </div>
                  </Col>
                  <Col span={6}>
                    <Text type="secondary">稳定性 (±20%)</Text>
                    <div
                      style={{
                        fontSize: 18,
                        color:
                          active.result.stability >= 0.8
                            ? '#3f8600'
                            : active.result.stability >= 0.5
                            ? '#faad14'
                            : '#cf1322',
                      }}
                    >
                      {(active.result.stability * 100).toFixed(0)}%
                    </div>
                  </Col>
                </Row>
              </Card>
            )}

            {active?.result && (
              <Row gutter={12}>
                <Col span={12}>
                  <Card size="small" title="参数重要度">
                    <ParamImportance importance={active.result.importance} height={220} />
                  </Card>
                </Col>
                <Col span={12}>
                  <Card size="small" title="Trial 目标分布">
                    <ObjectiveDistribution
                      trials={active.result.trials}
                      bestId={active.result.best.id}
                      height={220}
                    />
                  </Card>
                </Col>
              </Row>
            )}

            <Card
              size="small"
              title={active ? `Trials — ${active.id}` : 'Trials'}
              extra={
                selectedTrial && active ? (
                  <Space>
                    <Tag color="blue">
                      选中 #{selectedTrial.id} · obj {selectedTrial.objective.toFixed(3)}
                    </Tag>
                    <Button
                      size="small"
                      type="primary"
                      icon={<ThunderboltOutlined />}
                      onClick={onPromoteToBacktest}
                    >
                      以此参数去回测
                    </Button>
                  </Space>
                ) : null
              }
            >
              {trials.length === 0 ? (
                <Alert type="info" message="尚未运行任何优化" />
              ) : (
                <Table<Trial>
                  columns={columns}
                  dataSource={trials}
                  rowKey="id"
                  size="small"
                  pagination={{ pageSize: 10 }}
                  onRow={(row) => ({
                    onClick: () => setSelectedTrialId(row.id),
                    style: {
                      cursor: 'pointer',
                      background:
                        row.id === selectedTrialId
                          ? '#e6f4ff'
                          : row.id === active?.result?.best.id
                          ? '#fff1f0'
                          : undefined,
                    },
                  })}
                />
              )}
            </Card>
          </Space>
        </Col>
      </Row>

      <Card
        size="small"
        title="最近优化"
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
        <Table<OptimizationRecord>
          columns={[
            { title: 'ID', dataIndex: 'id', width: 260, ellipsis: true, render: (v) => <Text code>{v}</Text> },
            {
              title: '状态',
              dataIndex: 'status',
              width: 90,
              render: (s) => {
                const color =
                  s === 'done' ? 'success' : s === 'failed' ? 'error' : s === 'running' ? 'processing' : 'default'
                return <Tag color={color}>{s}</Tag>
              },
            },
            { title: '策略', dataIndex: ['request', 'strategy_type'], width: 120 },
            { title: 'Trials', dataIndex: ['result', 'trials'], width: 80, render: (t?: Trial[]) => (t ? t.length : '—') },
            {
              title: 'Best obj',
              dataIndex: ['result', 'best', 'objective'],
              width: 110,
              render: (v?: number) =>
                Number.isFinite(v) && (v as number) > -1e8 ? (v as number).toFixed(3) : '—',
            },
            {
              title: '稳定性',
              dataIndex: ['result', 'stability'],
              width: 90,
              render: (v?: number) => (typeof v === 'number' ? `${(v * 100).toFixed(0)}%` : '—'),
            },
            {
              title: '创建时间',
              dataIndex: 'created_at',
              width: 170,
              render: (s: string) => new Date(s).toLocaleString(),
            },
          ]}
          dataSource={list?.items ?? []}
          rowKey="id"
          size="small"
          pagination={{ pageSize: 10 }}
          onRow={(rec) => ({
            onClick: () => {
              setActive(rec)
              if (rec.result?.best) setSelectedTrialId(rec.result.best.id)
            },
            style: { cursor: 'pointer', background: active?.id === rec.id ? '#e6f4ff' : undefined },
          })}
        />
      </Card>
    </Space>
  )
}
