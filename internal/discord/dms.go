package discord

import "context"

// DMChannel is a DM (type 1) or group DM (type 3).
type DMChannel struct {
	ID            string `json:"id"`
	Type          int    `json:"type"`
	Name          string `json:"name"`
	Recipients    []User `json:"recipients"`
	LastMessageID string `json:"last_message_id"`
	OwnerID       string `json:"owner_id"`
}

// IsGroup reports whether the DM has several participants.
func (d DMChannel) IsGroup() bool { return d.Type == ChannelGroupDM }

// DisplayName is the name a person knows the DM by: the other participant's
// username for a DM; the group name, else the participants, for a group.
func (d DMChannel) DisplayName() string {
	if d.IsGroup() {
		if d.Name != "" {
			return d.Name
		}
		names := make([]string, 0, len(d.Recipients))
		for _, r := range d.Recipients {
			names = append(names, r.Username)
		}
		return joinNames(names)
	}
	if len(d.Recipients) > 0 {
		return d.Recipients[0].Username
	}
	return d.ID
}

func joinNames(names []string) string {
	out := ""
	for i, n := range names {
		if i > 0 {
			out += ", "
		}
		out += n
	}
	if out == "" {
		return "(empty group)"
	}
	return out
}

// DMs lists the account's DMs and group DMs.
func (c *Client) DMs(ctx context.Context) ([]DMChannel, error) {
	var all []DMChannel
	if err := c.Get(ctx, "/users/@me/channels", nil, &all); err != nil {
		return nil, err
	}
	var out []DMChannel
	for _, d := range all {
		if d.Type == ChannelDM || d.Type == ChannelGroupDM {
			out = append(out, d)
		}
	}
	return out, nil
}
