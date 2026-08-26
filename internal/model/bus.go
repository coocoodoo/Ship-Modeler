package model

import (
	"encoding/json"
	"fmt"
	"time"

	"modeler/internal/geom"
)

// The command bus (SPEC-DATA §3). Every document mutation goes through it, so
// undo works everywhere for free (R17) and nothing can quietly mutate state
// behind its back.

// UndoCap is how many steps the history keeps before dropping the oldest.
const UndoCap = 200

// Command is one undoable document mutation.
//
// The atomicity contract is the important part: Do must compute its whole
// result and validate it *before* touching the document, so returning an error
// leaves the document exactly as it was (SPEC-DATA §3.1).
type Command interface {
	// Name is the human phrase used by toasts and the undo history.
	Name() string
	// Do applies the change, or returns an error having changed nothing.
	Do(doc *Document) error
	// Undo restores the state captured during Do.
	Undo(doc *Document)
}

// Feature is implemented by commands that want a specific feature-log entry.
// Commands that do not implement it are logged by name alone.
type Feature interface {
	FeatureKind() string
	FeatureParams() any
}

// EventKind names a change derived caches care about.
type EventKind uint8

const (
	// EvBodyChanged means one body's geometry, colour, name or visibility moved.
	EvBodyChanged EventKind = iota
	// EvBodyAdded and EvBodyRemoved bracket structural changes to the body list.
	EvBodyAdded
	EvBodyRemoved
	// EvSketchChanged, EvSketchAdded and EvSketchRemoved mirror those for sketches.
	EvSketchChanged
	EvSketchAdded
	EvSketchRemoved
	// EvPlanesChanged means a default plane was shown or hidden.
	EvPlanesChanged
	// EvDocReplaced means everything must be rebuilt: load, new, undo of a
	// structural change.
	EvDocReplaced
)

// Event is one change notification.
type Event struct {
	Kind   EventKind
	BodyID uint32
	Sketch uint32
	Plane  geom.PlaneKind
}

// Events is a tiny synchronous publisher. Listeners run on the caller's
// goroutine, which is always the main thread.
type Events struct {
	listeners []func(Event)
}

// Listen registers a listener for every event.
func (e *Events) Listen(fn func(Event)) {
	e.listeners = append(e.listeners, fn)
}

// Emit delivers an event to every listener.
func (e *Events) Emit(ev Event) {
	for _, fn := range e.listeners {
		fn(ev)
	}
}

// Bus runs commands against a document and keeps the undo history.
type Bus struct {
	doc    *Document
	Events Events

	undo []Command
	redo []Command

	// pending is the command a drag is currently building. It is live in the
	// document but not yet in the history (SPEC-DATA §3.2).
	pending Command

	// Now is the clock the feature log stamps with; tests override it.
	Now func() time.Time
}

// NewBus wraps a document.
func NewBus(doc *Document) *Bus {
	return &Bus{doc: doc, Now: time.Now}
}

// Doc returns the document the bus operates on.
func (b *Bus) Doc() *Document { return b.doc }

// Replace swaps in a whole new document, clearing the history. Used by New,
// Open and crash recovery.
func (b *Bus) Replace(doc *Document) {
	b.doc = doc
	b.undo, b.redo, b.pending = nil, nil, nil
	b.Events.Emit(Event{Kind: EvDocReplaced})
}

// Run executes a command. On success it lands on the undo stack, clears the
// redo stack, appends a feature record and marks the document dirty. On error
// nothing at all changes.
func (b *Bus) Run(cmd Command) error {
	if b.pending != nil {
		return fmt.Errorf("cannot run %q while a drag is in progress", cmd.Name())
	}
	if err := cmd.Do(b.doc); err != nil {
		return err
	}
	b.push(cmd)
	b.emitFor(cmd)
	return nil
}

func (b *Bus) push(cmd Command) {
	b.undo = append(b.undo, cmd)
	if len(b.undo) > UndoCap {
		b.undo = append(b.undo[:0], b.undo[len(b.undo)-UndoCap:]...)
	}
	b.redo = b.redo[:0]
	b.doc.Features = append(b.doc.Features, b.record(cmd))
	b.doc.DirtySinceSave = true
}

