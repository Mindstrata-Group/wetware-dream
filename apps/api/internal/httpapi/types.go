package httpapi

import "time"

type ModeOption struct {
	ID             int64       `json:"id"`
	Name           string      `json:"name"`
	WelcomeMessage string      `json:"welcomeMessage,omitempty"`
	Quota          *DailyQuota `json:"quota,omitempty"`
}

type DailyQuota struct {
	Limit     *int64 `json:"limit,omitempty"`
	Used      int64  `json:"used"`
	Remaining *int64 `json:"remaining,omitempty"`
}

type ChatMessage struct {
	ID        int64     `json:"id"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"createdAt"`
	ModeID    *int64    `json:"modeId,omitempty"`
	ModeName  string    `json:"modeName,omitempty"`
}

type DialogSummary struct {
	DialogID      int64         `json:"dialogId"`
	UserID        int64         `json:"userId"`
	ModeID        int64         `json:"modeId"`
	ModeName      string        `json:"modeName"`
	CreatedAt     time.Time     `json:"createdAt"`
	MessageCount  int           `json:"messageCount"`
	LastMessageAt *time.Time    `json:"lastMessageAt,omitempty"`
	Messages      []ChatMessage `json:"messages,omitempty"`
}

type StartResponse struct {
	OK                  bool          `json:"ok"`
	UserID              int64         `json:"userId"`
	Role                string        `json:"role,omitempty"`
	Email               string        `json:"email,omitempty"`
	CurrentModeID       *int64        `json:"currentModeId,omitempty"`
	CurrentDialog       *int64        `json:"currentDialogId,omitempty"`
	ChatMessageMaxChars int           `json:"chatMessageMaxChars"`
	Modes               []ModeOption  `json:"modes"`
	Quota               *DailyQuota   `json:"quota,omitempty"`
	Messages            []ChatMessage `json:"messages,omitempty"`
	MaxLinked           bool          `json:"maxLinked"`
	MaxBotLink          string        `json:"maxBotLink,omitempty"`
}

type SelectModeRequest struct {
	ModeID    int64 `json:"modeId"`
	NewDialog bool  `json:"newDialog"`
}

type SelectModeResponse struct {
	OK             bool         `json:"ok"`
	UserID         int64        `json:"userId"`
	DialogID       int64        `json:"dialogId"`
	ModeID         int64        `json:"modeId"`
	ModeName       string       `json:"modeName"`
	WelcomeMessage string       `json:"welcomeMessage"`
	Quota          *DailyQuota  `json:"quota,omitempty"`
	SwitchTrace    *ChatMessage `json:"switchTrace,omitempty"`
}

type SendMessageRequest struct {
	DialogID         int64   `json:"dialogId"`
	Text             string  `json:"text"`
	ResponseMode     string  `json:"responseMode"`
	KnowledgeModeIDs []int64 `json:"knowledgeModeIds"`
	AttachmentIDs    []int64 `json:"attachmentIds,omitempty"`
}

type SendMessageResponse struct {
	OK           bool         `json:"ok"`
	User         ChatMessage  `json:"user"`
	SwitchTrace  *ChatMessage `json:"switchTrace,omitempty"`
	Assistant    ChatMessage  `json:"assistant"`
	UsedLive     bool         `json:"usedLive"`
	ModeID       int64        `json:"modeId"`
	ModeName     string       `json:"modeName"`
	ModeSwitched bool         `json:"modeSwitched,omitempty"`
	Quota        *DailyQuota  `json:"quota,omitempty"`
	Attachments  []int64      `json:"attachmentIds,omitempty"`
}

type CompleteRequest struct {
	DialogID     int64  `json:"dialogId"`
	ResponseMode string `json:"responseMode"`
}

type CompleteResponse struct {
	OK       bool        `json:"ok"`
	Summary  ChatMessage `json:"summary"`
	UsedLive bool        `json:"usedLive"`
	Quota    *DailyQuota `json:"quota,omitempty"`
}
