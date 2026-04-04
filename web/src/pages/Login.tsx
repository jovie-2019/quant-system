import { useState } from 'react'
import { Card, Form, Input, Button, message, Typography } from 'antd'
import { LockOutlined } from '@ant-design/icons'
import { useAuth } from '../contexts/AuthContext'

const { Title } = Typography

export default function Login() {
  const { login } = useAuth()
  const [loading, setLoading] = useState(false)

  const onFinish = async (values: { password: string }) => {
    setLoading(true)
    try {
      await login(values.password)
      message.success('登录成功')
    } catch {
      message.error('登录失败，请检查密码')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div
      style={{
        display: 'flex',
        justifyContent: 'center',
        alignItems: 'center',
        minHeight: '100vh',
        background: '#f0f2f5',
      }}
    >
      <Card style={{ width: 400 }}>
        <Title level={3} style={{ textAlign: 'center', marginBottom: 32 }}>
          Quant System 登录
        </Title>
        <Form onFinish={onFinish} autoComplete="off">
          <Form.Item
            name="password"
            rules={[{ required: true, message: '请输入密码' }]}
          >
            <Input.Password
              prefix={<LockOutlined />}
              placeholder="请输入管理密码"
              size="large"
            />
          </Form.Item>
          <Form.Item>
            <Button type="primary" htmlType="submit" loading={loading} block size="large">
              登录
            </Button>
          </Form.Item>
        </Form>
      </Card>
    </div>
  )
}
