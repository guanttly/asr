import { existsSync, readFileSync } from 'node:fs'

interface ResolveBuildMetaOptions {
  packageJsonCandidates?: string[]
}

interface BuildMeta {
  version: string
  buildCode: string
  buildDate: string
}

function resolveVersion(packageJsonCandidates: string[]) {
  const envVersion = [process.env.ASR_APP_VERSION, process.env.VITE_APP_VERSION].find(
    value => typeof value === 'string' && value.trim(),
  )
  if (envVersion)
    return envVersion.trim()

  for (const candidate of packageJsonCandidates) {
    if (!candidate || !existsSync(candidate))
      continue

    const packageJson = JSON.parse(readFileSync(candidate, 'utf8')) as { version?: string }
    if (typeof packageJson.version === 'string' && packageJson.version.trim())
      return packageJson.version.trim()
  }

  throw new Error('failed to resolve app version from env or package.json')
}

function parseBuildDate(rawValue: string) {
  if (!rawValue)
    return new Date()

  const value = rawValue.trim()
  const plainDateMatch = value.match(/^(\d{4})-(\d{2})-(\d{2})$/)
  if (plainDateMatch) {
    const year = Number(plainDateMatch[1])
    const month = Number(plainDateMatch[2])
    const day = Number(plainDateMatch[3])
    const date = new Date(year, month - 1, day)
    if (date.getFullYear() !== year || date.getMonth() !== month - 1 || date.getDate() !== day)
      throw new Error(`invalid ASR_BUILD_DATE value: ${rawValue}`)
    return date
  }

  const date = new Date(value)
  if (Number.isNaN(date.getTime()))
    throw new Error(`invalid ASR_BUILD_DATE value: ${rawValue}`)
  return date
}

function encodeMonth(month: number) {
  return month <= 9 ? String(month) : String.fromCharCode('A'.charCodeAt(0) + month - 10)
}

function formatDatePart(value: number) {
  return String(value).padStart(2, '0')
}

export function resolveBuildMeta({ packageJsonCandidates = [] }: ResolveBuildMetaOptions = {}): BuildMeta {
  const version = resolveVersion(packageJsonCandidates)
  const buildDate = parseBuildDate(process.env.ASR_BUILD_DATE || process.env.VITE_BUILD_DATE || '')
  const year = buildDate.getFullYear()
  const month = buildDate.getMonth() + 1
  const day = buildDate.getDate()

  return {
    version,
    buildCode: `${String(year).slice(-2)}${encodeMonth(month)}${formatDatePart(day)}`,
    buildDate: `${year}-${formatDatePart(month)}-${formatDatePart(day)}`,
  }
}