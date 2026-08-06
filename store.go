package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// errTodoNotFound reports a mutation aimed at a todo that no longer exists —
// typically it was deleted or completed from another manager pane after this
// pane last loaded. Surfacing it keeps a stale pane honest instead of quietly
// claiming success.
var errTodoNotFound = errors.New("todo not found (changed from another pane?)")

// Todo is one saved prompt of future work.
type Todo struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Prompt string `json:"prompt"`
	// Images are the todo's attachments, as paths relative to the backlog
	// file's directory (see images.go). Omitted from the JSON when empty, so a
	// backlog without attachments reads exactly as it did before the field
	// existed — and an older binary ignores the field rather than choking.
	Images  []string  `json:"images,omitempty"`
	Done    bool      `json:"done"`
	Created time.Time `json:"created"`
	// Frozen marks a prompt the user has decided not to do — kept on the record
	// rather than deleted, and deliberately not the same flag as Done: done
	// claims the work happened, and a backlog that says so about work nobody
	// ever did is a backlog that lies. The two are mutually exclusive (see
	// setFrozen and setDone), so every todo is in exactly one of three states —
	// open, frozen, done — which is what the list renders as three groups.
	//
	// Same compat contract as Images and Schedule: omitted from the JSON when
	// false, so an unfrozen backlog reads exactly as it did before the field
	// existed, and an older binary sees a plain open todo rather than choking.
	Frozen bool `json:"frozen,omitempty"`
	// Schedule is the todo's pending one-shot auto-drop, nil when none. Same
	// compat contract as Images: omitted from the JSON when absent, so an
	// unscheduled backlog reads exactly as it did before the field existed and
	// an older binary ignores it. (An older binary that saves the backlog
	// drops the key — the same accepted limitation attachments carry.)
	Schedule *Schedule `json:"schedule,omitempty"`
}

// Schedule is a one-shot auto-drop waiting to fire: at time At, the todo's
// prompt is delivered to the recorded target as if the user had pressed "drop
// and run". Kind is a string rather than the UI's dropTargetKind int so the
// JSON stays self-describing and survives any reordering of that enum.
type Schedule struct {
	At   time.Time `json:"at"`
	Kind string    `json:"kind"` // scheduleKindPane | scheduleKindNew
	// Pane and Agent describe an existing-pane target: the cats pane ID the
	// prompt lands in, and its agent label for status lines. Panes are
	// ephemeral, so the ID is re-verified at fire time (see
	// performScheduledDrop).
	Pane  uint32 `json:"pane,omitempty"`
	Agent string `json:"agent,omitempty"`
	// Command and Cwd describe a new-session target: the agent command to
	// launch ("claude", …) and the directory to launch it in, captured at
	// schedule time because the fire happens with no form on screen to ask.
	Command string `json:"command,omitempty"`
	Cwd     string `json:"cwd,omitempty"`
	// Worktree marks a new-session target that cuts a fresh git checkout to
	// launch in (see worktree.go). Recorded rather than resolved at schedule
	// time: the branch is named and the checkout made when the drop fires, so
	// it branches off HEAD as it stands then, not hours earlier. Same compat
	// contract as the fields above — omitted when false, so an older binary
	// reads the schedule as the plain new-session drop it otherwise is.
	Worktree bool `json:"worktree,omitempty"`
	// Missed marks a schedule that came due when it couldn't fire — the
	// manager was closed past the grace window, the pane was gone, the drop
	// failed. The tick skips it, the row shows it, and sending by hand (which
	// completes the todo) is what clears it.
	Missed bool `json:"missed,omitempty"`
}

// Schedule target kinds. Stored in backlog files — the names are wire format,
// not just code.
const (
	scheduleKindPane = "pane"
	scheduleKindNew  = "new"
)

// The three states a todo can be in, in the order the list draws them within a
// scope. The numbering is the render order and nothing else — it is not stored,
// so it can be reordered freely.
const (
	groupOpen   = iota // still to do
	groupFrozen        // decided against, kept for the record
	groupDone          // finished
)

