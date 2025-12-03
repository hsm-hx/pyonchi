package handlers

import (
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/bwmarrin/discordgo"

	"pyonchi/notion"
)

type ExpenceState struct {
	Step     int    // 1 = タイトル, 2 = カテゴリ, 3 = 一人あたり金額, 4 = 人数, 5 = 財布
	Title    string // タイトル
	Category string // カテゴリ
	Amount   int    // 一人あたりの金額
	People   int    // 人数
	Wallet   string // 財布
}

var expenseConversationState = map[string]*ExpenceState{}
var client *notion.Client

func SetNotionClient(cli *notion.Client) {
	client = cli
}

// 会話中かどうかを判定
func IsInExpenseConversation(key string) bool {
	_, exists := expenseConversationState[key]
	return exists
}

const (
	StepInputTitle           = 100
	StepGetTitle             = 101
	StepInputCategory        = 200
	StepGetCategory          = 201
	StepInputAmountPerPerson = 300
	StepGetAmountPerPerson   = 301
	StepInputPeople          = 400
	StepGetPeople            = 401
	StepSelectWallet         = 500
)

func ExpenseHandleOngoing(s *discordgo.Session, m *discordgo.MessageCreate) {
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
		state.Step = StepGetTitle
		return
	case StepGetTitle:
		title := GetInputTitle(m)
		if title == "" {
			s.ChannelMessageSend(m.ChannelID, "⚠️ タイトル教えてよ")
			return
		}
		state.Title = title
		state.Step = StepInputCategory
		return
	case StepInputCategory:
		RequestInputCategory(s, m)
		state.Step = StepGetCategory
		return
	case StepGetCategory:
		category := GetInputCategory(m)
		if category == "" {
			s.ChannelMessageSend(m.ChannelID, "⚠️ カテゴリ教えてよ")
			return
		}
		state.Category = category
		state.Step = StepInputAmountPerPerson
		return
	case StepInputAmountPerPerson:
		RequestInputAmountPerPerson(s, m, state.Category)
		state.Step = StepGetAmountPerPerson
		return
	case StepGetAmountPerPerson:
		amt, err := strconv.Atoi(m.Content)
		if err != nil || amt <= 0 {
			s.ChannelMessageSend(m.ChannelID, "⚠️ 金額は整数にしてよね")
			return
		}
		state.Amount = amt

		if state.Category == "ぜいたくごはん" {
			state.Step = StepInputPeople
		} else {
			state.People = 1
			state.Step = StepSelectWallet
		}
		return
	case StepInputPeople:
		s.ChannelMessageSend(m.ChannelID, "何人分支払ったの？")
		return
	case StepGetPeople:
		people, err := GetInputPeople(m)
		if err != nil || people <= 0 {
			s.ChannelMessageSend(m.ChannelID, "⚠️ 人数が変じゃない？")
			return
		}
		state.People = people
		state.Step = StepSelectWallet
		return
	case StepSelectWallet:
		RequestInputWallet(s, m)
		return
	default:
		s.ChannelMessageSend(m.ChannelID, "⚠️ なんか変な状態になっちゃった")
		delete(expenseConversationState, key)
		return
	}
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

func RequestInputAmountPerPerson(s *discordgo.Session, m *discordgo.MessageCreate, category string) {
	if category == "ぜいたくごはん" {
		s.ChannelMessageSend(m.ChannelID, "一人あたりの金額はいくら？")
	} else {
		s.ChannelMessageSend(m.ChannelID, "金額はいくら？")
	}
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

func CategoryInteractionHandler(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.MessageComponentData().CustomID == "expense_category_select" {
		// ここで選択されたカテゴリの値を取得
		category := i.MessageComponentData().Values[0]

		fmt.Println(i.ChannelID, i.Member.User.ID)
		fmt.Println(expenseConversationState)
		state := expenseConversationState[i.ChannelID+"|"+i.Member.User.ID]

		// カテゴリ保存して次のステップへ
		state.Category = category
		state.Step = 4

		resp := &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "一人あたりの金額はいくら？",
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
