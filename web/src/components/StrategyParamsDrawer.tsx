// StrategyParamsDrawer surfaces hot-reload controls for a single live
// strategy. Opens from the Strategies list page via a row action and:
//
//   • Shows the currently persisted params (from strategy_configs.config_json)
//   • Lets the operator submit a new params JSON with a reason
//   • Polls the revisions audit log every 3 seconds so an ack that arrives
//     seconds after the POST shows up automatically
//   • Issues pause / resume / shadow commands through the same wire
//
// The drawer does NOT rebuild the whole strategy — it's for hot-reload
// only. Symbol swaps and other destructive changes should still go
// through the existing Strategies edit modal.

import { useEffect, useMemo, useState } from 'react'
import {
  Alert,
  Button,
  Descriptions,
  Divider,
  Drawer,
  Input,
  Radio,
  Space,
  Table,
  Tag,
  Typography,
  message,
} from 'antd'
import type { ColumnsType } from 'antd/es/table'
import {
  CheckCircleOutlined,
  ClockCircleOutlined,
  ExclamationCircleOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons'

import {
  useProposeStrategyParams,
  useStrategyRevisions,
  type ProposeParamsRequest,
  type RevisionRow,
  type StrategyControlType,
} from '../api/strategy_params'

const { Text, Paragraph } = Typography
const { TextArea } = Input

export interface StrategyParamsDrawerProps {
  open: boolean
  onClose: () => void
  strategyDBID: number | null
  strategyLabel: string // e.g. "momentum-btcusdt (momentum)"
  currentParamsJSON: string // pretty-printed, passed by parent from strategy_configs.config_json
}

const COMMAND_OPTIONS: { value: StrategyControlType; label: string }[] = [
  { value: 'update_params', label: '更新参数' },
  { value: 'pause', label: '暂停（保留市场数据热身）' },
  { value: 'resume', label: '恢复' },
  { value: 'shadow_on', label: '开启 Shadow（只产信号不下单）' },
  { value: 'shadow_off', label: '关闭 Shadow' },
]

export default function StrategyParamsDrawer({
  open,
  onClose,
  strategyDBID,
  strategyLabel,
  currentParamsJSON,
}: StrategyParamsDrawerProps) {
  const [commandType, setCommandType] = useState<StrategyControlType>('update_params')
  const [paramsDraft, setParamsDraft] = useState('')
  const [reason, setReason] = useState('')
  const [paramsErr, setParamsErr] = useState<string | null>(null)

  // Reset the form whenever the drawer opens on a new strategy.
  useEffect(() => {
    if (open) {
      setParamsDraft(currentParamsJSON)
      setReason('')
      setCommandType('update_params')
      setParamsErr(null)
    }
  }, [open, currentParamsJSON])

  const revisionsQuery = useStrategyRevisions(strategyDBID, {
    enabled: open,
    refetchMs: 3_000,
  })
  const mutation = useProposeStrategyParams(strategyDBID)

  async function onSubmit() {
    if (strategyDBID === null) return
    const req: ProposeParamsRequest = { type: commandType, reason }
    if (commandType === 'update_params') {
      try {
        const parsed = JSON.parse(paramsDraft)
        if (typeof parsed !== 'object' || parsed === null || Array.isArray(parsed)) {
          setParamsErr('params 必须是 JSON 对象')
          return
        }
        req.params = parsed
        setParamsErr(null)
      } catch (err) {
        setParamsErr(err instanceof Error ? err.message : String(err))
        return
      }
    }
    try {
      await mutation.mutateAsync(req)
      message.success(`${commandType} 已下发，等待 runner 确认`)
    } catch (err) {
      const detail =
        (err as { response?: { data?: { message?: string } }; message?: string })?.response?.data?.message ??
        (err instanceof Error ? err.message : String(err))
      message.error(`下发失败: ${detail}`)
    }
  }

  const columns: ColumnsType<RevisionRow> = useMemo(
    () => [
      { title: 'Rev', dataIndex: 'revision', width: 60 },
      {
        title: 'Type',
        dataIndex: 'command_type',
        width: 130,
        render: (v: StrategyControlType) => <Tag>{v}</Tag>,
      },
      {
        title: '状态',
        width: 110,
        render: (_: unknown, row: RevisionRow) => {
          if (row.ack_received_ms === undefined) {
            return (
              <Tag icon={<ClockCircleOutlined />} color="processing">
                pending
              </Tag>
            )
          }
          if (row.ack_accepted) {
            return (
              <Tag icon={<CheckCircleOutlined />} color="success">
                accepted
              </Tag>
            )
          }
          return (
            <Tag icon={<ExclamationCircleOutlined />} color="error">
              rejected
            </Tag>
          )
        },
      },
      {
        title: '发起人',
        dataIndex: 'actor',
        width: 110,
        ellipsis: true,
      },
      {
        title: '时间',
        dataIndex: 'issued_ms',
        width: 170,
        render: (v: number) => new Date(v).toLocaleString(),
      },
      {
        title: '原因 / 错误',
        render: (_: unknown, row: RevisionRow) =>
          row.ack_error ? (
            <Text type="danger">{row.ack_error}</Text>
          ) : (
            <Text type="secondary">{row.reason || '—'}</Text>
          ),
      },
    ],
    [],
  )

  const diff = useMemo(() => {
    if (commandType !== 'update_params' || !paramsDraft) return null
    try {
      const before = JSON.parse(currentParamsJSON || '{}')
      const after = JSON.parse(paramsDraft)
      const changed: Record<string, { before: unknown; after: unknown }> = {}
      const keys = new Set<string>([...Object.keys(before), ...Object.keys(after)])
      for (const k of keys) {
        if (JSON.stringify(before[k]) !== JSON.stringify(after[k])) {
          changed[k] = { before: before[k], after: after[k] }
        }
      }
      return changed
    } catch {
      return null
    }
  }, [commandType, paramsDraft, currentParamsJSON])

  return (
    <Drawer
      open={open}
      onClose={onClose}
      title={
        <Space>
          <ThunderboltOutlined />
          <span>{strategyLabel}</span>
        </Space>
      }
      width={800}
      destroyOnClose
    >
      <Space direction="vertical" size="middle" style={{ width: '100%' }}>
        <Alert
          type="info"
          showIcon
          message="热更新说明"
          description="参数更新会以 NATS 消息发送给 runner，不重启进程；runner 接收后原子替换参数并回传 ack。"
        />

        <Descriptions size="small" column={1} bordered>
          <Descriptions.Item label="当前持久化参数">
            <pre style={{ margin: 0, fontFamily: 'Menlo, Consolas, monospace', fontSize: 12 }}>
              {currentParamsJSON || '—'}
            </pre>
          </Descriptions.Item>
        </Descriptions>

        <div>
          <Text strong>命令类型</Text>
          <div style={{ marginTop: 4 }}>
            <Radio.Group
              value={commandType}
              onChange={(e) => setCommandType(e.target.value)}
              options={COMMAND_OPTIONS}
              optionType="button"
            />
          </div>
        </div>

        {commandType === 'update_params' && (
          <div>
            <Text strong>新参数 (JSON)</Text>
            <TextArea
              rows={8}
              value={paramsDraft}
              onChange={(e) => setParamsDraft(e.target.value)}
              style={{ fontFamily: 'Menlo, Consolas, monospace', fontSize: 12, marginTop: 4 }}
            />
            {paramsErr && (
              <Alert type="error" showIcon style={{ marginTop: 8 }} message={paramsErr} />
            )}
            {diff && Object.keys(diff).length > 0 && (
              <div style={{ marginTop: 8 }}>
                <Text type="secondary">变更字段：</Text>
                <pre
                  style={{
                    margin: 0,
                    padding: 8,
                    background: '#fff7e6',
                    fontFamily: 'Menlo, Consolas, monospace',
                    fontSize: 12,
                  }}
                >
                  {Object.entries(diff)
                    .map(([k, v]) => `${k}: ${JSON.stringify(v.before)} → ${JSON.stringify(v.after)}`)
                    .join('\n')}
                </pre>
              </div>
            )}
          </div>
        )}

        <div>
          <Text strong>变更原因（记入审计）</Text>
          <Input
            value={reason}
            onChange={(e) => setReason(e.target.value)}
            placeholder="例如：从参数优化 opt_xxxx 推荐的最优参数"
            maxLength={200}
            style={{ marginTop: 4 }}
          />
        </div>

        <Button
          type="primary"
          icon={<ThunderboltOutlined />}
          loading={mutation.isPending}
          onClick={onSubmit}
          block
        >
          下发
        </Button>

        <Divider>审计日志（每 3 秒刷新）</Divider>
        <Paragraph type="secondary" style={{ marginTop: -12, fontSize: 12 }}>
          pending 行在 runner 回 ack 后自动更新为 accepted 或 rejected；rejected 时右侧会显示 runner 的错误原因。
        </Paragraph>

        <Table<RevisionRow>
          columns={columns}
          dataSource={revisionsQuery.data?.items ?? []}
          rowKey="id"
          size="small"
          pagination={{ pageSize: 8 }}
          loading={revisionsQuery.isLoading}
        />
      </Space>
    </Drawer>
  )
}
