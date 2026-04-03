export type ApiEnvelope<T> = {
  code: number
  message: string
  data: T
}