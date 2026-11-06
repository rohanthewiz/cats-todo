// modelpick.go — switching a running Claude Code pane's model and effort for
// that session only, by driving the picker the bare `/model` command opens
// (N-079, N-070's road (c)).
//
// Why a picker and not `/model X`. Since Claude Code 2.1.283 a typed
// `/model X` or `/effort X` also saves the pick as the user's default for new
// sessions (~/.claude/settings.json), and no flag turns that off (N-070). The
// picker has a second way out: its footer offers `s to use this session only`
// (the modelPicker:thisSessionOnly binding), which applies the focused model —
// and the effort shown under the list, which ←/→ adjust — without writing
// anything. Enter in the same picker is "set as default", so this file never
// presses Enter while the picker is up.
//
// One picker carries both settings. The model picker's effort line (←/→) goes
// with the pick, and `s` reports both in one line:
//
//	Set model to Haiku 5.5 for this session only with low effort
//
// So an effort-only switch also goes through /model, on the row it opens on
// (the pane's current model), rather than through /effort's slider. The slider
// shows its level only as a ▲ under a row of labels, while the picker prints
// the level as a word ("○ Low effort"), which is what makes the drive below
// checkable. Picking the current model again changes nothing about the model.
//
// How it is driven. Nothing here counts keystrokes blind. After each key the
// screen is captured and parsed again (parsePicker), and the next key is
// chosen from what the picker says now. The picker as 2.1.318 draws it:
//
//	  Select model
//	  Switch between Claude models. Your pick becomes the default for new …
//	    1.  Default (recommended)  Opus 5.5 · Best for everyday, complex tasks
//	    2.  Opus 5.5               For complex work and everyday tasks
//	  ❯ 5.  Haiku 5.5 ✔            Fastest for quick answers     ← ❯ focus, ✔ current
//	  ↓ 7.  Sonnet 5               Efficient for routine tasks   ← ↓ more below
//	     … +6 models
//	  ◐ Medium effort (default) ←/→ to adjust                    ← the effort word
//	  Enter to set as default · s to use this session only · Esc to cancel
//
// Rows are numbered over the whole list, so a row is named by its number even
// while the window hides it. The steps:
//
//	/model ⏎ ──► picker up? ── no ──► stop (nothing to close)
//	               │ search mode (2.1.283 opened in it)? ──► ↓ into the list
//	               │ no "use this session only" in the footer ──► Esc, stop
//	               ▼
//	          model set? ── Home, read every row (↓ past each window), find
//	               │        the row, ↑/↓ to it, check ❯ landed there
//	               ▼
//	          effort set? ── ←/→ one press at a time until the word matches;
//	               │         a press that doesn't move it, or one that jumps
//	               │         past it, means this model has no such level: Esc, stop
//	               ▼
//	          s ──► "Set model to … for this session only"  ──► done
//	                "… saved as your default"               ──► stop, say so
//	                "Switch model?" / "Change effort level?" ──► Yes (Enter), as
//	                                                           applyPaneSetup does
//	                "Switch to Fable?" (usage credits)      ──► stop, left up
//
// Every refusal after the picker opened presses Esc first, which cancels the
// picker and saves nothing, so a refused drop leaves the pane as it found it.

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/rohanthewiz/cats/wire"
)

// The waits are variables only so tests can shrink them; nothing else sets them.
var (
	// pickerOpenWait is how long /model gets to draw its picker. It opens
	// without a network round trip, so this is only slack for a busy pane.
	pickerOpenWait = 3 * time.Second
	// pickerStepWait bounds one key's redraw. A key that hasn't changed the
	// picker by then didn't land (a remapped binding, a list end).
	pickerStepWait = time.Second
	// pickerOutcomeWait is how long `s` gets to report. The switch can run
	// PreModelSwitch hooks first ("Running PreModelSwitch hooks…"), which are
	// the user's own commands, so this is generous.
	pickerOutcomeWait = 10 * time.Second
)

const (
	// pickerPoll is the gap between captures while waiting on a redraw.
	pickerPoll = 40 * time.Millisecond
	// pickerMaxSteps bounds every loop that presses a key and looks again,
	// so a picker that stops answering ends the drive rather than spinning.
	pickerMaxSteps = 24
)

// effortOrder is the order the picker's ←/→ walk, low to high. A model that
// lacks a level skips it (xhigh and max are per model), which is how an
// overshoot is recognised.
var effortOrder = []string{effortLow, effortMedium, effortHigh, effortXHigh, effortMax}

