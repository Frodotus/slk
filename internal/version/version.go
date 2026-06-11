// Package version provides formatting helpers for slk's build-time
// version metadata. The build vars themselves live in package main
// (injected via -ldflags by GoReleaser); callers pass the version
// string in so this package stays free of build-time coupling.
package version

// ModalFooter returns the single line shown at the bottom of the TUI
// help modal — just the running version.
func ModalFooter(version string) string {
	return "slk " + version
}
