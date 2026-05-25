export const AUDIO_UPLOAD_MAX_SIZE_MB = 200
export const AUDIO_UPLOAD_MAX_SIZE_BYTES = AUDIO_UPLOAD_MAX_SIZE_MB * 1024 * 1024
export const AUDIO_UPLOAD_SIZE_LIMIT_MESSAGE = `音频文件不能超过 ${AUDIO_UPLOAD_MAX_SIZE_MB} MB，请压缩或切分后再上传`

export function isAudioFileOverSizeLimit(file: File | null | undefined) {
  return !!file && file.size > AUDIO_UPLOAD_MAX_SIZE_BYTES
}
