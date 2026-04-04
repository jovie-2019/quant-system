import React, { useEffect, useState, useCallback } from 'react';
import { Card, Form, InputNumber, Select, Button, message, Spin, Descriptions } from 'antd';
import { getRiskConfig, updateRiskConfig } from '../api/client';
import type { RiskConfig as RiskConfigType } from '../api/types';

const RiskConfig: React.FC = () => {
  const [form] = Form.useForm();
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [current, setCurrent] = useState<RiskConfigType | null>(null);

  const fetchConfig = useCallback(async () => {
    setLoading(true);
    try {
      const config = await getRiskConfig();
      setCurrent(config);
      form.setFieldsValue({
        max_order_qty: config.max_order_qty,
        max_order_amount: config.max_order_amount,
        allowed_symbols: config.allowed_symbols,
      });
    } catch {
      message.error('获取风控配置失败');
    } finally {
      setLoading(false);
    }
  }, [form]);

  useEffect(() => {
    fetchConfig();
  }, [fetchConfig]);

  const handleSave = async () => {
    try {
      const values = await form.validateFields();
      setSaving(true);
      const updated = await updateRiskConfig({
        max_order_qty: values.max_order_qty,
        max_order_amount: values.max_order_amount,
        allowed_symbols: values.allowed_symbols ?? [],
      });
      setCurrent(updated);
      message.success('风控配置已保存');
    } catch (err: unknown) {
      if (err && typeof err === 'object' && 'errorFields' in err) {
        // form validation error, antd handles display
        return;
      }
      message.error('保存风控配置失败');
    } finally {
      setSaving(false);
    }
  };

  if (loading) {
    return (
      <div style={{ textAlign: 'center', paddingTop: 100 }}>
        <Spin size="large" tip="加载中..." />
      </div>
    );
  }

  return (
    <div>
      {current && (
        <Card title="当前配置" style={{ marginBottom: 16 }}>
          <Descriptions column={1} bordered size="small">
            <Descriptions.Item label="单笔最大数量">
              {current.max_order_qty}
            </Descriptions.Item>
            <Descriptions.Item label="单笔最大金额">
              {current.max_order_amount}
            </Descriptions.Item>
            <Descriptions.Item label="允许交易的币对">
              {current.allowed_symbols.join(', ') || '-'}
            </Descriptions.Item>
          </Descriptions>
        </Card>
      )}

      <Card title="修改配置">
        <Form
          form={form}
          layout="vertical"
          style={{ maxWidth: 600 }}
        >
          <Form.Item
            label="单笔最大数量"
            name="max_order_qty"
            rules={[{ required: true, message: '请输入单笔最大数量' }]}
          >
            <InputNumber min={0} step={0.01} style={{ width: '100%' }} />
          </Form.Item>

          <Form.Item
            label="单笔最大金额"
            name="max_order_amount"
            rules={[{ required: true, message: '请输入单笔最大金额' }]}
          >
            <InputNumber min={0} step={100} style={{ width: '100%' }} />
          </Form.Item>

          <Form.Item
            label="允许交易的币对"
            name="allowed_symbols"
          >
            <Select
              mode="tags"
              placeholder="输入币对后按回车添加，如 BTC-USDT"
              style={{ width: '100%' }}
            />
          </Form.Item>

          <Form.Item>
            <Button type="primary" onClick={handleSave} loading={saving}>
              保存
            </Button>
          </Form.Item>
        </Form>
      </Card>
    </div>
  );
};

export default RiskConfig;
