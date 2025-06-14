package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/jomei/notionapi"

	"github.com/chaitin/panda-wiki/domain"
	"github.com/chaitin/panda-wiki/log"
)

// Block represents a Notion block
type ImageBlock struct {
	Object      string `json:"object"`
	ID          string `json:"id"`
	ParentID    string `json:"parent_id"`
	HasChildren bool   `json:"has_children"`
	Type        string `json:"type"`
	Image       Image  `json:"image"`
}

// Image represents an image block in Notion
type Image struct {
	Caption []interface{} `json:"caption"`
	Type    string        `json:"type"`
	File    File          `json:"file"`
}

// File represents the file details of an image block
type File struct {
	URL string `json:"url"`
}

type NotionClient struct {
	token  string
	client *notionapi.Client
	logger *log.Logger
}

func NewNotionClient(token string, logger *log.Logger) *NotionClient {
	return &NotionClient{
		token:  token,
		logger: logger.WithModule("usecase.NotionClient"),
		client: notionapi.NewClient(notionapi.Token(token)),
	}
}

// titleContain 表示按标题搜索含有titleContain的页面
func (c *NotionClient) GetList(ctx context.Context, titleContain string) ([]domain.PageInfo, error) {
	res, err := c.client.Search.Do(ctx, &notionapi.SearchRequest{
		Query: titleContain,
		Filter: notionapi.SearchFilter{
			Property: "object",
			Value:    "page",
		},
	})
	if err != nil {
		return nil, err
	}
	var result []domain.PageInfo
	for _, page := range res.Results {
		var id, title string
		switch page.GetObject().String() {
		case "page":
			page := page.(*notionapi.Page)
			id = page.ID.String()
			if titleProp, ok := page.Properties["title"].(*notionapi.TitleProperty); ok {

				if len(titleProp.Title) > 0 {
					title = titleProp.Title[0].PlainText
				}
			} else if titleProp, ok := page.Properties["Name"].(*notionapi.TitleProperty); ok {
				if len(titleProp.Title) > 0 {
					title = titleProp.Title[0].PlainText
				}
			}
		case "block":
			id = page.(notionapi.Block).GetID().String()
		case "database":
			id = page.(*notionapi.Database).ID.String()
		default:
		}
		if title != "" {
			result = append(result, domain.PageInfo{
				Id:    id,
				Title: title,
			})
		}
	}
	return result, nil
}

func (c *NotionClient) GetPagesContent(Pages []domain.PageInfo) ([]domain.Page, error) {
	var result []domain.Page
	for _, page := range Pages {
		res, err := c.getPageContent(page)
		if err != nil {
			return nil, fmt.Errorf("get Pages %s error: %s", page.Id, err.Error())
		}
		result = append(result, *res)
	}
	return result, nil
}

func (c *NotionClient) getPageContent(Page domain.PageInfo) (*domain.Page, error) {
	buf, err := c.getBlock(Page.Id)
	if err != nil {
		return nil, fmt.Errorf("get Page %s error: %s", Page.Id, err.Error())
	}

	return &domain.Page{
		ID:      Page.Id,
		Title:   Page.Title,
		Content: string(buf),
	}, nil
}
func (c *NotionClient) getBlock(id string) ([]byte, error) {
	var result []byte
	b, err := c.client.Block.Get(context.Background(), notionapi.BlockID(id))

	if err != nil {
		c.logger.Error("get block error", log.String("block_id", id), log.Error(err))
		return []byte{}, fmt.Errorf("get block %s error: %s", id, err.Error())
	}
	if b.GetType() == notionapi.BlockType(notionapi.BlockTypeUnsupported) {
		c.logger.Error("get block error", log.String("block_id", id), log.Error(err), log.String("block_type", b.GetType().String()))
		return []byte{}, nil
	}
	c.logger.Info("block", log.String("block_id", id), log.String("block_type", b.GetType().String()))

	if !b.GetHasChildren() {
		return []byte(c.BlockToMarkdown(b)), nil
	}

	childerns, err := c.client.Block.GetChildren(context.Background(), notionapi.BlockID(id), &notionapi.Pagination{})
	if err != nil {
		c.logger.Error("get block's children error", log.String("block_id", id), log.Error(err))
		return []byte{}, fmt.Errorf("get block's children %s error: %s", id, err.Error())
	}
	for _, childern := range childerns.Results {

		Id := childern.GetID().String()

		buf, err := c.getBlock(Id)
		if err != nil {
			c.logger.Error("get block child error", log.String("block_id", Id), log.Error(err))
		}
		result = append(result, buf...)

	}
	return result, nil
}

func (c *NotionClient) GetPages(req []domain.PageInfo) ([]*notionapi.Page, error) {
	var result []*notionapi.Page

	for _, r := range req {
		page, err := c.client.Page.Get(context.Background(), notionapi.PageID(r.Id))
		if err != nil {
			return nil, err
		}
		result = append(result, page)
	}
	return result, nil
}

