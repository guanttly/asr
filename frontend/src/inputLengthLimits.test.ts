import { existsSync, readdirSync, readFileSync } from 'node:fs'
import { dirname, extname, join, relative, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const repoRoot = resolve(fileURLToPath(new URL('../..', import.meta.url)))

const LIMIT_TYPES = {
  webDefault: 'Web 管理端未单独声明的 `NInput`',
  search: '搜索关键字',
  username: '用户名',
  password: '密码',
  shortIdentifier: 'OpenAPI 应用名、语音指令分组键、自定义意图值',
  name: '显示名、设备别名、会议标题、工作流名、词库名、模型名',
  speakerName: '说话人姓名',
  url: 'URL、服务地址、回调地址前缀',
  apiKey: 'API Key',
  note: '简短说明、备注',
  term: '词条、敏感词、语气词、替换文本',
  variantOrRegex: '误写变体、规则预览原文、正则表达式、正则替换',
  multiline: '自定义词列表、控制指令候选话术、附加提示、Meta JSON、回调白名单',
  promptOrJson: 'Prompt 模板、节点 JSON 配置、工作流测试文本',
  meetingMarkdown: '桌面会议纪要 Markdown',
} as const

type LimitType = typeof LIMIT_TYPES[keyof typeof LIMIT_TYPES]

interface SourceControl {
  relativePath: string
  tagName: 'NInput' | 'input' | 'textarea'
  line: number
  source: string
  maxlength?: number
}

function readWorkspaceFile(relativePath: string) {
  return readFileSync(join(repoRoot, relativePath), 'utf8')
}

function resolveWorkspaceModule(fromRelativePath: string, specifier: string) {
  let basePath: string | undefined
  if (specifier.startsWith('@/')) {
    const appRoot = fromRelativePath.startsWith('desktop/src/') ? 'desktop/src' : 'frontend/src'
    basePath = join(appRoot, specifier.slice(2))
  }
  else if (specifier.startsWith('.')) {
    basePath = join(dirname(fromRelativePath), specifier)
  }

  if (!basePath)
    return undefined

  const candidates = [basePath, `${basePath}.ts`, `${basePath}.vue`, join(basePath, 'index.ts')]
  return candidates.find(candidate => existsSync(join(repoRoot, candidate)))
}

function parseNumericConstants(source: string, exportedOnly = false) {
  const constants = new Map<string, number>()
  const prefix = exportedOnly ? 'export\\s+' : '(?:export\\s+)?'
  const constPattern = new RegExp(`\\b${prefix}const\\s+([A-Za-z_$][\\w$]*)(?:\\s*:[^=]+)?\\s*=\\s*(\\d+)\\b`, 'g')

  for (const match of source.matchAll(constPattern))
    constants.set(match[1], Number(match[2]))

  return constants
}

function parseImportedNumericConstants(relativePath: string, content: string) {
  const constants = new Map<string, number>()
  const importPattern = /\bimport\s*\{([^}]+)\}\s*from\s*["']([^"']+)["']/g

  for (const importMatch of content.matchAll(importPattern)) {
    const modulePath = resolveWorkspaceModule(relativePath, importMatch[2])
    if (!modulePath)
      continue

    const moduleConstants = parseNumericConstants(readWorkspaceFile(modulePath), true)
    for (const rawSpecifier of importMatch[1].split(',')) {
      const specifier = rawSpecifier.trim().replace(/^type\s+/, '')
      const aliasMatch = specifier.match(/^([A-Za-z_$][\w$]*)(?:\s+as\s+([A-Za-z_$][\w$]*))?$/)
      if (!aliasMatch)
        continue

      const exportedName = aliasMatch[1]
      const localName = aliasMatch[2] ?? exportedName
      const value = moduleConstants.get(exportedName)
      if (value !== undefined)
        constants.set(localName, value)
    }
  }

  return constants
}

function collectNumericConstants(relativePath: string, content: string) {
  return new Map([
    ...parseImportedNumericConstants(relativePath, content),
    ...parseNumericConstants(content),
  ])
}

function listVueFiles(relativeDirectory: string): string[] {
  const absoluteDirectory = join(repoRoot, relativeDirectory)
  return readdirSync(absoluteDirectory, { withFileTypes: true }).flatMap((entry) => {
    const absolutePath = join(absoluteDirectory, entry.name)
    const workspacePath = relative(repoRoot, absolutePath)

    if (entry.isDirectory())
      return listVueFiles(workspacePath)

    if (entry.isFile() && extname(entry.name) === '.vue')
      return [workspacePath]

    return []
  })
}

function parseDocumentedLimits() {
  const markdown = readWorkspaceFile('docs/输入长度限制.md')
  const limits = new Map<string, number>()

  for (const line of markdown.split('\n')) {
    const cells = line.split('|').slice(1, -1).map(cell => cell.trim())
    if (cells.length < 2 || cells[0].includes('---') || cells[0] === '类型')
      continue

    const limit = Number(cells[1])
    if (Number.isInteger(limit))
      limits.set(cells[0], limit)
  }

  return limits
}

