export type StreamingMessage = {
  type: string
  text: string
  is_final: boolean
  sequence: number
  received_bytes?: number
}