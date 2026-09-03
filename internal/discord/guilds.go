package discord

import (
	"context"
	"net/url"
)

// Guild is the part of a guild object the tool uses.
type Guild struct {
	ID                       string `json:"id"`
	Name                     string `json:"name"`
	Owner                    bool   `json:"owner"`
	ApproximateMemberCount   int    `json:"approximate_member_count"`
	ApproximatePresenceCount int    `json:"approximate_presence_count"`
}

// Guilds lists the guilds the account belongs to, with counts.
func (c *Client) Guilds(ctx context.Context) ([]Guild, error) {
	q := url.Values{"with_counts": {"true"}, "limit": {"200"}}
	var out []Guild
	if err := c.Get(ctx, "/users/@me/guilds", q, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Guild fetches one guild with counts.
func (c *Client) Guild(ctx context.Context, id string) (*Guild, error) {
	var out Guild
	if err := c.Get(ctx, "/guilds/"+id, url.Values{"with_counts": {"true"}}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Channel types the tool cares about.
const (
	ChannelText         = 0
	ChannelDM           = 1
	ChannelVoice        = 2
	ChannelGroupDM      = 3
	ChannelCategory     = 4
	ChannelAnnouncement = 5
	ChannelAnnThread    = 10
	ChannelPublicThread = 11
	ChannelPrivThread   = 12
	ChannelStage        = 13
	ChannelForum        = 15
	ChannelMedia        = 16
)

// IsMessageChannel reports whether a channel type is exported and listed:
// text, announcement, and forum.
func IsMessageChannel(t int) bool {
	return t == ChannelText || t == ChannelAnnouncement || t == ChannelForum
}

// IsThread reports whether a channel type is a thread.
func IsThread(t int) bool {
	return t == ChannelAnnThread || t == ChannelPublicThread || t == ChannelPrivThread
}

// Channel is the part of a channel object the tool uses.
type Channel struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     int    `json:"type"`
	ParentID string `json:"parent_id,omitempty"`
	Position int    `json:"position"`
	GuildID  string `json:"guild_id,omitempty"`
	// ThreadMetadata is present on threads.
	ThreadMetadata *ThreadMetadata `json:"thread_metadata,omitempty"`
}

// ThreadMetadata is the thread-only part of a channel object.
type ThreadMetadata struct {
	Archived         bool   `json:"archived"`
	ArchiveTimestamp string `json:"archive_timestamp,omitempty"`
}

// Channels lists a guild's channels, including categories and voice.
func (c *Client) Channels(ctx context.Context, guildID string) ([]Channel, error) {
	var out []Channel
	if err := c.Get(ctx, "/guilds/"+guildID+"/channels", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}
