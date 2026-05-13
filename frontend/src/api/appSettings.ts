import type { ProductEdition } from '@/constants/product'

import request from './request'

export interface ProductCapabilitiesPayload {
	realtime: boolean
	batch: boolean
	meeting: boolean
	voiceprint: boolean
	voice_control: boolean
}

export interface ProductFeaturesPayload {
	edition: ProductEdition
	capabilities: ProductCapabilitiesPayload
	supported_languages: ProductLanguagePayload[]
	hardware_tier: ProductEdition
	hardware_requirements: Partial<Record<ProductEdition, HardwareProfilePayload>>
}

export interface ProductLanguagePayload {
	code: string
	label: string
}

export interface HardwareSpecPayload {
	cpu: string
	memory: string
	storage: string
	acceleration: string
}

export interface HardwareProfilePayload {
	tier: ProductEdition
	minimum: HardwareSpecPayload
	recommended: HardwareSpecPayload
}

export interface VoiceControlPayload {
	command_timeout_ms: number
	enabled: boolean
}

export function getProductFeatures() {
	return request.get<ProductFeaturesPayload>('/api/admin/app-settings/product-features')
}

export function getVoiceControl() {
	return request.get<VoiceControlPayload>('/api/admin/app-settings/voice-control')
}

export function updateVoiceControl(payload: VoiceControlPayload) {
	return request.put<VoiceControlPayload>('/api/admin/app-settings/voice-control', payload)
}