// pickerRow is one model row of the picker.
type pickerRow struct {
	num     int    // the row's number over the whole list ("5." → 5)
	label   string // the model's name, without the ✔ ("Haiku 5.5")
	current bool   // ✔: the pane's model now
	focused bool   // ❯: what `s` would pick
}

// pickerState is the model picker as one capture shows it.
type pickerState struct {
	open        bool // header and footer seen, footer on the last line drawn
	search      bool // the footer says "Type to search/filter" (2.1.283's opening mode)
	sessionOnly bool // the footer offers "use this session only"
	rows        []pickerRow
	moreBelow   bool   // a ↓ row or a "… +N models" line: rows hidden below
	effort      string // the effort word folded to our spelling, "" when none is drawn
	noEffort    bool   // "Effort not supported" for the focused model
}

// focused returns the row the picker's ❯ is on.
func (s pickerState) focused() (pickerRow, bool) {
	for _, r := range s.rows {
		if r.focused {
			return r, true
		}
	}
	return pickerRow{}, false
}

var (
	// pickerRowRe reads one row: an optional marker (❯ focus, ↑/↓ scroll),
	// the row number, then the label and description, which the picker
	// separates by a run of at least two spaces.
	pickerRowRe    = regexp.MustCompile(`^\s*([❯↑↓])?\s*(\d+)\.\s+(.*)$`)
	pickerColGap   = regexp.MustCompile(`\s{2,}`)
	pickerMoreRe   = regexp.MustCompile(`^\s*…\s*\+\d+`)
	pickerEffortRe = regexp.MustCompile(`^\s*\S+\s+(Low|Medium|High|xHigh|Max) effort\b`)
	// pickerEchoRe is the input line /model leaves in the transcript. The
	// outcome is read below the last one, so an older "Set model to …" further
	// up (an earlier drop's) can't be taken for this one's.
	pickerEchoRe = regexp.MustCompile(`^\s*[❯>]\s*/model\s*$`)
	// pickerConsentRe is the title of the usage-credits consent the picker
	// raises for a model that bills them (2.1.318: "Switch to Fable?"). It is
	// a spending decision, so it is never answered from here (N-044).
	pickerConsentRe = regexp.MustCompile(`Switch to [A-Z][^?\n]*\?|uses usage credits`)
	// switchConfirmRe is switchConfirmPattern compiled once for the polls.
	switchConfirmRe = regexp.MustCompile(switchConfirmPattern)
)

// parsePicker reads the model picker off a capture of the pane's screen. Only
// the lines below the last "Select model" header are read, so a numbered list
// in the conversation above can't be taken for model rows, and the picker
// counts as open only while its footer is the last line drawn: the picker
// replaces the input box at the bottom of the screen, so conversation text that
// merely quotes it is never at the bottom.
func parsePicker(screen string) pickerState {
	lines := strings.Split(strings.ReplaceAll(screen, "\r\n", "\n"), "\n")
	last := len(lines) - 1
	for last >= 0 && strings.TrimSpace(lines[last]) == "" {
		last--
	}
	head := -1
	for i := last; i >= 0; i-- {
		if strings.Contains(lines[i], "Select model") {
			head = i
			break
		}
	}
	var s pickerState
	if head < 0 {
		return s
	}
	for i := head + 1; i <= last; i++ {
		line := lines[i]
		switch {
		case pickerRowRe.MatchString(line):
			m := pickerRowRe.FindStringSubmatch(line)
			num, _ := strconv.Atoi(m[2])
			label := strings.TrimSpace(pickerColGap.Split(strings.TrimSpace(m[3]), 2)[0])
			row := pickerRow{num: num, focused: m[1] == "❯"}
			if l, ok := strings.CutSuffix(label, "✔"); ok {
				row.current, label = true, strings.TrimSpace(l)
			}
			row.label = label
			if m[1] == "↓" {
				s.moreBelow = true
			}
			s.rows = append(s.rows, row)
		case pickerMoreRe.MatchString(line):
			s.moreBelow = true
		case pickerEffortRe.MatchString(line):
			s.effort = strings.ToLower(pickerEffortRe.FindStringSubmatch(line)[1])
		case strings.Contains(line, "Effort not supported"):
			s.noEffort = true
		}
	}
	footer := lines[last]
	s.search = strings.Contains(footer, "Type to search") || strings.Contains(footer, "Type to filter")
	s.sessionOnly = strings.Contains(footer, "use this session only")
	s.open = strings.Contains(strings.ToLower(footer), "esc to")
	return s
}

