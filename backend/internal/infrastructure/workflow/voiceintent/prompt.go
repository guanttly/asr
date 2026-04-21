package voiceintent

import (
	"fmt"
	"strings"
)

const DefaultPromptTemplate = `你是桌面端语音控制工作流中的“意图识别节点”。
请只根据给定的控制指令组，判断用户转写文本是否命中了其中一条有效控制指令。

输出要求：
- 只允许输出 JSON，不要输出 markdown 或解释。
- 如果没有匹配项，返回：{"matched":false,"intent":"","group_key":"","command_id":0,"confidence":0,"reason":"未命中有效指令"}
- 如果命中，返回：{"matched":true,"intent":"...","group_key":"...","command_id":123,"confidence":0到1之间的小数,"reason":"简短原因"}

控制指令组：
{{COMMAND_LIBRARY}}

用户文本：
{{TEXT}}

{{EXTRA_PROMPT}}`

func BuildPrompt(template string, extraPrompt string, inputText string, catalog Catalog) string {
	if strings.TrimSpace(template) == "" {
		template = DefaultPromptTemplate
	}
	prompt := strings.ReplaceAll(template, "{{TEXT}}", strings.TrimSpace(inputText))
	prompt = strings.ReplaceAll(prompt, "{{COMMAND_LIBRARY}}", FormatCatalog(catalog))
	prompt = strings.ReplaceAll(prompt, "{{EXTRA_PROMPT}}", strings.TrimSpace(extraPrompt))
	if !strings.Contains(prompt, strings.TrimSpace(inputText)) {
		prompt = strings.TrimSpace(prompt) + "\n\n用户文本：\n" + strings.TrimSpace(inputText)
	}
	if extra := strings.TrimSpace(extraPrompt); extra != "" && !strings.Contains(prompt, extra) {
		prompt = strings.TrimSpace(prompt) + "\n\n附加约束：\n" + extra
	}
	return prompt
}

func FormatCatalog(catalog Catalog) string {
	if len(catalog.Commands) == 0 {
		return "- 无可用控制指令"
	}
	var builder strings.Builder
	currentGroup := ""
	for _, command := range catalog.Commands {
		if command.GroupKey != currentGroup {
			if builder.Len() > 0 {
				builder.WriteString("\n")
			}
			currentGroup = command.GroupKey
			builder.WriteString(fmt.Sprintf("- 分组 %s (%s)\n", command.GroupName, command.GroupKey))
		}
		builder.WriteString(fmt.Sprintf("  - command_id=%d, intent=%s, label=%s, utterances=%s\n", command.EntryID, command.Intent, command.Label, strings.Join(command.Utterances, " / ")))
	}
	return strings.TrimSpace(builder.String())
}
