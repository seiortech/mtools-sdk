package tool

import "time"

// PluginID identifies a plugin by reverse-DNS notation.
type PluginID string

// ToolID identifies a tool within a plugin.
type ToolID string

// ToolMeta describes a user-facing tool.
type ToolMeta struct {
	ID          ToolID   `json:"id"`
	PluginID    PluginID `json:"plugin_id"`
	Name        string   `json:"name"`
	Category    string   `json:"category"`
	Description string   `json:"description"`
	Icon        string   `json:"icon,omitempty"`
}

// HistoryEntry is a normalized view of a completed task for global history.
type HistoryEntry struct {
	ID        string    `json:"id"`
	ToolID    ToolID    `json:"tool_id"`
	ToolName  string    `json:"tool_name"`
	Title     string    `json:"title"`
	Subtitle  string    `json:"subtitle"`
	FilePath  string    `json:"file_path"`
	Timestamp time.Time `json:"timestamp"`
}
