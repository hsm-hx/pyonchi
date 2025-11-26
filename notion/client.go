package notion

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Client struct {
	apiKey string
	dbID   string
	http   *http.Client
}

type PageProperty struct {
	Title *[]struct {
		Text struct {
			Content string `json:"content"`
		} `json:"text"`
	} `json:"title,omitempty"`

	Number *int `json:"number,omitempty"`

	Select *struct {
		Name string `json:"name"`
	} `json:"select,omitempty"`

	Date *struct {
		Start string `json:"start"`
	} `json:"date,omitempty"`
}

type CreatePageRequest struct {
	Parent struct {
		DatabaseID string `json:"database_id"`
	} `json:"parent"`
	Properties map[string]PageProperty `json:"properties"`
}

type QueryResponse struct {
	Results []struct {
		Properties map[string]struct {
			Type    string `json:"type"`
			Number  *int   `json:"number,omitempty"`
			Formula *struct {
				Type   string `json:"type"`
				Number int    `json:"number"`
			} `json:"formula,omitempty"`
			Title *[]struct {
				Text struct {
					Content string `json:"content"`
				} `json:"text"`
			} `json:"title,omitempty"`
			Select *struct {
				Name string `json:"name"`
			} `json:"select,omitempty"`
			Date *struct {
				Start string `json:"start"`
			} `json:"date,omitempty"`
		} `json:"properties"`
	} `json:"results"`
}

func NewClient(apiKey, dbID string) *Client {
	return &Client{
		apiKey: apiKey,
		dbID:   dbID,
		http:   &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *Client) CreateExpenseRecord(title string, category string, amount int, people int, wallet string, date time.Time) error {
	reqBody := CreatePageRequest{}
	reqBody.Parent.DatabaseID = c.dbID
	reqBody.Properties = map[string]PageProperty{
		"費目": {
			Title: &[]struct {
				Text struct {
					Content string `json:"content"`
				} `json:"text"`
			}{
				{
					Text: struct {
						Content string `json:"content"`
					}{Content: title},
				},
			},
		},
		"一人あたりの支払額": {
			Number: &amount,
		},
		"支払人数": {
			Number: &people,
		},
		"カテゴリ": {
			Select: &struct {
				Name string `json:"name"`
			}{Name: category},
		},
		"財布": {
			Select: &struct {
				Name string `json:"name"`
			}{Name: wallet},
		},
		"支払日時": {
			Date: &struct {
				Start string `json:"start"`
			}{Start: date.Format("2006-01-02")},
		},
	}

	b, _ := json.Marshal(reqBody)
	request, err := http.NewRequest("POST", "https://api.notion.com/v1/pages", bytes.NewBuffer(b))
	if err != nil {
		return err
	}

	request.Header.Set("Authorization", "Bearer "+c.apiKey)
	request.Header.Set("Notion-Version", "2022-06-28")
	request.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body := new(bytes.Buffer)
		body.ReadFrom(resp.Body)
		fmt.Println(body.String())
		return fmt.Errorf("notion API error: %d", resp.StatusCode)
	}

	return nil
}

func (c *Client) GetMonthlyExpenseTotal(category string) (int, error) {
	now := time.Now()
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local)

	// Notion API のフィルター構造
	type queryRequest struct {
		Filter struct {
			And []interface{} `json:"and"`
		} `json:"filter"`
	}

	type dateFilter struct {
		Property string `json:"property"`
		Date     struct {
			After string `json:"after"`
		} `json:"date"`
	}

	type selectFilter struct {
		Property string `json:"property"`
		Select   struct {
			Equals string `json:"equals"`
		} `json:"select"`
	}

	q := queryRequest{}
	q.Filter.And = []interface{}{
		dateFilter{
			Property: "支払日時",
			Date: struct {
				After string `json:"after"`
			}{
				After: startOfMonth.Format("2006-01-02"),
			},
		},
		selectFilter{
			Property: "カテゴリ",
			Select: struct {
				Equals string `json:"equals"`
			}{
				Equals: category,
			},
		},
	}

	b, _ := json.Marshal(q)
	request, err := http.NewRequest("POST", fmt.Sprintf("https://api.notion.com/v1/databases/%s/query", c.dbID), bytes.NewBuffer(b))
	if err != nil {
		return 0, err
	}

	request.Header.Set("Authorization", "Bearer "+c.apiKey)
	request.Header.Set("Notion-Version", "2022-06-28")
	request.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(request)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body := new(bytes.Buffer)
	body.ReadFrom(resp.Body)

	fmt.Println("Response body:", body.String())

	var result QueryResponse
	if err := json.Unmarshal(body.Bytes(), &result); err != nil {
		fmt.Println("Error decoding Notion response:", err)
		return 0, err
	}

	var sum int
	for _, r := range result.Results {
		fmt.Println("Processing record:", r)
		if amt, ok := r.Properties["総支払額"]; ok {
			if amt.Formula != nil {
				fmt.Println("Found amount:", amt.Formula.Number)
				sum += amt.Formula.Number
			}
		}
	}

	return sum, nil
}
