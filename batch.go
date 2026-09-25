// batch.go — a batch: several prompts picked, ordered, and dropped as one unit.
//
// A single drop is one prompt → one target. A batch is the answer to "these
// five, onto five fresh worktrees, all on sonnet" without five trips through
// the target picker, and it leaves a record that the five went out together.
// This file holds the record, its store, and the pure logic a drop of it needs:
//
//	Batch            the record — what was picked, in delivery order, how it is
//	                 delivered, where to, and with which shared session options
//	batchStore       batches.json beside a backlog's todos.json
//	overlaySession   the batch's options laid over each prompt's own
//	combinedPrompt   the "one prompt, listed" body
//	batchWatch       the tick's cheap view of both files (fire times, ⧉ badges)
//
// The composer that builds one is batchcompose.go, the page that lists them is
// batches.go, and the dispatch is batchrun.go.
//
// Why a file of its own rather than a key in todos.json: a batch can mix
// project and global prompts, so it belongs to neither backlog, and keeping it
// out of todos.json is what leaves that file byte-identical for everyone who
// never builds one (the compatibility contract every Todo field is held to).
// It is JSON rather than an embedded database for the same reason todos.json
// is: a person can read it, and the load/save discipline — reload before every
// mutation, temp file and rename on every write — carries over unchanged.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// batchesFileName sits beside todos.json in the same directory, so a project's
// batches live in its .cats-todo/ and the global ones in the config dir.
const batchesFileName = "batches.json"

// The delivery modes. The empty string is "all at once" so that the common case
// writes nothing, the same empty-means-default rule SessionOpts keeps. The
// values are wire format: they are written to batches.json.
//
// "loop" (one prompt after another, each waiting on the last) is reserved for
// the loop delivery; this build neither offers nor fires it.
const (
	deliverEach     = ""         // every prompt its own new session (or worktree)
	deliverCombined = "combined" // one body, the prompts as numbered sections
)

// The states a batch record can be in. A batch is written as running before
// the first prompt goes, so a manager that dies mid-delivery leaves a record
// that says so rather than one that looks like it never started.
//
// The three states before "running" are the ones a batch can be edited in —
// nothing has been sent, so the record is still a plan rather than history:
//
//	scheduled ──fire (claimBatch)──► running ──last step──► done
//	    │  ▲
//	    │  └── edit / reschedule ──┐
//	    ├──► missed ───────────────┤   (too late to fire, or no socket)
//	    └──► unscheduled ──────────┘   (✕ Unschedule on the page)
//
// Every arrow out of scheduled is a compare-and-swap on the record's state and
// fire time (swapBatch), because two manager panes can be looking at the same
// batches.json, and only one of them may act on a given fire time.
const (
	batchScheduled   = "scheduled"
	batchMissed      = "missed"
	batchUnscheduled = "unscheduled"
	batchRunning     = "running"
	batchDone        = "done"
)

// Batch is one batch: its picks in delivery order, how and where they go, and
// what happened when they went.
type Batch struct {
	ID      string    `json:"id"`
	Name    string    `json:"name,omitempty"`
	Created time.Time `json:"created"`
	// Items are the batch's prompts in delivery order. The order is the user's
	// (dragged, or sorted once), and it is what "launch order" and "reading
	// order" mean for the two delivery modes.
	Items   []BatchItem `json:"items"`
	Deliver string      `json:"deliver,omitempty"`
	Target  batchTarget `json:"target"`
	// Session is the batch's own options, laid over each prompt's field by
	// field (overlaySession). nil when the batch leaves every one to the
	// prompts, which is sessionPtr's rule for a Todo too.
	Session *SessionOpts `json:"session,omitempty"`
	State   string       `json:"state"`
	// At is the fire time of a scheduled batch. It is kept once the batch
	// fires (or misses), so the record can say it went on a schedule and for
	// when; zero for a batch dropped straight from the composer.
	At time.Time `json:"at,omitzero"`
	// Why says why a scheduled batch missed its time — the one fact the page
	// cannot rebuild from the rest of the record.
	Why string `json:"why,omitempty"`
	// Dropped is when delivery started; zero for a batch never sent.
	Dropped time.Time  `json:"dropped,omitzero"`
	Runs    []BatchRun `json:"runs,omitempty"`

	// scope is which batches.json the record was read from and is written back
	// to. Not serialised: it is where the file is, not what it says.
	scope scope
}

