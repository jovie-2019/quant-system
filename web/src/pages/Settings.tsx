import React from 'react';
import { Card, Descriptions, Button, message, Row, Col } from 'antd';

const Settings: React.FC = () => {
  const handleTestWebhook = () => {
    message.info('功能开发中');
  };

  return (
    <Row gutter={[16, 16]}>
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
            <Button onClick={handleTestWebhook}>
              发送测试消息
            </Button>
          </div>
        </Card>
      </Col>

      <Col xs={24} lg={12}>
        <Card title="系统信息">
          <Descriptions column={1} bordered size="small">
            <Descriptions.Item label="版本">v1.0.0</Descriptions.Item>
            <Descriptions.Item label="运行环境">本机部署</Descriptions.Item>
            <Descriptions.Item label="数据库">MySQL</Descriptions.Item>
            <Descriptions.Item label="消息队列">NATS JetStream</Descriptions.Item>
            <Descriptions.Item label="API 地址">localhost:8090</Descriptions.Item>
          </Descriptions>
        </Card>
      </Col>
    </Row>
  );
};

export default Settings;
