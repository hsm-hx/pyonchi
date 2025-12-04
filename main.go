package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"

	"pyonchi/handlers"
	"pyonchi/notion"
)

func main() {
	err := godotenv.Load(".env")
	if err != nil {
		log.Fatalf("Error loading env target")
	}

	geminiToken := os.Getenv("GEMINI_API_KEY")
	if geminiToken == "" {
		log.Println("GEMINI_API_KEY を設定してください")
		return
	}

	discordToken := os.Getenv("DISCORD_TOKEN")
	if discordToken == "" {
		log.Fatal("DISCORD_TOKEN を設定してください")
		return
	}

	// ぴょんちーが反応できるチャンネルの ID (カンマ区切り)
	allowedChannelIDsStr := os.Getenv("ALLOWED_CHANNEL_IDS")
	var allowedChannelIDs []string
	if allowedChannelIDsStr != "" {
		allowedChannelIDs = strings.Split(allowedChannelIDsStr, ",")
		// トリミング
		for i := range allowedChannelIDs {
			allowedChannelIDs[i] = strings.TrimSpace(allowedChannelIDs[i])
		}
	}

	notionKey := os.Getenv("NOTION_API_KEY")
	notionDB := os.Getenv("NOTION_EXPENSES_DB_ID")
	if notionKey == "" || notionDB == "" {
		log.Println("NOTION_API_KEY または NOTION_EXPENSES_DB_ID が未設定です")
		return
	}

	// Notion クライアントを作成してハンドラにセット
	notionClient := notion.NewClient(notionKey, notionDB)
	handlers.SetNotionClient(notionClient)

	// Discord Bot
	dg, err := discordgo.New("Bot " + discordToken)
	if err != nil {
		log.Fatalf("discordgo.New error: %v", err)
		return
	}

	// 必要な Intents
	dg.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsDirectMessages | discordgo.IntentsMessageContent

	// ハンドラ登録
	dg.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		// filter bots
		if m.Author == nil || m.Author.Bot {
			return
		}

		// チャンネル制限
		if len(allowedChannelIDs) > 0 && !slices.Contains(allowedChannelIDs, m.ChannelID) {
			return
		}

		content := m.Content

		// 割り勘トリガー
		if isSplitTrigger(content) {
			handlers.SplitHandleOngoing(s, m)
			return
		}

		// 家計簿記録トリガー
		if isExpenseManualTrigger(content) {
			handlers.ExpenseManualHandleOngoing(s, m)
			return
		}

		// レシート画像トリガー
		if isExpenseReceiptTrigger(m) {
			handlers.ExpenseReceiptHandleOngoing(s, m)
			return
		}

		// 進行中の会話があれば各ハンドラが処理する
		handlers.RouteOngoingConversations(s, m)
	})

	dg.AddHandler(handlers.WalletInteractionHandler)
	dg.AddHandler(handlers.CategoryInteractionHandler)

	if err := dg.Open(); err != nil {
		log.Fatalf("Discord Open error: %v", err)
		return
	}
	defer dg.Close()

	log.Println("Discord bot connected")

	// HTTP サーバ（Cloud Run 用）
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "OK")
	})
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// サーバを別 goroutine で起動
	go func() {
		log.Printf("HTTP server start on :%s", port)
		if err := http.ListenAndServe(":"+port, nil); err != nil {
			log.Fatalf("http.ListenAndServe: %v", err)
		}
	}()

	// 終了シグナルを待つ
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	log.Println("Shutting down")
	dg.Close()
	time.Sleep(1 * time.Second)
}

func isSplitTrigger(content string) bool {
	c := normalize(content)
	return c == "ぴょんちー　割り勘" || c == "ぴょんちー 割り勘" || c == "ぴょんちー割り勘"
}

func isExpenseManualTrigger(content string) bool {
	c := normalize(content)
	return c == "ぴょんちー 家計簿つけて" || c == "ぴょんちー家計簿つけて" || c == "ぴょんちー　家計簿つけて"
}

func isExpenseReceiptTrigger(m *discordgo.MessageCreate) bool {
	// メッセージに画像添付があるか
	if len(m.Attachments) == 0 {
		return false
	}
	c := normalize(m.Content)
	return c == "ぴょんちー レシート" || c == "ぴょんちーレシート" || c == "ぴょんちー　レシート"
}

func normalize(s string) string {
	// トリミングなど最低限の正規化
	return strings.TrimSpace(s)
}