// group is which of the three render groups this todo belongs to. Done outranks
// Frozen so that a backlog hand-edited into claiming both still lands in exactly
// one group — the renderer walks the groups in separate passes, and a todo that
// answered yes twice would otherwise be drawn twice.
func (t Todo) group() int {
	switch {
	case t.Done:
		return groupDone
	case t.Frozen:
		return groupFrozen
	}
	return groupOpen
}

// closed reports whether a todo has left the active backlog — finished or
// abandoned. Everything that asks "is there work here" (the fold, the header's
// hidden count, the pane title's open count) asks it this way: neither state is
// work outstanding, and only the row itself needs to tell the two apart.
func (t Todo) closed() bool { return t.Done || t.Frozen }

// scope marks where a todo lives: with the project (committed in the repo) or in
// the user's global backlog.
type scope int

const (
	// scopeProject is a todo stored in <workDir>/.cats-todo/todos.json — it
	// travels with the repo and is only visible when you launch from that project.
	scopeProject scope = iota
	// scopeGlobal is a todo stored in the user's cats-todo config dir, visible
	// from anywhere.
	scopeGlobal
)

func (s scope) String() string {
	if s == scopeProject {
		return "Project"
	}
	return "Global"
}

// projectConfigDirName is the directory a repo gets for its cats-todo config.
const projectConfigDirName = ".cats-todo"

// configDirEnvVar overrides the global-backlog config directory outright —
// mostly for tests and for anyone who keeps their backlog somewhere synced.
const configDirEnvVar = "CATS_TODO_CONFIG_DIR"

// configBaseDir returns the root config directory for cats-todo's global
// backlog: the CATS_TODO_CONFIG_DIR override when set, else
// $XDG_CONFIG_HOME/cats-todo, then ~/.config/cats-todo.
func configBaseDir() (string, error) {
	if d := os.Getenv(configDirEnvVar); d != "" {
		return d, nil
	}
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "cats-todo"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "cats-todo"), nil
}

// globalTodosPath returns the path to the global backlog file.
func globalTodosPath() (string, error) {
	base, err := configBaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "todos.json"), nil
}

// projectTodosPath returns the project backlog file within workDir, or "" when
// workDir is unknown (no project scope is available in that case).
func projectTodosPath(workDir string) string {
	if workDir == "" {
		return ""
	}
	return filepath.Join(workDir, projectConfigDirName, "todos.json")
}

// store is a file-backed list of todos for one scope. A store with an empty path
// is "unavailable" (e.g. the project store when launched outside a project): it
// loads and saves to nothing and holds no todos.
type store struct {
	scope scope
	path  string
	todos []Todo
}

// available reports whether this scope has a backing file (a project store has
// none when there is no project directory).
func (s *store) available() bool { return s.path != "" }

// load reads the store's file into s.todos. A missing file is not an error — it
// just yields an empty list, which is the natural first-run state.
func (s *store) load() error {
	s.todos = nil
	if s.path == "" {
		return nil
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, &s.todos)
}

// save writes s.todos back to its file, creating the parent directory as needed.
// It writes to a temp file in the same directory and renames it over the target,
// so a crash mid-write can never truncate the backlog — the file is always
// either the old contents or the new, never half of one. It is a no-op for an
// unavailable store.
func (s *store) save() error {
	if s.path == "" {
		return nil
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.todos, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(dir, ".todos-*.tmp")
	if err != nil {
		return err
	}
	if _, err = tmp.Write(data); err == nil {
		err = tmp.Chmod(0o644)
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), s.path)
	}
	if err != nil {
		os.Remove(tmp.Name())
	}
	return err
}

// reload refreshes s.todos from disk so a mutation starts from the latest
// state. Every manager pane shares the global backlog (and long-lived panes can
// share a project one), so mutating from a stale in-memory copy would silently
// clobber what another pane wrote. An unavailable store keeps its in-memory
// list — load would wipe it.
func (s *store) reload() error {
	if s.path == "" {
		return nil
	}
	return s.load()
}

