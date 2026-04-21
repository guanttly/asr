import request from './request'

export interface VoiceControlPayload {
	command_timeout_ms: number
	enabled: boolean
}

export function getVoiceControl() {
	return request.get<VoiceControlPayload>('/api/admin/app-settings/voice-control')
}

export function updateVoiceControl(payload: VoiceControlPayload) {
	return request.put<VoiceControlPayload>('/api/admin/app-settings/voice-control', payload)
}
