package components

import (
	"fmt"

	"github.com/egoist/mygo/ui"

	"github.com/seiortech/mtools-sdk/ui/theme"
)

// HistoryCardItem represents one completed item to render in the history panel.
type HistoryCardItem struct {
	ID       string
	Title    string
	Subtitle string
	FilePath string
	Badge    string
}

// HistoryPanelProps configures the shared Bauhaus history panel.
type HistoryPanelProps struct {
	Title          string
	Count          int
	Items          []HistoryCardItem
	ToastMessage   string
	EmptyTitle     string
	EmptySubtitle  string
	ClearAllLabel  string
	HideLabel      string
	ShowLabel      string
	ShowHide       bool
	OnHide         func()
	OnClearAll     func()
	OnDismissToast func()
	OnShow         func(filePath string)
	OnDelete       func(id string)
}

// HistoryPanel renders a uniform history sidebar or inspector across tools and shell.
func HistoryPanel(c *ui.Context, props HistoryPanelProps) ui.Element {
	title := props.Title
	if title == "" {
		title = "Recent Activity"
	}
	emptyTitle := props.EmptyTitle
	if emptyTitle == "" {
		emptyTitle = "No history yet"
	}
	emptySubtitle := props.EmptySubtitle
	if emptySubtitle == "" {
		emptySubtitle = "Completed items will appear here."
	}
	clearAllLabel := props.ClearAllLabel
	if clearAllLabel == "" {
		clearAllLabel = "Clear All"
	}
	hideLabel := props.HideLabel
	if hideLabel == "" {
		hideLabel = "Hide »"
	}
	showLabel := props.ShowLabel
	if showLabel == "" {
		showLabel = "Show"
	}

	return ui.Column(c).
		Fill().
		Padding(20).
		Gap(16).
		Children(func() {
			// Header Row
			ui.Row(c).Justify(ui.SpaceBetween).AlignItems(ui.Center).Children(func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, title).
						FontSize(15).
						FontWeight(700).
						TextColor(theme.Ink)

					ui.Box(c).
						Padding(2, 6).
						Border(1, theme.Ink).
						Background(theme.FieldSupport).
						Children(func() {
							ui.Text(c, fmt.Sprintf("%d", props.Count)).
								FontSize(11).
								FontWeight(700).
								TextColor(theme.Ink)
						})
				})

				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					if props.Count > 0 && props.OnClearAll != nil {
						SecondaryButton(c, clearAllLabel, props.OnClearAll)
					}
					if props.ShowHide && props.OnHide != nil {
						SecondaryButton(c, hideLabel, props.OnHide)
					}
				})
			})

			// Missing-file or warning toast banner
			if props.ToastMessage != "" {
				ui.Row(c).
					Justify(ui.SpaceBetween).
					AlignItems(ui.Center).
					Border(1, theme.Danger).
					Background(theme.Paper).
					Padding(8, 10).
					Children(func() {
						ui.Text(c, props.ToastMessage).
							FontSize(12).
							FontWeight(600).
							TextColor(theme.Danger)

						if props.OnDismissToast != nil {
							SecondaryButton(c, "✕", props.OnDismissToast)
						}
					})
			}

			// Empty State
			if len(props.Items) == 0 {
				ui.Column(c).
					Gap(6).
					Padding(32, 0).
					AlignItems(ui.Center).
					Children(func() {
						ui.Text(c, emptyTitle).
							FontSize(13).
							FontWeight(600).
							TextColor(theme.InkMuted)
						ui.Text(c, emptySubtitle).
							FontSize(12).
							TextColor(theme.InkMuted)
					})
				return
			}

			// Scrollable Items List
			ui.Scroll(c).Grow(1).Children(func() {
				ui.Column(c).Gap(10).Children(func() {
					for _, it := range props.Items {
						item := it
						ui.Column(c).
							Border(1, theme.Ink).
							Background(theme.Ground).
							Padding(12).
							Gap(8).
							Children(func() {
								if item.Badge != "" {
									ui.Row(c).Children(func() {
										ui.Box(c).
											Padding(2, 6).
											Border(1, theme.Ink).
											Background(theme.FieldSupport).
											Children(func() {
												ui.Text(c, item.Badge).
													FontSize(10).
													FontWeight(700).
													TextColor(theme.Ink)
											})
									})
								}

								// Title
								ui.Text(c, item.Title).
									FontSize(13).
									FontWeight(700).
									TextColor(theme.Ink).
									MaxLines(2)

								// Subtitle
								if item.Subtitle != "" {
									ui.Text(c, item.Subtitle).
										FontSize(11).
										TextColor(theme.InkMuted)
								}

								// Actions row
								ui.Row(c).Justify(ui.SpaceBetween).AlignItems(ui.Center).Children(func() {
									if item.FilePath != "" && props.OnShow != nil {
										SecondaryButton(c, showLabel, func() {
											props.OnShow(item.FilePath)
										})
									} else {
										ui.Box(c)
									}

									if props.OnDelete != nil {
										SecondaryButton(c, "✕", func() {
											props.OnDelete(item.ID)
										})
									}
								})
							})
					}
				})
			})
		})
}
