package handlers

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/bwmarrin/discordgo"

	"pyonchi/gemini"
	"pyonchi/notion"
)

type ExpenceState struct {
	Step     int
	Title    string
	Category string
	Amount   int
	People   int
	Wallet   string
}
type ReceiptData struct {
	Merchant string
	Items    []gemini.Item
	Date     string
}

var expenseConversationState = map[string]*ExpenceState{}
var expenseReceiptConversationState = map[string]*ReceiptData{}

var client *notion.Client

func SetNotionClient(cli *notion.Client) {
	client = cli
}

// 会話中かどうかを判定
func IsInExpenseConversation(key string) bool {
	_, exists := expenseConversationState[key]
	return exists
}

// レシート画像から家計簿記録を行う会話中かどうかを判定
func IsInExpenseReceiptConversation(key string) bool {
	_, exists := expenseReceiptConversationState[key]
	return exists
}

const (
	StepInputTitle                 = 100
	StepGetTitleAndRequestCategory = 101
	StepInputCategory              = 200
	StepGetCategory                = 201
	StepInputAmountPerPerson       = 300
	StepGetAmountPerPerson         = 301
	StepInputPeople                = 400
	StepGetPeople                  = 401
	StepSelectWallet               = 500
	StepGetReceiptData             = 600
)

func ExpenseManualHandleOngoing(s *discordgo.Session, m *discordgo.MessageCreate) {
	key := m.ChannelID + "|" + m.Author.ID
	state, ok := expenseConversationState[key]
	if !ok {
		state = &ExpenceState{
			Step: StepInputTitle,
		}
		expenseConversationState[key] = state
	}

	switch state.Step {
	case StepInputTitle:
		RequestInputTitle(s, m)
		state.Step = StepGetTitleAndRequestCategory
		return
	case StepGetTitleAndRequestCategory:
		title := GetInputTitle(m)
		if title == "" {
			s.ChannelMessageSend(m.ChannelID, "⚠️ タイトル教えてよ")
			return
		}
		state.Title = title

		RequestInputCategory(s, m)
		state.Step = StepInputAmountPerPerson
		return
	case StepInputAmountPerPerson:
		amt, err := strconv.Atoi(m.Content)
		if err != nil || amt <= 0 {
			s.ChannelMessageSend(m.ChannelID, "⚠️ 金額は整数にしてよね")
			return
		}
		state.Amount = amt

		if state.Category == "ぜいたくごはん" {
			s.ChannelMessageSend(m.ChannelID, "何人分支払ったの？")
			state.Step = StepGetPeople
		} else {
			state.People = 1
			RequestInputWallet(s, m)
		}
		return
	case StepGetPeople:
		people, err := GetInputPeople(m)
		if err != nil || people <= 0 {
			s.ChannelMessageSend(m.ChannelID, "⚠️ 人数が変じゃない？")
			return
		}
		state.People = people
		RequestInputWallet(s, m)
		return
	default:
		s.ChannelMessageSend(m.ChannelID, "⚠️ なんか変な状態になっちゃった")
		delete(expenseConversationState, key)
		return
	}
}

// レシート画像から家計簿記録を行うハンドラ
func ExpenseReceiptHandleOngoing(s *discordgo.Session, m *discordgo.MessageCreate, geminiClient *gemini.Client) {
	key := m.ChannelID + "|" + m.Author.ID
	_, ok := expenseReceiptConversationState[key]
	if !ok {
		expenseReceiptConversationState[key] = &ReceiptData{
			Merchant: "",
			Items:    nil,
			Date:     "",
		}
	}

	// 受け取ったレシート画像を処理してデータを取得
	// 画像添付の最初のものを使う
	imageURL := m.Attachments[0].URL

	// 画像を一時ファイルにダウンロード
	imagePath, err := downloadImageToTempFile(imageURL)
	if err != nil {
		s.ChannelMessageSend(m.ChannelID, "⚠️ 画像のダウンロードに失敗したよ")
		delete(expenseReceiptConversationState, key)
		return
	}
	defer os.Remove(imagePath)

	// Gemini API を使ってレシートデータを取得
	receiptData, err := geminiClient.GetReceiptData(imagePath)
	if err != nil {
		s.ChannelMessageSend(m.ChannelID, "⚠️ レシートの解析に失敗したよ")
		delete(expenseReceiptConversationState, key)
		return
	}
	defer os.Remove(imagePath)

	// 解析結果をもとに map に保存
	expenseReceiptConversationState[key] = &ReceiptData{
		Merchant: receiptData.Merchant,
		Items:    receiptData.Items,
		Date:     receiptData.Date,
	}

	RequestInputWalletForReceipt(s, m)
}

