// 资源名称（词库、分组等）通用校验：禁止使用容易引发解析或显示异常的特殊字符。
const ILLEGAL_NAME_PATTERN = /[@#$%^&*=+`~<>/\\{}[\]|]/

export function findIllegalNameChar(value: string): string | null {
  const match = value.match(ILLEGAL_NAME_PATTERN)
  return match ? match[0] : null
}

export function validateResourceName(value: string): string | null {
  const trimmed = value.trim()
  if (!trimmed)
    return '名称不能为空'
  const illegal = findIllegalNameChar(trimmed)
  if (illegal)
    return `名称不能包含特殊字符「${illegal}」，仅支持中文、字母、数字及常规标点`
  return null
}
