package handlers

import (
	"github.com/bwmarrin/discordgo"

	"pyonchi/internal/convo"
)

func RouteOngoingConversations(s *discordgo.Session, m *discordgo.MessageCreate) bool {

	key := convo.Key(m.ChannelID, m.Author.ID)

	// 割り勘ボットの ongoing state?
	if IsInSplitConversation(key) {
		SplitHandleOngoing(s, m)
		return true
	}

	// 外食ボットの ongoing state?
	if IsInExpenseConversation(key) {
		ExpenseManualHandleOngoing(s, m)
		return true
	}

	// レシート画像ボットの ongoing state?
	if IsInExpenseReceiptConversation(key) {
		ExpenseReceiptHandleOngoing(s, m)
		return true
	}

	// どちらでもない
	return false
}