func RequestInputTitle(s *discordgo.Session, m *discordgo.MessageCreate) {
	s.ChannelMessageSend(m.ChannelID, "タイトル教えて")
}

func RequestInputCategory(s *discordgo.Session, m *discordgo.MessageCreate) {
	s.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
		Content: "どんな出費？",
		Components: []discordgo.MessageComponent{
			discordgo.ActionsRow{
				Components: []discordgo.MessageComponent{
					discordgo.SelectMenu{
						MenuType: discordgo.StringSelectMenu,
						CustomID: "expense_category_select",
						Options: []discordgo.SelectMenuOption{
							{
								Label: "いつもごはん",
								Value: "いつもごはん",
							},
							{
								Label: "ぜいたくごはん",
								Value: "ぜいたくごはん",
							},
							{
								Label: "消耗品費",
								Value: "消耗品費",
							},
							{
								Label: "その他",
								Value: "その他",
							},
						},
						Placeholder: "支出カテゴリを選んでよね",
					},
				},
			},
		},
	})
}

func RequestInputWallet(s *discordgo.Session, m *discordgo.MessageCreate) {
	s.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
		Content: "どの財布から払ったの？",
		Components: []discordgo.MessageComponent{
			discordgo.ActionsRow{
				Components: []discordgo.MessageComponent{
					discordgo.SelectMenu{
						MenuType: discordgo.StringSelectMenu,
						CustomID: "expense_wallet_select",
						Options: []discordgo.SelectMenuOption{
							{
								Label: "おひ財布",
								Value: "おひ財布",
							},
							{
								Label: "ぽよ財布",
								Value: "ぽよ財布",
							},
							{
								Label: "B/43",
								Value: "B/43",
							},
						},
						Placeholder: "支払い財布を選んでよね",
					},
				},
			},
		},
	})
}

func RequestInputWalletForReceipt(s *discordgo.Session, m *discordgo.MessageCreate) {
	s.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
		Content: "どの財布から払ったの？",
		Components: []discordgo.MessageComponent{
			discordgo.ActionsRow{
				Components: []discordgo.MessageComponent{
					discordgo.SelectMenu{
						MenuType: discordgo.StringSelectMenu,
						CustomID: "expense_receipt_wallet_select",
						Options: []discordgo.SelectMenuOption{
							{
								Label: "おひ財布",
								Value: "おひ財布",
							},
							{
								Label: "ぽよ財布",
								Value: "ぽよ財布",
							},
							{
								Label: "B/43",
								Value: "B/43",
							},
						},
						Placeholder: "支払い財布を選んでよね",
					},
				},
			},
		},
	})
}

func GetInputTitle(m *discordgo.MessageCreate) string {
	title := m.Content
	return title
}

func GetInputCategory(m *discordgo.MessageCreate) string {
	category := m.Content
	return category
}

func GetInputPeople(m *discordgo.MessageCreate) (int, error) {
	ppl, err := strconv.Atoi(m.Content)
	if err != nil {
		return 0, err
	}
	return ppl, nil
}

// --- 財布を選択するプルダウンのインタラクションをハンドリングする関数 ---
func WalletInteractionHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.MessageComponentData().CustomID == "expense_wallet_select" {
		// ここで選択された財布の値を取得
		wallet := i.MessageComponentData().Values[0]

		fmt.Println(i.ChannelID, i.Member.User.ID)
		fmt.Println(expenseConversationState)
		state := expenseConversationState[i.ChannelID+"|"+i.Member.User.ID]

		total := state.Amount * state.People
		now := time.Now()

		// Notion に書き込み
		err := client.CreateExpenseRecord(state.Title, state.Category, state.Amount, state.People, wallet, now)

		if err != nil {
			s.ChannelMessageSend(i.ChannelID, "⚠️ Notion に記録できなかった")
			delete(expenseConversationState, i.ChannelID+"|"+i.Member.User.ID)
			return
		}

		budgets := getBudgetText(s, i, state.Category)

		// 結果を Discord に送信
		msg := "🍽 家計簿つけたよ\n" +
			"タイトル: " + state.Title + "\n" +
			"一人あたり: " + strconv.Itoa(state.Amount) + "円\n" +
			"人数: " + strconv.Itoa(state.People) + "人\n" +
			"合計: " + strconv.Itoa(total) + "円\n" +
			"財布: " + wallet + "\n\n" +
			budgets

		resp := &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Flags:   discordgo.MessageFlagsHasThread,
				Content: msg,
			},
		}
		if err := s.InteractionRespond(i.Interaction, resp); err != nil {
			log.Fatalln(err)
		}

		// 🔚 会話終了
		delete(expenseConversationState, i.ChannelID+"|"+i.Member.User.ID)
	}
}

