package discord

import (
	"context"
	"net/url"
	"strconv"
)

type threadSearchResponse struct {
	Threads      []Channel `json:"threads"`
	HasMore      bool      `json:"has_more"`
	TotalResults int       `json:"total_results"`
}

// Threads lists a channel's threads the way a user account can: the
// per-channel thread search endpoint, an archived=false pass then an
// archived=true pass, paging with offset until has_more is false. The
// guild-wide active-threads endpoint is bot-only and never used.
//
// A channel the account cannot search (403, 404) yields no threads rather
// than an error, matching DiscordChatExporter.
func (c *Client) Threads(ctx context.Context, channelID string) ([]Channel, error) {
	var all []Channel
	for _, archived := range []string{"false", "true"} {
		offset := 0
		for {
			q := url.Values{
				"sort_by":    {"last_message_time"},
				"sort_order": {"desc"},
				"archived":   {archived},
				"limit":      {"25"},
				"offset":     {strconv.Itoa(offset)},
			}
			var page threadSearchResponse
			err := c.Get(ctx, "/channels/"+channelID+"/threads/search", q, &page)
			if err != nil {
				if IsNotFound(err) {
					return all, nil
				}
				return nil, err
			}
			for i := range page.Threads {
				if page.Threads[i].ParentID == "" {
					page.Threads[i].ParentID = channelID
				}
			}
			all = append(all, page.Threads...)
			if !page.HasMore || len(page.Threads) == 0 {
				break
			}
			offset += len(page.Threads)
		}
	}
	return all, nil
}
