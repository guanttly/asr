import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

import { assessDesktopUseCase, type DesktopUseCaseRow } from './useCaseCoverage'

function parseCsv(content: string): string[][] {
  const rows: string[][] = []
  let row: string[] = []
  let field = ''
  let inQuotes = false

  for (let index = 0; index < content.length; index += 1) {
    const char = content[index]
    const next = content[index + 1]

    if (inQuotes) {
      if (char === '"' && next === '"') {
        field += '"'
        index += 1
      }
      else if (char === '"') {
        inQuotes = false
      }
      else {
        field += char
      }
      continue
    }

    if (char === '"') {
      inQuotes = true
    }
    else if (char === ',') {
      row.push(field)
      field = ''
    }
    else if (char === '\n') {
      row.push(field.replace(/\r$/, ''))
      rows.push(row)
      row = []
      field = ''
    }
    else {
      field += char
    }
  }

  if (field.length > 0 || row.length > 0) {
    row.push(field)
    rows.push(row)
  }
  return rows
}

function readUseCases(): DesktopUseCaseRow[] {
  const path = fileURLToPath(new URL('../../../tests/语音转写 会议纪要生成软件-所有用例.csv', import.meta.url))
  const rows = parseCsv(readFileSync(path, 'utf8'))
  const headers = rows[0]
  return rows.slice(1).map((row) => {
    const record: Record<string, string> = {}
    headers.forEach((header, index) => {
      record[header] = row[index] || ''
    })
    return record as unknown as DesktopUseCaseRow
  })
}

describe('desktop use case coverage matrix', () => {
  it('keeps every CSV use case classified for source review, unit coverage, or manual focus', () => {
    const rows = readUseCases()
    const assessments = rows.map(assessDesktopUseCase)

    expect(rows).toHaveLength(193)
    expect(new Set(assessments.map(item => item.id)).size).toBe(rows.length)
    expect(assessments.filter(item => item.kind === 'unclassified')).toEqual([])
  })

  it('tracks previously high-risk hot-plug and config-sync cases as resolved', () => {
    const byId = new Map(readUseCases().map(row => [row.用例编号, assessDesktopUseCase(row)]))

    expect(byId.get('121310')).toMatchObject({ marker: 'OK', area: '麦克风热插拔' })
    expect(byId.get('121311')).toMatchObject({ marker: 'OK', area: '麦克风热插拔' })
    expect(byId.get('121326')).toMatchObject({ marker: 'OK', area: '配置同步失败' })
    expect(byId.get('121327')).toMatchObject({ marker: 'OK', area: '配置同步失败' })
  })
})