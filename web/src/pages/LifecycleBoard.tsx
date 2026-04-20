// LifecycleBoard — Kanban-style view of every registered strategy,
// grouped by its lifecycle stage. Each stage is a column; each
// strategy is a card with buttons for the two legal neighbour moves
// (promote to the next stage, demote to the previous) plus a
// dedicated "deprecate" kill switch.
//
// Clicking a card opens a Drawer that shows:
//   • the current health snapshot (backtest Sharpe, shadow/canary
//     duration, Sharpe drift)
//   • the full transition audit log
//   • a reason textarea + confirm button for the proposed action
//
// The server enforces both the "only adjacent stages" rule and the
// numeric guards (MinBacktestSharpe, shadow duration, etc.); this UI
// just surfaces the rejection reason when a guard fires so the
// operator can see why a promotion was blocked.

import { useMemo, useState } from 'react'
import {
  Alert,
  Button,
  Card,
  Col,
  Drawer,
  Empty,
  Input,
  Row,
  Space,
  Statistic,
  Table,
  Tag,
  Tooltip,
  Typography,
  message,
} from 'antd'
import {
  ArrowDownOutlined,
  ArrowUpOutlined,
  ReloadOutlined,
  StopOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'

import {
  useLifecycleBoard,
  useProposeLifecycle,
  useStrategyHealth,
  useStrategyLifecycle,
  type LifecycleBoardCard,
  type LifecycleStage,
  type LifecycleTransitionRow,
  type TransitionKind,
} from '../api/lifecycle'

const { Title, Text, Paragraph } = Typography
const { TextArea } = Input

// The canonical forward-promotion order. Must match the Go
// lifecycle.AllStages() slice; the UI uses it to figure out which
// neighbour buttons to show on each card.
const STAGE_ORDER: LifecycleStage[] = [
  'draft',
  'backtested',
  'paper',
  'canary',
  'live',
  'deprecated',
]

const STAGE_LABEL: Record<LifecycleStage, string> = {
  draft: '草稿 Draft',
  backtested: '已回测 Backtested',
  paper: '纸上 Paper',
  canary: '灰度 Canary',
  live: '实盘 Live',
  deprecated: '已退役 Deprecated',
}

const STAGE_COLOR: Record<LifecycleStage, string> = {
  draft: 'default',
  backtested: 'blue',
  paper: 'cyan',
  canary: 'gold',
  live: 'green',
  deprecated: 'red',
}

// nextStage returns the stage immediately after the given one in the
// forward flow, or null if there is none. previousStage does the
// inverse. Deprecated has no adjacent promotion / demotion targets.
function nextStage(stage: LifecycleStage): LifecycleStage | null {
  const idx = STAGE_ORDER.indexOf(stage)
  if (idx < 0 || idx >= STAGE_ORDER.length - 2) return null // nothing above live
  return STAGE_ORDER[idx + 1]
}
function previousStage(stage: LifecycleStage): LifecycleStage | null {
  const idx = STAGE_ORDER.indexOf(stage)
  if (idx <= 0 || stage === 'deprecated') return null
  return STAGE_ORDER[idx - 1]
}

export default function LifecycleBoard() {
  const boardQuery = useLifecycleBoard({ refetchMs: 5_000 })
  const [drawerCard, setDrawerCard] = useState<LifecycleBoardCard | null>(null)

  const boardStages = boardQuery.data?.stages ?? STAGE_ORDER
  const totalCount = boardQuery.data?.total_count ?? 0

  const cardsByStage: Partial<Record<LifecycleStage, LifecycleBoardCard[] | null>> =
    boardQuery.data?.by_stage ?? {}

  return (
    <Space direction="vertical" size="large" style={{ width: '100%' }}>
      <Row justify="space-between" align="bottom">
        <Col>
          <Title level={4} style={{ marginBottom: 0 }}>
            策略生命周期 (Lifecycle Board)
          </Title>
          <Paragraph type="secondary" style={{ marginTop: 4 }}>
            共 {totalCount} 个策略。点击卡片查看健康度与审计；晋升/降级按钮会触发 guard 校验，guard 失败时服务端会返回拒绝原因。
          </Paragraph>
        </Col>
        <Col>
          <Button
            icon={<ReloadOutlined />}
            loading={boardQuery.isFetching}
            onClick={() => boardQuery.refetch()}
          >
            刷新
          </Button>
        </Col>
      </Row>

      {boardQuery.error && (
        <Alert
          type="error"
          showIcon
          message="加载失败"
          description={(boardQuery.error as Error).message}
        />
      )}

      <Row gutter={12} wrap={false} style={{ overflowX: 'auto', paddingBottom: 8 }}>
        {boardStages.map((stage) => (
          <Col key={stage} flex="220px">
            <Card
              size="small"
              title={
                <Space>
                  <Tag color={STAGE_COLOR[stage]}>{STAGE_LABEL[stage]}</Tag>
                  <Text type="secondary" style={{ fontSize: 12 }}>
                    {(cardsByStage[stage] ?? []).length}
                  </Text>
                </Space>
              }
              style={{ background: '#fafafa', minHeight: 380 }}
              styles={{ body: { padding: 8 } }}
            >
              {(cardsByStage[stage] ?? []).length === 0 ? (
                <Empty
                  imageStyle={{ height: 36 }}
                  description={<span style={{ fontSize: 12, color: '#bfbfbf' }}>无</span>}
                />
              ) : (
                <Space direction="vertical" size={8} style={{ width: '100%' }}>
                  {(cardsByStage[stage] ?? []).map((card) => (
                    <StageCard
                      key={card.id}
                      card={card}
                      onOpen={() => setDrawerCard(card)}
                    />
                  ))}
                </Space>
              )}
            </Card>
          </Col>
        ))}
      </Row>

      <StrategyDrawer
        card={drawerCard}
        onClose={() => setDrawerCard(null)}
        onTransitionSettled={() => {
          boardQuery.refetch()
        }}
      />
    </Space>
  )
}

// -----------------------------------------------------------------------------
// Card
// -----------------------------------------------------------------------------

function StageCard({ card, onOpen }: { card: LifecycleBoardCard; onOpen: () => void }) {
  const last = card.last_transition_ms ?? card.updated_ms
  return (
    <Card
      size="small"
      hoverable
      onClick={onOpen}
      styles={{ body: { padding: 10 } }}
    >
      <Space direction="vertical" size={2} style={{ width: '100%' }}>
        <Text strong style={{ fontSize: 13 }}>
          {card.strategy_id}
        </Text>
        <Text type="secondary" style={{ fontSize: 11 }}>
          {card.strategy_type} · runner {card.status}
        </Text>
        <Text type="secondary" style={{ fontSize: 11 }}>
          {last ? new Date(last).toLocaleString() : '—'}
        </Text>
      </Space>
    </Card>
  )
}

// -----------------------------------------------------------------------------
// Drawer (health + audit + transition controls)
// -----------------------------------------------------------------------------

function StrategyDrawer({
  card,
  onClose,
  onTransitionSettled,
}: {
  card: LifecycleBoardCard | null
  onClose: () => void
  onTransitionSettled: () => void
}) {
  const open = card !== null
  const id = card?.id ?? null
  const lifecycleQuery = useStrategyLifecycle(id)
  const healthQuery = useStrategyHealth(id)
  const mutation = useProposeLifecycle(id)

  const [reason, setReason] = useState('')

  async function runTransition(to: LifecycleStage, kind: TransitionKind) {
    try {
      await mutation.mutateAsync({ to_stage: to, reason })
      message.success(`${kind} → ${to}`)
      setReason('')
      onTransitionSettled()
    } catch (err) {
      const detail =
        (err as { response?: { data?: { message?: string } }; message?: string })?.response?.data?.message ??
        (err instanceof Error ? err.message : String(err))
      message.error(`拒绝: ${detail}`)
    }
  }

  const currentStage: LifecycleStage = card?.stage ?? 'draft'
  const promoteTarget = nextStage(currentStage)
  const demoteTarget = previousStage(currentStage)
  const canDeprecate = currentStage !== 'deprecated'

  const columns: ColumnsType<LifecycleTransitionRow> = useMemo(
    () => [
      {
        title: '变更',
        render: (_: unknown, row: LifecycleTransitionRow) => (
          <Space>
            <Tag color={STAGE_COLOR[row.from_stage]}>{row.from_stage}</Tag>
            <span>→</span>
            <Tag color={STAGE_COLOR[row.to_stage]}>{row.to_stage}</Tag>
          </Space>
        ),
      },
      {
        title: '类型',
        dataIndex: 'kind',
        width: 100,
        render: (k: TransitionKind) => <Tag>{k}</Tag>,
      },
      { title: '发起人', dataIndex: 'actor', width: 110, ellipsis: true },
      {
        title: '时间',
        dataIndex: 'transitioned_ms',
        width: 170,
        render: (v: number) => new Date(v).toLocaleString(),
      },
      {
        title: '原因',
        render: (_: unknown, row: LifecycleTransitionRow) =>
          row.reason ? <Text>{row.reason}</Text> : <Text type="secondary">—</Text>,
      },
    ],
    [],
  )

  const health = healthQuery.data

  return (
    <Drawer
      open={open}
      onClose={onClose}
      title={
        card ? (
          <Space>
            <span>{card.strategy_id}</span>
            <Tag color={STAGE_COLOR[card.stage]}>{STAGE_LABEL[card.stage]}</Tag>
          </Space>
        ) : (
          ''
        )
      }
      width={760}
      destroyOnClose
    >
      {card && (
        <Space direction="vertical" size="middle" style={{ width: '100%' }}>
          {/* Health snapshot */}
          <Card size="small" title="健康度">
            {healthQuery.isLoading ? (
              <Text type="secondary">加载中...</Text>
            ) : health ? (
              <>
                <Row gutter={12}>
                  <Col span={6}>
                    <Statistic
                      title="Backtest Sharpe"
                      value={health.best_backtest_sharpe}
                      precision={2}
                    />
                  </Col>
                  <Col span={6}>
                    <Statistic
                      title="Canary Sharpe"
                      value={health.canary_live_sharpe}
                      precision={2}
                    />
                  </Col>
                  <Col span={6}>
                    <Statistic
                      title="Sharpe Drift"
                      value={health.sharpe_drift}
                      precision={2}
                      valueStyle={{
                        color: health.sharpe_drift > 0.6 ? '#cf1322' : undefined,
                      }}
                    />
                  </Col>
                  <Col span={6}>
                    <Statistic
                      title="Shadow PnL"
                      value={health.shadow_virtual_pnl}
                      precision={2}
                    />
                  </Col>
                </Row>
                {health.message && (
                  <Alert
                    showIcon
                    type="info"
                    style={{ marginTop: 8 }}
                    message={health.message}
                  />
                )}
              </>
            ) : (
              <Text type="secondary">无数据</Text>
            )}
          </Card>

          {/* Transition controls */}
          <Card size="small" title="阶段操作">
            <Space direction="vertical" size="small" style={{ width: '100%' }}>
              <TextArea
                rows={2}
                placeholder="变更原因（可选，记入审计）"
                value={reason}
                onChange={(e) => setReason(e.target.value)}
                maxLength={300}
              />
              <Space wrap>
                <Tooltip
                  title={
                    promoteTarget ? `晋升到 ${STAGE_LABEL[promoteTarget]}` : '当前阶段已是顶端'
                  }
                >
                  <Button
                    type="primary"
                    icon={<ArrowUpOutlined />}
                    disabled={!promoteTarget}
                    loading={mutation.isPending}
                    onClick={() => promoteTarget && runTransition(promoteTarget, 'promote')}
                  >
                    {promoteTarget ? `晋升 → ${STAGE_LABEL[promoteTarget]}` : '无可晋升'}
                  </Button>
                </Tooltip>

                <Tooltip
                  title={
                    demoteTarget ? `降级到 ${STAGE_LABEL[demoteTarget]}` : '当前已是最低阶段'
                  }
                >
                  <Button
                    icon={<ArrowDownOutlined />}
                    disabled={!demoteTarget}
                    loading={mutation.isPending}
                    onClick={() => demoteTarget && runTransition(demoteTarget, 'demote')}
                  >
                    {demoteTarget ? `降级 → ${STAGE_LABEL[demoteTarget]}` : '无可降级'}
                  </Button>
                </Tooltip>

                <Tooltip title="退役策略（不可逆）">
                  <Button
                    danger
                    icon={<StopOutlined />}
                    disabled={!canDeprecate}
                    loading={mutation.isPending}
                    onClick={() => runTransition('deprecated', 'deprecate')}
                  >
                    退役
                  </Button>
                </Tooltip>

                <Tag icon={<ThunderboltOutlined />} color="orange">
                  guard 由服务端校验，拒绝原因会出现在消息里
                </Tag>
              </Space>
            </Space>
          </Card>

          {/* Audit log */}
          <Card size="small" title="迁移审计">
            <Table<LifecycleTransitionRow>
              columns={columns}
              dataSource={lifecycleQuery.data?.transitions ?? []}
              rowKey="id"
              size="small"
              pagination={{ pageSize: 8 }}
              loading={lifecycleQuery.isLoading}
            />
          </Card>
        </Space>
      )}
    </Drawer>
  )
}
