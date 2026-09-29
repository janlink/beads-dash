// Package ui holds the Bubble Tea program: the shell around the views, the
// dialogs and the full-screen states.
//
// # Views
//
// The shell owns everything around the body of a view: the header, footer,
// notice row, dialogs, the current issue, the marks, the back stack and the
// layer stack. A view is registered per slot in Options.Views and reads and
// changes the session only through Env.
//
// Key handling. The shell resolves a key in the context View.Context reports
// (or Panel and Bar while those layers are on top) and hands the resulting
// action to View.Handle together with a fresh Env. Handle returns the command
// to run and whether the view acted; the shell only performs its own default
// for actions the view declined (cursor movement over Visible). Actions the
// shell owns, such as help, quit and view switching, never reach the view.
//
// Messages. A view that loads data lazily implements Updater; the shell
// forwards every message it does not consume itself to all registered views,
// so a result arriving after a view switch still lands.
//
// Rows. A view draws rows with Env.Rows.Line, passing its own name and a body
// provider; the renderer caches bodies per (view, id, width) and composes the
// gutter per frame.
//
// # Dialogs
//
// Overlays implement Dialog and stack; the top one owns the keys, and closing
// through keys.Close is the cancel path the shell also uses when a fatal error
// covers them.
package ui