// BatchItem is one prompt in a batch. The title is a snapshot, so the record
// still reads after the prompt it names has been edited, completed, or deleted
// — history is the one thing a batch keeps that the backlog does not.
type BatchItem struct {
	Scope string `json:"scope"` // "project" | "global"
	ID    string `json:"id"`
	Title string `json:"title"`
}

// ref is the item as the rest of the program addresses a todo.
func (it BatchItem) ref() todoRef {
	sc := scopeProject
	if it.Scope == batchScopeGlobal {
		sc = scopeGlobal
	}
	return todoRef{scope: sc, id: it.ID}
}

// The two scope spellings BatchItem writes. Words rather than the scope enum's
// integers because the file is read by people, and "0" says nothing.
const (
	batchScopeProject = "project"
	batchScopeGlobal  = "global"
)

func batchScopeName(s scope) string {
	if s == scopeGlobal {
		return batchScopeGlobal
	}
	return batchScopeProject
}

// batchTarget is where a batch goes, recorded as the Schedule's destination
// fields (see scheduleFromTarget) minus the fire time. It is not a Schedule
// itself because Schedule.At is not omitempty, and a batch dropped now would
// write a year-one timestamp into every record.
type batchTarget struct {
	Kind     string `json:"kind"` // scheduleKindPane | scheduleKindNew
	Pane     uint32 `json:"pane,omitempty"`
	Agent    string `json:"agent,omitempty"`
	Command  string `json:"command,omitempty"`
	Cwd      string `json:"cwd,omitempty"`
	Worktree bool   `json:"worktree,omitempty"`
	// Label is the picker row's text, kept so the Batches page can say where a
	// batch went in the words the user chose it by.
	Label string `json:"label,omitempty"`
}

// batchTargetFrom records a picker row; dropTarget turns it back into one. Both
// go through the Schedule conversions so a batch and a schedule can never
// disagree about what a destination is.
func batchTargetFrom(t dropTarget, cwd string) batchTarget {
	sc := scheduleFromTarget(t, time.Time{}, cwd)
	return batchTarget{Kind: sc.Kind, Pane: sc.Pane, Agent: sc.Agent, Command: sc.Command,
		Cwd: sc.Cwd, Worktree: sc.Worktree, Label: t.label}
}

func (bt batchTarget) dropTarget() dropTarget {
	t := targetFromSchedule(Schedule{Kind: bt.Kind, Pane: bt.Pane, Agent: bt.Agent,
		Command: bt.Command, Cwd: bt.Cwd, Worktree: bt.Worktree})
	t.label = bt.Label
	return t
}

// BatchRun is one delivery: which items it carried (one for all at once, every
// item for a combined drop), when, where it landed, and why it failed if it did.
type BatchRun struct {
	Items []int     `json:"items"`
	At    time.Time `json:"at"`
	Where string    `json:"where,omitempty"`
	Err   string    `json:"err,omitempty"`
}

// editable says the batch is still a plan: nothing in it has been sent, so
// opening it means changing it (the composer) rather than reading what
// happened (the record view).
func (b Batch) editable() bool {
	switch b.State {
	case batchScheduled, batchMissed, batchUnscheduled:
		return true
	}
	return false
}

// displayName is what a batch is called wherever it is listed: its name, or —
// since naming one is optional — its first prompt's title and how many more.
func (b Batch) displayName() string {
	if n := strings.TrimSpace(b.Name); n != "" {
		return n
	}
	switch len(b.Items) {
	case 0:
		return "empty batch"
	case 1:
		return b.Items[0].Title
	}
	return fmt.Sprintf("%s +%d", b.Items[0].Title, len(b.Items)-1)
}