// --- 財布を選択するプルダウンのインタラクションをハンドリングする関数 ---
func ReceiptWalletInteractionHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	fmt.Println("ReceiptWalletInteractionHandler called")
	if i.MessageComponentData().CustomID == "expense_receipt_wallet_select" {
		fmt.Println("Processing receipt wallet selection")
		// ここで選択された財布の値を取得
		wallet := i.MessageComponentData().Values[0]

		state := expenseReceiptConversationState[i.ChannelID+"|"+i.Member.User.ID]
		fmt.Println(i.ChannelID, i.Member.User.ID)
		fmt.Println(expenseReceiptConversationState)

		// レシートの各アイテムをカテゴリごとに集計
		var categoryItems = make(map[string][]gemini.Item)
		for _, item := range state.Items {
			categoryItems[item.Category] = append(categoryItems[item.Category], item)
		}

		// カテゴリごとに記録単位を作成
		var categoryTotals = make(map[string]gemini.Item)
		for category, items := range categoryItems {
			var totalAmount int
			for _, item := range items {
				totalAmount += item.Amount
			}
			categoryTotals[category] = gemini.Item{
				Name:     state.Merchant + " - " + category,
				Amount:   totalAmount,
				Category: category,
			}
		}

		// 各カテゴリごとに Notion に記録
		for _, item := range categoryTotals {
			title := state.Merchant + " - " + item.Name
			amount := int(item.Amount)
			people := 1
			category := item.Category

			dateTime, err := time.Parse("2006-01-02", state.Date)
			if err != nil {
				s.ChannelMessageSend(i.ChannelID, "⚠️ 日付の解析に失敗したよ")
				delete(expenseReceiptConversationState, i.ChannelID+"|"+i.Member.User.ID)
				return
			}

			// Notion に書き込み
			err = client.CreateExpenseRecord(title, category, amount, people, wallet, dateTime)

			if err != nil {
				s.ChannelMessageSend(i.ChannelID, "⚠️ Notion に記録できなかった")
				delete(expenseReceiptConversationState, i.ChannelID+"|"+i.Member.User.ID)
				return
			}

			budgets := getBudgetText(s, i, category)

			// 結果を Discord に送信
			msg := "🍽 家計簿つけたよ\n" +
				"タイトル: " + title + "\n" +
				"一人あたり: " + strconv.Itoa(amount) + "円\n" +
				"人数: " + strconv.Itoa(people) + "人\n" +
				"合計: " + strconv.Itoa(amount*people) + "円\n" +
				"財布: " + wallet + "\n\n" +
				budgets

			resp := &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Flags:   discordgo.MessageFlagsHasThread,
					Content: msg,
				},
			}
			if err := s.InteractionRespond(i.Interaction, resp); err != nil {
				log.Fatalln(err)
			}
		}

		// 🔚 会話終了
		delete(expenseConversationState, i.ChannelID+"|"+i.Member.User.ID)
	}
}

func CategoryInteractionHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.MessageComponentData().CustomID == "expense_category_select" {
		// ここで選択されたカテゴリの値を取得
		category := i.MessageComponentData().Values[0]

		fmt.Println(i.ChannelID, i.Member.User.ID)
		fmt.Println(expenseConversationState)
		state := expenseConversationState[i.ChannelID+"|"+i.Member.User.ID]

		// カテゴリ保存して次のステップへ
		state.Category = category
		state.Step = StepInputAmountPerPerson

		var msg string
		if category == "ぜいたくごはん" {
			msg = "一人あたりの金額はいくら？"
		} else {
			msg = "金額はいくら？"
		}
		resp := &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: msg,
			},
		}
		if err := s.InteractionRespond(i.Interaction, resp); err != nil {
			log.Fatalln(err)
		}
	}
}

func getBudgetText(s *discordgo.Session, i *discordgo.InteractionCreate, category string) string {
	var monthTotal int
	var err error

	// 今月の外食合計を取得
	monthTotal, err = client.GetMonthlyExpenseTotal(category)
	if err != nil {
		s.ChannelMessageSend(i.ChannelID, "⚠️ 今月の"+category+"代が取得できなかったんだけど")
		delete(expenseConversationState, i.ChannelID+"|"+i.Member.User.ID)
		return ""
	}

	return "📊 今月の" + category + "合計は **" + strconv.Itoa(monthTotal) + "円** みたい"
}

func downloadImageToTempFile(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	tmpFile, err := os.CreateTemp("", "receipt_*.jpg")
	if err != nil {
		return "", err
	}
	defer tmpFile.Close()

	_, err = io.Copy(tmpFile, resp.Body)
	if err != nil {
		return "", err
	}

	return tmpFile.Name(), nil
}
