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
import type { APIKey, Exchange } from '../api/types';

const Accounts: React.FC = () => {
  const [data, setData] = useState<APIKey[]>([]);
  const [exchanges, setExchanges] = useState<Exchange[]>([]);
  const [loading, setLoading] = useState(true);
  const [modalOpen, setModalOpen] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [form] = Form.useForm();

  const fetchData = async () => {
    setLoading(true);
    try {
      const [accounts, exchangeList] = await Promise.all([
        api.getAccounts(),
        api.getExchanges(),
      ]);
      setData(accounts);
      setExchanges(exchangeList);
    } catch {
      message.error('获取账户列表失败');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchData();
  }, []);

  const openCreate = () => {
    form.resetFields();
    setModalOpen(true);
  };

  const handleSubmit = async () => {
    try {
      const values = await form.validateFields();
      setSubmitting(true);
      await api.createAccount(values);
      message.success('创建成功');
      setModalOpen(false);
      form.resetFields();
      fetchData();
    } catch (err: any) {
      if (err?.errorFields) return;
      message.error('创建失败');
    } finally {
      setSubmitting(false);
    }
  };

  const handleDelete = async (id: number) => {
    try {
      await api.deleteAccount(id);
      message.success('删除成功');
      fetchData();
    } catch {
      message.error('删除失败');
    }
  };

  const formatTime = (ms: number) => {
    return new Date(ms).toLocaleString('zh-CN');
  };

  const getExchangeName = (exchangeId: number) => {
    const ex = exchanges.find((e) => e.id === exchangeId);
    return ex ? ex.name : String(exchangeId);
  };

  const columns: ColumnsType<APIKey> = [
    { title: 'ID', dataIndex: 'id', key: 'id', width: 60 },
    {
      title: '交易所',
      dataIndex: 'exchange_id',
      key: 'exchange_id',
      render: (id: number) => getExchangeName(id),
    },
    { title: '标签', dataIndex: 'label', key: 'label' },
    {
      title: 'API Key',
      dataIndex: 'api_key',
      key: 'api_key',
      render: (val: string) => (
        <code style={{ fontSize: 12 }}>{val}</code>
      ),
    },
    { title: '权限', dataIndex: 'permissions', key: 'permissions' },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (status: string) => (
        <Tag color={status === 'active' ? 'green' : 'red'}>
          {status === 'active' ? '正常' : '禁用'}
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
      render: (_, record) => (
        <Space>
          <Popconfirm
            title="确认删除该账户？删除后不可恢复。"
            onConfirm={() => handleDelete(record.id)}
            okText="确认"
            cancelText="取消"
          >
            <Button type="link" size="small" danger>
              删除
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <div>
      <div style={{ marginBottom: 16 }}>
        <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
          新增账户
        </Button>
      </div>

      <Spin spinning={loading}>
        <Table<APIKey>
          columns={columns}
          dataSource={data}
          rowKey="id"
          pagination={{ pageSize: 10 }}
        />
      </Spin>

      <Modal
        title="新增账户"
        open={modalOpen}
        onOk={handleSubmit}
        onCancel={() => {
          setModalOpen(false);
          form.resetFields();
        }}
        confirmLoading={submitting}
        okText="确认"
        cancelText="取消"
      >
        <Form form={form} layout="vertical">
          <Form.Item
            name="exchange_id"
            label="交易所"
            rules={[{ required: true, message: '请选择交易所' }]}
          >
            <Select placeholder="请选择交易所">
              {exchanges.map((ex) => (
                <Select.Option key={ex.id} value={ex.id}>
                  {ex.name} ({ex.venue})
                </Select.Option>
              ))}
            </Select>
          </Form.Item>
          <Form.Item
            name="label"
            label="标签"
            rules={[{ required: true, message: '请输入标签' }]}
          >
            <Input placeholder="例如: main-key" />
          </Form.Item>
          <Form.Item
            name="api_key"
            label="API Key"
            rules={[{ required: true, message: '请输入 API Key' }]}
          >
            <Input placeholder="请输入 API Key" />
          </Form.Item>
          <Form.Item
            name="api_secret"
            label="API Secret"
            rules={[{ required: true, message: '请输入 API Secret' }]}
          >
            <Input.Password placeholder="请输入 API Secret" />
          </Form.Item>
          <Form.Item name="passphrase" label="Passphrase（OKX 需要）">
            <Input.Password placeholder="OKX 交易所需填写" />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
};

export default Accounts;
