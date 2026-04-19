import { Routes, Route, Navigate } from 'react-router-dom'
import { AuthProvider } from './contexts/AuthContext'
import AdminLayout from './layouts/AdminLayout'
import Login from './pages/Login'
import Dashboard from './pages/Dashboard'
import Exchanges from './pages/Exchanges'
import Accounts from './pages/Accounts'
import Strategies from './pages/Strategies'
import Positions from './pages/Positions'
import Orders from './pages/Orders'
import RiskConfig from './pages/RiskConfig'
import Assets from './pages/Assets'
import Settings from './pages/Settings'
import BacktestWorkbench from './pages/BacktestWorkbench'
import RegimeDashboard from './pages/RegimeDashboard'
import ProtectedRoute from './components/ProtectedRoute'

export default function App() {
  return (
    <AuthProvider>
      <Routes>
        <Route path="/login" element={<Login />} />
        <Route element={<ProtectedRoute />}>
          <Route element={<AdminLayout />}>
            <Route path="/dashboard" element={<Dashboard />} />
            <Route path="/exchanges" element={<Exchanges />} />
            <Route path="/accounts" element={<Accounts />} />
            <Route path="/strategies" element={<Strategies />} />
            <Route path="/backtests" element={<BacktestWorkbench />} />
            <Route path="/regime" element={<RegimeDashboard />} />
            <Route path="/positions" element={<Positions />} />
            <Route path="/orders" element={<Orders />} />
            <Route path="/risk-config" element={<RiskConfig />} />
            <Route path="/assets" element={<Assets />} />
            <Route path="/settings" element={<Settings />} />
          </Route>
        </Route>
        <Route path="*" element={<Navigate to="/dashboard" replace />} />
      </Routes>
    </AuthProvider>
  )
}
