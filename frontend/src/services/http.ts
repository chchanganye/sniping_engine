import axios from 'axios'
import { ElNotification } from 'element-plus'

export const http = axios.create({
  timeout: 20000,
  withCredentials: true,
})

let lastNotifyAt = 0
function notifyOnce(message: string) {
  const now = Date.now()
  if (now-lastNotifyAt < 3000) return
  lastNotifyAt = now
  ElNotification({
    title: '网络异常',
    message,
    type: 'error',
    position: 'bottom-right',
    duration: 4000,
  })
}

http.interceptors.response.use(
  (resp) => resp,
  (error) => {
    const status = error?.response?.status
    if (status === 502) {
      notifyOnce('后端服务不可用(502)，请检查后端进程/容器是否运行。')
    } else if (!error?.response && error?.code !== 'ERR_CANCELED') {
      notifyOnce('网络连接失败，请检查后端地址或代理配置。')
    }
    return Promise.reject(error)
  },
)