function extractMaxlength(source: string, numericConstants: Map<string, number>) {
  if (!source.includes('maxlength'))
    return undefined

  const limitMatch = source.match(/\b:?maxlength\s*=\s*["'](\d+)["']/)
  if (!limitMatch) {
    const bindingMatch = source.match(/(?::|v-bind:)maxlength\s*=\s*["']([A-Za-z_$][\w$]*)["']/)
    const bindingValue = bindingMatch ? numericConstants.get(bindingMatch[1]) : undefined
    if (bindingValue === undefined)
      throw new Error(`Unsupported maxlength expression: ${source}`)
    return bindingValue
  }

  return Number(limitMatch[1])
}

function extractAttribute(source: string, name: string) {
  const attributeMatch = source.match(new RegExp(`\\b${name}\\s*=\\s*["']([^"']+)["']`))
  return attributeMatch?.[1]
}

function hasBooleanAttribute(source: string, name: string) {
  return new RegExp(`\\b${name}\\b`).test(source)
}

function isScopedTextControl(control: SourceControl) {
  if (control.tagName === 'NInput')
    return true

  if (control.tagName === 'textarea')
    return !hasBooleanAttribute(control.source, 'readonly')

  const type = extractAttribute(control.source, 'type')?.toLowerCase() ?? 'text'
  return ['text', 'password', 'search', 'url'].includes(type)
}

function extractControls(relativePath: string) {
  const content = readWorkspaceFile(relativePath)
  const numericConstants = collectNumericConstants(relativePath, content)
  const controls: SourceControl[] = []
  const tagNames = ['NInput', 'input', 'textarea'] as const

  for (const tagName of tagNames) {
    const tagPattern = new RegExp(`<${tagName}\\b[^>]*>`, 'g')
    for (const tagMatch of content.matchAll(tagPattern)) {
      const source = tagMatch[0]
      const line = content.slice(0, tagMatch.index).split('\n').length
      controls.push({
        relativePath,
        tagName,
        line,
        source,
        maxlength: extractMaxlength(source, numericConstants),
      })
    }
  }

  return controls.filter(isScopedTextControl)
}

function allScopedTextControls() {
  return ['frontend/src', 'desktop/src'].flatMap(relativeDirectory => listVueFiles(relativeDirectory).flatMap(extractControls))
}

function classifyControl(control: SourceControl): LimitType | undefined {
  const text = `${control.relativePath}\n${control.source}`

  if (/keyword|docsKeyword|dictKeyword|entryKeyword|v-model="search"|搜索|按标题搜索会议/.test(text))
    return LIMIT_TYPES.search

  if (/speakerName|说话人姓名/.test(text))
    return LIMIT_TYPES.speakerName

  if (/pages\/system\/openapi\.vue[\s\S]*form\.name|dictForm\.groupKey|entryForm\.intent/.test(text))
    return LIMIT_TYPES.shortIdentifier

  if (/username/.test(text))
    return LIMIT_TYPES.username

  if (/api_key|API Key/.test(text))
    return LIMIT_TYPES.apiKey

  if (/password/.test(text))
    return LIMIT_TYPES.password

  if (/custom_words|utterancesText|callback_whitelist_text|meta_json|extra_prompt|wake_words|homophone_words/.test(text))
    return LIMIT_TYPES.multiline

  if (/prompt_template|configText|nodeTestInput|executeInput|Prompt 模板|JSON 配置|工作流测试文本/.test(text))
    return LIMIT_TYPES.promptOrJson

  if (/endpoint|audioUrl|serverUrl|callback.*地址|DEFAULT_SERVER_URL|https?:\/\//.test(text))
    return LIMIT_TYPES.url

  if (/wrongVariants|ruleForm\.pattern|ruleForm\.replacement|ruleForm\.previewSource|rule\.pattern|rule\.replacement|正则表达式/.test(text))
    return LIMIT_TYPES.variantOrRegex

  if (/draftContent/.test(text))
    return LIMIT_TYPES.meetingMarkdown

  if (/description|notes|备注|说明|用途说明/.test(text))
    return LIMIT_TYPES.note

  if (/correctTerm|entryForm\.word|sensitiveWord|selectedConfig\.replacement|替换文本|敏感词|语气词/.test(text))
    return LIMIT_TYPES.term

  if (/display_name|deviceAlias|draftTitle|department|domain|scene|selectedConfig\.model|\bmodel\b|模型|label|form\.name|dictForm\.name|workflow\.name|saveAsForm\.name|newDictName|newDictTag/.test(text))
    return LIMIT_TYPES.name

  return undefined
}

function formatControl(control: SourceControl) {
  return `${control.relativePath}:${control.line} ${control.source.replace(/\s+/g, ' ').trim()}`
}

describe('input length limits', () => {
  it('keeps the Web NInput fallback aligned with the documented default', () => {
    const documentedLimits = parseDocumentedLimits()
    const expectedLimit = documentedLimits.get(LIMIT_TYPES.webDefault)
    const appSource = readWorkspaceFile('frontend/src/App.vue')
    const fallbackMatch = appSource.match(/\bmaxlength:\s*(\d+)/)

    expect(expectedLimit).toBeDefined()
    expect(Number(fallbackMatch?.[1])).toBe(expectedLimit)
  })

  it('keeps explicit text input maxlength values aligned with the documented rules', () => {
    const documentedLimits = parseDocumentedLimits()
    const explicitControls = allScopedTextControls().filter(control => control.maxlength !== undefined)

    const problems = explicitControls.flatMap((control) => {
      const limitType = classifyControl(control)
      if (!limitType)
        return [`Unclassified maxlength: ${formatControl(control)}`]

      const expectedLimit = documentedLimits.get(limitType)
      if (expectedLimit === undefined)
        return [`Missing documented limit for ${limitType}: ${formatControl(control)}`]

      if (control.maxlength !== expectedLimit)
        return [`Expected ${expectedLimit} for ${limitType}, got ${control.maxlength}: ${formatControl(control)}`]

      return []
    })

    expect(problems).toEqual([])
  })

  it('requires native text inputs and editable textareas to declare maxlength explicitly', () => {
    const missingLimits = allScopedTextControls()
      .filter(control => control.tagName !== 'NInput' && control.maxlength === undefined)
      .map(formatControl)

    expect(missingLimits).toEqual([])
  })
})
