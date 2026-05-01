package svt

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

type ChatMessage struct {
	Attachment  ChatMessageAttachment  `json:"attachment,omitempty"`
	Author      ChatMessageAuthor      `json:"author"`
	Body        string                 `json:"body"`
	CreatedAt   time.Time              `json:"createdAt"`
	ModifiedAt  time.Time              `json:"modifiedAt,omitzero"`
	Highlighted bool                   `json:"highlighted"`
	Pinned      bool                   `json:"pinned"`
	Signature   string                 `json:"signature,omitempty"`
	Replies     map[string]ChatMessage `json:"replies,omitempty"`
}

type ChatMessageAuthor struct {
	DisplayName string `json:"displayName"`
	Avatar      string `json:"avatar,omitempty"`
	Role        string `json:"role"`
	UnoID       string `json:"unoId,omitempty"`
	Title       string `json:"title,omitempty"`
}

// AvatarURL returns a URL to the author's image.
func (m ChatMessageAuthor) AvatarURL() (string, error) {
	// Editors typically have an avatar assigned, visitors don't
	if m.Avatar != "" {
		return url.JoinPath("https://svt-direktcenter-avatar.imgix.net", url.PathEscape(strings.TrimLeft(m.Avatar, "/")))
	}

	url, err := url.JoinPath("https://avatar-static.prod.uno.svt.se/avatars", url.PathEscape(m.Avatar))
	if err != nil {
		return "", err
	}

	return url + ".png", nil
}

type ChatMessageAttachment interface {
	Type() string
}

var _ ChatMessageAttachment = (*ChatMessageImageAttachment)(nil)

type ChatMessageImageAttachment struct {
	Alt            string
	Caption        string
	OriginalHeight int
	OriginalWidth  int
	Path           string
}

func (m ChatMessageImageAttachment) Type() string {
	return "image"
}

// URL returns a URL to the attachment's image.
func (m ChatMessageImageAttachment) URL() (string, error) {
	url, err := url.JoinPath("https://svt-direktcenter.imgix.net", url.PathEscape(strings.TrimLeft(m.Path, "/")))
	if err != nil {
		return "", err
	}

	return url + "?auto=format", nil
}

type attachment struct {
	Type           string `firestore:"type"`
	Alt            string `firestore:"alt"`
	Caption        string `firestore:"caption"`
	OriginalHeight int    `firestore:"originalHeight"`
	OriginalWidth  int    `firestore:"originalWidth"`
	Path           string `firestore:"path"`
}

type avatar struct {
	Path string `firestore:"path"`
}

type author struct {
	DisplayName string `firestore:"displayName"`
	Avatar      avatar `firestore:"avatar"`
	Role        string `firestore:"role"`
	Title       string `firestore:"title,omitempty"`
	UnoID       string `firestore:"unoId,omitempty"`
}

type message struct {
	Attachment  *attachment        `firestore:"attachment"`
	Author      author             `firestore:"author"`
	Body        string             `firestore:"body"`
	CreatedAt   time.Time          `firestore:"createdAt"`
	ModifiedAt  time.Time          `firestore:"modifiedAt"`
	Highlighted bool               `firestore:"highlighted"`
	Pinned      bool               `firestore:"pinned"`
	Signature   string             `firestore:"signature,omitempty"`
	Replies     map[string]message `firestore:"replies,omitempty"`
}

func newChatMessage(m message) (ChatMessage, error) {
	var attachment ChatMessageAttachment
	if m.Attachment != nil {
		switch m.Attachment.Type {
		case "image":
			attachment = ChatMessageImageAttachment{
				Alt:            m.Attachment.Alt,
				Caption:        m.Attachment.Caption,
				OriginalHeight: m.Attachment.OriginalWidth,
				OriginalWidth:  m.Attachment.OriginalWidth,
				Path:           m.Attachment.Path,
			}
		default:
			return ChatMessage{}, fmt.Errorf("unsupported attachment type: %s", m.Attachment.Type)
		}
	}

	var replies map[string]ChatMessage
	if m.Replies != nil {
		replies = make(map[string]ChatMessage)
		for k, v := range m.Replies {
			var err error
			replies[k], err = newChatMessage(v)
			if err != nil {
				return ChatMessage{}, err
			}
		}
	}

	return ChatMessage{
		Attachment: attachment,
		Author: ChatMessageAuthor{
			DisplayName: m.Author.DisplayName,
			Avatar:      m.Author.Avatar.Path,
			Role:        m.Author.Role,
			UnoID:       m.Author.UnoID,
			Title:       m.Author.Title,
		},
		Body:        m.Body,
		CreatedAt:   m.CreatedAt,
		ModifiedAt:  m.ModifiedAt,
		Highlighted: m.Highlighted,
		Pinned:      m.Pinned,
		Signature:   m.Signature,
		Replies:     replies,
	}, nil
}

type ChatClient struct {
	client *firestore.Client
}

func NewChatClient(ctx context.Context) (*ChatClient, error) {
	client, err := firestore.NewClientWithDatabase(ctx, "direktcenter-prod", "(default)", option.WithoutAuthentication())
	if err != nil {
		return nil, err
	}

	return &ChatClient{
		client: client,
	}, nil
}

func (c *ChatClient) Messages(ctx context.Context, id string) (iter.Seq[ChatMessage], error) {
	query := c.client.
		Collection("streams").
		Doc(id).
		Collection("posts").
		Where("pinned", "==", false).
		Where("createdAt", ">", time.Now()).
		OrderBy("createdAt", firestore.Desc).
		Limit(1).
		Snapshots(ctx)

	return iterMessages(query)
}

func (c *ChatClient) Pinned(ctx context.Context, id string) (iter.Seq[ChatMessage], error) {
	query := c.client.
		Collection("streams").
		Doc(id).
		Collection("posts").
		Where("pinned", "==", true).
		Snapshots(ctx)

	return iterMessages(query)
}

func (c *ChatClient) GetEmojiRain(ctx context.Context, id string) (map[string]int, error) {
	url, err := url.JoinPath("https://api.svt.se/emoji-rain", id, "rain")
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", res.StatusCode)
	}

	var rain map[string]int
	if err := json.NewDecoder(res.Body).Decode(&rain); err != nil {
		return nil, fmt.Errorf("unexpected content: %w", err)
	}

	return rain, nil
}

func iterMessages(query *firestore.QuerySnapshotIterator) (iter.Seq[ChatMessage], error) {
	return iter.Seq[ChatMessage](func(yield func(ChatMessage) bool) {
		for {
			snapshot, err := query.Next()
			if err == iterator.Done {
				break
			} else if err != nil {
				slog.Error("Failed to get next snapshot", slog.Any("error", err))
				return
			}

			for {
				doc, err := snapshot.Documents.Next()
				if err == iterator.Done {
					break
				} else if err != nil {
					slog.Error("Failed to get next", slog.Any("error", err))
					return
				}

				var m message
				if err := doc.DataTo(&m); err != nil {
					slog.Warn("Failed to parse message", slog.Any("error", err), slog.Any("message", doc.Data()))
					continue
				}

				message, err := newChatMessage(m)
				if err != nil {
					slog.Warn("Failed to map message", slog.Any("error", err), slog.Any("message", doc.Data()))
					continue
				}

				ok := yield(message)
				if !ok {
					return
				}
			}
		}
	}), nil
}

func (c *ChatClient) Close() error {
	return c.client.Close()
}
