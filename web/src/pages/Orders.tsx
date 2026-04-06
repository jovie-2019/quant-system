import React, { useEffect, useState, useCallback, useRef, useMemo } from 'react';
import { Table, Tag, Input, Space, Spin, message } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { getOrders } from '../api/client';
import type { Order } from '../api/types';

const REFRESH_INTERVAL = 10000;

const STATUS_COLOR: Record<string, string> = {
  filled: 'green',
  ack: 'blue',
  partial_filled: 'orange',
  canceled: 'default',
  rejected: 'red',
  new: 'processing',
};

const STATUS_LABEL: Record<string, string> = {
  filled: '已成交',
  ack: '已确认',
  partial_filled: '部分成交',
  canceled: '已撤销',
  rejected: '已拒绝',
  new: '新建',
};

function formatTime(ms: number): string {
  return new Date(ms).toLocaleString('zh-CN');
}

const Orders: React.FC = () => {
  const [data, setData] = useState<Order[]>([]);
  const [loading, setLoading] = useState(true);
  const [searchSymbol, setSearchSymbol] = useState('');
  const mountedRef = useRef(true);

  const fetchData = useCallback(async (showLoading = false) => {
    if (showLoading) setLoading(true);
    try {
      const orders = await getOrders();
      if (mountedRef.current) {
        setData(orders);
      }
    } catch {
      if (mountedRef.current) {
        message.error('获取订单数据失败');
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

  const filteredData = useMemo(() => {
    if (!searchSymbol.trim()) return data;
    const keyword = searchSymbol.trim().toUpperCase();
    return data.filter((order) =>
      order.symbol.toUpperCase().includes(keyword),
    );
  }, [data, searchSymbol]);

  const columns: ColumnsType<Order> = [
    {
      title: '客户端订单号',
      dataIndex: 'client_order_id',
      key: 'client_order_id',
      ellipsis: true,
    },
    {
      title: '交易所订单号',
      dataIndex: 'venue_order_id',
      key: 'venue_order_id',
      ellipsis: true,
    },
    {
      title: '币对',
      dataIndex: 'symbol',
      key: 'symbol',
    },
    {
      title: '状态',
      dataIndex: 'state',
      key: 'state',
      render: (state: string) => (
        <Tag color={STATUS_COLOR[state] ?? 'default'}>
          {STATUS_LABEL[state] ?? state}
        </Tag>
      ),
    },
    {
      title: '成交量',
      dataIndex: 'filled_qty',
      key: 'filled_qty',
      align: 'right',
    },
    {
      title: '均价',
      dataIndex: 'avg_price',
      key: 'avg_price',
      align: 'right',
      render: (val: number) => val.toFixed(2),
    },
    {
      title: '版本',
      dataIndex: 'state_version',
      key: 'state_version',
      align: 'center',
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
      <Space direction="vertical" style={{ width: '100%' }} size="middle">
        <Input.Search
          placeholder="按币对搜索，如 BTC-USDT"
          allowClear
          onSearch={(value) => setSearchSymbol(value)}
          onChange={(e) => setSearchSymbol(e.target.value)}
          style={{ maxWidth: 360 }}
        />
        <Table<Order>
          columns={columns}
          dataSource={filteredData}
          rowKey="client_order_id"
          pagination={{ pageSize: 20, showSizeChanger: true, showTotal: (total) => `共 ${total} 条` }}
          locale={{ emptyText: '暂无订单记录' }}
          size="middle"
        />
      </Space>
    </div>
  );
};

export default Orders;
