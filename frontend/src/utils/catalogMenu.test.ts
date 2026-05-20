import type { RulesTreeNode } from '@/api/rulesCatalog'
import type { CatalogTreeNode } from '@/api/termCatalog'

import { describe, expect, it } from 'vitest'

import { findFirstRulesFile, findRulesNodeByPath, findRulesParentPaths, rulesCatalogDirMenuKey, rulesCatalogMenuLabel, rulesCatalogRouteKey } from './rulesCatalogMenu'
import { catalogMenuLabel, findCatalogNodeByPath, findCatalogParentPaths, findFirstCatalogFile, termCatalogDirMenuKey, termCatalogRouteKey } from './termCatalogMenu'

const termTree: CatalogTreeNode[] = [
  {
    name: 'radiology',
    path: 'radiology',
    is_dir: true,
    children: [
      { name: 'README.md', path: 'radiology/README.md', is_dir: false },
      { name: '01-胸部.md', path: 'radiology/chest.md', is_dir: false },
    ],
  },
]

const rulesTree: RulesTreeNode[] = [
  {
    name: 'radiology',
    path: 'radiology',
    is_dir: true,
    children: [
      { name: '00-总则.md', path: 'radiology/00.md', is_dir: false },
      { name: '02_报告格式.md', path: 'radiology/format.md', is_dir: false },
    ],
  },
]

describe('catalog menu helpers', () => {
  it('builds stable route and directory keys', () => {
    expect(termCatalogRouteKey('影像/胸部.md')).toBe('/system/terms-catalog?path=%E5%BD%B1%E5%83%8F%2F%E8%83%B8%E9%83%A8.md')
    expect(termCatalogDirMenuKey('radiology')).toBe('system-term-dir:radiology')
    expect(rulesCatalogRouteKey('rules/a b.md')).toBe('/system/rules-catalog?path=rules%2Fa%20b.md')
    expect(rulesCatalogDirMenuKey('radiology')).toBe('system-rules-dir:radiology')
  })

  it('formats directory and numbered markdown labels', () => {
    expect(catalogMenuLabel(termTree[0])).toBe('影像科')
    expect(catalogMenuLabel(termTree[0].children![0])).toBe('总览')
    expect(catalogMenuLabel(termTree[0].children![1])).toBe('01. 胸部')
    expect(rulesCatalogMenuLabel(rulesTree[0].children![0])).toBe('总则')
    expect(rulesCatalogMenuLabel(rulesTree[0].children![1])).toBe('02. 报告格式')
  })

  it('finds first files, nodes, and parent paths recursively', () => {
    expect(findFirstCatalogFile(termTree)?.path).toBe('radiology/README.md')
    expect(findCatalogNodeByPath(termTree, 'radiology/chest.md')?.name).toBe('01-胸部.md')
    expect(findCatalogParentPaths(termTree, 'radiology/chest.md')).toEqual(['radiology'])

    expect(findFirstRulesFile(rulesTree)?.path).toBe('radiology/00.md')
    expect(findRulesNodeByPath(rulesTree, 'radiology/format.md')?.name).toBe('02_报告格式.md')
    expect(findRulesParentPaths(rulesTree, 'radiology/format.md')).toEqual(['radiology'])
  })
})
