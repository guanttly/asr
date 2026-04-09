export interface WorkflowTemplateMeta {
  scenarios: string[]
  summary: string
}

function fromDescription(description = ''): WorkflowTemplateMeta {
  const value = description.toLowerCase()
  const scenarios = new Set<string>()

  if (value.includes('实时'))
    scenarios.add('实时整理')
  if (value.includes('批量'))
    scenarios.add('批量转写')
  if (value.includes('会议'))
    scenarios.add('会议纪要')
  if (value.includes('规则'))
    scenarios.add('规则清洗')
  if (value.includes('术语'))
    scenarios.add('术语纠正')

  return {
    scenarios: Array.from(scenarios),
    summary: description || '适合作为工作流编排的起始模板。',
  }
}

export function getWorkflowTemplateMeta(name?: string, description?: string): WorkflowTemplateMeta {
  switch (name) {
    case '标准转写整理':
      return {
        scenarios: ['批量转写', '实时整理'],
        summary: '先做口语清洗，再接规则和术语纠正，适合作为通用整理基线。',
      }
    case '会议纪要增强':
      return {
        scenarios: ['会议纪要', '会后整理'],
        summary: '面向会议纪要生成，优先保留纪要和摘要链路，适合会后统一整理。',
      }
    case '规则优先精修':
      return {
        scenarios: ['规则清洗', '行业口径'],
        summary: '先用正则与规则稳定清洗文本，再叠加术语和纠错节点。',
      }
    default:
      return fromDescription(description)
  }
}
