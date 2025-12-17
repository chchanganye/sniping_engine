import { http } from '@/services/http'

export interface BackendAccount {
  id: string
  mobile: string
  token?: string
  userAgent?: string
  deviceId?: string
  uuid?: string
  proxy?: string
  createdAt?: string
  updatedAt?: string
}

export type TargetMode = 'rush' | 'scan'

export interface BackendTarget {
  id: string
  name?: string
  itemId: number
  skuId: number
  shopId?: number
  mode: TargetMode
  targetQty: number
  perOrderQty: number
  rushAtMs?: number
  enabled: boolean
  createdAt?: string
  updatedAt?: string
}

export interface EngineTaskState {
  targetId: string
  running: boolean
  purchasedQty: number
  targetQty: number
  lastError?: string
  lastAttemptMs?: number
  lastSuccessMs?: number
}

export interface EngineState {
  running: boolean
  tasks: EngineTaskState[]
}

type DataEnvelope<T> = { data: T }

export async function beListAccounts(): Promise<BackendAccount[]> {
  const resp = await http.get<DataEnvelope<BackendAccount[]>>('/api/v1/accounts')
  return resp.data.data ?? []
}

export async function beUpsertAccount(account: Partial<BackendAccount> & { mobile: string }): Promise<BackendAccount> {
  const resp = await http.post<DataEnvelope<BackendAccount>>('/api/v1/accounts', account)
  return resp.data.data
}

export async function beDeleteAccount(id: string): Promise<void> {
  await http.delete('/api/v1/accounts', { params: { id } })
}

export async function beListTargets(): Promise<BackendTarget[]> {
  const resp = await http.get<DataEnvelope<BackendTarget[]>>('/api/v1/targets')
  return resp.data.data ?? []
}

export async function beUpsertTarget(target: Partial<BackendTarget> & Pick<BackendTarget, 'itemId' | 'skuId' | 'mode' | 'targetQty' | 'enabled'>): Promise<BackendTarget> {
  const resp = await http.post<DataEnvelope<BackendTarget>>('/api/v1/targets', target)
  return resp.data.data
}

export async function beDeleteTarget(id: string): Promise<void> {
  await http.delete('/api/v1/targets', { params: { id } })
}

export async function beEngineStart(): Promise<void> {
  await http.post('/api/v1/engine/start')
}

export async function beEngineStop(): Promise<void> {
  await http.post('/api/v1/engine/stop')
}

export async function beEngineState(): Promise<EngineState> {
  const resp = await http.get<DataEnvelope<EngineState>>('/api/v1/engine/state')
  return resp.data.data
}

