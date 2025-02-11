package feeds

import "norsky/config"

// PublishInfo contains the metadata needed for publishing a feed
type PublishInfo struct {
	Id          string
	DisplayName string
	Description string
	AvatarPath  string
}

// GetPublishInfo creates a slice of PublishInfo from TOML configuration
func GetPublishInfo(cfg *config.TomlConfig) []PublishInfo {
	feeds := make([]PublishInfo, len(cfg.Feeds))
	for i, feed := range cfg.Feeds {
		feeds[i] = PublishInfo{
			Id:          feed.Id,
			DisplayName: feed.DisplayName,
			Description: feed.Description,
			AvatarPath:  feed.AvatarPath,
		}
	}
	return feeds
}
