import React, { useEffect, useState } from 'react';
import {
  Table,
  Button,
  Modal,
  Form,
  Input,
  Select,
  Tag,
  Popconfirm,
  message,
  Space,
  Spin,
} from 'antd';
import { PlusOutlined } from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';
import api from '../api/client';
import type { Exchange, APIKey, StrategyConfig } from '../api/types';

const Strategies: React.FC = () => {
  const [data, setData] = useState<StrategyConfig[]>([]);
  const [exchanges, setExchanges] = useState<Exchange[]>([]);
  const [accounts, setAccounts] = useState<APIKey[]>([]);
  const [loading, setLoading] = useState(true);
  const [modalOpen, setModalOpen] = useState(false);
  const [editingRecord, setEditingRecord] = useState<StrategyConfig | null>(
    null,
  );
  const [submitting, setSubmitting] = useState(false);
  const [actionLoading, setActionLoading] = useState<Record<number, boolean>>(
    {},
  );
  const [form] = Form.useForm();

  const selectedExchangeId = Form.useWatch('exchange_id', form);

  const filteredAccounts = accounts.filter(
    (a) => a.exchange_id === selectedExchangeId,
  );

  const fetchData = async () => {
    setLoading(true);
    try {
      const [strategies, exchangeList, accountList] = await Promise.all([
        api.getStrategies(),
        api.getExchanges(),
        api.getAccounts(),
      ]);
      setData(strategies);
      setExchanges(exchangeList);
      setAccounts(accountList);
    } catch {
      message.error('获取策略列表失败');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchData();
  }, []);

  const openCreate = () => {
    setEditingRecord(null);
    form.resetFields();
    setModalOpen(true);
  };

  const openEdit = (record: StrategyConfig) => {
    setEditingRecord(record);
    form.setFieldsValue({
      strategy_id: record.strategy_id,
      strategy_type: record.strategy_type,
      exchange_id: record.exchange_id,
      api_key_id: record.api_key_id,
      config: JSON.stringify(record.config, null, 2),
    });
    setModalOpen(true);
  };

  const handleSubmit = async () => {
    try {
      const values = await form.validateFields();
      let configObj: Record<string, any>;
      try {
        configObj = JSON.parse(values.config || '{}');
      } catch {
        message.error('策略参数必须是合法的 JSON 格式');
        return;
      }
      const payload = {
        ...values,
        config: configObj,
      };
      setSubmitting(true);
      if (editingRecord) {
        await api.updateStrategy(editingRecord.id, {
          exchange_id: payload.exchange_id,
          api_key_id: payload.api_key_id,
          config: payload.config,
        });
        message.success('更新成功');
      } else {
        await api.createStrategy(payload);
        message.success('创建成功');
      }
      setModalOpen(false);
      form.resetFields();
      fetchData();
    } catch (err: any) {
      if (err?.errorFields) return;
      message.error(editingRecord ? '更新失败' : '创建失败');
    } finally {
      setSubmitting(false);
    }
  };

  const handleDelete = async (id: number) => {
    try {
      await api.deleteStrategy(id);
      message.success('删除成功');
      fetchData();
    } catch {
      message.error('删除失败');
    }
  };

  const handleStart = async (id: number) => {
    setActionLoading((prev) => ({ ...prev, [id]: true }));
    try {
      await api.startStrategy(id);
      message.success('策略已启动');
      fetchData();
    } catch {
      message.error('启动失败');
    } finally {
      setActionLoading((prev) => ({ ...prev, [id]: false }));
    }
  };

  const handleStop = async (id: number) => {
    setActionLoading((prev) => ({ ...prev, [id]: true }));
    try {
      await api.stopStrategy(id);
      message.success('策略已停止');
      fetchData();
    } catch {
      message.error('停止失败');
    } finally {
      setActionLoading((prev) => ({ ...prev, [id]: false }));
    }
  };

  const formatTime = (ms: number) => {
    return new Date(ms).toLocaleString('zh-CN');
  };

  const getExchangeName = (exchangeId: number) => {
    const ex = exchanges.find((e) => e.id === exchangeId);
    return ex ? ex.name : String(exchangeId);
  };

  const columns: ColumnsType<StrategyConfig> = [
    { title: 'ID', dataIndex: 'id', key: 'id', width: 60 },
    { title: '策略ID', dataIndex: 'strategy_id', key: 'strategy_id' },
    { title: '策略类型', dataIndex: 'strategy_type', key: 'strategy_type' },
    {
      title: '交易所',
      dataIndex: 'exchange_id',
      key: 'exchange_id',
      render: (id: number) => getExchangeName(id),
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (status: string) => (
        <Tag color={status === 'running' ? 'green' : 'default'}>
          {status === 'running' ? '运行中' : '已停止'}
        </Tag>
      ),
    },
    {
      title: '创建时间',
      dataIndex: 'created_ms',
      key: 'created_ms',
      render: (ms: number) => formatTime(ms),
    },
    {
      title: '操作',
      key: 'action',
      width: 280,
      render: (_, record) => {
        const isRunning = record.status === 'running';
        const isLoading = actionLoading[record.id] ?? false;
        return (
          <Space>
            {isRunning ? (
              <Popconfirm
                title="确认停止该策略？"
                onConfirm={() => handleStop(record.id)}
                okText="确认"
                cancelText="取消"
              >
                <Button
                  type="link"
                  size="small"
                  danger
                  loading={isLoading}
                >
                  停止
                </Button>
              </Popconfirm>
            ) : (
              <Button
                type="link"
                size="small"
                loading={isLoading}
                onClick={() => handleStart(record.id)}
              >
                启动
              </Button>
            )}
            <Button
              type="link"
              size="small"
              onClick={() => openEdit(record)}
              disabled={isRunning}
            >
              编辑
            </Button>
            <Popconfirm
              title="确认删除该策略？"
              onConfirm={() => handleDelete(record.id)}
              okText="确认"
              cancelText="取消"
              disabled={isRunning}
            >
              <Button type="link" size="small" danger disabled={isRunning}>
                删除
              </Button>
            </Popconfirm>
          </Space>
        );
      },
    },
  ];

  return (
    <div>
      <div style={{ marginBottom: 16 }}>
        <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
          新增策略
        </Button>
      </div>

      <Spin spinning={loading}>
        <Table<StrategyConfig>
          columns={columns}
          dataSource={data}
          rowKey="id"
          pagination={{ pageSize: 10 }}
        />
      </Spin>

      <Modal
        title={editingRecord ? '编辑策略' : '新增策略'}
        open={modalOpen}
        onOk={handleSubmit}
        onCancel={() => {
          setModalOpen(false);
          form.resetFields();
        }}
        confirmLoading={submitting}
        okText="确认"
        cancelText="取消"
        width={600}
      >
        <Form form={form} layout="vertical">
          <Form.Item
            name="strategy_id"
            label="策略ID"
            rules={[{ required: true, message: '请输入策略ID' }]}
          >
            <Input
              placeholder="例如: momentum-btc"
              disabled={!!editingRecord}
            />
          </Form.Item>
          <Form.Item
            name="strategy_type"
            label="策略类型"
            rules={[{ required: true, message: '请选择策略类型' }]}
          >
            <Select placeholder="请选择策略类型">
              <Select.Option value="momentum">Momentum</Select.Option>
            </Select>
          </Form.Item>
          <Form.Item
            name="exchange_id"
            label="交易所"
            rules={[{ required: true, message: '请选择交易所' }]}
          >
            <Select
              placeholder="请选择交易所"
              onChange={() => form.setFieldValue('api_key_id', undefined)}
            >
              {exchanges.map((ex) => (
                <Select.Option key={ex.id} value={ex.id}>
                  {ex.name} ({ex.venue})
                </Select.Option>
              ))}
            </Select>
          </Form.Item>
          <Form.Item
            name="api_key_id"
            label="API Key"
            rules={[{ required: true, message: '请选择 API Key' }]}
          >
            <Select
              placeholder={
                selectedExchangeId
                  ? '请选择 API Key'
                  : '请先选择交易所'
              }
              disabled={!selectedExchangeId}
            >
              {filteredAccounts.map((acc) => (
                <Select.Option key={acc.id} value={acc.id}>
                  {acc.label} ({acc.api_key})
                </Select.Option>
              ))}
            </Select>
          </Form.Item>
          <Form.Item
            name="config"
            label="策略参数（JSON 格式）"
            rules={[
              {
                validator: (_, value) => {
                  if (!value) return Promise.resolve();
                  try {
                    JSON.parse(value);
                    return Promise.resolve();
                  } catch {
                    return Promise.reject(new Error('请输入合法的 JSON'));
                  }
                },
              },
            ]}
          >
            <Input.TextArea
              rows={6}
              placeholder='{"symbol": "BTC-USDT", "window_size": 20}'
            />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
};

export default Strategies;
