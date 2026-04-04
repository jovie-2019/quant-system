import { useState, useEffect, useCallback } from 'react'
import { Card, Select, Table, Spin, Empty, Tag, Statistic, Button, message, Space, Row, Col } from 'antd'
import { ReloadOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import api from '../api/client'
import type { APIKey, AssetBalance, AccountBalance } from '../api/types'

export default function Assets() {
  const [accounts, setAccounts] = useState<APIKey[]>([])
  const [selectedId, setSelectedId] = useState<number | null>(null)
  const [balance, setBalance] = useState<AccountBalance | null>(null)
  const [loading, setLoading] = useState(false)
  const [accountsLoading, setAccountsLoading] = useState(true)

  useEffect(() => {
    setAccountsLoading(true)
    api.getAccounts()
      .then(setAccounts)
      .catch(() => message.error('加载账户列表失败'))
      .finally(() => setAccountsLoading(false))
  }, [])

  const fetchBalance = useCallback(async (id: number) => {
    setLoading(true)
    try {
      const data = await api.getAccountBalance(id)
      setBalance(data)
    } catch {
      message.error('查询余额失败')
      setBalance(null)
    } finally {
      setLoading(false)
    }
  }, [])

  const handleSelect = (id: number) => {
    setSelectedId(id)
    fetchBalance(id)
  }

  const handleRefresh = () => {
    if (selectedId !== null) {
      fetchBalance(selectedId)
    }
  }

  // Filter out zero-balance rows and sort by total descending.
  const filteredBalances: AssetBalance[] = (balance?.balances ?? [])
    .filter((b) => b.total > 0)
    .sort((a, b) => b.total - a.total)

  // Find USDT total if available.
  const usdtBalance = filteredBalances.find((b) => b.asset === 'USDT')

  const columns: ColumnsType<AssetBalance> = [
    {
      title: '币种',
      dataIndex: 'asset',
      key: 'asset',
      render: (val: string) => <Tag color="blue">{val}</Tag>,
    },
    {
      title: '可用',
      dataIndex: 'free',
      key: 'free',
      align: 'right',
      render: (val: number) => val.toFixed(8),
    },
    {
      title: '冻结',
      dataIndex: 'locked',
      key: 'locked',
      align: 'right',
      render: (val: number) => val.toFixed(8),
    },
    {
      title: '总计',
      dataIndex: 'total',
      key: 'total',
      align: 'right',
      render: (val: number) => <strong>{val.toFixed(8)}</strong>,
    },
  ]

  return (
    <div>
      <Card title="账户资产" style={{ marginBottom: 16 }}>
        <Space style={{ marginBottom: 16 }}>
          <Select
            style={{ width: 320 }}
            placeholder="选择账户"
            loading={accountsLoading}
            value={selectedId}
            onChange={handleSelect}
            options={accounts.map((a) => ({
              value: a.id,
              label: `${a.label} (${a.api_key})`,
            }))}
          />
          <Button
            icon={<ReloadOutlined />}
            onClick={handleRefresh}
            disabled={selectedId === null}
            loading={loading}
          >
            刷新
          </Button>
        </Space>

        {balance && (
          <Row gutter={16} style={{ marginBottom: 16 }}>
            <Col>
              <Statistic
                title="交易所"
                value={balance.exchange}
              />
            </Col>
            <Col>
              <Statistic
                title="类型"
                value={balance.venue.toUpperCase()}
              />
            </Col>
            {usdtBalance && (
              <Col>
                <Statistic
                  title="USDT 总额"
                  value={usdtBalance.total}
                  precision={2}
                />
              </Col>
            )}
            <Col>
              <Statistic
                title="币种数量"
                value={filteredBalances.length}
              />
            </Col>
            <Col>
              <Statistic
                title="查询时间"
                value={balance.queried_at ? new Date(balance.queried_at).toLocaleString() : '-'}
              />
            </Col>
          </Row>
        )}

        <Spin spinning={loading}>
          {selectedId === null ? (
            <Empty description="请选择一个账户查询余额" />
          ) : filteredBalances.length === 0 && !loading ? (
            <Empty description="该账户无持仓资产" />
          ) : (
            <Table
              columns={columns}
              dataSource={filteredBalances}
              rowKey="asset"
              pagination={false}
              size="middle"
            />
          )}
        </Spin>
      </Card>
    </div>
  )
}