// add appends a new todo and persists.
func (s *store) add(t Todo) error {
	if err := s.reload(); err != nil {
		return err
	}
	s.todos = append(s.todos, t)
	return s.save()
}

// update replaces the todo with the same ID (title/prompt) and persists.
//
// The field-by-field copy is deliberate rather than a whole-struct assignment:
// the caller is the edit form, which knows only the two fields it showed. Done,
// Created and Images stay as they are on disk, so editing a prompt's text can
// never silently drop its attachments — or resurrect ones deleted from another
// pane. Attaching and detaching are their own operations (see images.go).
func (s *store) update(t Todo) error {
	if err := s.reload(); err != nil {
		return err
	}
	for i := range s.todos {
		if s.todos[i].ID == t.ID {
			s.todos[i].Title = t.Title
			s.todos[i].Prompt = t.Prompt
			return s.save()
		}
	}
	return errTodoNotFound
}

// setImages replaces the attachment list of the todo with id and persists. It is
// separate from update so that update stays a text-only operation (see its
// comment): a caller that knows nothing about images cannot blank them, and a
// caller that does — the form's attachment editor — says so explicitly. It
// records paths only; the files themselves are the caller's business, since
// which ones to copy in or delete depends on what the edit did.
func (s *store) setImages(id string, images []string) error {
	if err := s.reload(); err != nil {
		return err
	}
	for i := range s.todos {
		if s.todos[i].ID == id {
			s.todos[i].Images = images
			return s.save()
		}
	}
	return errTodoNotFound
}

// delete removes the todo with id and persists, along with any attachments it
// owned. The files go only after the save succeeds: a backlog still listing a
// todo whose images we had already deleted is the worse of the two failures.
func (s *store) delete(id string) error {
	if err := s.reload(); err != nil {
		return err
	}
	for i, t := range s.todos {
		if t.ID == id {
			s.todos = append(s.todos[:i], s.todos[i+1:]...)
			if err := s.save(); err != nil {
				return err
			}
			s.removeImages(id)
			return nil
		}
	}
	return errTodoNotFound
}

// toggle flips the done flag of the todo with id and persists. Completing a
// todo clears its schedule: "schedule implies open" is the invariant the tick
// scans by, and a done todo auto-dropping later would be work resurrected from
// the grave. It also thaws a frozen todo — done and frozen are exclusive (see
// Todo.Frozen), and of the two, "this happened" is the one the user just
// asserted.
func (s *store) toggle(id string) error {
	if err := s.reload(); err != nil {
		return err
	}
	for i := range s.todos {
		if s.todos[i].ID == id {
			s.todos[i].Done = !s.todos[i].Done
			if s.todos[i].Done {
				s.todos[i].Schedule = nil
				s.todos[i].Frozen = false
				s.fileAsLatestDone(i)
			}
			return s.save()
		}
	}
	return errTodoNotFound
}

// toggleFrozen flips the frozen flag of the todo with id and persists,
// returning the state it landed in.
//
// The landing state is returned rather than left for the caller to infer from
// what it had on screen. The flip happens after a reload, so another pane may
// have frozen or thawed this todo since this one last drew the row, and a status
// line that reports the caller's guess would be confidently wrong exactly when
// the two panes disagree.
func (s *store) toggleFrozen(id string) (bool, error) {
	if err := s.reload(); err != nil {
		return false, err
	}
	for i := range s.todos {
		if s.todos[i].ID == id {
			frozen := !s.todos[i].Frozen
			s.freeze(i, frozen)
			return frozen, s.save()
		}
	}
	return false, errTodoNotFound
}

// setFrozen sets the frozen flag of the todo with id to frozen and persists.
// Unlike toggleFrozen it is idempotent — the no-op case skips the save — which
// is what a caller wants when it is enforcing a state rather than flipping one.
func (s *store) setFrozen(id string, frozen bool) error {
	if err := s.reload(); err != nil {
		return err
	}
	for i := range s.todos {
		if s.todos[i].ID == id {
			if s.todos[i].Frozen == frozen {
				return nil
			}
			s.freeze(i, frozen)
			return s.save()
		}
	}
	return errTodoNotFound
}

