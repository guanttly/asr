package voiceintent

import (
	"fmt"
	"sort"

	voicecommand "github.com/lgt/asr/internal/domain/voicecommand"
)

type Command struct {
	EntryID     uint64
	GroupKey    string
	GroupName   string
	Intent      string
	Label       string
	Utterances  []string
	IsBaseGroup bool
}

type Catalog struct {
	Commands  []Command
	DictIDs   []uint64
	GroupKeys []string
}

func BuildCatalog(dicts []*voicecommand.Dict, entries []voicecommand.Entry, selectedDictIDs []uint64, includeBase bool) (Catalog, error) {
	selected := map[uint64]struct{}{}
	for _, id := range selectedDictIDs {
		if id > 0 {
			selected[id] = struct{}{}
		}
	}
	dictByID := map[uint64]*voicecommand.Dict{}
	orderedDictIDs := make([]uint64, 0, len(dicts))
	for _, dict := range dicts {
		if dict == nil {
			continue
		}
		if !includeBase && dict.IsBase {
			if _, ok := selected[dict.ID]; !ok {
				continue
			}
		}
		if !dict.IsBase {
			if _, ok := selected[dict.ID]; !ok {
				continue
			}
		}
		dictByID[dict.ID] = dict
		orderedDictIDs = append(orderedDictIDs, dict.ID)
	}
	if len(dictByID) == 0 {
		return Catalog{}, fmt.Errorf("voice_intent 节点没有可用的控制指令组")
	}

	commands := make([]Command, 0, len(entries))
	groupSeen := map[string]struct{}{}
	groups := make([]string, 0, len(dictByID))
	for _, entry := range entries {
		dict := dictByID[entry.DictID]
		if dict == nil || !entry.Enabled {
			continue
		}
		commands = append(commands, Command{
			EntryID:     entry.ID,
			GroupKey:    dict.GroupKey,
			GroupName:   dict.Name,
			Intent:      entry.Intent,
			Label:       entry.Label,
			Utterances:  append([]string(nil), entry.Utterances...),
			IsBaseGroup: dict.IsBase,
		})
		if _, ok := groupSeen[dict.GroupKey]; !ok {
			groupSeen[dict.GroupKey] = struct{}{}
			groups = append(groups, dict.GroupKey)
		}
	}
	if len(commands) == 0 {
		return Catalog{}, fmt.Errorf("voice_intent 节点的控制指令组下没有启用指令")
	}
	sort.SliceStable(commands, func(i, j int) bool {
		if commands[i].GroupKey == commands[j].GroupKey {
			if commands[i].Intent == commands[j].Intent {
				return commands[i].EntryID < commands[j].EntryID
			}
			return commands[i].Intent < commands[j].Intent
		}
		return commands[i].GroupKey < commands[j].GroupKey
	})
	sort.SliceStable(orderedDictIDs, func(i, j int) bool { return orderedDictIDs[i] < orderedDictIDs[j] })
	sort.Strings(groups)
	return Catalog{Commands: commands, DictIDs: orderedDictIDs, GroupKeys: groups}, nil
}
