import React, { useEffect, useState } from 'react';
import { Card, Row, Col, Statistic, Table, Alert, Skeleton, message } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import {
  RocketOutlined,
  FundOutlined,
  FileTextOutlined,
  DollarOutlined,
} from '@ant-design/icons';
import api from '../api/client';
import type { Overview } from '../api/types';

interface ExchangeSummary {
  id: number;
  name: string;
  status: string;
}

const Dashboard: React.FC = () => {
  const [overview, setOverview] = useState<Overview | null>(null);
  const [loading, setLoading] = useState(true);

  const fetchOverview = async () => {
    setLoading(true);
    try {
      const data = await api.getOverview();
      setOverview(data);
    } catch {
      message.error('获取总览数据失败');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchOverview();
  }, []);

  const exchangeColumns: ColumnsType<ExchangeSummary> = [
    { title: 'ID', dataIndex: 'id', key: 'id', width: 80 },
    { title: '名称', dataIndex: 'name', key: 'name' },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (status: string) => (
        <span style={{ color: status === 'active' ? '#52c41a' : '#ff4d4f' }}>
          {status === 'active' ? '正常' : '禁用'}
        </span>
      ),
    },
  ];

  if (loading) return <Skeleton active paragraph={{ rows: 6 }} />;

  return (
    <div>
      {overview && overview.exchanges.length === 0 && (
        <Alert
          message="未配置交易所"
          description="请先在「交易所管理」中添加交易所，并在「账户管理」中配置 API Key。"
          type="warning"
          showIcon
          style={{ marginBottom: 16 }}
        />
      )}
      <Row gutter={[16, 16]}>
        <Col xs={24} sm={12} lg={6}>
          <Card>
            <Statistic
              title="运行中策略"
              value={overview?.running_strategies ?? 0}
              suffix={`/ ${overview?.total_strategies ?? 0}`}
              prefix={<RocketOutlined />}
              valueStyle={{ color: '#1890ff' }}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card>
            <Statistic
              title="总持仓数"
              value={overview?.total_positions ?? 0}
              prefix={<FundOutlined />}
              valueStyle={{ color: '#722ed1' }}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card>
            <Statistic
              title="总订单数"
              value={overview?.total_orders ?? 0}
              prefix={<FileTextOutlined />}
              valueStyle={{ color: '#13c2c2' }}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card>
            <Statistic
              title="累计已实现PnL"
              value={overview?.total_realized_pnl ?? 0}
              precision={2}
              prefix={<DollarOutlined />}
              valueStyle={{
                color:
                  (overview?.total_realized_pnl ?? 0) >= 0
                    ? '#52c41a'
                    : '#ff4d4f',
              }}
            />
          </Card>
        </Col>
      </Row>

      <Card title="交易所列表" style={{ marginTop: 16 }}>
        <Table<ExchangeSummary>
          columns={exchangeColumns}
          dataSource={overview?.exchanges ?? []}
          rowKey="id"
          pagination={false}
          size="small"
        />
      </Card>
    </div>
  );
};

export default Dashboard;