// pickerLabelFor turns the model a prompt asks for into the row label it
// expects in the picker, and whether that label is a family to take the first
// (newest) row of rather than an exact name:
//
//	"default"             → "Default (recommended)" row (exact prefix)
//	"opus" "sonnet" …     → the first row of that family (the picker lists a
//	                        family newest first, which is what the alias means)
//	"claude-opus-5-5"     → "Opus 5.5"; a trailing -YYYYMMDD date is dropped
//
// Anything else is matched as a label as it stands, case aside. A model the
// picker has no row for is refused rather than guessed at: picking a near
// neighbour would run the prompt on a model nobody chose.
func pickerLabelFor(model string) (label string, family bool) {
	m := strings.ToLower(strings.TrimSpace(model))
	switch m {
	case "default":
		return "default", true
	case "opus", "sonnet", "haiku", "fable":
		return m, true
	}
	if rest, ok := strings.CutPrefix(m, "claude-"); ok {
		parts := strings.Split(rest, "-")
		if n := len(parts); n > 1 && len(parts[n-1]) == 8 && isDigits(parts[n-1]) {
			parts = parts[:n-1]
		}
		if len(parts) >= 2 && !isDigits(parts[0]) {
			var nums []string
			for _, p := range parts[1:] {
				if !isDigits(p) {
					return m, false
				}
				nums = append(nums, p)
			}
			return parts[0] + " " + strings.Join(nums, "."), false
		}
	}
	return m, false
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// pickerRowFor finds the row for model among every row of the list, in list
// order. A family takes its first row by the label's first word, never the
// "Default (recommended)" row, whose description also names a model.
func pickerRowFor(rows []pickerRow, model string) (pickerRow, bool) {
	want, family := pickerLabelFor(model)
	for _, r := range rows {
		l := strings.ToLower(r.label)
		switch {
		case want == "default":
			if strings.HasPrefix(l, "default") {
				return r, true
			}
		case family:
			if first, _, _ := strings.Cut(l, " "); first == want {
				return r, true
			}
		case l == want:
			return r, true
		}
	}
	return pickerRow{}, false
}

// sessionOnlyKey is the key that presses modelPicker:thisSessionOnly in the
// user's Claude Code: `s` by default, or whatever keybindings.json moved it to
// (the ModelPicker context). It is read before anything is sent, because the
// one thing this road must never do is press a key that has become something
// else — in particular a key bound to select:accept, which is Enter's "set as
// default". So:
//
//	no keybindings.json                       → s
//	a key bound to modelPicker:thisSessionOnly → that key (a letter or digit,
//	                                            ctrl/alt/shift allowed)
//	s rebound or unbound, nothing else bound  → refuse
//	s bound by the user in the Select context → refuse (it may outrank ours)
//	a file that doesn't parse                 → refuse: what Claude Code makes
//	                                            of it can't be known from here
func sessionOnlyKey(path string) (wire.KeyStroke, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return wire.KeyStroke{Text: "s"}, nil
	}
	if err != nil {
		return wire.KeyStroke{}, fmt.Errorf("reading %s: %w", path, err)
	}
	var file struct {
		Bindings []struct {
			Context  string             `json:"context"`
			Bindings map[string]*string `json:"bindings"`
		} `json:"bindings"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		return wire.KeyStroke{}, fmt.Errorf("%s doesn't parse, so the picker's session-only key can't be known: %w", path, err)
	}
	// Later blocks win, as later keys in one object would.
	picker := map[string]*string{}
	selectCtx := map[string]*string{}
	for _, b := range file.Bindings {
		dst := picker
		switch b.Context {
		case "ModelPicker":
		case "Select":
			dst = selectCtx
		default:
			continue
		}
		for k, v := range b.Bindings {
			dst[strings.ToLower(strings.TrimSpace(k))] = v
		}
	}
	key := "s"
	if v, ok := picker["s"]; ok && (v == nil || *v != "modelPicker:thisSessionOnly") {
		key = ""
	}
	// A key the user bound to the action outright is the surest, and the
	// first in sorted order keeps the choice stable between runs.
	var bound []string
	for k, v := range picker {
		if v != nil && *v == "modelPicker:thisSessionOnly" {
			bound = append(bound, k)
		}
	}
	slices.Sort(bound)
	if len(bound) > 0 {
		key = bound[0]
	}
	if key == "" {
		return wire.KeyStroke{}, fmt.Errorf("%s rebinds s in the model picker and binds nothing to modelPicker:thisSessionOnly", path)
	}
	if _, ok := selectCtx[key]; ok {
		return wire.KeyStroke{}, fmt.Errorf("%s binds %q in the Select context too, so it may not reach the model picker's session-only pick", path, key)
	}
	ks, ok := keyStrokeFor(key)
	if !ok {
		return wire.KeyStroke{}, fmt.Errorf("%s binds the session-only pick to %q, which cats-todo can't press", path, key)
	}
	return ks, nil
}

// keyStrokeFor turns a keybindings.json chord ("s", "ctrl+x") into a stroke.
// Only one letter or digit, with ctrl/alt/shift, is accepted: those are the
// chords that can't mean anything else on the way through a terminal.
func keyStrokeFor(chord string) (wire.KeyStroke, bool) {
	parts := strings.Split(chord, "+")
	base := parts[len(parts)-1]
	if len(base) != 1 || !(base[0] >= 'a' && base[0] <= 'z' || base[0] >= '0' && base[0] <= '9') {
		return wire.KeyStroke{}, false
	}
	var mods uint8
	for _, m := range parts[:len(parts)-1] {
		switch m {
		case "ctrl", "control":
			mods |= wire.ModCtrl
		case "alt", "opt", "option", "meta":
			mods |= wire.ModAlt
		case "shift":
			mods |= wire.ModShift
		default:
			return wire.KeyStroke{}, false
		}
	}
	if mods == 0 {
		return wire.KeyStroke{Text: base}, true
	}
	code := "Key" + strings.ToUpper(base)
	if base[0] <= '9' {
		code = "Digit" + base
	}
	return wire.KeyStroke{Code: code, Mods: mods}, true
}

// claudeKeybindingsPath is where Claude Code reads keybindings.json: under
// CLAUDE_CONFIG_DIR when it is set, else ~/.claude. The pane's own claude could
// in principle have been started with another config dir; this is the one a
// pane started from this environment would read.
func claudeKeybindingsPath() string {
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, "keybindings.json")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "keybindings.json")
}

// Named keys the drive presses.
var (
	keyDown  = wire.KeyStroke{Code: "ArrowDown"}
	keyUp    = wire.KeyStroke{Code: "ArrowUp"}
	keyLeft  = wire.KeyStroke{Code: "ArrowLeft"}
	keyRight = wire.KeyStroke{Code: "ArrowRight"}
	keyHome  = wire.KeyStroke{Code: "Home"}
	keyEsc   = wire.KeyStroke{Code: "Escape"}
)

// pickerDrive is one run of the picker: the pane, the driver, and the last
// state read, so each step decides from what the screen said after the
// previous one.
type pickerDrive struct {
	d    paneDriver
	pane uint32
	st   pickerState
}

// read captures the screen and parses it.
func (p *pickerDrive) read() (string, error) {
	screen, err := p.d.capture(p.pane)
	if err != nil {
		return "", err
	}
	p.st = parsePicker(screen)
	return screen, nil
}

// await reads until cond holds or wait runs out, reporting which. The first
// read is immediate, so a picker already in the wanted state costs one capture.
func (p *pickerDrive) await(wait time.Duration, cond func(pickerState) bool) (bool, error) {
	deadline := time.Now().Add(wait)
	for {
		if _, err := p.read(); err != nil {
			return false, err
		}
		if cond(p.st) {
			return true, nil
		}
		if time.Now().After(deadline) {
			return false, nil
		}
		time.Sleep(pickerPoll)
	}
}

// press sends keys in one call, then waits for cond.
func (p *pickerDrive) press(cond func(pickerState) bool, keys ...wire.KeyStroke) (bool, error) {
	if err := p.d.sendKeys(p.pane, keys); err != nil {
		return false, err
	}
	return p.await(pickerStepWait, cond)
}

// refuse cancels the picker with Esc (which saves nothing) and returns why the
// drive stopped. If the picker is still up afterwards the error says so, since
// the prompt is not sent and the pane needs the user.
func (p *pickerDrive) refuse(format string, args ...any) error {
	why := fmt.Sprintf(format, args...)
	closed, err := p.press(func(s pickerState) bool { return !s.open }, keyEsc)
	if err != nil || !closed {
		return fmt.Errorf("%s — and the /model picker is still open in the pane: press Esc there; the prompt was not sent", why)
	}
	return fmt.Errorf("%s; the picker was cancelled and the prompt was not sent", why)
}

// focusedNum is the ❯ row's number, or 0 when no row is focused.
func focusedNum(s pickerState) int {
	if r, ok := s.focused(); ok {
		return r.num
	}
	return 0
}

// pickSessionOnly switches a running Claude Code pane to model and effort for
// this session only (either may be empty, not both), through the /model
// picker. It returns a note for the drop's status line and an error that stops
// the drop before the prompt is sent. settle is spent after the switch, as
// applyPaneSetup spends it after each command, so the prompt's first keys are
// read by the input box rather than lost to the redraw.
func pickSessionOnly(d paneDriver, pane uint32, model, effort string, settle time.Duration) (string, error) {
	if model == "" && effort == "" {
		return "", nil
	}
	what := strings.TrimSpace(strings.Join([]string{model, effortWords(effort)}, " "))
	// Everything that can refuse without touching the pane goes first.
	key, err := sessionOnlyKey(claudeKeybindingsPath())
	if err != nil {
		return "", fmt.Errorf("not switching the pane to %s for this session: %w", what, err)
	}
	// An empty send_keys is refused by a server that has the verb ("no
	// keys"), and answered "not supported" by one that doesn't — a probe that
	// types nothing either way. Without the verb the picker could be opened
	// but never steered or closed, so it isn't opened.
	if err := d.sendKeys(pane, nil); errors.Is(err, errNoSendKeys) {
		return "", fmt.Errorf("this cats server can't press keys (no pane.send_keys), which switching the pane to %s for this session needs — update cats, or clear the prompt's model and effort", what)
	}

	p := &pickerDrive{d: d, pane: pane}
	if err := d.sendInput(pane, "/model", true); err != nil {
		return "", fmt.Errorf("opening /model in the pane: %w", err)
	}
	if up, err := p.await(pickerOpenWait, func(s pickerState) bool { return s.open }); err != nil || !up {
		// Nothing is known to be open, so there is nothing to cancel.
		return "", fmt.Errorf("the /model picker did not open in the pane, so %s was not applied and the prompt was not sent", what)
	}
	// 2.1.283's picker opened in a search box, where `s` is typed into the
	// filter; ↓ leaves it for the list. 2.1.318 opens on the list.
	if p.st.search {
		if ok, err := p.press(func(s pickerState) bool { return !s.search }, keyDown); err != nil || !ok {
			return "", p.refuse("the /model picker stayed in its search box")
		}
	}
	// No session-only pick in the footer: an older Claude Code, or a picker
	// opened in some other mode. Enter is the only way out that applies, and
	// Enter saves, so the drive stops here.
	if !p.st.sessionOnly {
		return "", p.refuse("this Claude Code's /model picker offers no session-only pick (its footer has no \"use this session only\"), and Enter would save %s as your default", what)
	}

	if model != "" {
		if err := p.focusModel(model); err != nil {
			return "", err
		}
	}
	if effort != "" {
		if err := p.setEffort(effort); err != nil {
			return "", err
		}
	}
	return p.commit(key, what, settle)
}

// effortWords is "low effort" for "low", "" for "".
func effortWords(effort string) string {
	if effort == "" {
		return ""
	}
	return effort + " effort"
}

// focusModel moves the picker's ❯ onto model's row. It reads the whole list
// first: Home, then a ↓ past the last visible row for each window, since a
// family alias means the family's first row in the whole list and the window
// may start below it. Then it moves by the row numbers and checks ❯ landed.
func (p *pickerDrive) focusModel(model string) error {
	if ok, err := p.press(func(s pickerState) bool { return focusedNum(s) == 1 }, keyHome); err != nil || !ok {
		return p.refuse("Home didn't reach the top of the /model picker")
	}
	all := map[int]pickerRow{}
	for range pickerMaxSteps {
		lastVisible := 0
		for _, r := range p.st.rows {
			all[r.num] = r
			lastVisible = max(lastVisible, r.num)
		}
		if !p.st.moreBelow {
			break
		}
		from := focusedNum(p.st)
		n := lastVisible + 1 - from
		if from == 0 || n <= 0 {
			return p.refuse("the /model picker's rows couldn't be read")
		}
		want := lastVisible + 1
		if ok, err := p.press(func(s pickerState) bool { return focusedNum(s) == want }, repeatKey(keyDown, n)...); err != nil || !ok {
			return p.refuse("the /model picker didn't scroll to row %d", want)
		}
	}
	rows := make([]pickerRow, 0, len(all))
	for _, r := range all {
		rows = append(rows, r)
	}
	slices.SortFunc(rows, func(a, b pickerRow) int { return a.num - b.num })
	target, ok := pickerRowFor(rows, model)
	if !ok {
		return p.refuse("the /model picker has no row for %q", model)
	}
	from := focusedNum(p.st)
	if from != target.num {
		k, n := keyDown, target.num-from
		if n < 0 {
			k, n = keyUp, -n
		}
		if ok, err := p.press(func(s pickerState) bool { return focusedNum(s) == target.num }, repeatKey(k, n)...); err != nil || !ok {
			return p.refuse("the /model picker didn't land on %s", target.label)
		}
	}
	if r, _ := p.st.focused(); r.label != target.label {
		return p.refuse("the /model picker's row %d is %s now, not %s", target.num, r.label, target.label)
	}
	return nil
}

// setEffort walks the picker's effort word to effort one press at a time. It
// can't jump: the picker has no absolute position for effort, and the levels a
// model offers are the model's (xhigh and max come and go), so the next press
// is chosen from the word on screen each time.
func (p *pickerDrive) setEffort(effort string) error {
	want := slices.Index(effortOrder, effort)
	if want < 0 {
		return p.refuse("%q is not an effort level the picker has", effort)
	}
	if p.st.noEffort {
		r, _ := p.st.focused()
		return p.refuse("%s takes no effort setting", r.label)
	}
	dir := 0
	for range pickerMaxSteps {
		at := slices.Index(effortOrder, p.st.effort)
		if at < 0 {
			return p.refuse("the /model picker's effort line couldn't be read")
		}
		if at == want {
			return nil
		}
		step := 1
		if want < at {
			step = -1
		}
		// The direction flipping means the last press jumped over the level:
		// this model doesn't offer it.
		if dir != 0 && step != dir {
			r, _ := p.st.focused()
			return p.refuse("%s has no %s effort", r.label, effort)
		}
		dir = step
		k := keyRight
		if step < 0 {
			k = keyLeft
		}
		before := p.st.effort
		moved, err := p.press(func(s pickerState) bool { return s.effort != before }, k)
		if err != nil {
			return p.refuse("pressing %s in the /model picker: %v", k.Code, err)
		}
		if !moved {
			// Clamped at an end: the level is past what this model offers.
			r, _ := p.st.focused()
			return p.refuse("%s has no %s effort", r.label, effort)
		}
	}
	return p.refuse("the /model picker's effort didn't settle on %s", effort)
}

// repeatKey is n copies of k, for one send_keys call.
func repeatKey(k wire.KeyStroke, n int) []wire.KeyStroke {
	keys := make([]wire.KeyStroke, n)
	for i := range keys {
		keys[i] = k
	}
	return keys
}

// pickerOutcome is what the screen says after the session-only key.
type pickerOutcome int

const (
	outcomePending pickerOutcome = iota // picker still up, or nothing new yet
	outcomeSession                      // "Set model to … for this session only"
	outcomeSaved                        // "… saved as your default": the one thing this road exists to avoid
	outcomeOther                        // a result line of another kind ("Kept model as …", a blocked switch)
	outcomeConfirm                      // the Switch model? / Change effort level? dialog
	outcomeHookAsk                      // the same dialog, asked by a PreModelSwitch hook
	outcomeConsent                      // the usage-credits consent ("Switch to Fable?")
)

// readOutcome classifies the screen below the last `/model` echo. The result
// line Claude Code prints sits right under the echo; the dialogs replace the
// picker at the bottom. Only those rows are read, so text further up — an
// earlier drop's result, a conversation quoting the dialogs — can't match.
func readOutcome(screen string) (pickerOutcome, string) {
	if parsePicker(screen).open {
		return outcomePending, ""
	}
	lines := strings.Split(strings.ReplaceAll(screen, "\r\n", "\n"), "\n")
	echo := -1
	for i := len(lines) - 1; i >= 0; i-- {
		if pickerEchoRe.MatchString(lines[i]) {
			echo = i
			break
		}
	}
	if echo < 0 {
		return outcomePending, ""
	}
	below := strings.Join(lines[echo+1:], "\n")
	switch {
	case strings.Contains(below, switchHookMarker):
		return outcomeHookAsk, ""
	case switchConfirmRe.MatchString(below):
		return outcomeConfirm, ""
	case pickerConsentRe.MatchString(below):
		return outcomeConsent, ""
	}
	for i := echo + 1; i < len(lines); i++ {
		t := resultLine(lines[i:])
		switch {
		case strings.HasPrefix(t, "Set model to ") && strings.Contains(t, "saved as your default"):
			return outcomeSaved, t
		case strings.HasPrefix(t, "Set model to ") && strings.Contains(t, "for this session only"):
			return outcomeSession, t
		case strings.HasPrefix(t, "Set model to "), strings.HasPrefix(t, "Kept model as "),
			strings.HasPrefix(t, "Model switch "), strings.HasPrefix(t, "Model unchanged"):
			return outcomeOther, t
		}
	}
	return outcomePending, ""
}

// resultLine is the ⎿ line at the top of lines, with the rows Claude Code
// wrapped it onto joined back: Ink breaks a long line itself, so a capture's
// unwrap can't rejoin it, and in a narrow pane "for this session only" can land
// on the second row. A continuation is an indented row that is not blank, not a
// rule, not the input box and not another ⎿ line.
func resultLine(lines []string) string {
	t := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(lines[0]), "⎿"))
	for _, l := range lines[1:] {
		c := strings.TrimSpace(l)
		if c == "" || strings.HasPrefix(c, "─") || strings.HasPrefix(c, "❯") ||
			strings.HasPrefix(c, ">") || strings.HasPrefix(c, "⎿") || !strings.HasPrefix(l, " ") {
			break
		}
		t += " " + c
	}
	return t
}

// commit presses the session-only key and reads what came of it. The switch
// confirm is answered Yes with a bare Enter — that dialog is not the picker,
// and its Yes applies the pick the way `s` chose it, session only — exactly as
// applyPaneSetup answers it for a typed switch. A hook's ask and the usage
// credits consent are the user's to answer, so the drop stops with them up.
func (p *pickerDrive) commit(key wire.KeyStroke, what string, settle time.Duration) (string, error) {
	if err := p.d.sendKeys(p.pane, []wire.KeyStroke{key}); err != nil {
		return "", p.refuse("pressing the session-only key in the /model picker: %v", err)
	}
	confirmed := false
	deadline := time.Now().Add(pickerOutcomeWait)
	for {
		screen, err := p.read()
		if err != nil {
			return "", fmt.Errorf("reading the pane after the session-only pick: %w", err)
		}
		out, line := readOutcome(screen)
		switch out {
		case outcomeSession:
			time.Sleep(settle)
			note := strings.Replace(line, "Set model to ", "set ", 1)
			if confirmed {
				note += " (confirmed the switch: the conversation is re-read uncached)"
			}
			return note, nil
		case outcomeSaved:
			return "", fmt.Errorf("the pane reported %q — it was saved as your default, not for this session only; check ~/.claude/settings.json. The prompt was not sent", line)
		case outcomeOther:
			return "", fmt.Errorf("the pane answered %q instead of switching to %s for this session; the prompt was not sent", line, what)
		case outcomeHookAsk:
			return "", fmt.Errorf("the pane is asking to confirm the switch to %s (a PreModelSwitch hook) — answer it there; the prompt was not sent", what)
		case outcomeConsent:
			return "", fmt.Errorf("the pane asks for consent to a model that uses usage credits — answer it there; the prompt was not sent")
		case outcomeConfirm:
			if confirmed {
				// Still up after the Yes: give the redraw a moment, below.
				break
			}
			if err := p.d.sendInput(p.pane, "", true); err != nil {
				return "", fmt.Errorf("confirming the switch to %s in the pane: %w", what, err)
			}
			confirmed = true
		}
		if time.Now().After(deadline) {
			if p.st.open {
				return "", p.refuse("the session-only key didn't close the /model picker")
			}
			return "", fmt.Errorf("the pane didn't report the switch to %s within %s; the prompt was not sent", what, pickerOutcomeWait)
		}
		time.Sleep(pickerPoll)
	}
}
