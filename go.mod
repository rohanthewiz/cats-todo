module github.com/rohanthewiz/cats-todo

go 1.26.1

require (
	charm.land/bubbles/v2 v2.1.1
	charm.land/bubbletea/v2 v2.0.8
	charm.land/lipgloss/v2 v2.0.5
	github.com/charmbracelet/x/ansi v0.11.7
	github.com/charmbracelet/x/term v0.2.2
	github.com/mattn/go-runewidth v0.0.24
	github.com/rivo/uniseg v0.4.7
	// cats is the §7 wire contract, and this line IS the pin. The vocabulary is
	// imported, never copied: `wire` is a public stdlib-only leaf package, so
	// this dependency adds no transitive ones. Bump it with
	//
	//	go get github.com/rohanthewiz/cats@<sha> && go mod tidy
	//
	// and let the compiler name whatever moved. (internal/ctlproto and
	// internal/integration are still hand-copied — those live under cats'
	// internal/ and cannot be imported.)
	github.com/rohanthewiz/cats v0.2.3-0.20260904234655-5d1e4a6716fe
	github.com/sahilm/fuzzy v0.1.3
)

require (
	github.com/atotto/clipboard v0.1.4 // indirect
	github.com/charmbracelet/colorprofile v0.4.3 // indirect
	github.com/charmbracelet/ultraviolet v0.0.0-20260703014108-f5a850f9c2b7 // indirect
	github.com/charmbracelet/x/termios v0.1.1 // indirect
	github.com/charmbracelet/x/windows v0.2.2 // indirect
	github.com/clipperhouse/displaywidth v0.11.0 // indirect
	github.com/clipperhouse/uax29/v2 v2.7.0 // indirect
	github.com/lucasb-eyer/go-colorful v1.4.0 // indirect
	github.com/muesli/cancelreader v0.2.2 // indirect
	github.com/xo/terminfo v0.0.0-20220910002029-abceb7e1c41e // indirect
	golang.org/x/sync v0.21.0 // indirect
	golang.org/x/sys v0.46.0 // indirect
)