func (c *NotionClient) BlockToMarkdown(block notionapi.Block) string {
	switch block.GetType() {
	case notionapi.BlockTypeHeading1:
		return fmt.Sprintf("# %s\n", block.GetRichTextString())
	case notionapi.BlockTypeParagraph:
		return fmt.Sprintf("%s\n", block.GetRichTextString())
	case notionapi.BlockTypeHeading2:
		return fmt.Sprintf("## %s\n", block.GetRichTextString())
	case notionapi.BlockTypeHeading3:
		return fmt.Sprintf("### %s\n", block.GetRichTextString())
	case notionapi.BlockTypeBulletedListItem:
		return fmt.Sprintf("- %s\n", block.GetRichTextString())
	case notionapi.BlockTypeNumberedListItem:
		num := c.getNumberedListNumber(block)
		return fmt.Sprintf("%d. %s\n", num, block.GetRichTextString())
	case notionapi.BlockTypeToggle:
		return fmt.Sprintf("::: toggle\n%s\n:::\n", block.GetRichTextString())
	case notionapi.BlockTypeQuote:
		return fmt.Sprintf("> %s\n", block.GetRichTextString())
	case notionapi.BlockTypeCode:

		return fmt.Sprintf("```\n%s\n```\n", block.GetRichTextString())
	case notionapi.BlockTypeTableRowBlock:

		cells := block.(*notionapi.TableRowBlock).TableRow.Cells
		nums := len(cells)
		buf := strings.Builder{}
		buf.WriteString("| ")
		for i := 0; i < nums; i++ {
			if len(cells[i]) > 0 {
				buf.WriteString(cells[i][0].PlainText)
			}
			if i != nums-1 {
				buf.WriteString(" | ")
			}
		}
		buf.WriteString(" |\n")
		return buf.String()

	case notionapi.BlockTypeTableBlock:
		ch, _ := c.client.Block.GetChildren(context.Background(), notionapi.BlockID(block.GetID().String()), &notionapi.Pagination{})
		hasRow := block.(*notionapi.TableBlock).Table.HasRowHeader
		var res strings.Builder

		for i, temp := range ch.Results {

			res.Write([]byte(c.BlockToMarkdown(temp)))
			if i == 0 && hasRow {
				len := len(temp.(*notionapi.TableRowBlock).TableRow.Cells) + 1

				for j := 0; j < len; j++ {
					res.Write([]byte("| ---"))
				}
				res.Write([]byte("|\n"))
			}
		}
		return res.String()

	case notionapi.BlockTypeDivider:
		return "---\n"
	case notionapi.BlockTypeVideo:
		url := block.(*notionapi.AudioBlock).Audio.GetURL()
		return fmt.Sprintf("<iframe src=\"%s\" width=\"300\" height=\"200\" frameborder=\"0\" allowfullscreen></iframe>", url)
	case notionapi.BlockTypeEmbed:
		url := block.(notionapi.EmbedBlock).Embed.URL
		return fmt.Sprintf("{%s}", url)
	case notionapi.BlockTypeCallout:
		return fmt.Sprintf("⚠️ %s\n", block.GetRichTextString())
	case notionapi.BlockTypeToDo:
		if block.(*notionapi.ToDoBlock).ToDo.Checked {
			return fmt.Sprintf("- [x] %s\n", block.GetRichTextString())
		}
		return fmt.Sprintf("- [ ] %s\n", block.GetRichTextString())
	case notionapi.BlockTypeImage:
		url, err := c.getImageURL(block)
		if err != nil {
			return err.Error()
		}
		return fmt.Sprintf("![%s](%s)\n", "", url)
	default:
		return ""
	}
}
func (c *NotionClient) getImageURL(block notionapi.Block) (string, error) {
	url := "https://api.notion.com/v1/blocks/" + block.GetID().String()
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Add("Authorization", "Bearer "+c.token)
	req.Header.Add("Notion-Version", "2021-08-16")
	req.Header.Add("Content-Type", "application/json")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var data ImageBlock
	err = json.NewDecoder(resp.Body).Decode(&data)
	if err != nil {
		return "", err
	}
	return data.Image.File.URL, nil

}

// 获取当前ListBlock的序号
func (c *NotionClient) getNumberedListNumber(block notionapi.Block) int {

	parentId := block.GetParent().BlockID.String()
	children, err := c.client.Block.GetChildren(context.Background(), notionapi.BlockID(parentId), &notionapi.Pagination{})
	if err != nil {
		return 1
	}
	i := 0
	for _, child := range children.Results {

		if child.GetID().String() == block.GetID().String() {
			return i + 1
		}
		if child.GetType() == notionapi.BlockTypeNumberedListItem {
			i++
		}
	}
	return i
}
