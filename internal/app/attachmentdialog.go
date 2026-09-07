package app

import (
	rl "github.com/gen2brain/raylib-go/raylib"
	"modeler/internal/model"
	"modeler/internal/ui"
)

func (a *App) beginAttachmentDialog(index int) {
	if a.Mode != ModeIdle {
		return
	}
	a.CancelMarkerPick()
	st := &a.markers
	st.editIndex, st.attachmentOpen = index, true
	st.slot, st.appendText = "A", ""
	if index >= 0 && index < len(a.Doc().Markers) {
		m := a.Doc().Markers[index]
		st.slot, st.appendText = m.Slot, m.AppendText
	} else {
		for letter := 'A'; letter <= 'Z'; letter++ {
			used := false
			for _, m := range a.Doc().Markers {
				if m.Kind == model.MarkerAttachment && m.Slot == string(letter) {
					used = true
					break
				}
			}
			if !used {
				st.slot = string(letter)
				break
			}
		}
	}
	a.UI.ClearFocus()
}

func (a *App) buildAttachmentDialog() {
	st := &a.markers
	if !st.attachmentOpen || a.UI.ModalOpen() {
		return
	}
	_, _, err := model.CleanAttachmentName(st.slot, st.appendText)
	why := ""
	if err != nil {
		why = err.Error()
	}
	title, confirm := "Ship Part Attachment", "Place"
	if st.editIndex >= 0 {
		title, confirm = "Edit Ship Part Attachment", "Save"
	}
	card := a.UI.FloatingCard(ui.MakeID("attachment.dialog"), a.libraryDialogBox(430, 332), title,
		ui.FloatingCardOpts{Footer: true, ConfirmLabel: confirm, CancelLabel: "Cancel", ConfirmDisabled: err != nil, ConfirmWhy: why})
	body := card.Body
	row := func(h float32) rl.Rectangle { var r rl.Rectangle; r, body = ui.SplitTop(body, h); return r }
	a.UI.Text(row(a.px(24)), "Part letter", ui.FontSizeSmall, ui.ColorTextDim)
	for line := 0; line < 2; line++ {
		r := row(a.px(30))
		w := r.Width / 13
		for column := 0; column < 13; column++ {
			letter := string(rune('A' + line*13 + column))
			style := ui.ButtonGhost
			if st.slot == letter {
				style = ui.ButtonPrimary
			}
			if a.UI.Button(ui.MakeID("attachment.letter."+letter), ui.Rect(r.X+float32(column)*w, r.Y, w-a.px(2), r.Height-a.px(2)), letter, ui.ButtonOpts{Style: style}) {
				st.slot = letter
			}
		}
	}
	row(a.px(8))
	a.UI.Text(row(a.px(22)), "Append text (optional)", ui.FontSizeSmall, ui.ColorTextDim)
	text := a.UI.TextField(ui.MakeID("attachment.append"), row(a.px(28)), st.appendText,
		ui.TextFieldOpts{Placeholder: "e.g. Left wing", SelectAllOnFocus: true, Live: true})
	st.appendText = text.Text
	row(a.px(8))
	a.UI.Text(row(a.px(24)), model.AttachmentName(st.slot, st.appendText), ui.FontSizeUI, attachmentPairColor(st.appendText))
	hint := "Click Place, then click a face. Its outward direction becomes the attachment direction."
	if st.editIndex >= 0 {
		hint = "Update this attachment's name. Select its marker to move it with the gizmo."
	}
	a.UI.TextWrapped(row(a.px(36)), hint, ui.FontSizeSmall, ui.ColorTextDim)
	a.UI.Text(row(a.px(20)), "Matching [append text] shares the same color.", ui.FontSizeSmall, ui.ColorTextDim)
	if why != "" {
		a.UI.Text(row(a.px(20)), why, ui.FontSizeSmall, ui.ColorError)
	}
	a.UI.ClaimPointer(a.layout.Screen)
	if card.Cancelled || a.UI.In.KeyPressed(rl.KeyEscape) {
		st.attachmentOpen = false
		a.UI.ClearFocus()
		return
	}
	if card.Confirmed {
		// Clicking the footer commits the text field in this same frame.
		slot, text, err := model.CleanAttachmentName(st.slot, st.appendText)
		if err != nil {
			return
		}
		st.slot, st.appendText = slot, text
		if st.editIndex >= 0 {
			if !a.Run(&model.RenameAttachment{Index: st.editIndex, Slot: slot, AppendText: text}) {
				return
			}
		} else {
			a.Sel.Clear()
			a.dropPushPull()
			a.transform.tool = nil
			st.awaiting, st.armed = model.MarkerAttachment, true
		}
		st.attachmentOpen = false
		a.UI.ClearFocus()
	}
}