func (b *Bus) record(cmd Command) FeatureRec {
	rec := FeatureRec{Kind: cmd.Name(), Time: b.Now()}
	if f, ok := cmd.(Feature); ok {
		rec.Kind = f.FeatureKind()
		if data, err := json.Marshal(f.FeatureParams()); err == nil {
			rec.Params = data
		}
	}
	return rec
}

// emitFor publishes the change notifications a command declares.
func (b *Bus) emitFor(cmd Command) {
	if e, ok := cmd.(interface{ Events() []Event }); ok {
		for _, ev := range e.Events() {
			b.Events.Emit(ev)
		}
		return
	}
	b.Events.Emit(Event{Kind: EvDocReplaced})
}

// CanUndo reports whether there is anything to undo.
func (b *Bus) CanUndo() bool { return b.pending == nil && len(b.undo) > 0 }

// CanRedo reports whether there is anything to redo.
func (b *Bus) CanRedo() bool { return b.pending == nil && len(b.redo) > 0 }

// UndoName and RedoName are what the toolbar tooltips show.
func (b *Bus) UndoName() string {
	if !b.CanUndo() {
		return ""
	}
	return b.undo[len(b.undo)-1].Name()
}

func (b *Bus) RedoName() string {
	if !b.CanRedo() {
		return ""
	}
	return b.redo[len(b.redo)-1].Name()
}

// Undo reverses the last command and returns its name for the toast.
func (b *Bus) Undo() (string, bool) {
	if !b.CanUndo() {
		return "", false
	}
	cmd := b.undo[len(b.undo)-1]
	b.undo = b.undo[:len(b.undo)-1]
	cmd.Undo(b.doc)
	b.redo = append(b.redo, cmd)
	b.doc.DirtySinceSave = true
	b.emitFor(cmd)
	return cmd.Name(), true
}

// Redo re-applies the last undone command.
func (b *Bus) Redo() (string, bool) {
	if !b.CanRedo() {
		return "", false
	}
	cmd := b.redo[len(b.redo)-1]
	b.redo = b.redo[:len(b.redo)-1]
	if err := cmd.Do(b.doc); err != nil {
		// A redo that no longer applies is dropped rather than half-applied.
		return "", false
	}
	b.undo = append(b.undo, cmd)
	b.doc.DirtySinceSave = true
	b.emitFor(cmd)
	return cmd.Name(), true
}

// UndoDepth and RedoDepth expose the history size for tests and diagnostics.
func (b *Bus) UndoDepth() int { return len(b.undo) }
func (b *Bus) RedoDepth() int { return len(b.redo) }

// Drag interactions (SPEC-DATA §3.2): a drag applies live every frame but
// lands as ONE history entry on release, and Escape reverts it entirely.

// BeginDrag applies a command live without recording it.
func (b *Bus) BeginDrag(cmd Command) error {
	if b.pending != nil {
		b.CancelDrag()
	}
	if err := cmd.Do(b.doc); err != nil {
		return err
	}
	b.pending = cmd
	b.emitFor(cmd)
	return nil
}

// UpdateDrag replaces the live command with a newer one, so the document only
// ever holds the latest state of the drag.
func (b *Bus) UpdateDrag(cmd Command) error {
	if b.pending == nil {
		return b.BeginDrag(cmd)
	}
	b.pending.Undo(b.doc)
	if err := cmd.Do(b.doc); err != nil {
		// Put the previous state back so a rejected update does not strand the
		// document mid-drag.
		if reErr := b.pending.Do(b.doc); reErr != nil {
			b.pending = nil
		}
		return err
	}
	b.pending = cmd
	b.emitFor(cmd)
	return nil
}

// CommitDrag records the live command as a single history entry.
func (b *Bus) CommitDrag() (string, bool) {
	if b.pending == nil {
		return "", false
	}
	cmd := b.pending
	b.pending = nil
	b.push(cmd)
	return cmd.Name(), true
}

// CancelDrag reverts the live command and records nothing, which is what Escape
// during a drag does.
func (b *Bus) CancelDrag() {
	if b.pending == nil {
		return
	}
	cmd := b.pending
	b.pending = nil
	cmd.Undo(b.doc)
	b.emitFor(cmd)
}

// Dragging reports whether a drag is live.
func (b *Bus) Dragging() bool { return b.pending != nil }
