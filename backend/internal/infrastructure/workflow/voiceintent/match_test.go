package voiceintent

import "testing"

func TestMatchCatalogMatchesCommandWithoutLLM(t *testing.T) {
	catalog := Catalog{
		Commands: []Command{
			{
				EntryID:    2,
				GroupKey:   "scene_mode",
				GroupName:  "场景模式",
				Intent:     "scene_meeting_switch",
				Label:      "会议模式",
				Utterances: []string{"会议模式", "切换到会议模式", "切到会议模式"},
			},
		},
		DictIDs:   []uint64{1},
		GroupKeys: []string{"scene_mode"},
	}

	result := MatchCatalog("请帮我切换到会议模式", catalog)
	if !result.Matched {
		t.Fatal("expected command to match")
	}
	if result.Intent != "scene_meeting_switch" {
		t.Fatalf("expected intent scene_meeting_switch, got %q", result.Intent)
	}
	if result.GroupKey != "scene_mode" {
		t.Fatalf("expected group key scene_mode, got %q", result.GroupKey)
	}
	if result.CommandID != 2 {
		t.Fatalf("expected command id 2, got %d", result.CommandID)
	}
	if result.RawOutput != "切换到会议模式" {
		t.Fatalf("expected raw output to preserve utterance, got %q", result.RawOutput)
	}
}

func TestMatchCatalogReturnsUnmatchedForUnknownCommand(t *testing.T) {
	catalog := Catalog{
		Commands: []Command{{
			EntryID:    1,
			GroupKey:   "scene_mode",
			GroupName:  "场景模式",
			Intent:     "scene_report_switch",
			Label:      "报告模式",
			Utterances: []string{"报告模式"},
		}},
	}

	result := MatchCatalog("帮我打开录音", catalog)
	if result.Matched {
		t.Fatalf("expected unmatched result, got %+v", result)
	}
	if result.Reason != "未命中有效指令" {
		t.Fatalf("expected unmatched reason, got %q", result.Reason)
	}
}
