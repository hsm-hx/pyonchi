package gemini

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type Client struct {
	apiKey string
	http   *http.Client
}

type ReceiptDataResponse struct {
	Merchant string `json:"merchant"`
	Items    []Item `json:"items"`
	Date     string `json:"date"`
}

type Item struct {
	Name     string `json:"name"`
	Category string `json:"category"`
	Amount   int    `json:"amount"`
	Date     string `json:"date"`
}

func NewClient(apiKey string) *Client {
	return &Client{
		apiKey: apiKey,
		http:   &http.Client{Timeout: 10 * time.Second},
	}
}

const geminiReceiptPrompt = `
あなたは画像解析の専門家です。次の画像に基づいて、レシートから以下の情報を抽出し、JSON 形式で返してください。
レシートに外税と記載のある場合、アイテム名の頭に * マークが記されている場合は 8%、記されていない場合は 10% の消費税を付加した税込価格を計算して返してください。
レシートに内税と記載のある場合、表示されている価格は税込価格です。そのままの価格を返してください。
- 店舗名(merchant): レシートに記載されている店舗の名前
- アイテム(items): 各商品の名前と価格のリスト
    - 名前(name): 文字列
	- 価格(amount): 数値
	- カテゴリ(category): アイテム名と店舗名をもとに、以下のカテゴリから最も適切なものを選んでください: ぜいたくごはん, いつもごはん, 日用品, 住居費, 旅行, その他
- 日付(date): レシートの日付 (YYYY-MM-DD 形式)

なお、カテゴリの判断は以下の基準に従ってください:
- ぜいたくごはん: カフェ、レストラン、スイーツ店での購入品。または、スーパーでのジュース・お菓子・アルコール類の購入品
- いつもごはん: スーパー、コンビニでの食料品購入品
- 日用品: トイレットペーパー、洗剤、シャンプーなどの生活必需品
- 住居費: 家賃、光熱費などの住居関連費用
- 旅行: ホテル代、交通費などの旅行関連費用
- その他: 上記に該当しないもの

例:
{
	"merchant": "スーパーABC",
	"items": [
		{"name": "牛乳", "amount": 200, "category": "いつもごはん"},
		{"name": "トイレットペーパー", "amount": 400, "category": "日用品"}
	],
	"date": "2024-06-15"
}
{
	"merchant": "カフェXYZ",
	"items": [
		{"name": "コーヒー", "amount": 300, "category": "ぜいたくごはん"},
		{"name": "サンドイッチ", "amount": 500, "category": "ぜいたくごはん"}
	],
	"date": "2024-06-16"
}

必ず上記のJSON形式で返してください。
`

func (c *Client) GetReceiptData(imagePath string) (*ReceiptDataResponse, error) {
	url := "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent"

	imageData, err := os.ReadFile(imagePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read image: %w", err)
	}

	base64Image := base64.StdEncoding.EncodeToString(imageData)

	// jpg, png, webp に対応
	mimeType := "image/jpeg"
	if strings.HasSuffix(strings.ToLower(imagePath), ".png") {
		mimeType = "image/png"
	} else if strings.HasSuffix(strings.ToLower(imagePath), ".webp") {
		mimeType = "image/webp"
	}

	requestBody := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": []map[string]interface{}{
					{
						"text": geminiReceiptPrompt,
					},
					{
						"inline_data": map[string]string{
							"mime_type": mimeType,
							"data":      base64Image,
						},
					},
				},
			},
		},
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		fmt.Println("Failed to marshal request body:", err)
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Println("Failed to create request:", err)
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-goog-api-key", c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		fmt.Println("Failed to send request:", err)
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Println("API request failed with status", resp.StatusCode, "body:", string(body))
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	bodystr := new(bytes.Buffer)
	bodystr.ReadFrom(resp.Body)

	// 以下の JSON をパースして text フィールドを抽出
	// {
	//   "candidates": [
	//     {
	//       "content": {
	//         "parts": [
	//           {
	//             "text": "```json\n{\n\t\"merchant\": \"root C\",\n\t\"items\": [\n\t\t{\"name\": \"ブラジル セルタオ/HOT\", \"amount\": 500, \"category\": \"ぜいたくごはん\"}\n\t],\n\t\"date\": \"2025-11-29\"\n}\n```"
	//           }
	//         ],
	//         "role": "model"
	//       },
	//       "finishReason": "STOP",
	//       "avgLogprobs": -2.4589365348219872e-05
	//     }
	//   ],
	//   "usageMetadata": {
	//     "promptTokenCount": 2140,
	//     "candidatesTokenCount": 75,
	//     "totalTokenCount": 2215,
	//     "promptTokensDetails": [
	//       {
	//         "modality": "IMAGE",
	//         "tokenCount": 1806
	//       },
	//       {
	//         "modality": "TEXT",
	//         "tokenCount": 334
	//       }
	//     ],
	//     "candidatesTokensDetails": [
	//       {
	//         "modality": "TEXT",
	//         "tokenCount": 75
	//       }
	//     ]
	//   },
	//   "modelVersion": "gemini-2.0-flash",
	//   "responseId": "SAUxaeLlN-iB0-kP5cqJkAE"
	// }
	type apiResponse struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}

	var apiResp apiResponse
	if err := json.NewDecoder(bytes.NewReader(bodystr.Bytes())).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to decode API response: %w", err)
	}

	if len(apiResp.Candidates) == 0 || len(apiResp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("no content in API response")
	}

	textResponse := apiResp.Candidates[0].Content.Parts[0].Text
	// テキストの前後に ```json と ``` が含まれている場合は取り除く
	textResponse = strings.TrimSpace(textResponse)
	if strings.HasPrefix(textResponse, "```json") && strings.HasSuffix(textResponse, "```") {
		textResponse = strings.TrimPrefix(textResponse, "```json")
		textResponse = strings.TrimSuffix(textResponse, "```")
		textResponse = strings.TrimSpace(textResponse)
	}

	fmt.Println("Full response text:", textResponse)

	var result ReceiptDataResponse
	if err := json.NewDecoder(strings.NewReader(textResponse)).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}
