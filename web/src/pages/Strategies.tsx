import React, { useCallback, useEffect, useState } from 'react';
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
  Drawer,
  Card,
  Switch,
  Empty,
  Tooltip,
} from 'antd';
import { PlusOutlined, ExclamationCircleOutlined, BookOutlined, ReloadOutlined, FileTextOutlined } from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';
import api from '../api/client';
import type { Exchange, APIKey, StrategyConfig, StrategyMeta, LogLine } from '../api/types';

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
  const [stopAllLoading, setStopAllLoading] = useState(false);
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [strategyTypes, setStrategyTypes] = useState<StrategyMeta[]>([]);
  const [typesLoading, setTypesLoading] = useState(false);
  const [logDrawerVisible, setLogDrawerVisible] = useState(false);
  const [logStrategy, setLogStrategy] = useState<StrategyConfig | null>(null);
  const [logs, setLogs] = useState<LogLine[]>([]);
  const [logCount, setLogCount] = useState(0);
  const [logSince, setLogSince] = useState('1h');
  const [logLoading, setLogLoading] = useState(false);
  const [autoRefresh, setAutoRefresh] = useState(false);
  const [expandedFields, setExpandedFields] = useState<Record<number, boolean>>({});
  const [form] = Form.useForm();

  const selectedExchangeId = Form.useWatch('exchange_id', form);

  const filteredAccounts = accounts.filter(
    (a) => a.exchange_id === selectedExchangeId,
  );

  const generateConfigTemplate = (meta: StrategyMeta): string => {
    const config: Record<string, any> = {};
    for (const field of meta.config_fields) {
      if (field.default) {
        config[field.field] = field.type === 'number' ? Number(field.default) : field.default;
      } else {
        config[field.field] = field.type === 'number' ? 0 : '';
      }
    }
    return JSON.stringify(config, null, 2);
  };

  const fetchStrategyTypes = async () => {
    setTypesLoading(true);
    try {
      const types = await api.getStrategyTypes();
      setStrategyTypes(types);
    } catch {
      message.error('获取策略类型失败');
    } finally {
      setTypesLoading(false);
    }
  };

  useEffect(() => {
    if (drawerOpen && strategyTypes.length === 0) {
      fetchStrategyTypes();
    }
  }, [drawerOpen]);

  const handleStrategyTypeChange = (value: string) => {
    const meta = strategyTypes.find((t) => t.type === value);
    if (meta) {
      form.setFieldValue('config', generateConfigTemplate(meta));
    }
  };

  const fetchData = async () => {
    setLoading(true);
    try {
      const [strategies, exchangeList, accountList, types] = await Promise.all([
        api.getStrategies(),
        api.getExchanges(),
        api.getAccounts(),
        api.getStrategyTypes(),
      ]);
      setData(strategies);
      setExchanges(exchangeList);
      setAccounts(accountList);
      setStrategyTypes(types);
    } catch {
      message.error('获取策略列表失败');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchData();
  }, []);

  const handleStopAll = () => {
    Modal.confirm({
      title: '紧急停止所有策略',
      icon: <ExclamationCircleOutlined />,
      content: '确认停止所有运行中的策略？此操作不可撤销。',
      okText: '确认停止',
      okType: 'danger',
      cancelText: '取消',
      onOk: async () => {
        setStopAllLoading(true);
        try {
          const result = await api.stopAllStrategies();
          message.success(`已停止 ${result.stopped_count} 个策略`);
          fetchData();
        } catch {
          message.error('停止所有策略失败');
        } finally {
          setStopAllLoading(false);
        }
      },
    });
  };

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

  const fetchLogs = useCallback(async () => {
    if (!logStrategy) return;
    setLogLoading(true);
    try {
      const result = await api.getStrategyLogs(logStrategy.id, undefined, logSince);
      setLogs(result.lines || []);
      setLogCount(result.count);
    } catch {
      message.error('获取日志失败');
    } finally {
      setLogLoading(false);
    }
  }, [logStrategy, logSince]);

  useEffect(() => {
    if (logDrawerVisible && logStrategy) {
      fetchLogs();
    }
  }, [logDrawerVisible, logStrategy, logSince, fetchLogs]);

  useEffect(() => {
    if (!autoRefresh || !logDrawerVisible || !logStrategy) return;
    const timer = setInterval(fetchLogs, 5000);
    return () => clearInterval(timer);
  }, [autoRefresh, logDrawerVisible, logStrategy, logSince, fetchLogs]);

  const openLogDrawer = (record: StrategyConfig) => {
    setLogStrategy(record);
    setLogs([]);
    setLogCount(0);
    setExpandedFields({});
    setLogDrawerVisible(true);
  };

  const closeLogDrawer = () => {
    setLogDrawerVisible(false);
    setAutoRefresh(false);
    setLogStrategy(null);
  };

  const formatLogTime = (ts: string) => {
    try {
      const d = new Date(ts);
      const hh = String(d.getHours()).padStart(2, '0');
      const mm = String(d.getMinutes()).padStart(2, '0');
      const ss = String(d.getSeconds()).padStart(2, '0');
      const ms = String(d.getMilliseconds()).padStart(3, '0');
      return `${hh}:${mm}:${ss}.${ms}`;
    } catch {
      return ts;
    }
  };

  const getLevelColor = (level: string): string | undefined => {
    const upper = level.toUpperCase();
    if (upper === 'INFO') return 'blue';
    if (upper === 'WARN' || upper === 'WARNING') return 'orange';
    if (upper === 'ERROR') return 'red';
    return undefined;
  };

  const toggleFields = (index: number) => {
    setExpandedFields((prev) => ({ ...prev, [index]: !prev[index] }));
  };

  const logStyles = {
    container: { fontFamily: 'monospace', fontSize: 12, lineHeight: '20px' } as React.CSSProperties,
    line: { padding: '4px 8px', borderBottom: '1px solid #f0f0f0', display: 'flex', gap: 8, alignItems: 'flex-start' } as React.CSSProperties,
    time: { color: '#888', whiteSpace: 'nowrap' as const, minWidth: 100 },
    msg: { flex: 1, wordBreak: 'break-all' as const },
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
      width: 340,
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
            <Button
              type="link"
              size="small"
              icon={<FileTextOutlined />}
              onClick={() => openLogDrawer(record)}
            >
              日志
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
      <div style={{ marginBottom: 16, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <Space>
          <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
            新增策略
          </Button>
          <Button icon={<BookOutlined />} onClick={() => setDrawerOpen(true)}>
            策略类型说明
          </Button>
        </Space>
        <Button
          danger
          icon={<ExclamationCircleOutlined />}
          loading={stopAllLoading}
          onClick={handleStopAll}
        >
          紧急停止所有策略
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
            <Select
              placeholder="请选择策略类型"
              onChange={handleStrategyTypeChange}
            >
              {strategyTypes.map((st) => (
                <Select.Option key={st.type} value={st.type}>
                  {st.name} ({st.type})
                </Select.Option>
              ))}
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

      <Drawer
        title="支持的策略类型"
        placement="right"
        width={600}
        open={drawerOpen}
        onClose={() => setDrawerOpen(false)}
      >
        <Spin spinning={typesLoading}>
          {strategyTypes.map((meta) => {
            const configColumns: ColumnsType<any> = [
              { title: '参数名', dataIndex: 'field', key: 'field' },
              { title: '类型', dataIndex: 'type', key: 'type' },
              {
                title: '必填',
                dataIndex: 'required',
                key: 'required',
                render: (val: boolean) => (
                  <Tag color={val ? 'red' : 'default'}>{val ? '是' : '否'}</Tag>
                ),
              },
              { title: '默认值', dataIndex: 'default', key: 'default' },
              { title: '说明', dataIndex: 'description', key: 'description' },
            ];
            return (
              <Card
                key={meta.type}
                title={`${meta.name} (${meta.type})`}
                style={{ marginBottom: 16 }}
              >
                <p>{meta.description}</p>
                <Table
                  columns={configColumns}
                  dataSource={meta.config_fields}
                  rowKey="field"
                  pagination={false}
                  size="small"
                />
              </Card>
            );
          })}
        </Spin>
      </Drawer>

      <Drawer
        title={`策略日志 - ${logStrategy?.strategy_id ?? ''}`}
        placement="right"
        width={720}
        open={logDrawerVisible}
        onClose={closeLogDrawer}
      >
        <div style={{ marginBottom: 16, display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: 8 }}>
          <Space>
            <Select
              value={logSince}
              onChange={setLogSince}
              style={{ width: 140 }}
              options={[
                { label: '最近5分钟', value: '5m' },
                { label: '最近15分钟', value: '15m' },
                { label: '最近1小时', value: '1h' },
                { label: '最近6小时', value: '6h' },
              ]}
            />
            <Switch
              checked={autoRefresh}
              onChange={setAutoRefresh}
              checkedChildren="自动刷新"
              unCheckedChildren="自动刷新"
            />
            <Tooltip title="刷新">
              <Button
                icon={<ReloadOutlined />}
                onClick={fetchLogs}
                loading={logLoading}
              />
            </Tooltip>
          </Space>
          <span style={{ color: '#888', fontSize: 13 }}>共 {logCount} 条</span>
        </div>

        <Spin spinning={logLoading}>
          {logs.length === 0 ? (
            <Empty description="暂无日志" />
          ) : (
            <div style={{ ...logStyles.container, maxHeight: 'calc(100vh - 200px)', overflowY: 'auto' }}>
              {logs.map((line, idx) => (
                <div key={idx}>
                  <div style={logStyles.line}>
                    <span style={logStyles.time}>{formatLogTime(line.ts)}</span>
                    <Tag color={getLevelColor(line.level)} style={{ margin: 0 }}>
                      {line.level.toUpperCase()}
                    </Tag>
                    <span style={logStyles.msg}>
                      {line.msg}
                      {line.fields && Object.keys(line.fields).length > 0 && (
                        <a
                          onClick={() => toggleFields(idx)}
                          style={{ marginLeft: 8, fontSize: 11 }}
                        >
                          {expandedFields[idx] ? '收起' : '详情'}
                        </a>
                      )}
                    </span>
                  </div>
                  {expandedFields[idx] && line.fields && (
                    <pre style={{ margin: '0 8px 4px 108px', padding: '4px 8px', background: '#f5f5f5', fontSize: 11, borderRadius: 4, overflowX: 'auto' }}>
                      {JSON.stringify(line.fields, null, 2)}
                    </pre>
                  )}
                </div>
              ))}
            </div>
          )}
        </Spin>
      </Drawer>
    </div>
  );
};

export default Strategies;
