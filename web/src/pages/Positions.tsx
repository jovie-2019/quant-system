import React, { useEffect, useState, useCallback, useRef } from 'react';
import { Table, Typography, Spin, message } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { getPositions } from '../api/client';
import type { Position } from '../api/types';

const { Text } = Typography;

const REFRESH_INTERVAL = 5000;

function formatTime(ms: number): string {
  return new Date(ms).toLocaleString('zh-CN');
}

const Positions: React.FC = () => {
  const [data, setData] = useState<Position[]>([]);
  const [loading, setLoading] = useState(true);
  const mountedRef = useRef(true);

  const fetchData = useCallback(async (showLoading = false) => {
    if (showLoading) setLoading(true);
    try {
      const positions = await getPositions();
      if (mountedRef.current) {
        setData(positions);
      }
    } catch {
      if (mountedRef.current) {
        message.error('获取持仓数据失败');
      }
    } finally {
      if (mountedRef.current) {
        setLoading(false);
      }
    }
  }, []);

  useEffect(() => {
    mountedRef.current = true;
    fetchData(true);

    const timer = setInterval(() => {
      fetchData(false);
    }, REFRESH_INTERVAL);

    return () => {
      mountedRef.current = false;
      clearInterval(timer);
    };
  }, [fetchData]);

  const columns: ColumnsType<Position> = [
    {
      title: '账户',
      dataIndex: 'account_id',
      key: 'account_id',
    },
    {
      title: '币对',
      dataIndex: 'symbol',
      key: 'symbol',
    },
    {
      title: '持仓量',
      dataIndex: 'quantity',
      key: 'quantity',
      align: 'right',
    },
    {
      title: '均价',
      dataIndex: 'avg_cost',
      key: 'avg_cost',
      align: 'right',
      render: (val: number) => val.toFixed(2),
    },
    {
      title: '已实现PnL',
      dataIndex: 'realized_pnl',
      key: 'realized_pnl',
      align: 'right',
      render: (pnl: number) => (
        <Text style={{ color: pnl >= 0 ? '#52c41a' : '#ff4d4f' }}>
          {pnl >= 0 ? '+' : ''}{pnl.toFixed(2)}
        </Text>
      ),
    },
    {
      title: '更新时间',
      dataIndex: 'updated_ms',
      key: 'updated_ms',
      render: (ms: number) => formatTime(ms),
    },
  ];

  if (loading) {
    return (
      <div style={{ textAlign: 'center', paddingTop: 100 }}>
        <Spin size="large" tip="加载中..." />
      </div>
    );
  }

  return (
    <div>
      <Table<Position>
        columns={columns}
        dataSource={data}
        rowKey={(record) => `${record.account_id}-${record.symbol}`}
        pagination={{ pageSize: 20, showSizeChanger: true, showTotal: (total) => `共 ${total} 条` }}
        locale={{ emptyText: '暂无持仓数据' }}
        size="middle"
      />
    </div>
  );
};

export default Positions;
