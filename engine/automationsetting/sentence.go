package automationsetting

// SentencePart is one piece of a sentence the designer shows, such as "Say
// in chat" followed by the message setting. Exactly one field is set.
type SentencePart struct {
	// Words are the sentence's own fixed text.
	Words string `json:"words,omitempty"`
	// SettingID places a setting's value in the sentence, where the user
	// clicks to edit it.
	SettingID string `json:"settingId,omitempty"`
	// UserText is text the user wrote, such as a choice in "If they type
	// !a1". The designer sets it apart from the fixed words, but it isn't
	// editable there.
	UserText string `json:"userText,omitempty"`
}

// Sentence is how the designer shows a trigger, a step or an outcome: as
// words with its settings in place, so an automation reads like what it
// does. See docs/design/automation-engine.md#what-definitions-tell-the-designer.
type Sentence []SentencePart

// Words returns a sentence part of fixed text.
func Words(text string) SentencePart {
	return SentencePart{Words: text}
}

// Setting returns a sentence part that places a setting's value.
func Setting(settingID string) SentencePart {
	return SentencePart{SettingID: settingID}
}

// UserText returns a sentence part of text the user wrote.
func UserText(text string) SentencePart {
	return SentencePart{UserText: text}
}

// SettingIDs returns the IDs of the settings the sentence places, in order.
func (sentence Sentence) SettingIDs() []string {
	var settingIDs []string
	for _, part := range sentence {
		if part.SettingID != "" {
			settingIDs = append(settingIDs, part.SettingID)
		}
	}
	return settingIDs
}
