export type DiffKind = 'same' | 'added' | 'removed'

export interface DiffSegment {
  text: string
  kind: DiffKind
}

export interface TextDiffResult {
  beforeSegments: DiffSegment[]
  afterSegments: DiffSegment[]
  addedCount: number
  removedCount: number
  changed: boolean
}

interface DiffOp {
  kind: DiffKind
  text: string
}

function compressSegments(segments: DiffSegment[]) {
  const compact: DiffSegment[] = []
  for (const segment of segments) {
    if (!segment.text)
      continue
    const last = compact.at(-1)
    if (last && last.kind === segment.kind) {
      last.text += segment.text
      continue
    }
    compact.push({ ...segment })
  }
  return compact
}

function diffChars(before: string, after: string) {
  const left = Array.from(before)
  const right = Array.from(after)
  const rows = left.length + 1
  const cols = right.length + 1
  const dp: number[][] = Array.from({ length: rows }, () => Array.from<number>({ length: cols }).fill(0))

  for (let i = 1; i < rows; i++) {
    for (let j = 1; j < cols; j++) {
      if (left[i - 1] === right[j - 1])
        dp[i][j] = dp[i - 1][j - 1] + 1
      else
        dp[i][j] = Math.max(dp[i - 1][j], dp[i][j - 1])
    }
  }

  const ops: DiffOp[] = []
  let i = left.length
  let j = right.length

  while (i > 0 && j > 0) {
    if (left[i - 1] === right[j - 1]) {
      ops.push({ kind: 'same', text: left[i - 1] })
      i--
      j--
      continue
    }

    if (dp[i - 1][j] >= dp[i][j - 1]) {
      ops.push({ kind: 'removed', text: left[i - 1] })
      i--
    }
    else {
      ops.push({ kind: 'added', text: right[j - 1] })
      j--
    }
  }

  while (i > 0) {
    ops.push({ kind: 'removed', text: left[i - 1] })
    i--
  }

  while (j > 0) {
    ops.push({ kind: 'added', text: right[j - 1] })
    j--
  }

  return ops.reverse()
}

function collectSegments(ops: DiffOp[], target: 'before' | 'after') {
  const segments: DiffSegment[] = []
  for (const op of ops) {
    if (op.kind === 'same') {
      segments.push({ kind: 'same', text: op.text })
      continue
    }
    if (target === 'before' && op.kind === 'removed') {
      segments.push({ kind: 'removed', text: op.text })
      continue
    }
    if (target === 'after' && op.kind === 'added')
      segments.push({ kind: 'added', text: op.text })
  }
  return compressSegments(segments)
}

export function buildTextDiff(before = '', after = ''): TextDiffResult {
  const ops = diffChars(before, after)
  return {
    beforeSegments: collectSegments(ops, 'before'),
    afterSegments: collectSegments(ops, 'after'),
    addedCount: ops.filter(op => op.kind === 'added').length,
    removedCount: ops.filter(op => op.kind === 'removed').length,
    changed: before !== after,
  }
}