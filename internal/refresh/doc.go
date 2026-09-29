// Package refresh keeps the snapshot current: one goroutine owns a serialized
// bd queue, polls or follows the events journal, diffs snapshots into
// activity events and reports health signals. It never draws anything.
//
// Threading contract for the UI: [Engine.Updates] is the only output and the
// engine never blocks on it. [Engine.Refresh], [Engine.SetFocus] and
// [Engine.SetNotify] only post and return at once, so they are safe to call
// from Update. [Engine.Do] and [Engine.Write] wait for the bd queue and do
// I/O in it, so call them only from a tea.Cmd. No method runs bd on the
// caller's goroutine. The engine runs under the context given to Start;
// contexts passed to Do and Write bound only that call.
package refresh
