import axios from "axios"

export const axiosInstance = axios.create({
  baseURL: "http://localhost:19999/api/v1/",
  // timeout: 30 * 1000, // todo
  headers: {
    "Cache-Control": "no-cache, no-store",
    Pragma: "no-cache",
  },
  withCredentials: true,
})
