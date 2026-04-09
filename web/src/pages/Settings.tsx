import React, { useEffect, useState } from 'react';
import { Card, Descriptions, Button, message, Row, Col, Tag, Table, Spin, Divider, Badge } from 'antd';
import { LinkOutlined, ReloadOutlined } from '@ant-design/icons';
import { getSystemStatus, sendTestAlert } from '../api/client';
import type { SystemStatus, NATSStream, TableStats } from '../api/types';

const Settings: React.FC = () => {
  const [systemStatus, setSystemStatus] = useState<SystemStatus | null>(null);
  const [loading, setLoading] = useState(false);
  const [alertLoading, setAlertLoading] = useState(false);

  const fetchStatus = async () => {
    setLoading(true);
    try {
      const data = await getSystemStatus();
      setSystemStatus(data);
    } catch {
      message.error('获取系统状态失败');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchStatus();
  }, []);

  const handleTestWebhook = async () => {
    setAlertLoading(true);
    try {
      await sendTestAlert();
      message.success('测试消息已发送到飞书');
    } catch {
      message.error('发送测试消息失败');
    } finally {
      setAlertLoading(false);
    }
  };

  const formatBytes = (bytes: number): string => {
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
    return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GB`;
  };

  const natsColumns = [
    { title: '名称', dataIndex: 'name', key: 'name' },
    {
      title: '消息数',
      dataIndex: 'messages',
      key: 'messages',
      render: (v: number) => v.toLocaleString(),
    },
    {
      title: '大小',
      dataIndex: 'bytes',
      key: 'bytes',
      render: (v: number) => formatBytes(v),
    },
    { title: '消费者数', dataIndex: 'consumers', key: 'consumers' },
  ];

  const mysqlColumns = [
    { title: '表名', dataIndex: 'name', key: 'name' },
    {
      title: '行数',
      dataIndex: 'rows',
      key: 'rows',
      render: (v: number, record: TableStats) =>
        record.error ? <Tag color="red">{record.error}</Tag> : v.toLocaleString(),
    },
  ];

  const externalLinks = [
    { label: 'Grafana 监控面板', url: 'http://localhost:3001' },
    { label: '  行情质量面板', url: 'http://localhost:3001/d/market-quality-v1' },
    { label: '  服务运行面板', url: 'http://localhost:3001/d/service-runtime-v1' },
    { label: '  引擎状态面板', url: 'http://localhost:3001/d/engine-k8s-status-v1' },
    { label: 'Prometheus 查询', url: 'http://localhost:9090' },
    { label: 'Alertmanager 告警', url: 'http://localhost:9093' },
    { label: 'phpMyAdmin 数据库', url: 'http://localhost:8880' },
    { label: 'NATS 监控', url: 'http://localhost:8222' },
    { label: 'Loki 日志查询', url: 'http://localhost:3001/explore' },
  ];

  const renderServiceBadge = (name: string) => {
    if (!systemStatus) return <Badge status="default" text="未知" />;
    const svc = systemStatus.services[name];
    if (!svc) return <Badge status="default" text="未知" />;
    return svc.status === 'ok' ? (
      <span>
        <Badge status="success" text={svc.info} />
      </span>
    ) : (
      <span>
        <Badge status="error" text={svc.info} />
      </span>
    );
  };

  return (
    <Row gutter={[16, 16]}>
      {/* Section 1: 技术栈信息 */}
      <Col xs={24} lg={12}>
        <Card title="技术栈信息">
          <Descriptions column={1} bordered size="small">
            <Descriptions.Item label="版本">v1.0.0</Descriptions.Item>
            <Descriptions.Item label="后端语言">Go 1.26</Descriptions.Item>
            <Descriptions.Item label="前端框架">React 18 + Ant Design 5</Descriptions.Item>
            <Descriptions.Item label="数据库">MySQL 8.0</Descriptions.Item>
            <Descriptions.Item label="消息队列">NATS JetStream</Descriptions.Item>
            <Descriptions.Item label="监控">Prometheus + Grafana</Descriptions.Item>
            <Descriptions.Item label="日志">Loki + Promtail</Descriptions.Item>
            <Descriptions.Item label="告警">Alertmanager → 飞书</Descriptions.Item>
            <Descriptions.Item label="部署方式">Docker Compose</Descriptions.Item>
          </Descriptions>
        </Card>
      </Col>

      {/* Section 2: 外部服务链接 */}
      <Col xs={24} lg={12}>
        <Card title="外部服务链接">
          <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
            {externalLinks.map((link) => (
              <Button
                key={link.url}
                type="link"
                icon={<LinkOutlined />}
                href={link.url}
                target="_blank"
                style={{ textAlign: 'left', paddingLeft: link.label.startsWith('  ') ? 32 : 0 }}
              >
                {link.label.trim()}
              </Button>
            ))}
          </div>
        </Card>
      </Col>

      {/* Section 3: 系统运行状态 */}
      <Col xs={24} lg={12}>
        <Card
          title="系统运行状态"
          extra={
            <Button
              icon={<ReloadOutlined />}
              onClick={fetchStatus}
              loading={loading}
              size="small"
            >
              刷新
            </Button>
          }
        >
          <Spin spinning={loading}>
            <Descriptions column={1} bordered size="small" style={{ marginBottom: 16 }}>
              <Descriptions.Item label="MySQL">{renderServiceBadge('mysql')}</Descriptions.Item>
              <Descriptions.Item label="NATS">{renderServiceBadge('nats')}</Descriptions.Item>
              <Descriptions.Item label="Loki">{renderServiceBadge('loki')}</Descriptions.Item>
            </Descriptions>

            {systemStatus && systemStatus.nats_streams.length > 0 && (
              <>
                <Divider orientation="left" plain>
                  NATS Streams
                </Divider>
                <Table<NATSStream>
                  dataSource={systemStatus.nats_streams}
                  columns={natsColumns}
                  rowKey="name"
                  size="small"
                  pagination={false}
                />
              </>
            )}

            {systemStatus && systemStatus.mysql_tables.length > 0 && (
              <>
                <Divider orientation="left" plain>
                  MySQL 表统计
                </Divider>
                <Table<TableStats>
                  dataSource={systemStatus.mysql_tables}
                  columns={mysqlColumns}
                  rowKey="name"
                  size="small"
                  pagination={false}
                />
              </>
            )}
          </Spin>
        </Card>
      </Col>

      {/* Section 4: 飞书告警配置 */}
      <Col xs={24} lg={12}>
        <Card title="飞书告警配置">
          <Descriptions column={1} bordered size="small">
            <Descriptions.Item label="Webhook URL">
              <span style={{ color: '#888' }}>
                飞书 Webhook 通过环境变量配置，请修改 .env 文件中的 FEISHU_WEBHOOK_URL
              </span>
            </Descriptions.Item>
          </Descriptions>
          <div style={{ marginTop: 16 }}>
            <Button onClick={handleTestWebhook} loading={alertLoading}>
              发送测试消息
            </Button>
          </div>
        </Card>
      </Col>
    </Row>
  );
};

export default Settings;
