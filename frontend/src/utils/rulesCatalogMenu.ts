import type { RulesTreeNode } from '@/api/rulesCatalog'

export const RULES_CATALOG_ROUTE = '/system/rules-catalog'

export function rulesCatalogRouteKey(path: string) {
  return `${RULES_CATALOG_ROUTE}?path=${encodeURIComponent(path)}`
}

export function rulesCatalogDirMenuKey(path: string) {
  return `system-rules-dir:${path}`
}

export function rulesCatalogMenuLabel(node: RulesTreeNode) {
  const fileBase = node.name.replace(/\.md$/i, '').trim()
  if (!node.is_dir) {
    const numbered = fileBase.match(/^(\d+)[-_－—](.+)$/)
    if (numbered) {
      const index = numbered[1]
      const section = numbered[2].split(/[-_－—]/)[0]?.trim() || numbered[2].trim()
      if (index === '00')
        return section
      return `${index}. ${section}`
    }
    if (fileBase.toLowerCase() === 'readme')
      return '总览'
  }
  const label = node.title?.trim() || fileBase || node.name
  return label.replace(/\.md$/i, '').replace(/\s*·\s*/g, ' · ')
}

export function findFirstRulesFile(nodes: RulesTreeNode[]): RulesTreeNode | null {
  for (const node of nodes) {
    if (!node.is_dir)
      return node
    if (node.children?.length) {
      const found = findFirstRulesFile(node.children)
      if (found)
        return found
    }
  }
  return null
}

export function findRulesNodeByPath(nodes: RulesTreeNode[], path: string): RulesTreeNode | null {
  for (const node of nodes) {
    if (node.path === path)
      return node
    if (node.is_dir && node.children?.length) {
      const found = findRulesNodeByPath(node.children, path)
      if (found)
        return found
    }
  }
  return null
}

export function findRulesParentPaths(nodes: RulesTreeNode[], path: string, parents: string[] = []): string[] {
  for (const node of nodes) {
    if (node.path === path)
      return parents
    if (node.is_dir && node.children?.length) {
      const found = findRulesParentPaths(node.children, path, [...parents, node.path])
      if (found.length || node.children.some(child => child.path === path))
        return found
    }
  }
  return []
}

export function findRulesPathNodes(nodes: RulesTreeNode[], path: string, parents: RulesTreeNode[] = []): RulesTreeNode[] {
  for (const node of nodes) {
    const trail = [...parents, node]
    if (node.path === path)
      return trail
    if (node.is_dir && node.children?.length) {
      const found = findRulesPathNodes(node.children, path, trail)
      if (found.length)
        return found
    }
  }
  return []
}
