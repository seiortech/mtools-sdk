package components

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestHistoryPanelEmpty(t *testing.T) {
	tt := ui.NewTester(func(c *ui.Context) {
		HistoryPanel(c, HistoryPanelProps{
			Title: "Test History",
			Count: 0,
			Items: nil,
		})
	}, 320, 500)

	tt.Frame()

	if !tt.HasText("Test History") {
		t.Fatalf("expected title 'Test History'")
	}
	if !tt.HasText("No history yet") {
		t.Fatalf("expected empty state text 'No history yet'")
	}
	if !tt.HasText("0") {
		t.Fatalf("expected count badge '0'")
	}
}

func TestHistoryPanelWithItemsAndActions(t *testing.T) {
	cleared := false
	deletedID := ""
	shownPath := ""

	items := []HistoryCardItem{
		{
			ID:       "item_1",
			Title:    "First Video",
			Subtitle: "Channel A · 05:00",
			FilePath: "/path/video1.mp4",
		},
	}

	tt := ui.NewTester(func(c *ui.Context) {
		HistoryPanel(c, HistoryPanelProps{
			Title: "Download History",
			Count: len(items),
			Items: items,
			OnClearAll: func() {
				cleared = true
			},
			OnDelete: func(id string) {
				deletedID = id
			},
			OnShow: func(p string) {
				shownPath = p
			},
		})
	}, 320, 500)

	tt.Frame()

	if !tt.HasText("Download History") {
		t.Fatalf("expected 'Download History'")
	}
	if !tt.HasText("First Video") {
		t.Fatalf("expected item title 'First Video'")
	}
	if !tt.HasText("Channel A · 05:00") {
		t.Fatalf("expected item subtitle")
	}

	// Click Show
	if err := tt.Click("Show"); err != nil {
		t.Fatalf("Click 'Show' failed: %v", err)
	}
	if shownPath != "/path/video1.mp4" {
		t.Fatalf("expected shownPath '/path/video1.mp4', got %q", shownPath)
	}

	// Click Delete
	if err := tt.Click("✕"); err != nil {
		t.Fatalf("Click '✕' failed: %v", err)
	}
	if deletedID != "item_1" {
		t.Fatalf("expected deletedID 'item_1', got %q", deletedID)
	}

	// Click Clear All
	if err := tt.Click("Clear All"); err != nil {
		t.Fatalf("Click 'Clear All' failed: %v", err)
	}
	if !cleared {
		t.Fatalf("expected cleared == true")
	}
}

func TestHistoryPanelToast(t *testing.T) {
	dismissed := false

	tt := ui.NewTester(func(c *ui.Context) {
		HistoryPanel(c, HistoryPanelProps{
			Title:        "History",
			ToastMessage: "File was moved or deleted",
			OnDismissToast: func() {
				dismissed = true
			},
		})
	}, 320, 500)

	tt.Frame()

	if !tt.HasText("File was moved or deleted") {
		t.Fatalf("expected toast text")
	}

	if err := tt.Click("✕"); err != nil {
		t.Fatalf("Click toast '✕' failed: %v", err)
	}
	if !dismissed {
		t.Fatalf("expected toast dismiss callback called")
	}
}