// deliveredCounts is how many of the batch's items reached an agent, out of
// how many it holds. An item counts as delivered when a run that carried it
// succeeded; a combined run carries all of them at once.
func (b Batch) deliveredCounts() (ok, total int) {
	seen := map[int]bool{}
	for _, r := range b.Runs {
		if r.Err != "" {
			continue
		}
		for _, i := range r.Items {
			seen[i] = true
		}
	}
	return len(seen), len(b.Items)
}

// itemRun is the run that carried item i, if one has. The last one wins: in a
// record written by this build each item rides exactly one run.
func (b Batch) itemRun(i int) (BatchRun, bool) {
	for j := len(b.Runs) - 1; j >= 0; j-- {
		if slices.Contains(b.Runs[j].Items, i) {
			return b.Runs[j], true
		}
	}
	return BatchRun{}, false
}

// deliverLabel names a delivery mode in the words the composer's radio uses.
func deliverLabel(d string) string {
	if d == deliverCombined {
		return "one prompt, listed"
	}
	return "all at once"
}

// --- The store -------------------------------------------------------------------

// batchStore is one batches.json. It follows store's discipline exactly:
// every mutation reloads first, so two manager panes sharing a backlog
// directory never write back each other's stale copy, and every write is a
// temp file renamed over the target, so a crash cannot leave half a file.
//
// A store with no path is unavailable (a project store outside a project) and
// holds nothing, the same as a backlog store.
type batchStore struct {
	scope   scope
	path    string
	batches []Batch
}

// batchStoreFor is the batch store beside a backlog's todos.json.
func batchStoreFor(s *store) *batchStore {
	bs := &batchStore{scope: s.scope}
	if s.available() {
		bs.path = filepath.Join(filepath.Dir(s.path), batchesFileName)
	}
	return bs
}

func (bs *batchStore) available() bool { return bs.path != "" }

// load reads the file. A missing or empty file is an empty list, the
// first-run state, as it is for todos.json.
func (bs *batchStore) load() error {
	bs.batches = nil
	if bs.path == "" {
		return nil
	}
	data, err := os.ReadFile(bs.path)
	if errors.Is(err, os.ErrNotExist) || (err == nil && len(data) == 0) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, &bs.batches); err != nil {
		return err
	}
	for i := range bs.batches {
		bs.batches[i].scope = bs.scope
	}
	return nil
}

