// ParamCandidates — Phase 7's review surface. The ReoptimizeJob stages
// new params whenever it finds a better Sharpe than the currently-
// deployed set; this page lets operators:
//
//   • See pending candidates with a side-by-side diff of baseline vs
//     proposed params and the Sharpe improvement.
//   • Approve → which fires the existing hot-reload path, identical
//     to a manual POST /strategies/:id/params with actor=reviewer.
//   • Reject → with a required reason; row is retained for the audit.
//   • Trigger the reoptimise pipeline out of schedule (for dev / when
//     "I want to re-run the overnight batch now, not in 18 hours").
//
// Polls the list every 5s so a reoptimise run done in a separate
// browser tab surfaces here without a manual refresh.

import { useMemo, useState } from 'react'
import {
  Alert,
  Button,
  Card,
  Col,
  Input,
  Modal,
  Popconfirm,
  Row,
  Segmented,
  Space,
  Statistic,
  Table,
  Tag,
  Typography,
  message,
} from 'antd'
import {
  CheckOutlined,
  CloseOutlined,
  ReloadOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'

import {
  useApproveCandidate,
  useCandidates,
  useRejectCandidate,
  useRunReoptimize,
  type Candidate,
  type CandidateStatus,
} from '../api/candidates'

const { Title, Text, Paragraph } = Typography
const { TextArea } = Input

const STATUS_TABS: { value: CandidateStatus | 'all'; label: string }[] = [
  { value: 'pending', label: '待审 Pending' },
  { value: 'applied', label: '已应用 Applied' },
  { value: 'rejected', label: '已拒绝 Rejected' },
  { value: 'all', label: '全部' },
]

const STATUS_COLOR: Record<CandidateStatus, string> = {
  pending: 'processing',
  approved: 'blue',
  applied: 'success',
  rejected: 'error',
  expired: 'default',
}

export default function ParamCandidates() {
  const [status, setStatus] = useState<'all' | CandidateStatus>('pending')
  const [reviewCandidate, setReviewCandidate] = useState<Candidate | null>(null)
  const [rejectModal, setRejectModal] = useState<Candidate | null>(null)
  const [rejectReason, setRejectReason] = useState('')

  const listQuery = useCandidates({
    status: status === 'all' ? undefined : status,
    limit: 100,
    refetchMs: 5_000,
  })
  const approve = useApproveCandidate()
  const reject = useRejectCandidate()
  const runNow = useRunReoptimize()

  const pendingCount = useMemo(() => {
    if (status === 'pending') return listQuery.data?.count ?? 0
    // Other tabs don't carry the number; display nothing instead of
    // a potentially confusing "applied: 3" badge.
    return null
  }, [status, listQuery.data])

  async function onApprove(c: Candidate) {
    try {
      await approve.mutateAsync({ id: c.id })
      message.success(`#${c.id} 已应用到 ${c.strategy_id}`)
      setReviewCandidate(null)
    } catch (err) {
      const detail =
        (err as { response?: { data?: { message?: string } }; message?: string })?.response?.data?.message ??
        (err instanceof Error ? err.message : String(err))
      message.error(`应用失败: ${detail}`)
    }
  }

  async function onReject() {
    if (!rejectModal || !rejectReason.trim()) return
    try {
      await reject.mutateAsync({ id: rejectModal.id, reason: rejectReason.trim() })
      message.success(`#${rejectModal.id} 已拒绝`)
      setRejectModal(null)
      setRejectReason('')
    } catch (err) {
      const detail =
        (err as { response?: { data?: { message?: string } }; message?: string })?.response?.data?.message ??
        (err instanceof Error ? err.message : String(err))
      message.error(`拒绝失败: ${detail}`)
    }
  }

  async function onRunNow() {
    try {
      const res = await runNow.mutateAsync()
      message.success(`reoptimize 完成，耗时 ${res.elapsed_ms} ms`)
    } catch (err) {
      const detail =
        (err as { response?: { data?: { message?: string } }; message?: string })?.response?.data?.message ??
        (err instanceof Error ? err.message : String(err))
      message.error(`触发失败: ${detail}`)
    }
  }

  const columns: ColumnsType<Candidate> = useMemo(
    () => [
      { title: 'ID', dataIndex: 'id', width: 60 },
      { title: '策略', dataIndex: 'strategy_id', width: 170, ellipsis: true },
      {
        title: '来源',
        dataIndex: 'origin',
        width: 160,
        render: (v: string) => <Tag>{v}</Tag>,
      },
      {
        title: '状态',
        dataIndex: 'status',
        width: 100,
        render: (v: CandidateStatus) => <Tag color={STATUS_COLOR[v]}>{v}</Tag>,
      },
      {
        title: 'Baseline Sharpe',
        dataIndex: 'baseline_sharpe',
        width: 130,
        render: (v: number) => (Number.isFinite(v) ? v.toFixed(2) : '—'),
      },
      {
        title: 'Proposed Sharpe',
        dataIndex: 'proposed_sharpe',
        width: 140,
        render: (v: number) => (Number.isFinite(v) ? v.toFixed(2) : '—'),
      },
      {
        title: '改进',
        dataIndex: 'improvement',
        width: 110,
        sorter: (a, b) => a.improvement - b.improvement,
        render: (v: number) => (
          <span style={{ color: v > 0 ? '#3f8600' : '#cf1322' }}>
            {v > 0 ? '+' : ''}
            {v.toFixed(3)}
          </span>
        ),
      },
      {
        title: '创建时间',
        dataIndex: 'created_ms',
        width: 170,
        render: (v: number) => new Date(v).toLocaleString(),
      },
      {
        title: '操作',
        width: 220,
        render: (_: unknown, c: Candidate) => (
          <Space>
            <Button size="small" onClick={() => setReviewCandidate(c)}>
              查看
            </Button>
            {c.status === 'pending' && (
              <>
                <Popconfirm
                  title="应用此候选参数？"
                  description="将通过热更新立即下发到 runner"
                  onConfirm={() => onApprove(c)}
                  okText="确认应用"
                  cancelText="取消"
                >
                  <Button size="small" type="primary" icon={<CheckOutlined />}>
                    应用
                  </Button>
                </Popconfirm>
                <Button
                  size="small"
                  danger
                  icon={<CloseOutlined />}
                  onClick={() => {
                    setRejectModal(c)
                    setRejectReason('')
                  }}
                >
                  拒绝
                </Button>
              </>
            )}
          </Space>
        ),
      },
    ],
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [approve.isPending, reject.isPending],
  )

  return (
    <Space direction="vertical" size="large" style={{ width: '100%' }}>
      <Row justify="space-between" align="bottom">
        <Col>
          <Title level={4} style={{ marginBottom: 0 }}>
            候选参数审批 (Param Candidates)
          </Title>
          <Paragraph type="secondary" style={{ marginTop: 4 }}>
            夜间 ReoptimizeJob 和手动触发的参数优化会把明显更好的参数组合入列在这里；审批通过即走热更新链路。
          </Paragraph>
        </Col>
        <Col>
          <Space>
            <Button
              icon={<ThunderboltOutlined />}
              loading={runNow.isPending}
              onClick={onRunNow}
            >
              立即触发 reoptimize
            </Button>
            <Button
              icon={<ReloadOutlined />}
              loading={listQuery.isFetching}
              onClick={() => listQuery.refetch()}
            >
              刷新
            </Button>
          </Space>
        </Col>
      </Row>

      <Segmented<'all' | CandidateStatus>
        options={STATUS_TABS.map((t) => ({
          label: t.value === 'pending' && pendingCount !== null && pendingCount > 0
            ? `${t.label} (${pendingCount})`
            : t.label,
          value: t.value,
        }))}
        value={status}
        onChange={(v) => setStatus(v)}
      />

      {listQuery.error && (
        <Alert
          type="error"
          showIcon
          message="加载失败"
          description={(listQuery.error as Error).message}
        />
      )}

      <Card size="small">
        <Table<Candidate>
          columns={columns}
          dataSource={listQuery.data?.items ?? []}
          rowKey="id"
          size="small"
          pagination={{ pageSize: 10 }}
          loading={listQuery.isLoading}
        />
      </Card>

      {/* Review modal — side-by-side diff */}
      <Modal
        open={reviewCandidate !== null}
        onCancel={() => setReviewCandidate(null)}
        title={reviewCandidate ? `候选 #${reviewCandidate.id} · ${reviewCandidate.strategy_id}` : ''}
        width={900}
        footer={
          reviewCandidate?.status === 'pending' ? (
            <Space>
              <Button onClick={() => setReviewCandidate(null)}>关闭</Button>
              <Button
                danger
                icon={<CloseOutlined />}
                onClick={() => {
                  setRejectModal(reviewCandidate)
                  setRejectReason('')
                  setReviewCandidate(null)
                }}
              >
                拒绝
              </Button>
              <Popconfirm
                title="应用此候选参数？"
                description="将通过热更新立即下发到 runner"
                onConfirm={() => reviewCandidate && onApprove(reviewCandidate)}
                okText="确认应用"
                cancelText="取消"
              >
                <Button type="primary" icon={<CheckOutlined />} loading={approve.isPending}>
                  应用
                </Button>
              </Popconfirm>
            </Space>
          ) : (
            <Button onClick={() => setReviewCandidate(null)}>关闭</Button>
          )
        }
      >
        {reviewCandidate && (
          <Space direction="vertical" size="middle" style={{ width: '100%' }}>
            <Row gutter={12}>
              <Col span={8}>
                <Statistic
                  title="Baseline Sharpe"
                  value={reviewCandidate.baseline_sharpe}
                  precision={3}
                />
              </Col>
              <Col span={8}>
                <Statistic
                  title="Proposed Sharpe"
                  value={reviewCandidate.proposed_sharpe}
                  precision={3}
                />
              </Col>
              <Col span={8}>
                <Statistic
                  title="Δ 改进"
                  value={reviewCandidate.improvement}
                  precision={3}
                  valueStyle={{
                    color: reviewCandidate.improvement > 0 ? '#3f8600' : '#cf1322',
                  }}
                />
              </Col>
            </Row>

            <Row gutter={12}>
              <Col span={12}>
                <Text strong>Baseline (当前)</Text>
                <pre
                  style={{
                    margin: 0,
                    padding: 8,
                    background: '#fafafa',
                    fontFamily: 'Menlo, Consolas, monospace',
                    fontSize: 12,
                    maxHeight: 320,
                    overflow: 'auto',
                  }}
                >
                  {prettyJSON(reviewCandidate.baseline_params)}
                </pre>
              </Col>
              <Col span={12}>
                <Text strong>Proposed (候选)</Text>
                <pre
                  style={{
                    margin: 0,
                    padding: 8,
                    background: '#e6f4ff',
                    fontFamily: 'Menlo, Consolas, monospace',
                    fontSize: 12,
                    maxHeight: 320,
                    overflow: 'auto',
                  }}
                >
                  {prettyJSON(reviewCandidate.proposed_params)}
                </pre>
              </Col>
            </Row>

            {reviewCandidate.rejection_reason && (
              <Alert
                type="warning"
                showIcon
                message="拒绝原因"
                description={reviewCandidate.rejection_reason}
              />
            )}
          </Space>
        )}
      </Modal>

      {/* Reject modal */}
      <Modal
        open={rejectModal !== null}
        onCancel={() => setRejectModal(null)}
        title={rejectModal ? `拒绝候选 #${rejectModal.id}` : ''}
        onOk={onReject}
        okText="确认拒绝"
        cancelText="取消"
        okButtonProps={{ danger: true, disabled: !rejectReason.trim(), loading: reject.isPending }}
      >
        <Paragraph type="secondary">拒绝原因将记入审计，不再可撤销。</Paragraph>
        <TextArea
          rows={4}
          value={rejectReason}
          onChange={(e) => setRejectReason(e.target.value)}
          placeholder="例如：改进幅度不足以补偿交易摩擦 / 只在最近两天跑赢，可能是过拟合"
          maxLength={300}
        />
      </Modal>
    </Space>
  )
}

function prettyJSON(raw: string): string {
  try {
    return JSON.stringify(JSON.parse(raw), null, 2)
  } catch {
    return raw
  }
}
