export interface RealtimeSegmentResponse {
  status: string
  text: string
  duration: number
}

export interface RealtimeStreamSessionResponse {
  session_id: string
}

export interface RealtimeStreamChunkResponse {
  session_id?: string
  language?: string
  text: string
  text_delta?: string
  is_final?: boolean
}