// save writes the list back through a temp file and a rename (see store.save).
func (bs *batchStore) save() error {
	if bs.path == "" {
		return errors.New("no backlog here to keep batches beside")
	}
	dir := filepath.Dir(bs.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(bs.batches, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(dir, ".batches-*.tmp")
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
		err = os.Rename(tmp.Name(), bs.path)
	}
	if err != nil {
		os.Remove(tmp.Name())
	}
	return err
}

// put writes b: replacing the record with its ID, or appending it when there
// is none. One call for both because the runner writes the same batch several
// times as its prompts land, and the first of those is the insert.
//
// Appending keeps the file in creation order. The Batches page sorts for
// display; the file is never reordered, the rule todos.json keeps too.
func (bs *batchStore) put(b Batch) error {
	if err := bs.load(); err != nil {
		return err
	}
	b.scope = bs.scope
	for i := range bs.batches {
		if bs.batches[i].ID == b.ID {
			bs.batches[i] = b
			return bs.save()
		}
	}
	bs.batches = append(bs.batches, b)
	return bs.save()
}

// delete removes the batch with id. A record already gone is not an error: the
// only caller is the page's delete, and "it is no longer there" is what was
// asked for.
func (bs *batchStore) delete(id string) error {
	if err := bs.load(); err != nil {
		return err
	}
	for i := range bs.batches {
		if bs.batches[i].ID == id {
			bs.batches = slices.Delete(bs.batches, i, i+1)
			return bs.save()
		}
	}
	return nil
}

// swapBatch replaces the record with orig's ID by next — but only while the
// record on disk is still the one orig was read as (same state, same fire
// time). It reports whether the swap happened.
//
// This is the claim rule claimSchedule keeps for a prompt's schedule, applied
// to every change to a batch that has not gone yet: firing it, marking it
// missed, unscheduling it, saving an edit to it. Two managers can hold the same
// scheduled batch; whichever swaps first wins, and the other finds the record
// changed and stands down instead of firing it a second time or writing an
// edit over a batch that has meanwhile started.
//
// A record that is gone (deleted from another pane) is a lost swap, not an
// insert: whoever deleted it meant it, and firing or resurrecting it would
// undo that.
func (bs *batchStore) swapBatch(orig, next Batch) (bool, error) {
	if err := bs.load(); err != nil {
		return false, err
	}
	for i := range bs.batches {
		cur := bs.batches[i]
		if cur.ID != orig.ID {
			continue
		}
		if cur.State != orig.State || !cur.At.Equal(orig.At) {
			return false, nil
		}
		next.scope = bs.scope
		bs.batches[i] = next
		return true, bs.save()
	}
	return false, nil
}

// takeBatch is swapBatch's delete: it removes the record only while it is still
// the one orig was read as. An edit that moves a batch to the other file (see
// scheduleBatch) takes it out of the old one this way.
func (bs *batchStore) takeBatch(orig Batch) (bool, error) {
	if err := bs.load(); err != nil {
		return false, err
	}
	for i := range bs.batches {
		cur := bs.batches[i]
		if cur.ID != orig.ID {
			continue
		}
		if cur.State != orig.State || !cur.At.Equal(orig.At) {
			return false, nil
		}
		bs.batches = slices.Delete(bs.batches, i, i+1)
		return true, bs.save()
	}
	return false, nil
}

// --- The tick's view of the files -------------------------------------------------

// batchWatch is a read cache of batches.json files, keyed by path and
// invalidated by the file's size and modification time.
//
// Two readers need the files far more often than they change: the schedule
// tick, once a second, looking for a batch whose time has come, and the list,
// on every rebuild, looking for prompts to give a ⧉ badge. The file holds every
// batch ever sent, so re-parsing it on each of those would be the one per-tick
// cost in the program that grows with use. A stat is cheap and constant; the
// parse happens only when another write (from this pane or any other) has
// changed the file. Every write goes through a rename, which always changes
// the modification time, so a stale hit would need two different files of the
// same size written within the clock's resolution.
//
// It is only ever a view. Anything that acts on a batch reloads the file and
// swaps under the claim rule (swapBatch), so a stale read can make the tick
// look one second late, but never fire twice.
type batchWatch struct {
	files map[string]batchSnap
}

type batchSnap struct {
	mod     time.Time
	size    int64
	batches []Batch
}

// read returns bs's batches, parsing the file only when it has changed since
// the last read. A missing file is no batches (and forgets any snapshot, so a
// file deleted and re-created is read afresh).
func (w *batchWatch) read(bs *batchStore) ([]Batch, error) {
	if !bs.available() {
		return nil, nil
	}
	fi, err := os.Stat(bs.path)
	if errors.Is(err, os.ErrNotExist) {
		delete(w.files, bs.path)
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if snap, ok := w.files[bs.path]; ok && snap.mod.Equal(fi.ModTime()) && snap.size == fi.Size() {
		return snap.batches, nil
	}
	if err := bs.load(); err != nil {
		return nil, err
	}
	if w.files == nil {
		w.files = map[string]batchSnap{}
	}
	w.files[bs.path] = batchSnap{mod: fi.ModTime(), size: fi.Size(), batches: bs.batches}
	return bs.batches, nil
}

// pendingRefs maps each prompt that sits in a scheduled batch to that batch's
// fire time — the earliest, if it sits in more than one — for the list's ⧉
// mark. Only scheduled batches count: a missed or unscheduled one will not send
// anything on its own, so its prompts are not spoken for.
func pendingRefs(batches []Batch) map[todoRef]time.Time {
	var out map[todoRef]time.Time
	for _, b := range batches {
		if b.State != batchScheduled {
			continue
		}
		for _, it := range b.Items {
			if out == nil {
				out = map[todoRef]time.Time{}
			}
			ref := it.ref()
			if at, ok := out[ref]; !ok || b.At.Before(at) {
				out[ref] = b.At
			}
		}
	}
	return out
}

// --- Session options: the batch over the prompt ---------------------------------

// overlaySession lays the batch's options over one prompt's, field by field: a
// field the batch sets wins, and a field it leaves at the default falls through
// to the prompt. So "every prompt on sonnet" costs one prompt nothing of its
// own /sess-use pattern.
//
// The two booleans are the one place the rule has an edge. A batch cannot say
// "not" for them — false is also what "not set" looks like — so a batch can
// turn Clear or Release on for every prompt, but cannot turn one off for a
// prompt that asked for it. That is the price of the empty-means-default rule
// the whole record keeps, and the composer's ✱ only ever marks what the batch
// actually replaced.
//
// The result goes through sessionPtr, so a prompt and a batch with nothing to
// say between them produce nil — the byte-for-byte "default session" drop.
func overlaySession(own, batch *SessionOpts) *SessionOpts {
	var o SessionOpts
	if own != nil {
		o = own.clone()
	}
	if batch == nil {
		return sessionPtr(o)
	}
	b := *batch
	if b.Model != "" {
		o.Model = b.Model
	}
	if b.Effort != "" {
		o.Effort = b.Effort
	}
	if b.Permission != "" {
		o.Permission = b.Permission
	}
	o.Clear = o.Clear || b.Clear
	// The context mode and its argument travel together: a batch that says
	// "/sess-load 2" replaces the pair, since the prompt's argument was written
	// for the prompt's mode and would mean something else under the batch's.
	if b.Context != ctxNone {
		o.Context, o.ContextArg = b.Context, b.ContextArg
	}
	if len(b.Files) > 0 {
		o.Files = slices.Clone(b.Files)
	}
	if b.Finish != finishNone {
		o.Finish = b.Finish
	}
	if len(b.Reviews) > 0 {
		o.Reviews = slices.Clone(b.Reviews)
	}
	o.Release = o.Release || b.Release
	return sessionPtr(o)
}

// overriddenFields names the prompt's own settings the batch replaces — what
// the composer's ✱ stands for. Only a real replacement counts: a batch field
// that agrees with the prompt's, or one the prompt never set, changes nothing
// the prompt said.
func overriddenFields(own, batch *SessionOpts) []string {
	if own == nil || batch == nil {
		return nil
	}
	var out []string
	diff := func(name, mine, theirs string) {
		if mine != "" && theirs != "" && mine != theirs {
			out = append(out, name)
		}
	}
	diff("model", own.Model, batch.Model)
	diff("effort", own.Effort, batch.Effort)
	diff("permission", own.Permission, batch.Permission)
	diff("context", own.contextCommand(), batch.contextCommand())
	diff("files", strings.Join(own.Files, ","), strings.Join(batch.Files, ","))
	diff("finish", own.Finish, batch.Finish)
	diff("reviews", strings.Join(own.Reviews, ","), strings.Join(batch.Reviews, ","))
	return out
}

// --- The combined body ------------------------------------------------------------

// combinedIntro opens a combined drop. It says two things the agent could not
// otherwise know: that the sections are separate tasks rather than one long
// one, and that the order is deliberate.
const combinedIntro = "This is a batch of %d tasks. Work through them in the order given; each section is a separate task."

// combinedPrompt joins the prompts into one body, numbered in batch order under
// their titles:
//
//	This is a batch of 3 tasks. …
//
//	## 1. Fix flaky drop test
//
//	<its prompt>
//
//	## 2. …
//
// A heading per task rather than a bare list, because a prompt is often several
// paragraphs with lists of its own, and a heading is the one structure its body
// cannot be mistaken for the continuation of. The session preamble and
// postamble are not here: composePrompt wraps the whole body once, with the
// batch's options, exactly as it wraps a single prompt.
func combinedPrompt(todos []Todo) string {
	var b strings.Builder
	fmt.Fprintf(&b, combinedIntro, len(todos))
	for i, td := range todos {
		title := firstNonEmpty(strings.TrimSpace(td.Title), firstLine(td.Prompt, 60))
		fmt.Fprintf(&b, "\n\n## %d. %s\n\n%s", i+1, title, strings.TrimSpace(td.Prompt))
	}
	return b.String()
}