// freeze applies the frozen state to the todo at index i, without saving — the
// shared half of toggleFrozen and setFrozen.
//
// Freezing clears Done (the two states are exclusive) and any pending schedule:
// a prompt the user has decided not to run must not run itself later, which is
// the same reason completing clears one.
//
// Array position is deliberately left alone, and that is the one place freezing
// differs from completing (compare fileAsLatestDone). A completed prompt is
// finished and belongs on top of the pile of finished things; a frozen one has
// not happened — it is still work, holding its place in the backlog's priority
// order — so a thaw hands it back the position it had rather than dropping it at
// the end of the open group.
func (s *store) freeze(i int, frozen bool) {
	s.todos[i].Frozen = frozen
	if frozen {
		s.todos[i].Done = false
		s.todos[i].Schedule = nil
	}
}

// fileAsLatestDone moves the todo at index i to the head of the done group, so
// the completed list reads newest-first: what was just finished is what the user
// is looking for, and the pile it lands on only gets longer.
//
// Completion order lives in the array rather than in a timestamp on the todo,
// which keeps two things true. Nothing has to be migrated — an existing backlog
// keeps the order it has, and newly completed prompts stack on top of it. And
// ctrl+up/down still reorders done todos by hand (see move), which a view sorted
// by a completion time would silently undo on the next rebuild.
//
// The todos between the head of the done group and i slide down one, keeping
// their order among themselves — including any open todos caught in the middle,
// whose own order is what the list renders.
func (s *store) fileAsLatestDone(i int) {
	first := -1
	for j := range s.todos {
		if j != i && s.todos[j].Done {
			first = j
			break
		}
	}
	// No other done todo, or i already precedes them all: nothing to move.
	if first < 0 || first > i {
		return
	}
	td := s.todos[i]
	copy(s.todos[first+1:i+1], s.todos[first:i])
	s.todos[first] = td
}

// setDone sets the done flag of the todo with id to done and persists. Unlike
// toggle it is idempotent (a no-op when already in that state) — used to
// auto-complete a todo after a "run" drop without risk of reopening one that
// was already done.
func (s *store) setDone(id string, done bool) error {
	if err := s.reload(); err != nil {
		return err
	}
	for i := range s.todos {
		if s.todos[i].ID == id {
			if s.todos[i].Done == done {
				return nil
			}
			s.todos[i].Done = done
			if done {
				// Same invariants as toggle: a completed todo holds no schedule,
				// and is not also frozen.
				s.todos[i].Schedule = nil
				s.todos[i].Frozen = false
				s.fileAsLatestDone(i)
			}
			return s.save()
		}
	}
	return errTodoNotFound
}

// setSchedule replaces the schedule of the todo with id and persists; nil
// clears it. Passing a Schedule with Missed set is also how a failed fire is
// recorded — one write path for every schedule state the row can show.
func (s *store) setSchedule(id string, sc *Schedule) error {
	if err := s.reload(); err != nil {
		return err
	}
	for i := range s.todos {
		if s.todos[i].ID == id {
			s.todos[i].Schedule = sc
			return s.save()
		}
	}
	return errTodoNotFound
}

// claimSchedule atomically takes the schedule of the todo with id off the
// backlog, returning whether this caller won it. The claim is the double-fire
// guard: the schedule is cleared on disk before any drop is attempted, so a
// second manager pane sharing this backlog reloads, finds the schedule gone
// (or changed — a claim is only good for the exact At it read), and stands
// down. A fire that later fails is written back as Missed rather than
// re-armed, which keeps "claimed" meaning "will never fire on its own again".
func (s *store) claimSchedule(id string, at time.Time) (bool, error) {
	if err := s.reload(); err != nil {
		return false, err
	}
	for i := range s.todos {
		if s.todos[i].ID != id {
			continue
		}
		sc := s.todos[i].Schedule
		if sc == nil || !sc.At.Equal(at) {
			return false, nil
		}
		s.todos[i].Schedule = nil
		if err := s.save(); err != nil {
			return false, err
		}
		return true, nil
	}
	// The todo itself is gone — deleted from another pane. Nothing to fire.
	return false, nil
}

