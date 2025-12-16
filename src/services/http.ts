import axios from 'axios'

export const http = axios.create({
  timeout: 20000,
})

http.interceptors.response.use(
  (resp) => resp,
  (error) => Promise.reject(error),
)
