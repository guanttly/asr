package voicecommand

import "strings"

const (
	GroupKeySceneMode = "scene_mode"

	IntentKeySceneReportSwitch  = "scene_report_switch"
	IntentKeySceneMeetingSwitch = "scene_meeting_switch"

	LegacyIntentReport  = "report"
	LegacyIntentMeeting = "meeting"
)

type BuiltinIntentSpec struct {
	Key               string
	HandlerName       string
	DefaultLabel      string
	Description       string
	DefaultUtterances []string
	SortOrder         int
	LegacyKeys        []string
}

type BuiltinGroupSpec struct {
	Key         string
	Name        string
	Description string
	IsBase      bool
	Intents     []BuiltinIntentSpec
	LegacyKeys  []string
}

var builtinGroupSpecs = []BuiltinGroupSpec{
	{
		Key:         GroupKeySceneMode,
		Name:        "场景切换控制",
		Description: "桌面端唤醒后默认可用的场景切换命令。",
		IsBase:      true,
		Intents: []BuiltinIntentSpec{
			{
				Key:               IntentKeySceneReportSwitch,
				HandlerName:       "切换到报告模式",
				DefaultLabel:      "报告模式",
				Description:       "把桌面端切换到报告模式。",
				DefaultUtterances: []string{"报告模式", "切到报告模式", "进入报告模式", "开始报告"},
				SortOrder:         10,
				LegacyKeys:        []string{LegacyIntentReport},
			},
			{
				Key:               IntentKeySceneMeetingSwitch,
				HandlerName:       "切换到会议模式",
				DefaultLabel:      "会议模式",
				Description:       "把桌面端切换到会议模式。",
				DefaultUtterances: []string{"会议模式", "切到会议模式", "进入会议模式", "开始会议纪要"},
				SortOrder:         20,
				LegacyKeys:        []string{LegacyIntentMeeting},
			},
		},
	},
}

func BuiltinGroups() []BuiltinGroupSpec {
	items := make([]BuiltinGroupSpec, len(builtinGroupSpecs))
	for i, group := range builtinGroupSpecs {
		items[i] = group
		items[i].LegacyKeys = append([]string(nil), group.LegacyKeys...)
		items[i].Intents = make([]BuiltinIntentSpec, len(group.Intents))
		for j, intent := range group.Intents {
			items[i].Intents[j] = intent
			items[i].Intents[j].LegacyKeys = append([]string(nil), intent.LegacyKeys...)
			items[i].Intents[j].DefaultUtterances = append([]string(nil), intent.DefaultUtterances...)
		}
	}
	return items
}

func NormalizeGroupKey(key string) (string, bool) {
	group, ok := BuiltinGroupByKey(key)
	if !ok {
		return "", false
	}
	return group.Key, true
}

func BuiltinGroupByKey(key string) (BuiltinGroupSpec, bool) {
	trimmed := strings.TrimSpace(key)
	for _, group := range builtinGroupSpecs {
		if trimmed == group.Key {
			return group, true
		}
		for _, legacy := range group.LegacyKeys {
			if trimmed == legacy {
				return group, true
			}
		}
	}
	return BuiltinGroupSpec{}, false
}

func NormalizeIntentKey(groupKey string, intentKey string) (string, bool) {
	intent, ok := BuiltinIntentByKey(groupKey, intentKey)
	if !ok {
		return "", false
	}
	return intent.Key, true
}

func BuiltinIntentByKey(groupKey string, intentKey string) (BuiltinIntentSpec, bool) {
	group, ok := BuiltinGroupByKey(groupKey)
	if !ok {
		return BuiltinIntentSpec{}, false
	}
	trimmed := strings.TrimSpace(intentKey)
	for _, intent := range group.Intents {
		if trimmed == intent.Key {
			return intent, true
		}
		for _, legacy := range intent.LegacyKeys {
			if trimmed == legacy {
				return intent, true
			}
		}
	}
	return BuiltinIntentSpec{}, false
}
