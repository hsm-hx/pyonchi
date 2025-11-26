package handlers

import (
	"math"
	"strconv"

	"github.com/bwmarrin/discordgo"
)

type SplitState struct {
	Step   int // 1 = 金額待ち, 2 = 人数待ち
	Total  int // 合計金額
	People int // 人数
}

var splitConversationState = map[string]*SplitState{}

// 🔍 会話中かどうかを判定
func IsInSplitConversation(key string) bool {
	_, exists := splitConversationState[key]
	return exists
}

// 🧠 会話の続きメッセージを処理
func SplitHandleOngoing(s *discordgo.Session, m *discordgo.MessageCreate) {
	key := m.ChannelID + "|" + m.Author.ID
	state, ok := splitConversationState[key]
	if !ok {
		state = &SplitState{
			Step: 1,
		}
		splitConversationState[key] = state
	}

	switch state.Step {

	// --- Step 1: 合計金額を受け取る ---
	case 1:
		state.Step = 2
		s.ChannelMessageSend(m.ChannelID, "全部で何円払ったの？")
	case 2:
		total, err := strconv.Atoi(m.Content)
		if err != nil || total <= 0 {
			s.ChannelMessageSend(m.ChannelID, "⚠️ 合計金額は整数にしてよね")
			return
		}
		state.Total = total
		state.Step = 3
		s.ChannelMessageSend(m.ChannelID, "何人でわりかんするの？")

	// --- Step 2: 人数入力 ---
	case 3:
		people, err := strconv.Atoi(m.Content)
		if err != nil || people <= 0 {
			s.ChannelMessageSend(m.ChannelID, "⚠️ 人数が変じゃない？")
			return
		}
		state.People = people

		// 計算
		per := int(math.Ceil(float64(state.Total) / float64(state.People)))

		// 結果表示
		msg := "💴 " +
			strconv.Itoa(state.Total) + "円を" +
			strconv.Itoa(state.People) + "人でわりかんしたら" +
			"**" + strconv.Itoa(per) + "円** じゃない？"

		s.ChannelMessageSend(m.ChannelID, msg)

		// 会話終了（削除）
		delete(splitConversationState, key)
	}
}
