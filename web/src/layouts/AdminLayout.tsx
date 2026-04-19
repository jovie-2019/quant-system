import { useState } from 'react'
import { Outlet, useNavigate, useLocation } from 'react-router-dom'
import { Layout, Menu, Button, theme } from 'antd'
import {
  DashboardOutlined,
  BankOutlined,
  KeyOutlined,
  RocketOutlined,
  FundOutlined,
  OrderedListOutlined,
  SafetyOutlined,
  WalletOutlined,
  SettingOutlined,
  LogoutOutlined,
  MenuFoldOutlined,
  MenuUnfoldOutlined,
  ExperimentOutlined,
} from '@ant-design/icons'
import { useAuth } from '../contexts/AuthContext'

const { Header, Sider, Content } = Layout

const menuItems = [
  { key: '/dashboard', icon: <DashboardOutlined />, label: '仪表盘' },
  { key: '/exchanges', icon: <BankOutlined />, label: '交易所' },
  { key: '/accounts', icon: <KeyOutlined />, label: 'API 密钥' },
  { key: '/strategies', icon: <RocketOutlined />, label: '策略管理' },
  { key: '/backtests', icon: <ExperimentOutlined />, label: '回测工作台' },
  { key: '/positions', icon: <FundOutlined />, label: '持仓' },
  { key: '/orders', icon: <OrderedListOutlined />, label: '订单' },
  { key: '/risk-config', icon: <SafetyOutlined />, label: '风控配置' },
  { key: '/assets', icon: <WalletOutlined />, label: '账户资产' },
  { key: '/settings', icon: <SettingOutlined />, label: '系统设置' },
]

export default function AdminLayout() {
  const [collapsed, setCollapsed] = useState(false)
  const navigate = useNavigate()
  const location = useLocation()
  const { logout } = useAuth()
  const {
    token: { colorBgContainer, borderRadiusLG },
  } = theme.useToken()

  return (
    <Layout style={{ minHeight: '100vh' }}>
      <Sider trigger={null} collapsible collapsed={collapsed}>
        <div
          style={{
            height: 32,
            margin: 16,
            color: '#fff',
            fontWeight: 'bold',
            fontSize: collapsed ? 14 : 18,
            textAlign: 'center',
            lineHeight: '32px',
            whiteSpace: 'nowrap',
            overflow: 'hidden',
          }}
        >
          {collapsed ? 'QS' : 'Quant System'}
        </div>
        <Menu
          theme="dark"
          mode="inline"
          selectedKeys={[location.pathname]}
          items={menuItems}
          onClick={({ key }) => navigate(key)}
        />
      </Sider>
      <Layout>
        <Header
          style={{
            padding: '0 16px',
            background: colorBgContainer,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
          }}
        >
          <Button
            type="text"
            icon={collapsed ? <MenuUnfoldOutlined /> : <MenuFoldOutlined />}
            onClick={() => setCollapsed(!collapsed)}
            style={{ fontSize: 16, width: 64, height: 64 }}
          />
          <Button type="text" icon={<LogoutOutlined />} onClick={logout}>
            退出登录
          </Button>
        </Header>
        <Content
          style={{
            margin: 24,
            padding: 24,
            background: colorBgContainer,
            borderRadius: borderRadiusLG,
            minHeight: 280,
          }}
        >
          <Outlet />
        </Content>
      </Layout>
    </Layout>
  )
}