// move shifts the todo with id one step toward the front (delta < 0) or back
// (delta > 0) of the backlog, swapping only with neighbors in the same render
// group — mirroring the list view, which shows open todos first, then frozen,
// then done within each scope. Array order is the backlog's priority order, so
// the swap is persisted. Moving past the edge of the group is a quiet no-op.
//
// Matching on the group rather than on Done alone is what keeps the keystroke
// honest now that there are three of them: swapping a done todo with the frozen
// one above it would move the row nowhere the user can see, since the two are
// drawn in different passes.
func (s *store) move(id string, delta int) error {
	if err := s.reload(); err != nil {
		return err
	}
	cur := -1
	for i := range s.todos {
		if s.todos[i].ID == id {
			cur = i
			break
		}
	}
	if cur < 0 {
		return errTodoNotFound
	}
	step := 1
	if delta < 0 {
		step = -1
	}
	for j := cur + step; j >= 0 && j < len(s.todos); j += step {
		if s.todos[j].group() == s.todos[cur].group() {
			s.todos[cur], s.todos[j] = s.todos[j], s.todos[cur]
			return s.save()
		}
	}
	return nil
}

// clearDone removes every done todo and persists, returning how many were
// removed. Zero removals skip the save.
//
// Frozen todos are untouched, and that is the point of the state: freezing is a
// decision the user wanted to keep a record of, so the sweep that clears
// finished work must not quietly throw it away. Deleting one stays a
// deliberate, per-todo act (ctrl+x).
func (s *store) clearDone() (int, error) {
	if err := s.reload(); err != nil {
		return 0, err
	}
	kept := s.todos[:0]
	var clearedIDs []string
	for _, t := range s.todos {
		if t.Done {
			clearedIDs = append(clearedIDs, t.ID)
			continue
		}
		kept = append(kept, t)
	}
	s.todos = kept
	if len(clearedIDs) == 0 {
		return 0, nil
	}
	if err := s.save(); err != nil {
		return 0, err
	}
	// Same ordering as delete: the backlog is authoritative, so the files go
	// only once it no longer mentions them.
	for _, id := range clearedIDs {
		s.removeImages(id)
	}
	return len(clearedIDs), nil
}

// find returns a copy of the todo with id, and whether it was found.
func (s *store) find(id string) (Todo, bool) {
	for _, t := range s.todos {
		if t.ID == id {
			return t, true
		}
	}
	return Todo{}, false
}

// loadStores builds and loads the project and global stores for a launch
// context. The project store is keyed off ctx.projectDir(); when that is empty
// (no project) or the launch is global-only (--global), the project store is
// unavailable and only the global backlog shows. Symmetrically, a
// project-only launch (--project) withholds the global store — same
// unavailable-store mechanism, opposite side.
func loadStores(ctx RunContext) (project *store, global *store, err error) {
	projectPath := ""
	if ctx.Scope != launchGlobalOnly {
		projectPath = projectTodosPath(ctx.projectDir())
	}
	project = &store{scope: scopeProject, path: projectPath}
	if err = project.load(); err != nil {
		return nil, nil, err
	}

	globalPath := ""
	if ctx.Scope != launchProjectOnly {
		// Resolved only when the launch will actually use it: a project-only
		// launch must not fail over a missing home directory it never reads.
		if globalPath, err = globalTodosPath(); err != nil {
			return nil, nil, err
		}
	}
	global = &store{scope: scopeGlobal, path: globalPath}
	if err = global.load(); err != nil {
		return nil, nil, err
	}
	return project, global, nil
}
