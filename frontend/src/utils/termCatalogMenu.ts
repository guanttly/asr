import type { CatalogTreeNode } from '@/api/termCatalog'

export const TERM_CATALOG_ROUTE = '/system/terms-catalog'

export function termCatalogRouteKey(path: string) {
  return `${TERM_CATALOG_ROUTE}?path=${encodeURIComponent(path)}`
}

export function termCatalogDirMenuKey(path: string) {
  return `system-term-dir:${path}`
}

export function catalogMenuLabel(node: CatalogTreeNode) {
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

export function findFirstCatalogFile(nodes: CatalogTreeNode[]): CatalogTreeNode | null {
  for (const node of nodes) {
    if (!node.is_dir)
      return node
    if (node.children?.length) {
      const found = findFirstCatalogFile(node.children)
      if (found)
        return found
    }
  }
  return null
}

export function findCatalogNodeByPath(nodes: CatalogTreeNode[], path: string): CatalogTreeNode | null {
  for (const node of nodes) {
    if (node.path === path)
      return node
    if (node.is_dir && node.children?.length) {
      const found = findCatalogNodeByPath(node.children, path)
      if (found)
        return found
    }
  }
  return null
}

export function findCatalogParentPaths(nodes: CatalogTreeNode[], path: string, parents: string[] = []): string[] {
  for (const node of nodes) {
    if (node.path === path)
      return parents
    if (node.is_dir && node.children?.length) {
      const found = findCatalogParentPaths(node.children, path, [...parents, node.path])
      if (found.length || node.children.some(child => child.path === path))
        return found
    }
  }
  return []
}

export function findCatalogPathNodes(nodes: CatalogTreeNode[], path: string, parents: CatalogTreeNode[] = []): CatalogTreeNode[] {
  for (const node of nodes) {
    const trail = [...parents, node]
    if (node.path === path)
      return trail
    if (node.is_dir && node.children?.length) {
      const found = findCatalogPathNodes(node.children, path, trail)
      if (found.length)
        return found
    }
  }
  return []
}
